package localtts

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fm-live-radio/internal/audiofmt"
	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts/irodori/metadata"
	"fm-live-radio/internal/localtts/irodori/pipeline"
)

const (
	irodoriSampleRate        = 48000
	irodoriChannels          = 1
	irodoriSentenceGap       = 300 * time.Millisecond
	irodoriSentenceFailPause = 3 * time.Second
)

var ErrEmptyText = errors.New("irodori text is empty")

type Service struct {
	mu sync.Mutex
}

func New() *Service {
	return &Service{}
}

func (s *Service) SynthesizeWav(ctx context.Context, cfg domain.AppConfig, text string) ([]byte, error) {
	if strings.TrimSpace(cfg.Irodori.ModelDir) == "" {
		return nil, generation.ErrProviderNotConfigured
	}
	if err := generation.ConfigureExecutionProvider(cfg.LocalInference.ExecutionProvider, cfg.LocalInference.DeviceID); err != nil {
		return nil, err
	}
	if err := generation.Init(cfg.LocalInference.ORTLibraryPath); err != nil {
		return nil, err
	}
	if err := validateModelAssets(cfg.Irodori.ModelDir); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.synthesizeSentences(ctx, cfg, text)
}

func (s *Service) synthesizeSentences(ctx context.Context, cfg domain.AppConfig, text string) ([]byte, error) {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return nil, ErrEmptyText
	}

	combined := make([]byte, 0)
	gap, err := audiofmt.SilencePCM16(irodoriSampleRate, irodoriChannels, irodoriSentenceGap)
	if err != nil {
		return nil, err
	}
	failSilence, err := audiofmt.SilencePCM16(irodoriSampleRate, irodoriChannels, irodoriSentenceFailPause)
	if err != nil {
		return nil, err
	}

	for i, sentence := range sentences {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		pcm, err := s.synthesizeSentencePCM(ctx, cfg, sentence)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			pcm = failSilence
		}
		combined = append(combined, pcm...)
		if i < len(sentences)-1 {
			combined = append(combined, gap...)
		}
	}

	return audiofmt.EncodeWavPCM16(combined, irodoriSampleRate, irodoriChannels)
}

func (s *Service) synthesizeSentencePCM(ctx context.Context, cfg domain.AppConfig, text string) ([]byte, error) {
	outPath, err := s.synthesizeToFile(ctx, cfg, text)
	if err != nil {
		return nil, err
	}
	defer os.Remove(outPath)

	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}
	wav, err := audiofmt.DecodeWavPCM16(data)
	if err != nil {
		return nil, err
	}
	if wav.SampleRate != irodoriSampleRate || wav.Channels != irodoriChannels {
		return nil, fmt.Errorf("irodori wav format mismatch: %d Hz, %d channels", wav.SampleRate, wav.Channels)
	}
	return wav.PCM, nil
}

func (s *Service) synthesizeToFile(ctx context.Context, cfg domain.AppConfig, text string) (string, error) {
	tmpDir := os.TempDir()
	name := fmt.Sprintf("irodori_%d.wav", time.Now().UnixNano())
	outPath := filepath.Join(tmpDir, name)

	opt := pipeline.DefaultOptions()
	opt.ModelDir = cfg.Irodori.ModelDir
	opt.Text = text
	opt.OutputWAV = outPath
	opt.Seconds = cfg.Irodori.Seconds
	opt.NumSteps = cfg.Irodori.NumSteps
	opt.CfgText = cfg.Irodori.CfgText
	opt.CfgCaption = cfg.Irodori.CfgCaption
	opt.CfgSpeaker = cfg.Irodori.CfgSpeaker
	opt.DurationScale = cfg.Irodori.DurationScale
	opt.RefWAV = resolveReferenceWAV(cfg.Irodori)
	opt.Seed = resolveSeed(cfg.Irodori.SeedMode, cfg.Irodori.FixedSeed)

	rt, err := pipeline.LoadInitialise(opt)
	if err != nil {
		return "", err
	}
	defer rt.Close()

	done := make(chan error, 1)
	go func() {
		done <- rt.Synthesize()
	}()

	select {
	case err := <-done:
		if err != nil {
			return "", err
		}
		return outPath, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func resolveReferenceWAV(cfg domain.IrodoriConfig) string {
	if strings.TrimSpace(cfg.RefWAV) != "" {
		return cfg.RefWAV
	}
	entries, err := os.ReadDir(cfg.NarratorDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".wav") {
			return filepath.Join(cfg.NarratorDir, entry.Name())
		}
	}
	return ""
}

func resolveSeed(mode string, fixed uint32) uint32 {
	switch strings.TrimSpace(mode) {
	case "fixed":
		return fixed
	case "sequential":
		return uint32(time.Now().Unix())
	default:
		return rand.Uint32()
	}
}

func validateModelAssets(modelDir string) error {
	if _, err := os.Stat(filepath.Join(modelDir, "tokenizer.json")); err != nil {
		return err
	}
	md, err := metadata.Load(modelDir)
	if err != nil {
		return err
	}
	for name := range md.Exports {
		p := md.FilePath(modelDir, name)
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			return err
		}
	}
	return nil
}

func splitSentences(text string) []string {
	var sentences []string
	var b strings.Builder
	flush := func() {
		s := strings.TrimSpace(b.String())
		if s != "" {
			sentences = append(sentences, s)
		}
		b.Reset()
	}

	for _, r := range text {
		switch r {
		case '\r', '\n':
			flush()
			continue
		}
		b.WriteRune(r)
		switch r {
		case '。', '！', '？', '!', '?':
			flush()
		}
	}
	flush()

	return sentences
}
