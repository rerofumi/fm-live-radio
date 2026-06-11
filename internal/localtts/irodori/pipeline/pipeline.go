// Package pipeline orchestrates the full Irodori-TTS inference:
// tokenize → text_encoder → (caption_encoder) → Euler DiT loop →
// dacvae_decoder → WAV.
//
// Only the v2-VoiceDesign caption mode MVP is implemented. Speaker mode
// and duration predictor are left for a future iteration.
package pipeline

import (
	"fmt"
	"math"
	"path/filepath"

	ort "fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts/irodori/metadata"
	"fm-live-radio/internal/localtts/irodori/sampler"
	"fm-live-radio/internal/localtts/irodori/tensor"
	"fm-live-radio/internal/localtts/irodori/tokenizer"
	"fm-live-radio/internal/localtts/irodori/wav"

	onnx "github.com/yalue/onnxruntime_go"
)

// Options holds the user-supplied synthesis parameters.
type Options struct {
	Text          string
	Caption       string
	OutputWAV     string
	ModelDir      string
	Seed          uint32
	NumSteps      int
	Seconds       float64
	CfgText       float64
	CfgCaption    float64
	RefWAV        string
	CfgSpeaker    float64
	DurationScale float64
}

// DefaultOptions returns sensible defaults for v2-VoiceDesign and v3.
func DefaultOptions() Options {
	return Options{
		NumSteps:      40,
		Seconds:       -1.0,
		Seed:          0,
		CfgText:       3.0,
		CfgCaption:    3.0,
		CfgSpeaker:    5.0,
		DurationScale: 1.0,
	}
}

// Runtime holds the loaded ONNX sessions and tokenizer for a single
// model directory. Call Close to release resources.
type Runtime struct {
	md         *metadata.Metadata
	opt        Options
	textTok    *tokenizer.Tokenizer
	capTok     *tokenizer.Tokenizer
	textEnc    *ort.Session
	capEnc     *ort.Session
	speakerEnc *ort.Session
	durPred    *ort.Session
	dacEnc     *ort.Session
	ditStep    *ort.Session
	decDAC     *ort.Session
	closed     bool
}

// LoadInitialise loads metadata, tokenizers, and all required ONNX
// sessions from modelDir, then validates the model manifest.
func LoadInitialise(opt Options) (*Runtime, error) {
	md, err := metadata.Load(opt.ModelDir)
	if err != nil {
		return nil, fmt.Errorf("pipeline: %w", err)
	}

	textTokPath := filepath.Join(opt.ModelDir, "tokenizer.json")
	textTok, err := tokenizer.FromFile(textTokPath, md.ModelConfig.TextAddBOS)
	if err != nil {
		return nil, fmt.Errorf("pipeline: load text tokenizer: %w", err)
	}

	var capTok *tokenizer.Tokenizer
	if md.IsCaptionMode() {
		capPath := filepath.Join(opt.ModelDir, "caption_tokenizer.json")
		addBOS := md.CaptionAddBOSSafe()
		capTok, err = tokenizer.FromFile(capPath, addBOS)
		if err != nil {
			return nil, fmt.Errorf("pipeline: load caption tokenizer: %w", err)
		}
	}

	textEncInputs, textEncOutputs := ioNames(md, "text_encoder")
	textEnc, err := ort.NewSession(
		md.FilePath(opt.ModelDir, "text_encoder"),
		textEncInputs, textEncOutputs,
	)
	if err != nil {
		return nil, fmt.Errorf("pipeline: load text_encoder: %w", err)
	}

	var capEnc *ort.Session
	if md.IsCaptionMode() {
		capEncInputs, capEncOutputs := ioNames(md, "caption_encoder")
		capEnc, err = ort.NewSession(
			md.FilePath(opt.ModelDir, "caption_encoder"),
			capEncInputs, capEncOutputs,
		)
		if err != nil {
			return nil, fmt.Errorf("pipeline: load caption_encoder: %w", err)
		}
	}

	var speakerEnc *ort.Session
	var dacEnc *ort.Session
	if !md.IsCaptionMode() {
		spkInputs, spkOutputs := ioNames(md, "speaker_encoder")
		speakerEnc, err = ort.NewSession(
			md.FilePath(opt.ModelDir, "speaker_encoder"),
			spkInputs, spkOutputs,
		)
		if err != nil {
			return nil, fmt.Errorf("pipeline: load speaker_encoder: %w", err)
		}

		dacEncInputs, dacEncOutputs := ioNames(md, "dacvae_encoder")
		dacEnc, err = ort.NewSession(
			md.FilePath(opt.ModelDir, "dacvae_encoder"),
			dacEncInputs, dacEncOutputs,
		)
		if err != nil {
			return nil, fmt.Errorf("pipeline: load dacvae_encoder: %w", err)
		}
	}

	var durPred *ort.Session
	if md.UseDurationPredictor {
		durInputs, durOutputs := ioNames(md, "duration_predictor")
		durPred, err = ort.NewSession(
			md.FilePath(opt.ModelDir, "duration_predictor"),
			durInputs, durOutputs,
		)
		if err != nil {
			return nil, fmt.Errorf("pipeline: load duration_predictor: %w", err)
		}
	}

	ditInputs, ditOutputs := ioNames(md, "dit_step")
	ditStep, err := ort.NewSession(
		md.FilePath(opt.ModelDir, "dit_step"),
		ditInputs, ditOutputs,
	)
	if err != nil {
		return nil, fmt.Errorf("pipeline: load dit_step: %w", err)
	}

	decInputs, decOutputs := ioNames(md, "dacvae_decoder")
	decDAC, err := ort.NewSession(
		md.FilePath(opt.ModelDir, "dacvae_decoder"),
		decInputs, decOutputs,
	)
	if err != nil {
		return nil, fmt.Errorf("pipeline: load dacvae_decoder: %w", err)
	}

	return &Runtime{
		md:         md,
		opt:        opt,
		textTok:    textTok,
		capTok:     capTok,
		textEnc:    textEnc,
		capEnc:     capEnc,
		speakerEnc: speakerEnc,
		durPred:    durPred,
		dacEnc:     dacEnc,
		ditStep:    ditStep,
		decDAC:     decDAC,
	}, nil
}

