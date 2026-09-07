package pipeline

// v4.go contains the schema-v2 Irodori runtime. Keeping this in a separate
// implementation preserves the legacy metadata.json/v3 path byte-for-byte
// while making the v4 graph contract explicit and name-driven.

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"

	ort "fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts/irodori/metadata"
	"fm-live-radio/internal/localtts/irodori/sampler"
	"fm-live-radio/internal/localtts/irodori/tensor"
	"fm-live-radio/internal/localtts/irodori/tokenizer"
	"fm-live-radio/internal/localtts/irodori/wav"

	onnx "github.com/yalue/onnxruntime_go"
	"golang.org/x/text/unicode/norm"
)

type v4Runtime struct {
	manifest     *metadata.Manifest
	opt          Options
	tok          *tokenizer.Tokenizer
	textCaption  *ort.Session
	speaker      *ort.Session
	duration     *ort.Session
	dit          *ort.Session
	codecEncoder *ort.Session
	codecDecoder *ort.Session
}

func loadV4Runtime(opt Options) (*Runtime, error) {
	m, err := metadata.LoadManifest(opt.ModelDir)
	if err != nil {
		return nil, fmt.Errorf("pipeline: v4 manifest: %w", err)
	}
	tokPath := filepath.Join(opt.ModelDir, filepath.FromSlash(m.Tokenizer.Path))
	tok, err := tokenizer.FromFile(tokPath, true)
	if err != nil {
		return nil, fmt.Errorf("pipeline: v4 tokenizer: %w", err)
	}
	newGraph := func(name string) (*ort.Session, error) {
		g := m.Graphs[name]
		s, e := ort.NewSession(filepath.Join(opt.ModelDir, filepath.FromSlash(g.Path)), g.Inputs, g.Outputs)
		if e != nil {
			return nil, fmt.Errorf("pipeline: v4 graph %s: %w", name, e)
		}
		return s, nil
	}
	textCaption, err := newGraph("text_caption_encoder")
	if err != nil {
		return nil, err
	}
	speaker, err := newGraph("speaker_encoder")
	if err != nil {
		textCaption.Destroy()
		return nil, err
	}
	duration, err := newGraph("duration_predictor")
	if err != nil {
		textCaption.Destroy()
		speaker.Destroy()
		return nil, err
	}
	dit, err := newGraph("dit_step")
	if err != nil {
		textCaption.Destroy()
		speaker.Destroy()
		duration.Destroy()
		return nil, err
	}
	codecEncoder, err := newGraph("codec_encoder")
	if err != nil {
		textCaption.Destroy()
		speaker.Destroy()
		duration.Destroy()
		dit.Destroy()
		return nil, err
	}
	codecDecoder, err := newGraph("codec_decoder")
	if err != nil {
		textCaption.Destroy()
		speaker.Destroy()
		duration.Destroy()
		dit.Destroy()
		codecEncoder.Destroy()
		return nil, err
	}
	return &Runtime{
		opt: opt, closeDone: make(chan struct{}), observer: opt.Observer,
		v4: &v4Runtime{manifest: m, opt: opt, tok: tok, textCaption: textCaption, speaker: speaker, duration: duration, dit: dit, codecEncoder: codecEncoder, codecDecoder: codecDecoder},
	}, nil
}

func (r *v4Runtime) close() {
	for _, s := range []*ort.Session{r.textCaption, r.speaker, r.duration, r.dit, r.codecEncoder, r.codecDecoder} {
		if s != nil {
			s.Destroy()
		}
	}
}

