package sampler_pingpong

import (
	"fmt"
	"math"

	"fm-live-radio/internal/musicgen/stableaudio/tensor"
)

// StepFn is the callback to execute one DiT model forward pass.
// It receives the current latents and the current timestep t, and returns the predicted velocity.
type StepFn func(x []float32, t float32) ([]float32, error)

// LogSNRShift maps t in [0,1] through log-SNR space.
func LogSNRShift(t float32, anchorLogSNR, logsnrEnd float32) float32 {
	if t <= 0.0 {
		return 0.0
	}
	if t >= 1.0 {
		return 1.0
	}
	logsnr := logsnrEnd - t*(logsnrEnd-anchorLogSNR)
	// Sigmoid(-logsnr) = 1.0 / (1.0 + exp(logsnr))
	return float32(1.0 / (1.0 + math.Exp(float64(logsnr))))
}

// BuildPingpongSchedule returns the sigmas schedule of length steps+1.
func BuildPingpongSchedule(steps int, sigmaMax float32, useLogSNRShift bool) []float32 {
	sigmas := make([]float32, steps+1)
	for i := 0; i <= steps; i++ {
		// linspace from sigmaMax to 0
		t := sigmaMax * (1.0 - float32(i)/float32(steps))
		if useLogSNRShift {
			t = LogSNRShift(t, -6.2, 2.0)
		}
		sigmas[i] = t
	}
	// Re-anchor start to sigmaMax if shifted
	if useLogSNRShift {
		sigmas[0] = sigmaMax
	}
	return sigmas
}

// SampleFlowPingpong executes the 8-step rectified flow pingpong sampling loop.
func SampleFlowPingpong(
	x []float32,
	sigmas []float32,
	seed uint32,
	stepFn StepFn,
	onStep func(step, totalSteps int),
) ([]float32, error) {
	numSteps := len(sigmas) - 1
	currentX := make([]float32, len(x))
	copy(currentX, x)

	for i := 0; i < numSteps; i++ {
		tCurr := sigmas[i]
		tNext := sigmas[i+1]

		// Execute DiT step forward pass to predict velocity
		v, err := stepFn(currentX, tCurr)
		if err != nil {
			return nil, fmt.Errorf("sampler: step %d failed: %w", i, err)
		}
		if len(v) != len(currentX) {
			return nil, fmt.Errorf("sampler: velocity length %d mismatch latent length %d", len(v), len(currentX))
		}

		// denoised = x - tCurr * v
		denoised := make([]float32, len(currentX))
		for idx := range denoised {
			denoised[idx] = currentX[idx] - tCurr*v[idx]
		}

		if i < numSteps-1 && tNext > 0.0 {
			// Generate standard normal noise using Mulberry32 seeded for this step
			// We offset the seed by the step index to split the random state per step
			rng := tensor.NewMulberry32(seed + uint32(i))
			noise := make([]float32, len(currentX))
			tensor.RandnF32(noise, rng)

			// x = (1.0 - tNext) * denoised + tNext * noise
			for idx := range currentX {
				currentX[idx] = (1.0-tNext)*denoised[idx] + tNext*noise[idx]
			}
		} else {
			currentX = denoised
		}

		if onStep != nil {
			onStep(i+1, numSteps)
		}
	}

	return currentX, nil
}