// Close releases all ONNX sessions.
func (r *Runtime) Close() {
	if r.closed {
		return
	}
	r.closed = true
	if r.textEnc != nil {
		r.textEnc.Destroy()
	}
	if r.capEnc != nil {
		r.capEnc.Destroy()
	}
	if r.speakerEnc != nil {
		r.speakerEnc.Destroy()
	}
	if r.durPred != nil {
		r.durPred.Destroy()
	}
	if r.dacEnc != nil {
		r.dacEnc.Destroy()
	}
	if r.ditStep != nil {
		r.ditStep.Destroy()
	}
	if r.decDAC != nil {
		r.decDAC.Destroy()
	}
}

// Synthesize runs the full inference pipeline and writes a 16-bit PCM
// WAV file to the path specified in Options.OutputWAV.
func (r *Runtime) Synthesize() error {
	if r.closed {
		return fmt.Errorf("pipeline: runtime closed")
	}
	md := r.md
	opt := r.opt

	// Step 1: Tokenize text
	textIDs, textMask := r.textTok.EncodePadded(opt.Text, 256)
	seqLen := int64(len(textIDs))

	// Step 2: Run text encoder
	textState, err := r.runTextEncoder(textIDs, textMask, seqLen)
	if err != nil {
		return fmt.Errorf("pipeline: text_encoder: %w", err)
	}

	// Step 3: Run caption or speaker encoder
	var capState []float32
	var capMask []bool
	capLen := int64(1)

	var speakerState []float32
	var speakerMask []bool
	speakerLen := int64(1)
	hasSpeaker := false

	if md.IsCaptionMode() {
		if opt.Caption != "" {
			capIDs, capMaskSlice := r.capTok.EncodePadded(opt.Caption, 64)
			capLen = int64(len(capIDs))
			capState, err = r.runCaptionEncoder(capIDs, capMaskSlice, capLen)
			if err != nil {
				return fmt.Errorf("pipeline: caption_encoder: %w", err)
			}
			capMask = capMaskSlice
		} else {
			capIDs := make([]int64, 1)
			capMaskRaw := make([]bool, 1)
			capLen = 1
			capState, err = r.runCaptionEncoder(capIDs, capMaskRaw, capLen)
			if err != nil {
				return fmt.Errorf("pipeline: caption_encoder (null): %w", err)
			}
			capMask = capMaskRaw
		}
	} else {
		if opt.RefWAV != "" {
			refSamples, refRate, err := wav.ReadWAV(opt.RefWAV)
			if err != nil {
				return fmt.Errorf("pipeline: read ref wav: %w", err)
			}
			refSamples = wav.Resample(refSamples, refRate, md.SampleRate)

			hop := md.HopLength
			if len(refSamples)%hop != 0 {
				padding := hop - (len(refSamples) % hop)
				padded := make([]float32, len(refSamples)+padding)
				copy(padded, refSamples)
				refSamples = padded
			}
			refLen := int64(len(refSamples) / hop)

			refLatent, err := r.runDACEncoder(refSamples, refLen)
			if err != nil {
				return fmt.Errorf("pipeline: dacvae_encoder for reference: %w", err)
			}

			refMask := make([]bool, refLen)
			for i := range refMask {
				refMask[i] = true
			}

			speakerState, speakerMask, err = r.runSpeakerEncoder(refLatent, refMask, refLen)
			if err != nil {
				return fmt.Errorf("pipeline: speaker_encoder: %w", err)
			}
			speakerLen = refLen
			hasSpeaker = true
		} else {
			refLen := int64(1)
			refLatent := make([]float32, refLen*int64(md.SpeakerPatchedLatentDim))
			refMask := make([]bool, refLen)

			speakerState, speakerMask, err = r.runSpeakerEncoder(refLatent, refMask, refLen)
			if err != nil {
				return fmt.Errorf("pipeline: speaker_encoder (null): %w", err)
			}
			speakerLen = refLen
			hasSpeaker = false
		}
	}

	// Step 4: Determine latent length
	var latentLen int64
	if opt.Seconds > 0 {
		latentLen = int64(mathCeil(opt.Seconds*float64(md.SampleRate)) / float64(md.HopLength))
	} else if !md.IsCaptionMode() && md.UseDurationPredictor && r.durPred != nil {
		logFrames, err := r.runDurationPredictor(textState, textMask, speakerState, hasSpeaker)
		if err != nil {
			return fmt.Errorf("pipeline: duration_predictor: %w", err)
		}
		predFrames := math.Expm1(float64(logFrames))
		scaled := predFrames * opt.DurationScale
		latentLen = int64(math.Round(scaled))
		minFrames := int64(math.Ceil(0.5 * float64(md.SampleRate) / float64(md.HopLength)))
		maxFrames := int64(math.Floor(30.0 * float64(md.SampleRate) / float64(md.HopLength)))
		if latentLen < minFrames {
			latentLen = minFrames
		} else if latentLen > maxFrames {
			latentLen = maxFrames
		}
	} else {
		latentLen = int64(mathCeil(10.0*float64(md.SampleRate)) / float64(md.HopLength))
	}
	latentLen = max(latentLen, 1)

	// Step 5: Initialize noise
	patchedDim := int64(md.EffectivePatchedLatentDim())
	rng := tensor.NewMulberry32(opt.Seed)
	noiseLen := latentLen * patchedDim
	xT := make([]float32, noiseLen)
	tensor.RandnF32(xT, rng)

	// Step 6: Build t-schedule
	tSchedule := tensor.EulerTSchedule(opt.NumSteps, true, -1.0)

	// Step 7: Build CFG config
	cfg := sampler.CfgConfig{
		ScaleText:     opt.CfgText,
		ScaleCaption:  opt.CfgCaption,
		ScaleSpeaker:  opt.CfgSpeaker,
		MinT:          0.5,
		MaxT:          1.0,
		UseTextCfg:    opt.CfgText > 0,
		UseCaptionCfg: md.IsCaptionMode() && opt.CfgCaption > 0 && opt.Caption != "",
		UseSpeakerCfg: !md.IsCaptionMode() && opt.CfgSpeaker > 0 && hasSpeaker,
	}

	// Step 8: Compute RoPE frequencies for DiT
	freqsCis := tensor.RoPEFreqs(md.HeadDims.DiT, int(latentLen), 10000.0)

	// Step 9: Euler denoising loop
	bundles := cfg.ActiveBundles()
	cfgBatch := int64(len(bundles))

	for i := 0; i < opt.NumSteps; i++ {
		tVal := float64(tSchedule[i])
		tNextVal := float64(tSchedule[i+1])
		dtVal := float32(tNextVal - tVal)
		tF32 := float32(tVal)

		if cfg.CfgActive(tVal) {
			err = r.runDiTStepCFG(xT, textState, textMask, capState, capMask, speakerState, speakerMask, seqLen, capLen, speakerLen, latentLen, patchedDim, cfgBatch, bundles, cfg, tF32, dtVal, freqsCis)
		} else {
			err = r.runDiTStepSingle(xT, textState, textMask, capState, capMask, speakerState, speakerMask, seqLen, capLen, speakerLen, latentLen, patchedDim, tF32, dtVal, freqsCis)
		}
		if err != nil {
			return fmt.Errorf("pipeline: dit step %d: %w", i, err)
		}
	}

	// Step 10: Decode latent to audio
	audio, err := r.runDACDecoder(xT, latentLen, patchedDim)
	if err != nil {
		return fmt.Errorf("pipeline: dacvae_decoder: %w", err)
	}

	// Step 11: Write WAV
	if err := wav.WriteMonoPCM16(opt.OutputWAV, audio, md.SampleRate); err != nil {
		return fmt.Errorf("pipeline: write wav: %w", err)
	}
	return nil
}