func (r *v4Runtime) synthesize() error {
	m := r.manifest
	text := normalizeV4SynthesisText(r.opt.Text)
	if text == "" {
		return fmt.Errorf("pipeline: v4 text is empty after normalization")
	}
	textIDs, textMask := r.tok.EncodePadded(text, m.Tokenizer.TextMaxLength)
	// Official v4 normalizes synthesis text, while captions are trim-only.
	caption := strings.TrimSpace(r.opt.Caption)
	captionIDs, captionMask := r.tok.EncodePadded(caption, m.Tokenizer.CaptionMaxLength)
	// Empty caption is the official null condition: retain the fixed tokenizer
	// shape but clear every mask element so the graph's null branch is selected.
	if strings.TrimSpace(caption) == "" {
		for i := range captionMask {
			captionMask[i] = false
		}
	}
	textState, captionState, err := r.runTextCaption(textIDs, textMask, captionIDs, captionMask)
	if err != nil {
		return fmt.Errorf("pipeline: v4 text/caption encoder: %w", err)
	}

	refLatent, refMask, hasSpeaker, err := r.prepareReference()
	if err != nil {
		return err
	}
	speakerState, speakerMask, err := r.runSpeaker(refLatent, refMask)
	if err != nil {
		return fmt.Errorf("pipeline: v4 speaker encoder: %w", err)
	}

	hasCaption := false
	for _, v := range captionMask {
		hasCaption = hasCaption || v
	}
	latentLen, err := r.resolveDuration(text, textMask, textState, textMask, speakerState, speakerMask, captionState, captionMask, hasSpeaker, hasCaption)
	if err != nil {
		return err
	}
	patchedDim := int64(m.Conditions.LatentDim * m.Conditions.LatentPatchSize)
	noise := make([]float32, latentLen*patchedDim)
	tensor.RandnF32(noise, tensor.NewMulberry32(r.opt.Seed))
	cfg := sampler.CfgConfig{ScaleText: r.opt.CfgText, ScaleCaption: r.opt.CfgCaption, ScaleSpeaker: r.opt.CfgSpeaker, MinT: 0.5, MaxT: 1.0, UseTextCfg: r.opt.CfgText > 0, UseCaptionCfg: hasCaption && r.opt.CfgCaption > 0, UseSpeakerCfg: hasSpeaker && r.opt.CfgSpeaker > 0}
	if err := r.runDenoising(noise, textState, textMask, speakerState, speakerMask, captionState, captionMask, latentLen, cfg, r.opt.NumSteps); err != nil {
		return fmt.Errorf("pipeline: v4 denoising: %w", err)
	}
	audio, err := r.runCodecDecoder(noise, latentLen)
	if err != nil {
		return fmt.Errorf("pipeline: v4 codec decoder: %w", err)
	}
	if err := validateAudio(audio); err != nil {
		return err
	}
	if r.opt.Seconds > 0 {
		targetSamples := int(math.Round(math.Max(minV4Seconds, math.Min(maxV4Seconds, r.opt.Seconds)) * float64(m.Codec.SampleRate)))
		if len(audio) > targetSamples {
			audio = audio[:targetSamples]
		}
	}
	return wav.WriteMonoPCM16(r.opt.OutputWAV, audio, m.Codec.SampleRate)
}

func (r *v4Runtime) prepareReference() ([]float32, []bool, bool, error) {
	m := r.manifest
	refLen := int64(m.Conditions.SpeakerPatchSize)
	if strings.TrimSpace(r.opt.RefWAV) == "" {
		return make([]float32, refLen*int64(m.Codec.LatentDim)), make([]bool, refLen), false, nil
	}
	samples, sr, err := wav.ReadWAV(r.opt.RefWAV)
	if err != nil {
		return nil, nil, false, fmt.Errorf("pipeline: v4 reference wav: %w", err)
	}
	if m.Tokenizer.MaxRefSeconds > 0 {
		maxSamples := int(float64(sr) * m.Tokenizer.MaxRefSeconds)
		if len(samples) > maxSamples {
			samples = samples[:maxSamples]
		}
	}
	samples = wav.Resample(samples, sr, m.Codec.SampleRate)
	hop := m.Codec.HopLength
	// Pass the complete waveform to the dynamic-padding codec.  Speaker patching
	// is applied to the latent result, never by truncating samples before encode.
	// codec_encoder emits ceil(samples/hop) frames for the dynamic input.
	latentLen, err := referenceCodecFrames(len(samples), hop, m.Conditions.SpeakerPatchSize)
	if err != nil {
		return nil, nil, false, err
	}
	latent, err := r.runCodecEncoder(samples, latentLen)
	if err != nil {
		return nil, nil, false, fmt.Errorf("pipeline: v4 codec encoder: %w", err)
	}
	// Speaker patching is floor/truncate after codec encoding.  This yields the
	// official output length floor(refLen/4)+1 (the extra speaker summary token).
	patch := int64(m.Conditions.SpeakerPatchSize)
	if rem := latentLen % patch; rem != 0 {
		latentLen -= rem
		latent = latent[:latentLen*int64(m.Codec.LatentDim)]
	}
	if latentLen < patch {
		return nil, nil, false, fmt.Errorf("pipeline: v4 reference has fewer than %d complete speaker patches", patch)
	}
	mask := make([]bool, latentLen)
	for i := range mask {
		mask[i] = true
	}
	return latent, mask, true, nil
}

