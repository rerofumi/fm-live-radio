package musicgen

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	Genre     string
}

type Service struct {
	observerMu sync.RWMutex
	observer   sa3.EventObserver
}

func New() *Service {
	return &Service{}
}

// SetObserver installs diagnostic runtime events for verification harnesses.
func (s *Service) SetObserver(observer sa3.EventObserver) {
	s.observerMu.Lock()
	s.observer = observer
	s.observerMu.Unlock()
}

func (s *Service) observerSnapshot() sa3.EventObserver {
	s.observerMu.RLock()
	defer s.observerMu.RUnlock()
	return s.observer
}

func (s *Service) Generate(ctx context.Context, cfg domain.AppConfig) (Result, error) {
	if strings.TrimSpace(cfg.StableAudio3.ModelDir) == "" || strings.TrimSpace(cfg.StableAudio3.OutputDir) == "" {
		return Result{}, generation.ErrProviderNotConfigured
	}
	if err := generation.ConfigureExecutionProvider(cfg.LocalInference.ExecutionProvider, cfg.LocalInference.DeviceID); err != nil {
		return Result{}, err
	}
	if err := generation.Init(cfg.LocalInference.ORTLibraryPath); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(cfg.StableAudio3.OutputDir, 0o755); err != nil {
		return Result{}, err
	}

	prompt := BuildPrompt(cfg)
	seed := resolveMusicSeed(cfg.StableAudio3.SeedMode, cfg.StableAudio3.FixedSeed)
	outPath := filepath.Join(cfg.StableAudio3.OutputDir, fmt.Sprintf("music_%d.wav", time.Now().UnixNano()))

	opt := sa3.DefaultOptions()
	opt.Prompt = prompt
	opt.Seconds = cfg.StableAudio3.Seconds
	opt.Steps = cfg.StableAudio3.Steps
	opt.Seed = seed
	opt.ModelDir = cfg.StableAudio3.ModelDir
	opt.OutputWAV = outPath
	opt.Observer = s.observerSnapshot()

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
		if observer := s.observerSnapshot(); observer != nil {
			observer(sa3.EventServiceJoined)
		}
		if ctx.Err() != nil {
			_ = os.Remove(outPath)
			return Result{}, ctx.Err()
		}
		if err != nil {
			_ = os.Remove(outPath)
			return Result{}, err
		}
	case <-ctx.Done():
		// Stable Audio has no portable in-flight abort API. Join the inference
		// goroutine before the deferred Runtime.Close, and never publish a WAV
		// that completed after cancellation.
		_ = <-done
		if observer := s.observerSnapshot(); observer != nil {
			observer(sa3.EventServiceJoined)
		}
		_ = os.Remove(outPath)
		return Result{}, ctx.Err()
	}

	_ = TrimCache(cfg.StableAudio3.OutputDir, cfg.StableAudio3.CacheLimit, outPath)
	return Result{
		AudioPath: outPath,
		Title:     "Stable Audio 3",
		Prompt:    prompt,
		Seed:      seed,
		Genre:     SelectedGenre(cfg),
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
		Genre:     SelectedGenre(cfg),
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
