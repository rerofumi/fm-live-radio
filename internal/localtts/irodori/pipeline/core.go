package pipeline

import (
	"math"

	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts/irodori/sampler"
	"fm-live-radio/internal/localtts/irodori/tensor"
	onnx "github.com/yalue/onnxruntime_go"
)

const (
	defaultV4Steps = 40
	minV4Seconds   = 0.5
	maxV4Seconds   = 30.0
)

// durationInput is the single raw duration-predictor contract shared by
// product synthesis and the value-bearing parity harness. Keeping tensor
// construction here prevents the two paths from silently using different
// shapes, masks, or condition flags.
type durationInput struct {
	textState, speakerState, captionState []float32
	textMask, speakerMask, captionMask    []bool
	features                              []float32
	hasSpeaker, hasCaption                bool
}

func (r *v4Runtime) runDurationRaw(in durationInput) ([]float32, error) {
	text, err := generation.NewFloat32Tensor(in.textState, []int64{1, int64(len(in.textMask)), int64(r.manifest.Conditions.TextDim)})
	if err != nil {
		return nil, err
	}
	defer text.Destroy()
	textMask, err := generation.NewBoolTensor(in.textMask, []int64{1, int64(len(in.textMask))})
	if err != nil {
		return nil, err
	}
	defer textMask.Destroy()
	speaker, err := generation.NewFloat32Tensor(in.speakerState, []int64{1, int64(len(in.speakerMask)), int64(r.manifest.Conditions.SpeakerDim)})
	if err != nil {
		return nil, err
	}
	defer speaker.Destroy()
	speakerMask, err := generation.NewBoolTensor(in.speakerMask, []int64{1, int64(len(in.speakerMask))})
	if err != nil {
		return nil, err
	}
	defer speakerMask.Destroy()
	caption, err := generation.NewFloat32Tensor(in.captionState, []int64{1, int64(len(in.captionMask)), int64(r.manifest.Conditions.CaptionDim)})
	if err != nil {
		return nil, err
	}
	defer caption.Destroy()
	captionMask, err := generation.NewBoolTensor(in.captionMask, []int64{1, int64(len(in.captionMask))})
	if err != nil {
		return nil, err
	}
	defer captionMask.Destroy()
	features, err := generation.NewFloat32Tensor(in.features, []int64{1, int64(len(in.features))})
	if err != nil {
		return nil, err
	}
	defer features.Destroy()
	hasSpeaker, err := generation.NewBoolTensor([]bool{in.hasSpeaker}, []int64{1})
	if err != nil {
		return nil, err
	}
	defer hasSpeaker.Destroy()
	hasCaption, err := generation.NewBoolTensor([]bool{in.hasCaption}, []int64{1})
	if err != nil {
		return nil, err
	}
	defer hasCaption.Destroy()
	out, err := generation.NewEmptyFloat32Tensor([]int64{1})
	if err != nil {
		return nil, err
	}
	defer out.Destroy()
	if err := r.runNamed(r.duration, "duration_predictor", map[string]onnx.ArbitraryTensor{
		"text_state": text, "text_mask": textMask,
		"speaker_state": speaker, "speaker_mask": speakerMask,
		"caption_state": caption, "caption_mask": captionMask,
		"duration_features": features, "has_speaker": hasSpeaker, "has_caption": hasCaption,
	}, map[string]onnx.ArbitraryTensor{"log_frames": out}); err != nil {
		return nil, err
	}
	return append([]float32(nil), out.GetData()...), nil
}

// durationFrames is the single product/parity duration contract. The model
// emits log(1+frames); scaling is applied after expm1, then round and clamp
// are performed in that order. Explicit seconds uses the same hop conversion
// and bypasses the predictor, as in the official runtime.
func durationFrames(rawLogFrames float32, durationScale, seconds float64, sampleRate, hopLength int) int64 {
	if sampleRate <= 0 || hopLength <= 0 {
		return 0
	}
	minFrames := int64(math.Ceil(minV4Seconds * float64(sampleRate) / float64(hopLength)))
	maxFrames := int64(math.Floor(maxV4Seconds * float64(sampleRate) / float64(hopLength)))
	if seconds > 0 {
		seconds = math.Max(minV4Seconds, math.Min(maxV4Seconds, seconds))
		return int64(math.Ceil(seconds * float64(sampleRate) / float64(hopLength)))
	}
	if durationScale <= 0 {
		durationScale = 1
	}
	frames := math.Expm1(float64(rawLogFrames)) * durationScale
	n := int64(math.Round(frames))
	if n < minFrames {
		return minFrames
	}
	if n > maxFrames {
		return maxFrames
	}
	return n
}

// runDenoising is the shared product/parity 40-step orchestration. The
// caller supplies the same initial latent and condition tensors used by the
// product path; parity therefore cannot accidentally validate a second outer
// loop with different schedule or CFG semantics.
func (r *v4Runtime) runDenoising(noise []float32, textState []float32, textMask []bool, speakerState []float32, speakerMask []bool, captionState []float32, captionMask []bool, latentLen int64, cfg sampler.CfgConfig, steps int) error {
	if steps <= 0 {
		steps = defaultV4Steps
	}
	schedule := tensor.EulerTSchedule(steps, false, 0)
	bundles := cfg.ActiveBundles()
	for i := 0; i < steps; i++ {
		t := schedule[i]
		dt := schedule[i+1] - t
		if cfg.CfgActive(float64(t)) {
			if err := r.runDiTCFG(noise, textState, textMask, speakerState, speakerMask, captionState, captionMask, latentLen, bundles, cfg, t, dt); err != nil {
				return err
			}
		} else if err := r.runDiTSingle(noise, textState, textMask, speakerState, speakerMask, captionState, captionMask, latentLen, t, dt); err != nil {
			return err
		}
	}
	return nil
}
