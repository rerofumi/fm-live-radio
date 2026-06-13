package audiofmt

import (
	"encoding/binary"
	"math"
	"testing"
)

func makePCM16(samples []int16) []byte {
	out := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[i*2:i*2+2], uint16(s))
	}
	return out
}

func buildMonoWav(t *testing.T, sampleRate int, samples []int16) []byte {
	t.Helper()
	pcm := makePCM16(samples)
	wav, err := EncodeWavPCM16(pcm, sampleRate, 1)
	if err != nil {
		t.Fatalf("EncodeWavPCM16: %v", err)
	}
	return wav
}

func buildStereoWav(t *testing.T, sampleRate int, interleaved []int16) []byte {
	t.Helper()
	if len(interleaved)%2 != 0 {
		t.Fatalf("interleaved length must be even, got %d", len(interleaved))
	}
	pcm := makePCM16(interleaved)
	wav, err := EncodeWavPCM16(pcm, sampleRate, 2)
	if err != nil {
		t.Fatalf("EncodeWavPCM16: %v", err)
	}
	return wav
}

func TestComputeWavLoudnessEnvelope_Silence(t *testing.T) {
	sr := 1000
	// 200ms of silence -> with 50ms window -> 4 windows
	samples := make([]int16, sr/5)
	wav := buildMonoWav(t, sr, samples)

	env, err := ComputeWavLoudnessEnvelope(wav, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.WindowMS != 50 {
		t.Fatalf("WindowMS=%d, want 50", env.WindowMS)
	}
	if env.SampleRate != sr {
		t.Fatalf("SampleRate=%d, want %d", env.SampleRate, sr)
	}
	if len(env.RMS) != 4 {
		t.Fatalf("len(RMS)=%d, want 4 (got %v)", len(env.RMS), env.RMS)
	}
	if len(env.Peak) != 4 {
		t.Fatalf("len(Peak)=%d, want 4", len(env.Peak))
	}
	for i, v := range env.RMS {
		if v != 0 {
			t.Errorf("RMS[%d]=%v, want 0", i, v)
		}
	}
	for i, v := range env.Peak {
		if v != 0 {
			t.Errorf("Peak[%d]=%v, want 0", i, v)
		}
	}
	if math.Abs(env.DurationSec-0.2) > 1e-9 {
		t.Errorf("DurationSec=%v, want ~0.2", env.DurationSec)
	}
}

func TestComputeWavLoudnessEnvelope_FullScalePeakAndRMS(t *testing.T) {
	sr := 1000
	// 50ms window at 1000 Hz = 50 samples per window.
	// Fill 50 samples with +32767 (near full scale) -> RMS ~= 32767/32768 ~ 1, peak = 32767/32768.
	samples := make([]int16, 50)
	for i := range samples {
		samples[i] = 32767
	}
	wav := buildMonoWav(t, sr, samples)

	env, err := ComputeWavLoudnessEnvelope(wav, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.RMS) != 1 {
		t.Fatalf("len(RMS)=%d, want 1", len(env.RMS))
	}
	if got := env.RMS[0]; got < 0.999 || got > 1.0 {
		t.Errorf("RMS[0]=%v, want ~1.0 (0..1 clamp)", got)
	}
	if got := env.Peak[0]; got < 0.999 || got > 1.0 {
		t.Errorf("Peak[0]=%v, want ~1.0", got)
	}
}

func TestComputeWavLoudnessEnvelope_WindowingHalfFull(t *testing.T) {
	sr := 1000
	// 50ms window = 50 samples. First window all 0, second window all 16384 (half full scale).
	samples := make([]int16, 100)
	for i := 50; i < 100; i++ {
		samples[i] = 16384
	}
	wav := buildMonoWav(t, sr, samples)

	env, err := ComputeWavLoudnessEnvelope(wav, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.RMS) != 2 {
		t.Fatalf("len(RMS)=%d, want 2", len(env.RMS))
	}
	if env.RMS[0] != 0 {
		t.Errorf("RMS[0]=%v, want 0 (silence window)", env.RMS[0])
	}
	want := 16384.0 / 32768.0
	if math.Abs(env.RMS[1]-want) > 1e-6 {
		t.Errorf("RMS[1]=%v, want %v", env.RMS[1], want)
	}
	if math.Abs(env.Peak[1]-want) > 1e-6 {
		t.Errorf("Peak[1]=%v, want %v", env.Peak[1], want)
	}
}

func TestComputeWavLoudnessEnvelope_TrailingPartialWindow(t *testing.T) {
	sr := 1000
	// 75 samples at sr=1000 = 75 ms -> 50ms window expects 2 windows (50 + 25).
	samples := make([]int16, 75)
	for i := range samples {
		samples[i] = 8000
	}
	wav := buildMonoWav(t, sr, samples)

	env, err := ComputeWavLoudnessEnvelope(wav, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.RMS) != 2 {
		t.Fatalf("len(RMS)=%d, want 2 (trailing partial)", len(env.RMS))
	}
	want := 8000.0 / 32768.0
	for i, v := range env.RMS {
		if math.Abs(v-want) > 1e-6 {
			t.Errorf("RMS[%d]=%v, want ~%v", i, v, want)
		}
	}
}

func TestComputeWavLoudnessEnvelope_StereoAveraging(t *testing.T) {
	sr := 1000
	// 50ms window = 50 frames * 2 channels = 100 samples.
	// Left channel +8000, right channel -8000 -> per-sample squared mean = 8000^2 -> RMS = 8000/32768.
	interleaved := make([]int16, 100)
	for f := 0; f < 50; f++ {
		interleaved[f*2] = 8000
		interleaved[f*2+1] = -8000
	}
	wav := buildStereoWav(t, sr, interleaved)
	env, err := ComputeWavLoudnessEnvelope(wav, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.RMS) != 1 {
		t.Fatalf("len(RMS)=%d, want 1", len(env.RMS))
	}
	want := 8000.0 / 32768.0
	if math.Abs(env.RMS[0]-want) > 1e-6 {
		t.Errorf("RMS[0]=%v, want %v (stereo averaging)", env.RMS[0], want)
	}
	if math.Abs(env.Peak[0]-want) > 1e-6 {
		t.Errorf("Peak[0]=%v, want %v", env.Peak[0], want)
	}
}

func TestComputeWavLoudnessEnvelope_RejectsNonWav(t *testing.T) {
	if _, err := ComputeWavLoudnessEnvelope([]byte("not a wav"), 50); err == nil {
		t.Fatalf("expected error for non-wav input")
	}
}

func TestComputeWavLoudnessEnvelope_DefaultWindow(t *testing.T) {
	sr := 1000
	samples := make([]int16, 100)
	wav := buildMonoWav(t, sr, samples)
	env, err := ComputeWavLoudnessEnvelope(wav, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.WindowMS != 50 {
		t.Errorf("WindowMS=%d, want default 50", env.WindowMS)
	}
}
