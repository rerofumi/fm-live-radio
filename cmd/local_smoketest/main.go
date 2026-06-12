package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts"
	"fm-live-radio/internal/musicgen"
	"fm-live-radio/internal/store"
)

type wavStats struct {
	SampleRate int
	Channels   int
	Frames     int
	Peak       float64
	RMS        float64
}

func main() {
	cfg := store.DefaultConfig()
	cfg.StableAudio3.Seconds = 6
	cfg.StableAudio3.Steps = 4
	cfg.StableAudio3.OutputDir = filepath.Join("generate_music", "smoketest")
	cfg.Irodori.Seconds = -1
	cfg.Irodori.NumSteps = 12
	cfg.Irodori.DurationScale = 0.9
	cfg.LocalInference.ExecutionProvider = resolveEnvOrDefault("FM_RADIO_ORT_EP", cfg.LocalInference.ExecutionProvider)
	cfg.LocalInference.DeviceID = resolveEnvInt("FM_RADIO_ORT_DEVICE_ID", cfg.LocalInference.DeviceID)
	cfg.LocalInference.ORTLibraryPath = resolveEnvOrDefault("FM_RADIO_ORT_LIB", cfg.LocalInference.ORTLibraryPath)

	if err := generation.ConfigureExecutionProvider(cfg.LocalInference.ExecutionProvider, cfg.LocalInference.DeviceID); err != nil {
		fail("configure ort execution provider", err)
	}

	_ = os.MkdirAll(cfg.StableAudio3.OutputDir, 0o755)

	musicSvc := musicgen.New()
	ttsSvc := localtts.New()

	fmt.Println("== Stable Audio 3 smoke test ==")
	musicStart := time.Now()
	musicCtx, musicCancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer musicCancel()
	musicRes, err := musicSvc.Generate(musicCtx, cfg, "ambient")
	if err != nil {
		fail("stable audio generate", err)
	}
	musicStats, err := inspectWav(musicRes.AudioPath)
	if err != nil {
		fail("stable audio inspect", err)
	}
	printStats(musicRes.AudioPath, musicStats, time.Since(musicStart))
	assertNonSilent("stable audio", musicStats)

	fmt.Println("== IrodoriTTS smoke test ==")
	ttsStart := time.Now()
	ttsCtx, ttsCancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer ttsCancel()
	wavBytes, err := ttsSvc.SynthesizeWav(ttsCtx, withIrodori(cfg), "こんにちは。こちらは FM Live Radio のローカル音声生成テストです。")
	if err != nil {
		fail("irodori synthesize", err)
	}
	ttsPath := filepath.Join("temp_audio", fmt.Sprintf("smoketest_tts_%d.wav", time.Now().UnixNano()))
	_ = os.MkdirAll(filepath.Dir(ttsPath), 0o755)
	if err := os.WriteFile(ttsPath, wavBytes, 0o600); err != nil {
		fail("write tts wav", err)
	}
	ttsStats, err := inspectWav(ttsPath)
	if err != nil {
		fail("irodori inspect", err)
	}
	printStats(ttsPath, ttsStats, time.Since(ttsStart))
	assertNonSilent("irodori", ttsStats)

	fmt.Println("smoke test passed")
}

func resolveEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func resolveEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func withIrodori(cfg domain.AppConfig) domain.AppConfig {
	cfg.TTSSource = domain.TTSSourceIrodori
	return cfg
}

func inspectWav(path string) (wavStats, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return wavStats{}, err
	}
	if len(data) < 44 {
		return wavStats{}, errors.New("wav too short")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return wavStats{}, errors.New("invalid wav header")
	}
	channels := int(binary.LittleEndian.Uint16(data[22:24]))
	sampleRate := int(binary.LittleEndian.Uint32(data[24:28]))
	bitsPerSample := int(binary.LittleEndian.Uint16(data[34:36]))
	if bitsPerSample != 16 {
		return wavStats{}, fmt.Errorf("unsupported bits per sample: %d", bitsPerSample)
	}
	dataOffset := 44
	if len(data) <= dataOffset {
		return wavStats{}, errors.New("wav has no pcm payload")
	}
	samples := (len(data) - dataOffset) / 2
	if samples == 0 {
		return wavStats{}, errors.New("wav has zero samples")
	}
	var peak float64
	var sum float64
	for i := dataOffset; i+1 < len(data); i += 2 {
		v := int16(binary.LittleEndian.Uint16(data[i : i+2]))
		f := math.Abs(float64(v) / 32768.0)
		if f > peak {
			peak = f
		}
		sum += f * f
	}
	frames := samples
	if channels > 0 {
		frames /= channels
	}
	return wavStats{
		SampleRate: sampleRate,
		Channels:   channels,
		Frames:     frames,
		Peak:       peak,
		RMS:        math.Sqrt(sum / float64(samples)),
	}, nil
}

func printStats(path string, st wavStats, elapsed time.Duration) {
	fmt.Printf("file: %s\n", path)
	fmt.Printf("sampleRate=%d channels=%d frames=%d peak=%.4f rms=%.4f elapsed=%s\n", st.SampleRate, st.Channels, st.Frames, st.Peak, st.RMS, elapsed.Round(time.Millisecond))
}

func assertNonSilent(name string, st wavStats) {
	if st.Peak <= 0 || st.RMS <= 0 {
		fail(name, errors.New("generated wav is silent"))
	}
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", step, err)
	os.Exit(1)
}
