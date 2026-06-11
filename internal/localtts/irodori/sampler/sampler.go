// Package sampler implements the Classifier-Free Guidance (CFG) bundle
// assembly and Euler sampling step for the DiT denoising loop.
//
// The Irodori-TTS DiT receives a batch of conditioning variants in a
// single forward pass, and the caller splits the output to compute the
// CFG-adjusted velocity. This package mirrors the JS reference
// pipeline.ts / sample_euler_rf_cfg.
package sampler

import "fm-live-radio/internal/localtts/irodori/tensor"

// CfgBundle identifies one row of the CFG batch.
type CfgBundle string

const (
	BundleCond      CfgBundle = "cond"
	BundleNoText    CfgBundle = "no_text"
	BundleNoSpeaker CfgBundle = "no_speaker"
	BundleNoCaption CfgBundle = "no_caption"
)

// CfgConfig holds the CFG scales and window bounds.
type CfgConfig struct {
	ScaleText     float64
	ScaleSpeaker  float64
	ScaleCaption  float64
	MinT          float64
	MaxT          float64
	UseTextCfg    bool
	UseSpeakerCfg bool
	UseCaptionCfg bool
}

// Defaults returns the standard CFG configuration for Irodori-TTS.
func Defaults() CfgConfig {
	return CfgConfig{
		ScaleText:     3.0,
		ScaleSpeaker:  5.0,
		ScaleCaption:  3.0,
		MinT:          0.5,
		MaxT:          1.0,
		UseTextCfg:    true,
		UseCaptionCfg: true,
		UseSpeakerCfg: false,
	}
}

// ActiveBundles returns the ordered list of CFG bundles that should be
// included in the DiT batch for this configuration.
func (c CfgConfig) ActiveBundles() []CfgBundle {
	bundles := []CfgBundle{BundleCond}
	if c.UseTextCfg {
		bundles = append(bundles, BundleNoText)
	}
	if c.UseSpeakerCfg {
		bundles = append(bundles, BundleNoSpeaker)
	}
	if c.UseCaptionCfg {
		bundles = append(bundles, BundleNoCaption)
	}
	return bundles
}

// CfgActive reports whether CFG should be applied at the given
// timestep t (within the [minT, maxT] window).
func (c CfgConfig) CfgActive(t float64) bool {
	return t >= c.MinT && t <= c.MaxT
}

// ApplyCFG computes the CFG-adjusted velocity from the split DiT
// outputs. The caller must provide vCond and, depending on the CFG
// config, vNoText, vNoSpeaker, and vNoCaption. All slices have length
// stride (= latentLen * patchedLatentDim).
func ApplyCFG(cfg CfgConfig, vCond []float32, vNoText, vNoSpeaker, vNoCaption []float32) []float32 {
	out := make([]float32, len(vCond))
	copy(out, vCond)
	if cfg.UseTextCfg && vNoText != nil {
		for i := range out {
			out[i] += float32(cfg.ScaleText) * (vCond[i] - vNoText[i])
		}
	}
	if cfg.UseSpeakerCfg && vNoSpeaker != nil {
		for i := range out {
			out[i] += float32(cfg.ScaleSpeaker) * (vCond[i] - vNoSpeaker[i])
		}
	}
	if cfg.UseCaptionCfg && vNoCaption != nil {
		for i := range out {
			out[i] += float32(cfg.ScaleCaption) * (vCond[i] - vNoCaption[i])
		}
	}
	return out
}

// StackCondition builds a batched condition tensor by repeating the
// base data for "cond" and filling with zeros for "no_*" bundles.
func StackCondition(baseData []float32, batchLen int, bundles []CfgBundle, condBundle CfgBundle) []float32 {
	return stackF32(baseData, batchLen, bundles, condBundle)
}

// StackMask builds a batched boolean mask by repeating the base mask
// for "cond" and filling with zeros for "no_*" bundles.
func StackMask(baseMask []bool, batchLen int, bundles []CfgBundle, condBundle CfgBundle) []uint8 {
	return stackU8(baseMask, batchLen, bundles, condBundle)
}

func stackF32(base []float32, batchLen int, bundles []CfgBundle, condBundle CfgBundle) []float32 {
	total := len(bundles) * batchLen
	out := make([]float32, total)
	zeros := make([]float32, batchLen)
	parts := make([][]float32, len(bundles))
	for i, b := range bundles {
		if b == condBundle {
			parts[i] = base
		} else {
			parts[i] = zeros
		}
	}
	copy(out, tensor.ConcatBatchF32(parts))
	return out
}

func stackU8(base []bool, batchLen int, bundles []CfgBundle, condBundle CfgBundle) []uint8 {
	baseU8 := boolsToU8(base)
	zeros := make([]uint8, batchLen)
	parts := make([][]uint8, len(bundles))
	for i, b := range bundles {
		if b == condBundle {
			parts[i] = baseU8
		} else {
			parts[i] = zeros
		}
	}
	return tensor.ConcatBatchU8(parts)
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

// EulerStep updates x_t in place by applying the velocity prediction
// with the given timestep interval.
func EulerStep(xT []float32, velocity []float32, dt float32) {
	for i := range xT {
		xT[i] += velocity[i] * dt
	}
}