func (r *Runtime) runTextEncoder(ids []int64, mask []bool, seqLen int64) ([]float32, error) {
	md := r.md
	textDim := int64(md.ModelConfig.TextDim)
	shape := []int64{1, seqLen}
	idTensor, err := ort.NewInt64Tensor(ids, shape)
	if err != nil {
		return nil, err
	}
	defer idTensor.Destroy()
	maskTensor, err := ort.NewBoolTensor(mask, shape)
	if err != nil {
		return nil, err
	}
	defer maskTensor.Destroy()
	freqsCis := tensor.RoPEFreqs(md.HeadDims.Text, int(seqLen), 10000.0)
	freqsShape := []int64{seqLen, int64(md.HeadDims.Text / 2), 2}
	freqsTensor, err := ort.NewFloat32Tensor(freqsCis, freqsShape)
	if err != nil {
		return nil, err
	}
	defer freqsTensor.Destroy()

	outShape := []int64{1, seqLen, textDim}
	outTensor, err := ort.NewEmptyFloat32Tensor(outShape)
	if err != nil {
		return nil, err
	}
	defer outTensor.Destroy()

	inputs := []onnx.ArbitraryTensor{idTensor, maskTensor, freqsTensor}
	outputs := []onnx.ArbitraryTensor{outTensor}
	if err := r.textEnc.Run(inputs, outputs); err != nil {
		return nil, err
	}
	result := make([]float32, len(outTensor.GetData()))
	copy(result, outTensor.GetData())
	return result, nil
}

