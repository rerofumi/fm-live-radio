package tensor

import (
	"math"
)

// Mulberry32 is a tiny deterministic 32-bit PRNG.
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
