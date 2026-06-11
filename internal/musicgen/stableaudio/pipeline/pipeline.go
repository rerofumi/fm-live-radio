package pipeline

import (
	"fmt"
	"math"
	"path/filepath"

	ort "fm-live-radio/internal/generation"
	"fm-live-radio/internal/musicgen/stableaudio/sampler_pingpong"
	"fm-live-radio/internal/musicgen/stableaudio/tensor"
	"fm-live-radio/internal/musicgen/stableaudio/tokenizer_t5gemma"
	"fm-live-radio/internal/musicgen/stableaudio/wav"

	onnx "github.com/yalue/onnxruntime_go"
)

// Options holds the synthesis parameters.
type Options struct {
	Prompt     string
	Seconds    float64
	Steps      int
	Seed       uint32
	ModelDir   string
	OutputWAV  string
	SampleRate int
}

// DefaultOptions returns sensible default parameters.
func DefaultOptions() Options {
	return Options{
		Seconds:    30.0,
		Steps:      8,
		Seed:       0,
		SampleRate: 44100,
	}
}

// Runtime holds the loaded models and tokenizer.
type Runtime struct {
	opt     Options
	tok     *tokenizer_t5gemma.Tokenizer
	encoder *ort.Session
	dit     *ort.Session
	decoder *ort.Session
	closed  bool
}

// LoadInitialise loads the tokenizer and ONNX sessions.
func LoadInitialise(opt Options) (*Runtime, error) {
	tokPath := filepath.Join(opt.ModelDir, "tokenizer", "tokenizer.json")
	tok, err := tokenizer_t5gemma.FromFile(tokPath)
	if err != nil {
		return nil, fmt.Errorf("pipeline: load tokenizer: %w", err)
	}

	encoderPath := filepath.Join(opt.ModelDir, "onnx", "t5gemma", "encoder.onnx")
	encoder, err := ort.NewSession(
		encoderPath,
		[]string{"input_ids", "attention_mask"},
		[]string{"hidden_states"},
	)
	if err != nil {
		return nil, fmt.Errorf("pipeline: load encoder: %w", err)
	}

	ditPath := filepath.Join(opt.ModelDir, "onnx", "sa3-sm-music", "dit_fp16mixed.onnx")
	dit, err := ort.NewSession(
		ditPath,
		[]string{"x", "t", "t5_hidden", "t5_mask", "seconds_total", "local_add_cond"},
		[]string{"velocity"},
	)
	if err != nil {
		encoder.Destroy()
		return nil, fmt.Errorf("pipeline: load dit: %w", err)
	}

	decoderPath := filepath.Join(opt.ModelDir, "onnx", "same-s", "dec_dynamic_bf16.onnx")
	decoder, err := ort.NewSession(
		decoderPath,
		[]string{"latent"},
		[]string{"pcm"},
	)
	if err != nil {
		encoder.Destroy()
		dit.Destroy()
		return nil, fmt.Errorf("pipeline: load decoder: %w", err)
	}

	return &Runtime{
		opt:     opt,
		tok:     tok,
		encoder: encoder,
		dit:     dit,
		decoder: decoder,
	}, nil
}

// Close destroys the ONNX sessions.
func (r *Runtime) Close() {
	if r.closed {
		return
	}
	r.closed = true
	if r.encoder != nil {
		r.encoder.Destroy()
	}
	if r.dit != nil {
		r.dit.Destroy()
	}
	if r.decoder != nil {
		r.decoder.Destroy()
	}
}