func (r *Runtime) runCaptionEncoder(ids []int64, mask []bool, seqLen int64) ([]float32, error) {
	md := r.md
	capDim := int64(md.CaptionDim)
	shape := []int64{1, seqLen}
	idTensor, err := ort.NewInt64Tensor(ids, shape)
	if err != nil {
		return nil, err
	}
	defer idTensor.Destroy()
	maskTensor, err := ort.NewBoolTensor(mask, shape)
	if err != nil {
		return nil, err
	}
	defer maskTensor.Destroy()
	freqsCis := tensor.RoPEFreqs(md.HeadDims.Caption, int(seqLen), 10000.0)
	freqsShape := []int64{seqLen, int64(md.HeadDims.Caption / 2), 2}
	freqsTensor, err := ort.NewFloat32Tensor(freqsCis, freqsShape)
	if err != nil {
		return nil, err
	}
	defer freqsTensor.Destroy()

	outShape := []int64{1, seqLen, capDim}
	outTensor, err := ort.NewEmptyFloat32Tensor(outShape)
	if err != nil {
		return nil, err
	}
	defer outTensor.Destroy()

	if err := r.capEnc.Run(
		[]onnx.ArbitraryTensor{idTensor, maskTensor, freqsTensor},
		[]onnx.ArbitraryTensor{outTensor},
	); err != nil {
		return nil, err
	}
	result := make([]float32, len(outTensor.GetData()))
	copy(result, outTensor.GetData())
	return result, nil
}