// referenceCodecFrames mirrors dynamic codec padding (ceil(samples/hop)).
// Speaker patch floor/truncation is applied only after the codec output exists.
func referenceCodecFrames(samples, hop, patch int) (int64, error) {
	if samples <= 0 || hop <= 0 || patch <= 0 {
		return 0, fmt.Errorf("pipeline: invalid reference length parameters samples=%d hop=%d patch=%d", samples, hop, patch)
	}
	frames := int64((samples + hop - 1) / hop)
	if frames < int64(patch) {
		return 0, fmt.Errorf("pipeline: v4 reference is shorter than speaker patch=%d latent frames", patch)
	}
	return frames, nil
}

func (r *v4Runtime) runTextCaption(textIDs []int64, textMask []bool, captionIDs []int64, captionMask []bool) ([]float32, []float32, error) {
	t, err := ort.NewInt64Tensor(textIDs, []int64{1, int64(len(textIDs))})
	if err != nil {
		return nil, nil, err
	}
	defer t.Destroy()
	tm, err := ort.NewBoolTensor(textMask, []int64{1, int64(len(textMask))})
	if err != nil {
		return nil, nil, err
	}
	defer tm.Destroy()
	c, err := ort.NewInt64Tensor(captionIDs, []int64{1, int64(len(captionIDs))})
	if err != nil {
		return nil, nil, err
	}
	defer c.Destroy()
	cm, err := ort.NewBoolTensor(captionMask, []int64{1, int64(len(captionMask))})
	if err != nil {
		return nil, nil, err
	}
	defer cm.Destroy()
	outT, err := ort.NewEmptyFloat32Tensor([]int64{1, int64(len(textIDs)), int64(r.manifest.Conditions.TextDim)})
	if err != nil {
		return nil, nil, err
	}
	defer outT.Destroy()
	outC, err := ort.NewEmptyFloat32Tensor([]int64{1, int64(len(captionIDs)), int64(r.manifest.Conditions.CaptionDim)})
	if err != nil {
		return nil, nil, err
	}
	defer outC.Destroy()
	if err := r.runNamed(r.textCaption, "text_caption_encoder", map[string]onnx.ArbitraryTensor{"text_input_ids": t, "text_mask": tm, "caption_input_ids": c, "caption_mask": cm}, map[string]onnx.ArbitraryTensor{"text_state": outT, "caption_state": outC}); err != nil {
		return nil, nil, err
	}
	ts := append([]float32(nil), outT.GetData()...)
	cs := append([]float32(nil), outC.GetData()...)
	return ts, cs, nil
}

func (r *v4Runtime) runCodecEncoder(samples []float32, latentLen int64) ([]float32, error) {
	in, err := ort.NewFloat32Tensor(samples, []int64{1, 1, int64(len(samples))})
	if err != nil {
		return nil, err
	}
	defer in.Destroy()
	out, err := ort.NewEmptyFloat32Tensor([]int64{1, latentLen, int64(r.manifest.Codec.LatentDim)})
	if err != nil {
		return nil, err
	}
	defer out.Destroy()
	if err := r.runNamed(r.codecEncoder, "codec_encoder", map[string]onnx.ArbitraryTensor{"waveform": in}, map[string]onnx.ArbitraryTensor{"latent": out}); err != nil {
		return nil, err
	}
	return append([]float32(nil), out.GetData()...), nil
}

