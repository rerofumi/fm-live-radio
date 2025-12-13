package talk

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/llm"
	"fm-live-radio/internal/rss"
	"fm-live-radio/internal/tts"
)

var ErrDisabled = errors.New("talk disabled")
var ErrNotReady = errors.New("talk not ready")

type Result struct {
	AudioPath    string
	ArticleURL   string
	ArticleTitle string
	FeedURL      string
}

type Service struct {
	picker *rss.Picker
	llm    *llm.OpenAICompat
	tts    *tts.GeminiClient

	tempDir string
}

func New(tempDir string) *Service {
	return &Service{
		picker:  rss.NewPicker(),
		llm:     &llm.OpenAICompat{},
		tts:     &tts.GeminiClient{},
		tempDir: tempDir,
	}
}

func (s *Service) Generate(ctx context.Context, cfg domain.AppConfig, hist domain.History) (Result, domain.History, error) {
	if !cfg.Talk.Enabled {
		return Result{}, hist, ErrDisabled
	}
	if len(cfg.RSSUrls) == 0 {
		return Result{}, hist, ErrNotReady
	}

	used := map[string]bool{}
	for _, u := range hist.UsedArticleUrls {
		used[u] = true
	}

	art, err := s.picker.Pick(ctx, cfg.RSSUrls, used)
	if err != nil {
		return Result{}, hist, err
	}

	s.llm.BaseURL = cfg.LLM.BaseURL
	s.llm.APIKey = cfg.LLM.APIKey
	s.llm.Model = cfg.LLM.Model

	systemPrompt := "あなたは落ち着いたラジオDJです。ニュースを分かりやすく1分で紹介します。口語で、導入→要点→締めの構成にしてください。誇張しすぎないでください。出典URLは読み上げないでください。個人情報を生成しないでください。"
	userPrompt := buildUserPrompt(art)

	script, err := s.llm.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return Result{}, hist, err
	}

	// DEBUG: log what we are sending to TTS (bounded to avoid huge logs).
	logged := strings.TrimSpace(script)
	const maxLogRunes = 1200
	r := []rune(logged)
	if len(r) > maxLogRunes {
		logged = string(r[:maxLogRunes]) + " ...[truncated]"
	}
	log.Printf("INFO: talk script (article=%q url=%s)\n%s", art.Title, art.Link, logged)

	s.tts.APIKey = cfg.GeminiAPIKey
	// model/voice are fixed for now; make configurable later if needed.
	s.tts.Model = "gemini-2.5-flash-preview-tts"
	s.tts.Voice = "Kore"

	wav, err := s.tts.SynthesizeWav(ctx, script)
	if err != nil {
		return Result{}, hist, err
	}

	audioPath, err := s.writeTempAudio(wav, ".wav")
	if err != nil {
		return Result{}, hist, err
	}

	// update history
	newHist := hist
	newHist.UsedArticleUrls = append(newHist.UsedArticleUrls, art.Link)
	if len(newHist.UsedArticleUrls) > 500 {
		newHist.UsedArticleUrls = newHist.UsedArticleUrls[len(newHist.UsedArticleUrls)-500:]
	}

	return Result{
		AudioPath:    audioPath,
		ArticleURL:   art.Link,
		ArticleTitle: art.Title,
		FeedURL:      art.FeedURL,
	}, newHist, nil
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