func (r *Runtime) runSpeakerEncoder(refLatent []float32, refMask []bool, refLen int64) ([]float32, []bool, error) {
	md := r.md
	spkDim := int64(md.ModelConfig.SpeakerDim)
	spkPatchedLatentDim := int64(md.SpeakerPatchedLatentDim)

	latentShape := []int64{1, refLen, spkPatchedLatentDim}
	latentTensor, err := ort.NewFloat32Tensor(refLatent, latentShape)
	if err != nil {
		return nil, nil, err
	}
	defer latentTensor.Destroy()

	maskShape := []int64{1, refLen}
	maskTensor, err := ort.NewBoolTensor(refMask, maskShape)
	if err != nil {
		return nil, nil, err
	}
	defer maskTensor.Destroy()

	freqsCis := tensor.RoPEFreqs(md.HeadDims.Speaker, int(refLen), 10000.0)
	freqsShape := []int64{refLen, int64(md.HeadDims.Speaker / 2), 2}
	freqsTensor, err := ort.NewFloat32Tensor(freqsCis, freqsShape)
	if err != nil {
		return nil, nil, err
	}
	defer freqsTensor.Destroy()

	outShape := []int64{1, refLen + 1, spkDim}
	outTensor, err := ort.NewEmptyFloat32Tensor(outShape)
	if err != nil {
		return nil, nil, err
	}
	defer outTensor.Destroy()

	outMaskShape := []int64{1, refLen + 1}
	outMaskTensor, err := onnx.NewEmptyTensor[bool](outMaskShape)
	if err != nil {
		return nil, nil, err
	}
	defer outMaskTensor.Destroy()

	inputs := []onnx.ArbitraryTensor{latentTensor, maskTensor, freqsTensor}
	outputs := []onnx.ArbitraryTensor{outTensor, outMaskTensor}
	if err := r.speakerEnc.Run(inputs, outputs); err != nil {
		return nil, nil, err
	}

	resultState := make([]float32, len(outTensor.GetData()))
	copy(resultState, outTensor.GetData())

	resultMask := make([]bool, len(outMaskTensor.GetData()))
	copy(resultMask, outMaskTensor.GetData())

	return resultState, resultMask, nil
}

func (r *Runtime) runDurationPredictor(textState []float32, textMask []bool, speakerState []float32, hasSpeaker bool) (float32, error) {
	md := r.md
	textSeqLen := int64(len(textMask))
	textDim := int64(md.ModelConfig.TextDim)
	speakerSeqLen := int64(len(speakerState) / md.ModelConfig.SpeakerDim)
	speakerDim := int64(md.ModelConfig.SpeakerDim)

	textTensor, err := ort.NewFloat32Tensor(textState, []int64{1, textSeqLen, textDim})
	if err != nil {
		return 0, err
	}
	defer textTensor.Destroy()

	textMaskTensor, err := ort.NewBoolTensor(textMask, []int64{1, textSeqLen})
	if err != nil {
		return 0, err
	}
	defer textMaskTensor.Destroy()

	speakerTensor, err := ort.NewFloat32Tensor(speakerState, []int64{1, speakerSeqLen, speakerDim})
	if err != nil {
		return 0, err
	}
	defer speakerTensor.Destroy()

	hasSpeakerVal := make([]bool, 1)
	hasSpeakerVal[0] = hasSpeaker
	hasSpeakerTensor, err := ort.NewBoolTensor(hasSpeakerVal, []int64{1})
	if err != nil {
		return 0, err
	}
	defer hasSpeakerTensor.Destroy()

	outTensor, err := ort.NewEmptyFloat32Tensor([]int64{1})
	if err != nil {
		return 0, err
	}
	defer outTensor.Destroy()

	inputs := []onnx.ArbitraryTensor{textTensor, textMaskTensor, speakerTensor, hasSpeakerTensor}
	outputs := []onnx.ArbitraryTensor{outTensor}
	if err := r.durPred.Run(inputs, outputs); err != nil {
		return 0, err
	}

	return outTensor.GetData()[0], nil
}