func (r *v4Runtime) runSpeaker(latent []float32, mask []bool) ([]float32, []bool, error) {
	seq := int64(len(mask))
	in, err := ort.NewFloat32Tensor(latent, []int64{1, seq, int64(r.manifest.Conditions.LatentDim)})
	if err != nil {
		return nil, nil, err
	}
	defer in.Destroy()
	im, err := ort.NewBoolTensor(mask, []int64{1, seq})
	if err != nil {
		return nil, nil, err
	}
	defer im.Destroy()
	outSeq := seq/int64(r.manifest.Conditions.SpeakerPatchSize) + 1
	out, err := ort.NewEmptyFloat32Tensor([]int64{1, outSeq, int64(r.manifest.Conditions.SpeakerDim)})
	if err != nil {
		return nil, nil, err
	}
	defer out.Destroy()
	outM, err := onnx.NewEmptyTensor[bool]([]int64{1, outSeq})
	if err != nil {
		return nil, nil, err
	}
	defer outM.Destroy()
	if err := r.runNamed(r.speaker, "speaker_encoder", map[string]onnx.ArbitraryTensor{"ref_latent": in, "ref_mask": im}, map[string]onnx.ArbitraryTensor{"speaker_state": out, "speaker_mask": outM}); err != nil {
		return nil, nil, err
	}
	return append([]float32(nil), out.GetData()...), append([]bool(nil), outM.GetData()...), nil
}

func (r *v4Runtime) resolveDuration(text string, textMask []bool, textState []float32, _ []bool, speakerState []float32, speakerMask []bool, captionState []float32, captionMask []bool, hasSpeaker, hasCaption bool) (int64, error) {
	m := r.manifest
	seconds := r.opt.Seconds
	if seconds > 0 {
		return durationFrames(0, r.opt.DurationScale, seconds, m.Codec.SampleRate, m.Codec.HopLength), nil
	}
	features := buildDurationFeatures(text, countTrue(textMask), m.Tokenizer.TextMaxLength, hasSpeaker)
	raw, err := r.runDurationRaw(durationInput{
		textState: textState, textMask: textMask,
		speakerState: speakerState, speakerMask: speakerMask,
		captionState: captionState, captionMask: captionMask,
		features: features, hasSpeaker: hasSpeaker, hasCaption: hasCaption,
	})
	if err != nil {
		return 0, err
	}
	if len(raw) != 1 {
		return 0, fmt.Errorf("pipeline: duration predictor returned %d values, want 1", len(raw))
	}
	return durationFrames(raw[0], r.opt.DurationScale, 0, m.Codec.SampleRate, m.Codec.HopLength), nil
}

func (r *v4Runtime) runDiTSingle(x, textState []float32, textMask []bool, speakerState []float32, speakerMask []bool, captionState []float32, captionMask []bool, latentLen int64, t, dt float32) error {
	v, err := r.runDiTBatch(x, textState, textMask, speakerState, speakerMask, captionState, captionMask, latentLen, []sampler.CfgBundle{sampler.BundleCond}, t)
	if err != nil {
		return err
	}
	sampler.EulerStep(x, v, dt)
	return nil
}

func (r *v4Runtime) runDiTCFG(x, textState []float32, textMask []bool, speakerState []float32, speakerMask []bool, captionState []float32, captionMask []bool, latentLen int64, bundles []sampler.CfgBundle, cfg sampler.CfgConfig, t, dt float32) error {
	vAll, err := r.runDiTBatch(x, textState, textMask, speakerState, speakerMask, captionState, captionMask, latentLen, bundles, t)
	if err != nil {
		return err
	}
	stride := int(latentLen) * r.manifest.Conditions.LatentDim
	vCond := vAll[:stride]
	idx := stride
	var vt, vs, vc []float32
	if cfg.UseTextCfg {
		vt = vAll[idx : idx+stride]
		idx += stride
	}
	if cfg.UseSpeakerCfg {
		vs = vAll[idx : idx+stride]
		idx += stride
	}
	if cfg.UseCaptionCfg {
		vc = vAll[idx : idx+stride]
	}
	sampler.EulerStep(x, sampler.ApplyCFG(cfg, vCond, vt, vs, vc), dt)
	return nil
}

