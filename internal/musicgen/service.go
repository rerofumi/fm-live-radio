package musicgen

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/fileprotect"
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
	arbiter    *generation.Arbiter
}

func New() *Service {
	return NewWithArbiter(generation.SharedArbiter())
}

// NewWithArbiter provides an isolated scheduler for deterministic service
// tests. Product construction uses the process-shared arbiter via New.
func NewWithArbiter(arbiter *generation.Arbiter) *Service {
	if arbiter == nil {
		arbiter = generation.SharedArbiter()
	}
	return &Service{arbiter: arbiter}
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
	arbiter := s.arbiter
	if arbiter == nil {
		arbiter = generation.SharedArbiter()
	}
	reservation := generation.ReservationFromContext(ctx, generation.KindMusic)
	if reservation != nil && reservation.Arbiter() != arbiter {
		reservation.Release()
		reservation = nil
	}
	if reservation == nil {
		var err error
		reservation, err = arbiter.Reserve(ctx, generation.KindMusic)
		if err != nil {
			return Result{}, err
		}
	}
	defer reservation.Release()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := prepareMusic(cfg); err != nil {
		return Result{}, err
	}
	lease, err := reservation.Acquire(ctx)
	if err != nil {
		return Result{}, err
	}
	defer lease.Release()
	if err := ctx.Err(); err != nil {
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

	rt, err := loadMusicRuntime(opt)
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

	if err := writeMetadata(outPath, Metadata{Genre: SelectedGenre(cfg)}); err != nil {
		_ = os.Remove(outPath)
		return Result{}, err
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

type musicRuntime interface {
	Synthesize(func(step, totalSteps int)) error
	Close()
}

var loadMusicRuntime = func(opt sa3.Options) (musicRuntime, error) {
	return sa3.LoadInitialise(opt)
}

// prepareMusic contains the non-runtime preflight and is replaceable by
// package tests that inject a fake runtime.
var prepareMusic = func(cfg domain.AppConfig) error {
	if strings.TrimSpace(cfg.StableAudio3.ModelDir) == "" || strings.TrimSpace(cfg.StableAudio3.OutputDir) == "" {
		return generation.ErrProviderNotConfigured
	}
	if err := generation.ConfigureExecutionProvider(cfg.LocalInference.ExecutionProvider, cfg.LocalInference.DeviceID); err != nil {
		return err
	}
	if err := generation.Init(cfg.LocalInference.ORTLibraryPath); err != nil {
		return err
	}
	return os.MkdirAll(cfg.StableAudio3.OutputDir, 0o755)
}

func (s *Service) Fallback(cfg domain.AppConfig) (Result, error) {
	path, err := PickFallbackForGenre(cfg.StableAudio3.OutputDir, SelectedGenre(cfg))
	if err != nil {
		return Result{}, err
	}
	return Result{
		AudioPath: path,
		Title:     filepath.Base(path),
		Genre:     SelectedGenre(cfg),
	}, nil
}

// ProtectResult prevents cache trimming from removing a WAV while it is in a
// Player FIFO or being handed to the audio server. The returned function is
// idempotent and should be called when ownership moves to another layer.
func ProtectResult(path string) func() { return fileprotect.Acquire(path) }

// RemoveResult removes only a generated result and its matching sidecar.
func RemoveResult(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
	_ = os.Remove(MetadataPath(path))
}

func writeMetadata(path string, metadata Metadata) error {
	b, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	tmp := MetadataPath(path) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, MetadataPath(path)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
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
