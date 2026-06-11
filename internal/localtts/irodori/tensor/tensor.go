// Package tensor holds tiny tensor utilities used by the Irodori-TTS
// runtime: precompute of RoPE frequency tables, Euler t-schedules with
// optional sway, and a deterministic Box-Muller RNG.
//
// All layouts match the ONNX sub-graphs produced by
// mtsmfm/Irodori-TTS-ONNX: for example freqs_cis is laid out as
// (seq_len, head_dim/2, 2) with the trailing 2 being (cos, sin).
package tensor

import (
	"math"
)

// RoPEFreqs returns the RoPE frequency table for one component.
//
// The output is laid out in row-major (seq_len, head_dim/2, 2):
//
//	out[(s * half + p) * 2 + 0] = cos(s * inv_freq[p])
//	out[(s * half + p) * 2 + 1] = sin(s * inv_freq[p])
//
// head_dim must be even. theta defaults to 10000.0 (matches the
// upstream Irodori-TTS precompute_freqs_cis).
func RoPEFreqs(headDim, seqLen int, theta float64) []float32 {
	if headDim%2 != 0 {
		panic("tensor: head_dim must be even")
	}
	if theta == 0 {
		theta = 10000.0
	}
	half := headDim / 2
	inv := make([]float64, half)
	for i := 0; i < half; i++ {
		inv[i] = 1.0 / math.Pow(theta, float64(2*i)/float64(headDim))
	}
	out := make([]float32, seqLen*half*2)
	for s := 0; s < seqLen; s++ {
		for p := 0; p < half; p++ {
			angle := float64(s) * inv[p]
			base := (s*half + p) * 2
			out[base+0] = float32(math.Cos(angle))
			out[base+1] = float32(math.Sin(angle))
		}
	}
	return out
}

// EulerTSchedule builds the t schedule used by sample_euler_rf_cfg.
// With sway disabled it produces (1 - i/n) * 0.999 for i in 0..n — i.e.
// a linear schedule that starts at ~0.999 and ends at 0.
//
// The output has length numSteps+1, with strictly decreasing values.
// With sway enabled, u = i/n is warped by u + coeff*(cos(0.5*pi*u) + u - 1)
// matching the upstream sway sampling helper.
func EulerTSchedule(numSteps int, sway bool, swayCoeff float64) []float32 {
	if swayCoeff == 0 {
		swayCoeff = -1.0
	}
	out := make([]float32, numSteps+1)
	for i := 0; i <= numSteps; i++ {
		u := float64(i) / float64(numSteps)
		if sway {
			u = u + swayCoeff*(math.Cos(0.5*math.Pi*u)+u-1.0)
			if u < 0 {
				u = 0
			}
			if u > 1 {
				u = 1
			}
		}
		out[i] = float32((1.0 - u) * 0.999)
	}
	// Enforce strict decrease: replace any non-decreasing step with the
	// previous value minus a tiny epsilon. With default sway this is
	// already strictly decreasing, so the assertion below only fires
	// when the user gave a sway that flattens the schedule.
	for i := 0; i < numSteps; i++ {
		if !(out[i] > out[i+1]) {
			out[i+1] = out[i] - 1e-6
		}
	}
	return out
}

// Mulberry32 is a tiny deterministic 32-bit PRNG. Identical to the
// implementation in the JS reference (tensor-utils.ts).
type Mulberry32 struct {
	state uint32
}

// NewMulberry32 returns a Mulberry32 RNG seeded with the given uint32.
func NewMulberry32(seed uint32) *Mulberry32 {
	return &Mulberry32{state: seed}
}

// Next returns the next float in [0, 1).
func (m *Mulberry32) Next() float64 {
	m.state = (m.state + 0x6d2b79f5) & 0xffffffff
	t := m.state
	t ^= t >> 15
	t *= t | 1
	t ^= t + ((t ^ (t >> 7)) * (t | 61))
	return float64((t^(t>>14))&0xffffffff) / 4294967296.0
}

// RandnF32 fills dst with standard normal samples drawn from the given
// RNG, using the Box-Muller transform.
func RandnF32(dst []float32, rng *Mulberry32) {
	for i := 0; i+1 < len(dst); i += 2 {
		u1 := math.Max(rng.Next(), math.SmallestNonzeroFloat64)
		u2 := rng.Next()
		mag := math.Sqrt(-2.0 * math.Log(u1))
		dst[i+0] = float32(mag * math.Cos(2.0*math.Pi*u2))
		dst[i+1] = float32(mag * math.Sin(2.0*math.Pi*u2))
	}
	if len(dst)%2 == 1 {
		u1 := math.Max(rng.Next(), math.SmallestNonzeroFloat64)
		u2 := rng.Next()
		mag := math.Sqrt(-2.0 * math.Log(u1))
		dst[len(dst)-1] = float32(mag * math.Cos(2.0*math.Pi*u2))
	}
}

// ConcatBatch stacks n copies of the same data slice into a single
// contiguous buffer. Used to build the cfgBatch dimension for DiT
// step inputs.
func ConcatBatchF32(parts [][]float32) []float32 {
	if len(parts) == 0 {
		return nil
	}
	perBatch := len(parts[0])
	total := perBatch * len(parts)
	out := make([]float32, total)
	for i, p := range parts {
		copy(out[i*perBatch:], p)
	}
	return out
}

// ConcatBatchU8 is the uint8 counterpart used for bool masks.
func ConcatBatchU8(parts [][]uint8) []uint8 {
	if len(parts) == 0 {
		return nil
	}
	perBatch := len(parts[0])
	total := perBatch * len(parts)
	out := make([]uint8, total)
	for i, p := range parts {
		copy(out[i*perBatch:], p)
	}
	return out
}