func (r *v4Runtime) runDiTBatch(x, textState []float32, textMask []bool, speakerState []float32, speakerMask []bool, captionState []float32, captionMask []bool, latentLen int64, bundles []sampler.CfgBundle, t float32) ([]float32, error) {
	b := len(bundles)
	dim := r.manifest.Conditions.LatentDim
	stride := int(latentLen) * dim
	xb := make([]float32, b*stride)
	ts := make([]float32, b*len(textState))
	ss := make([]float32, b*len(speakerState))
	cs := make([]float32, b*len(captionState))
	tm := make([]bool, b*len(textMask))
	sm := make([]bool, b*len(speakerMask))
	cm := make([]bool, b*len(captionMask))
	lm := make([]bool, b*int(latentLen))
	tt := make([]float32, b)
	for i, bundle := range bundles {
		copy(xb[i*stride:], x)
		copy(ts[i*len(textState):], textState)
		copy(ss[i*len(speakerState):], speakerState)
		copy(cs[i*len(captionState):], captionState)
		copy(tm[i*len(textMask):], textMask)
		copy(sm[i*len(speakerMask):], speakerMask)
		copy(cm[i*len(captionMask):], captionMask)
		for j := 0; j < int(latentLen); j++ {
			lm[i*int(latentLen)+j] = true
		}
		tt[i] = t
		switch bundle {
		case sampler.BundleNoText:
			for j := 0; j < len(textMask); j++ {
				tm[i*len(textMask)+j] = false
			}
		case sampler.BundleNoSpeaker:
			for j := 0; j < len(speakerMask); j++ {
				sm[i*len(speakerMask)+j] = false
			}
		case sampler.BundleNoCaption:
			for j := 0; j < len(captionMask); j++ {
				cm[i*len(captionMask)+j] = false
			}
		}
	}
	xT, e := ort.NewFloat32Tensor(xb, []int64{int64(b), latentLen, int64(dim)})
	if e != nil {
		return nil, e
	}
	defer xT.Destroy()
	tT, e := ort.NewFloat32Tensor(tt, []int64{int64(b)})
	if e != nil {
		return nil, e
	}
	defer tT.Destroy()
	tS, e := ort.NewFloat32Tensor(ts, []int64{int64(b), int64(len(textMask)), int64(r.manifest.Conditions.TextDim)})
	if e != nil {
		return nil, e
	}
	defer tS.Destroy()
	tM, e := ort.NewBoolTensor(tm, []int64{int64(b), int64(len(textMask))})
	if e != nil {
		return nil, e
	}
	defer tM.Destroy()
	sS, e := ort.NewFloat32Tensor(ss, []int64{int64(b), int64(len(speakerMask)), int64(r.manifest.Conditions.SpeakerDim)})
	if e != nil {
		return nil, e
	}
	defer sS.Destroy()
	sM, e := ort.NewBoolTensor(sm, []int64{int64(b), int64(len(speakerMask))})
	if e != nil {
		return nil, e
	}
	defer sM.Destroy()
	cS, e := ort.NewFloat32Tensor(cs, []int64{int64(b), int64(len(captionMask)), int64(r.manifest.Conditions.CaptionDim)})
	if e != nil {
		return nil, e
	}
	defer cS.Destroy()
	cM, e := ort.NewBoolTensor(cm, []int64{int64(b), int64(len(captionMask))})
	if e != nil {
		return nil, e
	}
	defer cM.Destroy()
	lM, e := ort.NewBoolTensor(lm, []int64{int64(b), latentLen})
	if e != nil {
		return nil, e
	}
	defer lM.Destroy()
	out, e := ort.NewEmptyFloat32Tensor([]int64{int64(b), latentLen, int64(dim)})
	if e != nil {
		return nil, e
	}
	defer out.Destroy()
	if e := r.runNamed(r.dit, "dit_step", map[string]onnx.ArbitraryTensor{"x_t": xT, "t": tT, "text_state": tS, "text_mask": tM, "speaker_state": sS, "speaker_mask": sM, "caption_state": cS, "caption_mask": cM, "latent_mask": lM}, map[string]onnx.ArbitraryTensor{"v_pred": out}); e != nil {
		return nil, e
	}
	return append([]float32(nil), out.GetData()...), nil
}