func (r *Runtime) runDACEncoder(audio []float32, refLen int64) ([]float32, error) {
	md := r.md
	decodedLen := refLen * int64(md.HopLength)

	audioShape := []int64{1, 1, decodedLen}
	audioTensor, err := ort.NewFloat32Tensor(audio, audioShape)
	if err != nil {
		return nil, err
	}
	defer audioTensor.Destroy()

	outShape := []int64{1, refLen, int64(md.ModelConfig.LatentDim)}
	outTensor, err := ort.NewEmptyFloat32Tensor(outShape)
	if err != nil {
		return nil, err
	}
	defer outTensor.Destroy()

	inputs := []onnx.ArbitraryTensor{audioTensor}
	outputs := []onnx.ArbitraryTensor{outTensor}
	if err := r.dacEnc.Run(inputs, outputs); err != nil {
		return nil, err
	}

	result := make([]float32, len(outTensor.GetData()))
	copy(result, outTensor.GetData())
	return result, nil
}

func (r *Runtime) runDiTStepCFG(
	xT []float32,
	textState []float32, textMask []bool,
	capState []float32, capMask []bool,
	speakerState []float32, speakerMask []bool,
	textSeqLen, capSeqLen, speakerSeqLen, latentLen, patchedDim int64,
	cfgBatch int64, bundles []sampler.CfgBundle, cfg sampler.CfgConfig,
	t, dt float32, freqsCis []float32,
) error {
	md := r.md
	stride := latentLen * patchedDim

	xTBatch := make([]float32, int(cfgBatch)*int(stride))
	for i := int64(0); i < cfgBatch; i++ {
		copy(xTBatch[i*stride:], xT)
	}

	textDim64 := int64(md.ModelConfig.TextDim)
	stackedText := stackTextState(textState, int(textSeqLen), bundles, int(textDim64))
	stackedTextMask := stackTextMask(textMask, int(textSeqLen), bundles)

	var stackedCap []float32
	var stackedCapMask []bool
	if md.IsCaptionMode() {
		capDim64 := int64(md.CaptionDim)
		stackedCap = stackCaptionState(capState, int(capSeqLen), bundles, int(capDim64))
		stackedCapMask = stackCaptionMask(capMask, int(capSeqLen), bundles)
	}

	var stackedSpeaker []float32
	var stackedSpeakerMask []bool
	if !md.IsCaptionMode() {
		speakerDim64 := int64(md.ModelConfig.SpeakerDim)
		stackedSpeaker = stackSpeakerState(speakerState, int(speakerSeqLen), bundles, int(speakerDim64))
		stackedSpeakerMask = stackSpeakerMask(speakerMask, int(speakerSeqLen), bundles)
	}

	tInput := make([]float32, cfgBatch)
	for i := range tInput {
		tInput[i] = t
	}

	xTShape := []int64{cfgBatch, latentLen, patchedDim}
	xTTensor, err := ort.NewFloat32Tensor(xTBatch, xTShape)
	if err != nil {
		return err
	}
	defer xTTensor.Destroy()

	tTensor, err := ort.NewFloat32Tensor(tInput, []int64{cfgBatch})
	if err != nil {
		return err
	}
	defer tTensor.Destroy()

	textShape := []int64{cfgBatch, textSeqLen, textDim64}
	textTensor, err := ort.NewFloat32Tensor(stackedText, textShape)
	if err != nil {
		return err
	}
	defer textTensor.Destroy()

	textMaskShape := []int64{cfgBatch, textSeqLen}
	textMaskTensor, err := ort.NewBoolTensor(stackedTextMask, textMaskShape)
	if err != nil {
		return err
	}
	defer textMaskTensor.Destroy()

	freqsShape := []int64{latentLen, int64(md.HeadDims.DiT / 2), 2}
	freqsTensor, err := ort.NewFloat32Tensor(freqsCis, freqsShape)
	if err != nil {
		return err
	}
	defer freqsTensor.Destroy()

	outShape := []int64{cfgBatch, latentLen, patchedDim}
	outTensor, err := ort.NewEmptyFloat32Tensor(outShape)
	if err != nil {
		return err
	}
	defer outTensor.Destroy()

	inputs := []onnx.ArbitraryTensor{xTTensor, tTensor, textTensor, textMaskTensor}
	if md.IsCaptionMode() {
		capDim64 := int64(md.CaptionDim)
		capTensor, err := ort.NewFloat32Tensor(stackedCap, []int64{cfgBatch, capSeqLen, capDim64})
		if err != nil {
			return err
		}
		defer capTensor.Destroy()
		capMaskTensor, err := ort.NewBoolTensor(stackedCapMask, []int64{cfgBatch, capSeqLen})
		if err != nil {
			return err
		}
		defer capMaskTensor.Destroy()
		inputs = append(inputs, capTensor, capMaskTensor)
	} else {
		speakerDim64 := int64(md.ModelConfig.SpeakerDim)
		spkTensor, err := ort.NewFloat32Tensor(stackedSpeaker, []int64{cfgBatch, speakerSeqLen, speakerDim64})
		if err != nil {
			return err
		}
		defer spkTensor.Destroy()
		spkMaskTensor, err := ort.NewBoolTensor(stackedSpeakerMask, []int64{cfgBatch, speakerSeqLen})
		if err != nil {
			return err
		}
		defer spkMaskTensor.Destroy()
		inputs = append(inputs, spkTensor, spkMaskTensor)
	}
	inputs = append(inputs, freqsTensor)
	outputs := []onnx.ArbitraryTensor{outTensor}

	if err := r.ditStep.Run(inputs, outputs); err != nil {
		return err
	}

	vAll := outTensor.GetData()
	vCond := vAll[0*stride : 1*stride]
	var vNoText, vNoSpeaker, vNoCaption []float32
	idx := int64(1)
	if cfg.UseTextCfg {
		vNoText = vAll[idx*stride : (idx+1)*stride]
		idx++
	}
	if cfg.UseSpeakerCfg {
		vNoSpeaker = vAll[idx*stride : (idx+1)*stride]
		idx++
	}
	if cfg.UseCaptionCfg {
		vNoCaption = vAll[idx*stride : (idx+1)*stride]
		idx++
	}

	vAdjusted := sampler.ApplyCFG(cfg, vCond, vNoText, vNoSpeaker, vNoCaption)
	sampler.EulerStep(xT, vAdjusted, dt)
	return nil
}