// Synthesize runs the full text-to-audio generation pipeline.
func (r *Runtime) Synthesize(onStep func(step, totalSteps int)) error {
	if r.closed {
		return fmt.Errorf("pipeline: runtime closed")
	}

	const (
		SamplesPerLatent = 4096
	)

	opt := r.opt

	// 1. Tokenize prompt (max len = 256)
	inputIDs, t5Mask := r.tok.EncodePadded(opt.Prompt, 256)

	// Prepare attention_mask for T5Gemma as int64
	attentionMaskInt64 := make([]int64, 256)
	for i, v := range t5Mask {
		attentionMaskInt64[i] = int64(v)
	}

	// 2. Run T5Gemma encoder
	t5Hidden, err := r.runT5Gemma(inputIDs, attentionMaskInt64)
	if err != nil {
		return fmt.Errorf("pipeline: t5gemma run: %w", err)
	}

	// 3. Determine latent length L
	tLat := int64(math.Ceil(opt.Seconds * float64(opt.SampleRate) / SamplesPerLatent))
	// SAME-S requires T_audio_patches divisible by 32 -> T_lat must be even (T_aud=T_lat*16)
	if tLat%2 != 0 {
		tLat++
	}
	tLat = maxInt64(tLat, 1)

	// 4. Initialize random latent noise
	xSize := 256 * tLat
	x := make([]float32, xSize)
	// Seed Mulberry32 using user-supplied seed
	rng := tensor.NewMulberry32(opt.Seed)
	tensor.RandnF32(x, rng)

	// 5. Generate pingpong schedule sigmas
	sigmas := sampler_pingpong.BuildPingpongSchedule(opt.Steps, 1.0, true)

	// 6. Pre-allocate static inputs for DiT
	localAddCond := make([]float32, 257*tLat) // text-to-audio passes all zeros

	// 7. Define step function for the sampler loop
	stepFn := func(currentX []float32, tCurr float32) ([]float32, error) {
		xTensor, err := ort.NewFloat32Tensor(currentX, []int64{1, 256, tLat})
		if err != nil {
			return nil, err
		}
		defer xTensor.Destroy()

		tTensor, err := ort.NewFloat32Tensor([]float32{tCurr}, []int64{1})
		if err != nil {
			return nil, err
		}
		defer tTensor.Destroy()

		t5HiddenTensor, err := ort.NewFloat32Tensor(t5Hidden, []int64{1, 256, 768})
		if err != nil {
			return nil, err
		}
		defer t5HiddenTensor.Destroy()

		t5MaskTensor, err := ort.NewFloat32Tensor(t5Mask, []int64{1, 256})
		if err != nil {
			return nil, err
		}
		defer t5MaskTensor.Destroy()

		secsTensor, err := ort.NewFloat32Tensor([]float32{float32(opt.Seconds)}, []int64{1})
		if err != nil {
			return nil, err
		}
		defer secsTensor.Destroy()

		lacTensor, err := ort.NewFloat32Tensor(localAddCond, []int64{1, 257, tLat})
		if err != nil {
			return nil, err
		}
		defer lacTensor.Destroy()

		// Output tensor
		outTensor, err := ort.NewEmptyFloat32Tensor([]int64{1, 256, tLat})
		if err != nil {
			return nil, err
		}
		defer outTensor.Destroy()

		inputs := []onnx.ArbitraryTensor{xTensor, tTensor, t5HiddenTensor, t5MaskTensor, secsTensor, lacTensor}
		outputs := []onnx.ArbitraryTensor{outTensor}

		if err := r.dit.Run(inputs, outputs); err != nil {
			return nil, err
		}

		v := make([]float32, len(outTensor.GetData()))
		copy(v, outTensor.GetData())
		return v, nil
	}

	// 8. Run pingpong sampler loop
	latents, err := sampler_pingpong.SampleFlowPingpong(x, sigmas, opt.Seed+1, stepFn, onStep)
	if err != nil {
		return fmt.Errorf("pipeline: sampling: %w", err)
	}

	// 9. Run SAME-S Decoder
	pcmData, err := r.runSAMEsDecoder(latents, tLat)
	if err != nil {
		return fmt.Errorf("pipeline: decoder: %w", err)
	}

	// 10. Trim to exact seconds length
	requestedSamples := int(opt.Seconds * float64(opt.SampleRate))
	numChannels := 2
	totalSamples := len(pcmData)

	if totalSamples > requestedSamples*numChannels {
		pcmData = pcmData[:requestedSamples*numChannels]
	}

	// 11. Write output WAV
	if err := wav.WriteStereoPCM16(opt.OutputWAV, pcmData, opt.SampleRate); err != nil {
		return fmt.Errorf("pipeline: save wav: %w", err)
	}

	return nil
}

func (r *Runtime) runT5Gemma(ids []int64, mask []int64) ([]float32, error) {
	idTensor, err := ort.NewInt64Tensor(ids, []int64{1, 256})
	if err != nil {
		return nil, err
	}
	defer idTensor.Destroy()

	maskTensor, err := ort.NewInt64Tensor(mask, []int64{1, 256})
	if err != nil {
		return nil, err
	}
	defer maskTensor.Destroy()

	outTensor, err := ort.NewEmptyFloat32Tensor([]int64{1, 256, 768})
	if err != nil {
		return nil, err
	}
	defer outTensor.Destroy()

	inputs := []onnx.ArbitraryTensor{idTensor, maskTensor}
	outputs := []onnx.ArbitraryTensor{outTensor}

	if err := r.encoder.Run(inputs, outputs); err != nil {
		return nil, err
	}

	res := make([]float32, len(outTensor.GetData()))
	copy(res, outTensor.GetData())
	return res, nil
}

func (r *Runtime) runSAMEsDecoder(latents []float32, tLat int64) ([]int32, error) {
	latentTensor, err := ort.NewFloat32Tensor(latents, []int64{1, 256, tLat})
	if err != nil {
		return nil, err
	}
	defer latentTensor.Destroy()

	outTensor, err := ort.NewEmptyInt32Tensor([]int64{1, 4096 * tLat, 2})
	if err != nil {
		return nil, err
	}
	defer outTensor.Destroy()

	inputs := []onnx.ArbitraryTensor{latentTensor}
	outputs := []onnx.ArbitraryTensor{outTensor}

	if err := r.decoder.Run(inputs, outputs); err != nil {
		return nil, err
	}

	res := make([]int32, len(outTensor.GetData()))
	copy(res, outTensor.GetData())
	return res, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
