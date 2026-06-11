package musicgen

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	sa3 "fm-live-radio/internal/musicgen/stableaudio/pipeline"
)

type Result struct {
	AudioPath string
	Title     string
	Prompt    string
	Seed      uint32
}

type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) Generate(ctx context.Context, cfg domain.AppConfig, genre string) (Result, error) {
	if strings.TrimSpace(cfg.StableAudio3.ModelDir) == "" || strings.TrimSpace(cfg.StableAudio3.OutputDir) == "" {
		return Result{}, generation.ErrProviderNotConfigured
	}
	if err := generation.Init(cfg.LocalInference.ORTLibraryPath); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(cfg.StableAudio3.OutputDir, 0o755); err != nil {
		return Result{}, err
	}

	prompt := BuildPrompt(cfg, genre)
	seed := resolveMusicSeed(cfg.StableAudio3.SeedMode, cfg.StableAudio3.FixedSeed)
	outPath := filepath.Join(cfg.StableAudio3.OutputDir, fmt.Sprintf("music_%d.wav", time.Now().UnixNano()))

	opt := sa3.DefaultOptions()
	opt.Prompt = prompt
	opt.Seconds = cfg.StableAudio3.Seconds
	opt.Steps = cfg.StableAudio3.Steps
	opt.Seed = seed
	opt.ModelDir = cfg.StableAudio3.ModelDir
	opt.OutputWAV = outPath

	rt, err := sa3.LoadInitialise(opt)
	if err != nil {
		return Result{}, err
	}
	defer rt.Close()

	done := make(chan error, 1)
	go func() {
		done <- rt.Synthesize(nil)
	}()
	select {
	case err := <-done:
		if err != nil {
			return Result{}, err
		}
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}

	_ = TrimCache(cfg.StableAudio3.OutputDir, cfg.StableAudio3.CacheLimit, outPath)
	return Result{
		AudioPath: outPath,
		Title:     fmt.Sprintf("Stable Audio 3 - %s", genre),
		Prompt:    prompt,
		Seed:      seed,
	}, nil
}

func (s *Service) Fallback(cfg domain.AppConfig) (Result, error) {
	path, err := PickFallback(cfg.StableAudio3.OutputDir)
	if err != nil {
		return Result{}, err
	}
	return Result{
		AudioPath: path,
		Title:     filepath.Base(path),
	}, nil
}

func resolveMusicSeed(mode string, fixed uint32) uint32 {
	switch strings.TrimSpace(mode) {
	case "fixed":
		return fixed
	case "sequential":
		return uint32(time.Now().Unix())
	default:
		return rand.Uint32()
	}
}
