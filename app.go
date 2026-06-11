package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fm-live-radio/internal/audio"
	"fm-live-radio/internal/bgm"
	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/musicgen"
	"fm-live-radio/internal/player"
	"fm-live-radio/internal/store"
	"fm-live-radio/internal/talk"
)

// App struct
type App struct {
	ctx context.Context

	mu sync.Mutex

	store    *store.Store
	cfg      domain.AppConfig
	history  domain.History
	audioSrv *audio.Server
	talkSvc  *talk.Service
	musicSvc *musicgen.Service
	player   *player.Player
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	s, err := store.New()
	if err != nil {
		log.Printf("ERROR: store init failed: %v", err)
		return
	}
	cfg, err := s.LoadConfig()
	if err != nil {
		log.Printf("ERROR: config load failed: %v", err)
		cfg = store.DefaultConfig()
	}
	h, err := s.LoadHistory()
	if err != nil {
		log.Printf("WARN: history load failed: %v", err)
		h = domain.History{UsedArticleUrls: []string{}}
	}

	as, err := audio.Start()
	if err != nil {
		log.Printf("ERROR: audio server start failed: %v", err)
		return
	}

	// Ensure temp_audio exists and do best-effort cleanup of stale files.
	tmpDir := s.TempAudioDir()
	_ = os.MkdirAll(tmpDir, 0o755)
	_ = cleanupTempAudio(tmpDir)

	talkSvc := talk.New(tmpDir)
	musicSvc := musicgen.New()

	a.mu.Lock()
	a.store = s
	a.cfg = cfg
	a.history = h
	a.audioSrv = as
	a.talkSvc = talkSvc
	a.musicSvc = musicSvc
	a.player = player.New(cfg)
	a.mu.Unlock()
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	as := a.audioSrv
	a.audioSrv = nil
	a.mu.Unlock()

	if as != nil {
		_ = as.Close(ctx)
	}
	_ = generation.Shutdown()
}

func cleanupTempAudio(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
	return nil
}

// LoadConfig returns current config (creates default if missing).
func (a *App) LoadConfig() (domain.AppConfig, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg, nil
}

// SaveConfig persists config and applies it to runtime.
func (a *App) SaveConfig(cfg domain.AppConfig) error {
	a.mu.Lock()
	s := a.store
	p := a.player
	a.cfg = cfg
	a.mu.Unlock()

	if s != nil {
		if err := s.SaveConfig(cfg); err != nil {
			return err
		}
	}
	if p != nil {
		p.UpdateConfig(cfg)
	}
	return nil
}

// ScanGenres lists genre folders under current BGM root.
func (a *App) ScanGenres() ([]string, error) {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	return bgm.ListGenres(cfg.BGMRootPath)
}

// GetNextItem returns the next playable item for the player.
func (a *App) GetNextItem(req domain.NextItemRequest) (domain.PlayableItem, error) {
	a.mu.Lock()
	p := a.player
	as := a.audioSrv
	ts := a.talkSvc
	ms := a.musicSvc
	h := a.history
	s := a.store
	a.mu.Unlock()
	if p == nil || as == nil {
		return domain.PlayableItem{}, player.ErrNotConfigured
	}

	item, newHist, histUpdated, err := p.NextItem(as, ts, ms, req, h)
	if err != nil {
		return domain.PlayableItem{}, err
	}
	if histUpdated {
		a.mu.Lock()
		a.history = newHist
		a.mu.Unlock()
		if s != nil {
			_ = s.SaveHistory(newHist)
		}
	}
	return item, nil
}

// SkipCurrent skips current item and returns the next.
func (a *App) SkipCurrent(req domain.SkipRequest) (domain.PlayableItem, error) {
	a.mu.Lock()
	p := a.player
	as := a.audioSrv
	ts := a.talkSvc
	ms := a.musicSvc
	h := a.history
	s := a.store
	a.mu.Unlock()
	if p == nil || as == nil {
		return domain.PlayableItem{}, player.ErrNotConfigured
	}

	item, newHist, histUpdated, err := p.Skip(as, ts, ms, req, h)
	if err != nil {
		return domain.PlayableItem{}, err
	}
	if histUpdated {
		a.mu.Lock()
		a.history = newHist
		a.mu.Unlock()
		if s != nil {
			_ = s.SaveHistory(newHist)
		}
	}
	return item, nil
}

// GetStatus returns lightweight runtime status for UI indicators.
func (a *App) GetStatus() (domain.AppStatus, error) {
	a.mu.Lock()
	p := a.player
	a.mu.Unlock()
	if p == nil {
		return domain.AppStatus{}, nil
	}
	return p.Status(), nil
}

// PrefetchTalk kicks off talk generation prefetch.
func (a *App) PrefetchTalk() {
	a.mu.Lock()
	p := a.player
	ts := a.talkSvc
	ms := a.musicSvc
	cfg := a.cfg
	h := a.history
	a.mu.Unlock()
	if p != nil {
		p.PrefetchTalk(ts, cfg, h)
		p.PrefetchMusic(ms, cfg, cfg.SelectedGenre)
	}
	// small delay to keep binding non-blocking even after implementation
	time.Sleep(0)
}
