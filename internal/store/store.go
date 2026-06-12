package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func DefaultAssetBaseDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func DefaultConfig() domain.AppConfig {
	base := DefaultAssetBaseDir()
	return domain.AppConfig{
		BGMRootPath:   "",
		SelectedGenre: "",
		RSSUrls:       []string{},
		GeminiAPIKey:  "",
		BGMSource:     domain.BGMSourceFiles,
		TTSSource:     domain.TTSSourceGemini,
		BGMVolume:     0.8,
		TalkVolume:    1.0,
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
		TTS: domain.TTSConfig{
			Enabled: true,
			Model:   "gemini-2.5-flash-preview-tts",
			Voice:   "Kore",
		},
		LocalInference: domain.LocalInferenceConfig{
			MaxWorkers:        1,
			ExecutionProvider: "auto",
			DeviceID:          0,
		},
		StableAudio3: domain.StableAudio3Config{
			Enabled:    true,
			ModelDir:   filepath.Join(base, "model", "sa3-sm-music"),
			OutputDir:  filepath.Join(base, "generate_music"),
			PromptBase: "instrumental background music for a radio show, seamless loop feel, no vocals",
			Seconds:    30,
			Steps:      8,
			SeedMode:   "random",
			CacheLimit: 20,
		},
		Irodori: domain.IrodoriConfig{
			Enabled:       true,
			ModelDir:      filepath.Join(base, "model", "irodori-v3"),
			NarratorDir:   filepath.Join(base, "narrator"),
			Seconds:       -1,
			NumSteps:      40,
			SeedMode:      "random",
			CfgText:       3,
			CfgCaption:    3,
			CfgSpeaker:    5,
			DurationScale: 1,
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
	if cfg.BGMSource == "" {
		cfg.BGMSource = def.BGMSource
	}
	if cfg.TTSSource == "" {
		cfg.TTSSource = def.TTSSource
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
	// default volumes
	if cfg.BGMVolume == 0 {
		cfg.BGMVolume = def.BGMVolume
	}
	if cfg.TalkVolume == 0 {
		cfg.TalkVolume = def.TalkVolume
	}

	// default TTS
	if cfg.TTS.Enabled == false {
		// If field is missing in older configs, Enabled will be false; default to enabled.
		cfg.TTS.Enabled = def.TTS.Enabled
	}
	if strings.TrimSpace(cfg.TTS.Model) == "" {
		cfg.TTS.Model = def.TTS.Model
	}
	if strings.TrimSpace(cfg.TTS.Voice) == "" {
		cfg.TTS.Voice = def.TTS.Voice
	}
	if cfg.LocalInference.MaxWorkers <= 0 {
		cfg.LocalInference.MaxWorkers = def.LocalInference.MaxWorkers
	}
	switch strings.ToLower(strings.TrimSpace(cfg.LocalInference.ExecutionProvider)) {
	case "":
		cfg.LocalInference.ExecutionProvider = def.LocalInference.ExecutionProvider
	case "cpu", "cuda", "auto":
		cfg.LocalInference.ExecutionProvider = strings.ToLower(strings.TrimSpace(cfg.LocalInference.ExecutionProvider))
	default:
		cfg.LocalInference.ExecutionProvider = "cpu"
	}
	if cfg.LocalInference.DeviceID < 0 {
		cfg.LocalInference.DeviceID = 0
	}
	if strings.TrimSpace(cfg.StableAudio3.ModelDir) == "" {
		cfg.StableAudio3.ModelDir = def.StableAudio3.ModelDir
	}
	if strings.TrimSpace(cfg.StableAudio3.OutputDir) == "" {
		cfg.StableAudio3.OutputDir = def.StableAudio3.OutputDir
	}
	if strings.TrimSpace(cfg.StableAudio3.PromptBase) == "" {
		cfg.StableAudio3.PromptBase = def.StableAudio3.PromptBase
	}
	if cfg.StableAudio3.Seconds <= 0 {
		cfg.StableAudio3.Seconds = def.StableAudio3.Seconds
	}
	if cfg.StableAudio3.Steps <= 0 {
		cfg.StableAudio3.Steps = def.StableAudio3.Steps
	}
	if strings.TrimSpace(cfg.StableAudio3.SeedMode) == "" {
		cfg.StableAudio3.SeedMode = def.StableAudio3.SeedMode
	}
	if cfg.StableAudio3.CacheLimit <= 0 {
		cfg.StableAudio3.CacheLimit = def.StableAudio3.CacheLimit
	}
	if strings.TrimSpace(cfg.Irodori.ModelDir) == "" {
		cfg.Irodori.ModelDir = def.Irodori.ModelDir
	}
	if strings.TrimSpace(cfg.Irodori.NarratorDir) == "" {
		cfg.Irodori.NarratorDir = def.Irodori.NarratorDir
	}
	if cfg.Irodori.Seconds == 0 {
		cfg.Irodori.Seconds = def.Irodori.Seconds
	}
	if cfg.Irodori.NumSteps <= 0 {
		cfg.Irodori.NumSteps = def.Irodori.NumSteps
	}
	if strings.TrimSpace(cfg.Irodori.SeedMode) == "" {
		cfg.Irodori.SeedMode = def.Irodori.SeedMode
	}
	if cfg.Irodori.CfgText <= 0 {
		cfg.Irodori.CfgText = def.Irodori.CfgText
	}
	if cfg.Irodori.CfgCaption <= 0 {
		cfg.Irodori.CfgCaption = def.Irodori.CfgCaption
	}
	if cfg.Irodori.CfgSpeaker <= 0 {
		cfg.Irodori.CfgSpeaker = def.Irodori.CfgSpeaker
	}
	if cfg.Irodori.DurationScale <= 0 {
		cfg.Irodori.DurationScale = def.Irodori.DurationScale
	}
	// Clamp volumes to [0..1]
	if cfg.BGMVolume < 0 {
		cfg.BGMVolume = 0
	}
	if cfg.BGMVolume > 1 {
		cfg.BGMVolume = 1
	}
	if cfg.TalkVolume < 0 {
		cfg.TalkVolume = 0
	}
	if cfg.TalkVolume > 1 {
		cfg.TalkVolume = 1
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
