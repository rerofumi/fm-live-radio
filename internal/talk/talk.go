package talk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/llm"
	"fm-live-radio/internal/localtts"
	"fm-live-radio/internal/rss"
	"fm-live-radio/internal/tts"
)

var ErrDisabled = errors.New("talk disabled")
var ErrNotReady = errors.New("talk not ready")
var ErrEmptyScript = errors.New("talk script is empty")

type Result struct {
	AudioPath    string
	ArticleURL   string
	ArticleTitle string
	FeedURL      string
}

type Service struct {
	picker *rss.Picker
	llm    *llm.OpenAICompat
	gemini *tts.GeminiClient
	local  *localtts.Service

	tempDir string
}

func New(tempDir string) *Service {
	return &Service{
		picker:  rss.NewPicker(),
		llm:     &llm.OpenAICompat{},
		gemini:  &tts.GeminiClient{},
		local:   localtts.New(),
		tempDir: tempDir,
	}
}

func (s *Service) Generate(ctx context.Context, cfg domain.AppConfig, used map[string]bool) (Result, error) {
	if !cfg.Talk.Enabled {
		return Result{}, ErrDisabled
	}
	if len(cfg.RSSUrls) == 0 {
		return Result{}, ErrNotReady
	}

	art, err := s.picker.Pick(ctx, cfg.RSSUrls, used)
	if err != nil {
		return Result{}, err
	}

	s.llm.BaseURL = cfg.LLM.BaseURL
	s.llm.APIKey = cfg.LLM.APIKey
	s.llm.Model = cfg.LLM.Model

	systemPrompt := "あなたは落ち着いたラジオDJです。ニュースを分かりやすく1分で紹介します。口語で、導入→要点→締めの構成にしてください。誇張しすぎないでください。出典URLは読み上げないでください。個人情報を生成しないでください。"
	userPrompt := buildUserPrompt(art)

	script, err := s.llm.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(script) == "" {
		return Result{}, ErrEmptyScript
	}

	provider := s.providerForConfig(cfg)
	wav, err := provider.SynthesizeWav(ctx, script)
	if err != nil {
		return Result{}, err
	}

	audioPath, err := s.writeTempAudio(wav, ".wav")
	if err != nil {
		return Result{}, err
	}

	return Result{
		AudioPath:    audioPath,
		ArticleURL:   art.Link,
		ArticleTitle: art.Title,
		FeedURL:      art.FeedURL,
	}, nil
}

func (s *Service) providerForConfig(cfg domain.AppConfig) tts.Provider {
	if cfg.TTSSource == domain.TTSSourceIrodori {
		return irodoriProvider{svc: s.local, cfg: cfg}
	}
	s.gemini.APIKey = cfg.GeminiAPIKey
	if cfg.TTS.Enabled {
		s.gemini.Model = cfg.TTS.Model
		s.gemini.Voice = cfg.TTS.Voice
	}
	return s.gemini
}

type irodoriProvider struct {
	svc *localtts.Service
	cfg domain.AppConfig
}

func (p irodoriProvider) SynthesizeWav(ctx context.Context, text string) ([]byte, error) {
	return p.svc.SynthesizeWav(ctx, p.cfg, text)
}

func buildUserPrompt(a rss.Article) string {
	b := strings.Builder{}
	b.WriteString("以下の記事を要約してラジオトーク原稿を作ってください。文字数は200〜300。固有名詞は必要最小限。\n")
	b.WriteString("記事タイトル: ")
	b.WriteString(a.Title)
	b.WriteString("\n")
	if a.FeedTitle != "" {
		b.WriteString("フィード: ")
		b.WriteString(a.FeedTitle)
		b.WriteString("\n")
	}
	if a.Content != "" {
		b.WriteString("本文: \n")
		// keep prompt bounded
		c := a.Content
		if len([]rune(c)) > 2000 {
			r := []rune(c)
			c = string(r[:2000])
		}
		b.WriteString(c)
		b.WriteString("\n")
	}
	return b.String()
}

func (s *Service) writeTempAudio(data []byte, ext string) (string, error) {
	if strings.TrimSpace(ext) == "" {
		ext = ".mp3"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	_ = os.MkdirAll(s.tempDir, 0o755)
	name := fmt.Sprintf("talk_%s%s", time.Now().UTC().Format("20060102_150405"), ext)
	p := filepath.Join(s.tempDir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return "", err
	}
	return p, nil
}