func (r *v4Runtime) runCodecDecoder(latent []float32, latentLen int64) ([]float32, error) {
	in, e := ort.NewFloat32Tensor(latent, []int64{1, latentLen, int64(r.manifest.Codec.LatentDim)})
	if e != nil {
		return nil, e
	}
	defer in.Destroy()
	out, e := ort.NewEmptyFloat32Tensor([]int64{1, 1, latentLen * int64(r.manifest.Codec.HopLength)})
	if e != nil {
		return nil, e
	}
	defer out.Destroy()
	if e := r.runNamed(r.codecDecoder, "codec_decoder", map[string]onnx.ArbitraryTensor{"latent": in}, map[string]onnx.ArbitraryTensor{"waveform": out}); e != nil {
		return nil, e
	}
	return append([]float32(nil), out.GetData()...), nil
}

func (r *v4Runtime) runNamed(s *ort.Session, name string, in map[string]onnx.ArbitraryTensor, out map[string]onnx.ArbitraryTensor) error {
	g := r.manifest.Graphs[name]
	inputs := make([]onnx.ArbitraryTensor, len(g.Inputs))
	outputs := make([]onnx.ArbitraryTensor, len(g.Outputs))
	for i, n := range g.Inputs {
		v, ok := in[n]
		if !ok {
			return fmt.Errorf("missing input %q", n)
		}
		inputs[i] = v
	}
	for i, n := range g.Outputs {
		v, ok := out[n]
		if !ok {
			return fmt.Errorf("missing output %q", n)
		}
		outputs[i] = v
	}
	return s.Run(inputs, outputs)
}

var v4StripChars = regexp.MustCompile(`[;▼♀♂《》≪≫①②③④⑤⑥]`)
var v4DashChars = regexp.MustCompile(`[˗‐‑‒–—―⁃−⎯⏤─━⸺⸻]`)
var v4WaveChars = regexp.MustCompile(`[～〜]`)
var v4EllipsisChars = regexp.MustCompile(`…{3,}`)
var v4OuterBrackets = map[rune]rune{'「': '」', '『': '』', '（': '）', '【': '】', '(': ')'}

// normalizeV4Text mirrors official text_normalization.normalize_text. The
// caller applies TrimSpace after this function, as the official runtime does.
func normalizeV4Text(text string) string {
	for old, newValue := range map[string]string{"\t": "", "[n]": "", `\[n\]`: "", "　": "", "？": "?", "！": "!", "♥": "♡", "●": "○", "◯": "○", "〇": "○"} {
		text = strings.ReplaceAll(text, old, newValue)
	}
	text = v4StripChars.ReplaceAllString(text, "")
	text = v4DashChars.ReplaceAllString(text, "")
	text = v4WaveChars.ReplaceAllString(text, "ー")
	text = v4EllipsisChars.ReplaceAllString(text, "……")
	text = stripV4OuterBrackets(text)
	text = norm.NFKC.String(text)
	text = strings.ReplaceAll(text, "...", "…")
	text = strings.ReplaceAll(text, "..", "…")
	return text
}

// normalizeV4SynthesisText is the single product boundary for text. The
// official runtime passes normalize_text(raw).strip() to both the tokenizer
// and duration feature builder, so trimming is part of the value passed to
// consumers rather than only an empty-input check.
func normalizeV4SynthesisText(raw string) string {
	return strings.TrimSpace(normalizeV4Text(raw))
}

