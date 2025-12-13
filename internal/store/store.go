package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"fm-live-radio/internal/domain"
)

const (
	appDirName      = "fm-live-radio"
	configFileName  = "config.json"
	historyFileName = "history.json"
	historyMaxItems = 500
)

type Store struct {
	baseDir string
}

func New() (*Store, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Join(cfgDir, appDirName)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	return &Store{baseDir: baseDir}, nil
}

func (s *Store) BaseDir() string { return s.baseDir }

func (s *Store) TempAudioDir() string { return filepath.Join(s.baseDir, "temp_audio") }

func DefaultConfig() domain.AppConfig {
	return domain.AppConfig{
		BGMRootPath:   "",
		SelectedGenre: "",
		RSSUrls:       []string{},
		GeminiAPIKey:  "",
		Talk: domain.TalkConfig{
			Enabled:           true,
			CycleBgmCount:     3,
			TargetDurationSec: 60,
			SilenceGapMinMs:   1000,
			SilenceGapMaxMs:   3000,
		},
		LLM: domain.LLMConfig{
			Enabled: true,
			BaseURL: "http://localhost:11434/v1",
			APIKey:  "",
			Model:   "gpt-4o-mini",
		},
	}
}

func (s *Store) LoadConfig() (domain.AppConfig, error) {
	p := filepath.Join(s.baseDir, configFileName)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := DefaultConfig()
			// write defaults so the dir is materialized and future loads are stable
			_ = s.SaveConfig(cfg)
			return cfg, nil
		}
		return domain.AppConfig{}, err
	}
	var cfg domain.AppConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return domain.AppConfig{}, err
	}
	cfg = applyConfigDefaults(cfg)
	return cfg, nil
}

func (s *Store) SaveConfig(cfg domain.AppConfig) error {
	p := filepath.Join(s.baseDir, configFileName)
	cfg = applyConfigDefaults(cfg)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(p, b, 0o600)
}

func (s *Store) LoadHistory() (domain.History, error) {
	p := filepath.Join(s.baseDir, historyFileName)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			h := domain.History{UsedArticleUrls: []string{}, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
			_ = s.SaveHistory(h)
			return h, nil
		}
		return domain.History{}, err
	}
	var h domain.History
	if err := json.Unmarshal(b, &h); err != nil {
		return domain.History{}, err
	}
	if h.UsedArticleUrls == nil {
		h.UsedArticleUrls = []string{}
	}
	return h, nil
}

func (s *Store) SaveHistory(h domain.History) error {
	p := filepath.Join(s.baseDir, historyFileName)
	if h.UsedArticleUrls == nil {
		h.UsedArticleUrls = []string{}
	}
	if len(h.UsedArticleUrls) > historyMaxItems {
		h.UsedArticleUrls = h.UsedArticleUrls[len(h.UsedArticleUrls)-historyMaxItems:]
	}
	h.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(p, b, 0o600)
}

func applyConfigDefaults(cfg domain.AppConfig) domain.AppConfig {
	def := DefaultConfig()
	if cfg.RSSUrls == nil {
		cfg.RSSUrls = []string{}
	}
	if cfg.Talk.CycleBgmCount == 0 {
		cfg.Talk.CycleBgmCount = def.Talk.CycleBgmCount
	}
	if cfg.Talk.TargetDurationSec == 0 {
		cfg.Talk.TargetDurationSec = def.Talk.TargetDurationSec
	}
	if cfg.Talk.SilenceGapMinMs == 0 {
		cfg.Talk.SilenceGapMinMs = def.Talk.SilenceGapMinMs
	}
	if cfg.Talk.SilenceGapMaxMs == 0 {
		cfg.Talk.SilenceGapMaxMs = def.Talk.SilenceGapMaxMs
	}
	// Clamp gaps to safe values
	if cfg.Talk.SilenceGapMinMs < 0 {
		cfg.Talk.SilenceGapMinMs = def.Talk.SilenceGapMinMs
	}
	if cfg.Talk.SilenceGapMaxMs < cfg.Talk.SilenceGapMinMs {
		cfg.Talk.SilenceGapMaxMs = cfg.Talk.SilenceGapMinMs
	}
	return cfg
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