func (r *Runtime) runDiTStepSingle(
	xT []float32,
	textState []float32, textMask []bool,
	capState []float32, capMask []bool,
	speakerState []float32, speakerMask []bool,
	textSeqLen, capSeqLen, speakerSeqLen, latentLen, patchedDim int64,
	t, dt float32, freqsCis []float32,
) error {
	md := r.md
	textDim64 := int64(md.ModelConfig.TextDim)

	xTShape := []int64{1, latentLen, patchedDim}
	xTTensor, err := ort.NewFloat32Tensor(xT, xTShape)
	if err != nil {
		return err
	}
	defer xTTensor.Destroy()

	tTensor, err := ort.NewFloat32Tensor([]float32{t}, []int64{1})
	if err != nil {
		return err
	}
	defer tTensor.Destroy()

	textTensor, err := ort.NewFloat32Tensor(textState, []int64{1, textSeqLen, textDim64})
	if err != nil {
		return err
	}
	defer textTensor.Destroy()

	textMaskTensor, err := ort.NewBoolTensor(textMask, []int64{1, textSeqLen})
	if err != nil {
		return err
	}
	defer textMaskTensor.Destroy()

	freqsShape := []int64{latentLen, int64(md.HeadDims.DiT / 2), 2}
	freqsTensor, err := ort.NewFloat32Tensor(freqsCis, freqsShape)
	if err != nil {
		return err
	}
	defer freqsTensor.Destroy()

	outShape := []int64{1, latentLen, patchedDim}
	outTensor, err := ort.NewEmptyFloat32Tensor(outShape)
	if err != nil {
		return err
	}
	defer outTensor.Destroy()

	inputs := []onnx.ArbitraryTensor{xTTensor, tTensor, textTensor, textMaskTensor}
	if md.IsCaptionMode() {
		capDim64 := int64(md.CaptionDim)
		capTensor, err := ort.NewFloat32Tensor(capState, []int64{1, capSeqLen, capDim64})
		if err != nil {
			return err
		}
		defer capTensor.Destroy()
		capMaskTensor, err := ort.NewBoolTensor(capMask, []int64{1, capSeqLen})
		if err != nil {
			return err
		}
		defer capMaskTensor.Destroy()
		inputs = append(inputs, capTensor, capMaskTensor)
	} else {
		speakerDim64 := int64(md.ModelConfig.SpeakerDim)
		spkTensor, err := ort.NewFloat32Tensor(speakerState, []int64{1, speakerSeqLen, speakerDim64})
		if err != nil {
			return err
		}
		defer spkTensor.Destroy()
		spkMaskTensor, err := ort.NewBoolTensor(speakerMask, []int64{1, speakerSeqLen})
		if err != nil {
			return err
		}
		defer spkMaskTensor.Destroy()
		inputs = append(inputs, spkTensor, spkMaskTensor)
	}
	inputs = append(inputs, freqsTensor)
	outputs := []onnx.ArbitraryTensor{outTensor}

	if err := r.ditStep.Run(inputs, outputs); err != nil {
		return err
	}
	vPred := outTensor.GetData()
	axpy(xT, vPred, dt)
	return nil
}