func stripV4OuterBrackets(text string) string {
	for len([]rune(text)) >= 2 {
		runes := []rune(text)
		end, ok := v4OuterBrackets[runes[0]]
		if !ok || runes[len(runes)-1] != end {
			break
		}
		depth, encloses := 0, true
		for i, r := range runes {
			if r == runes[0] {
				depth++
			}
			if r == end {
				depth--
			}
			if depth == 0 && i < len(runes)-1 {
				encloses = false
				break
			}
		}
		if !encloses || depth != 0 {
			break
		}
		text = string(runes[1 : len(runes)-1])
	}
	return text
}

func buildDurationFeatures(text string, tokenCount, maxLen int, hasSpeaker bool) []float32 {
	if maxLen <= 0 {
		maxLen = 256
	}
	runes := []rune(text)
	chars := len(runes)
	if chars < 1 {
		chars = 1
	}
	var kana, kanji, alnum int
	for _, r := range runes {
		if (r >= 0x3040 && r <= 0x309f) || (r >= 0x30a0 && r <= 0x30ff) {
			kana++
		}
		if (r >= 0x3400 && r <= 0x4dbf) || (r >= 0x4e00 && r <= 0x9fff) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0x20000 && r <= 0x2fa1f) {
			kanji++
		}
		if (r <= 'z' && r >= 'a') || (r <= 'Z' && r >= 'A') || (r <= '9' && r >= '0') {
			alnum++
		}
	}
	capLog := func(v, cap float64) float64 {
		if v < 0 {
			v = 0
		}
		if v > cap {
			v = cap
		}
		return math.Log1p(v) / math.Log1p(cap)
	}
	count := func(r rune) int {
		n := 0
		for _, x := range runes {
			if x == r {
				n++
			}
		}
		return n
	}
	return []float32{float32(math.Min(math.Max(float64(tokenCount), 0), float64(maxLen)) / float64(maxLen)), float32(capLog(float64(chars), 512)), float32(tokenCount) / float32(chars), float32(capLog(float64(count('。')+count('.')), 8)), float32(capLog(float64(count('、')+count(',')), 16)), float32(capLog(float64(count('ー')), 8)), float32(capLog(float64(count('…')), 8)), float32(capLog(float64(count('！')+count('!')), 8)), float32(capLog(float64(count('？')+count('?')), 8)), float32(capLog(float64(countAnnotationEmojis(text)), 8)), float32(kana) / float32(chars), float32(kanji) / float32(chars), float32(alnum) / float32(chars), boolF32(hasSpeaker)}
}

var v4AnnotationEmojis = []string{"⏩", "⏱️", "⏸️", "🌬️", "🍭", "🎛️", "🎭", "🎵", "🐢", "🐱", "👂", "👃", "👅", "👌", "👏", "💋", "💥", "💦", "💪", "📄", "📞", "📢", "📣", "😆", "😊", "😌", "😎", "😏", "😒", "😖", "😟", "😠", "😪", "😭", "😮", "😮‍💨", "😰", "😱", "😲", "😴", "🙄", "🙏", "🤐", "🤔", "🤢", "🤧", "🤭", "🥤", "🥱", "🥴", "🥵", "🥹", "🥺", "🫣", "🫶", "📖"}

func countAnnotationEmojis(text string) int {
	n := 0
	for _, emoji := range v4AnnotationEmojis {
		n += strings.Count(text, emoji)
	}
	// The official regex uses longest-first matching, so the composite breath
	// emoji is one annotation rather than both "😮‍💨" and its "😮" prefix.
	n -= strings.Count(text, "😮‍💨")
	return n
}
func boolF32(v bool) float32 {
	if v {
		return 1
	}
	return 0
}

func countTrue(values []bool) int {
	n := 0
	for _, value := range values {
		if value {
			n++
		}
	}
	return n
}
func validateAudio(audio []float32) error {
	var peak, sum float64
	for _, v := range audio {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Errorf("pipeline: v4 decoder returned non-finite audio")
		}
		a := math.Abs(float64(v))
		if a > peak {
			peak = a
		}
		sum += float64(v) * float64(v)
	}
	if len(audio) == 0 || peak == 0 || sum == 0 {
		return fmt.Errorf("pipeline: v4 decoder returned silent audio")
	}
	return nil
}