func (r *Runtime) runDACDecoder(latent []float32, latentLen, patchedDim int64) ([]float32, error) {
	md := r.md
	latentDim := int64(md.ModelConfig.LatentDim)
	decodedLen := latentLen * int64(md.HopLength)

	latentShape := []int64{1, latentLen, latentDim}
	latentTensor, err := ort.NewFloat32Tensor(latent, latentShape)
	if err != nil {
		return nil, err
	}
	defer latentTensor.Destroy()

	audioShape := []int64{1, 1, decodedLen}
	audioTensor, err := ort.NewEmptyFloat32Tensor(audioShape)
	if err != nil {
		return nil, err
	}
	defer audioTensor.Destroy()

	if err := r.decDAC.Run(
		[]onnx.ArbitraryTensor{latentTensor},
		[]onnx.ArbitraryTensor{audioTensor},
	); err != nil {
		return nil, err
	}
	return audioTensor.GetData(), nil
}

func ioNames(md *metadata.Metadata, exportName string) ([]string, []string) {
	ei := md.Exports[exportName]
	return ei.Inputs, ei.Outputs
}

func boolsToU8(bs []bool) []uint8 {
	out := make([]uint8, len(bs))
	for i, v := range bs {
		if v {
			out[i] = 1
		}
	}
	return out
}

func u8ToBools(u8s []uint8) []bool {
	out := make([]bool, len(u8s))
	for i, v := range u8s {
		out[i] = v != 0
	}
	return out
}

func axpy(y []float32, x []float32, a float32) {
	for i := range y {
		y[i] += a * x[i]
	}
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func mathCeil(f float64) float64 {
	v := float64(int64(f))
	if v < f {
		v++
	}
	return v
}

func stackTextState(base []float32, seqLen int, bundles []sampler.CfgBundle, textDim int) []float32 {
	stride := seqLen * textDim
	out := make([]float32, len(bundles)*stride)
	for i, b := range bundles {
		if b == sampler.BundleNoText {
			// zero fill
		} else {
			copy(out[i*stride:], base)
		}
	}
	return out
}

func stackTextMask(base []bool, seqLen int, bundles []sampler.CfgBundle) []bool {
	out := make([]bool, len(bundles)*seqLen)
	for i, b := range bundles {
		if b == sampler.BundleNoText {
			// zero fill (false)
		} else {
			copy(out[i*seqLen:], base)
		}
	}
	return out
}

func stackCaptionState(base []float32, seqLen int, bundles []sampler.CfgBundle, capDim int) []float32 {
	stride := seqLen * capDim
	out := make([]float32, len(bundles)*stride)
	for i, b := range bundles {
		if b == sampler.BundleNoCaption {
			// zero fill
		} else {
			copy(out[i*stride:], base)
		}
	}
	return out
}

func stackCaptionMask(base []bool, seqLen int, bundles []sampler.CfgBundle) []bool {
	out := make([]bool, len(bundles)*seqLen)
	for i, b := range bundles {
		if b == sampler.BundleNoCaption {
			// zero fill (false)
		} else {
			copy(out[i*seqLen:], base)
		}
	}
	return out
}

func stackSpeakerState(base []float32, seqLen int, bundles []sampler.CfgBundle, spkDim int) []float32 {
	stride := seqLen * spkDim
	out := make([]float32, len(bundles)*stride)
	for i, b := range bundles {
		if b == sampler.BundleNoSpeaker {
			// zero fill
		} else {
			copy(out[i*stride:], base)
		}
	}
	return out
}

func stackSpeakerMask(base []bool, seqLen int, bundles []sampler.CfgBundle) []bool {
	out := make([]bool, len(bundles)*seqLen)
	for i, b := range bundles {
		if b == sampler.BundleNoSpeaker {
			// zero fill (false)
		} else {
			copy(out[i*seqLen:], base)
		}
	}
	return out
}
