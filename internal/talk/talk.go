package talk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/llm"
	"fm-live-radio/internal/localtts"
	"fm-live-radio/internal/localtts/irodori/pipeline"
	"fm-live-radio/internal/rss"
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
	local  *localtts.Service

	tempDir    string
	observerMu sync.RWMutex
	observer   pipeline.EventObserver
}

func New(tempDir string) *Service {
	return &Service{
		picker:  rss.NewPicker(),
		llm:     &llm.OpenAICompat{},
		local:   localtts.New(),
		tempDir: tempDir,
	}
}

// SetObserver installs a diagnostic lifecycle observer. It lets verification
// distinguish a real inference call from a prefetch reservation.
func (s *Service) SetObserver(observer pipeline.EventObserver) {
	s.observerMu.Lock()
	s.observer = observer
	s.observerMu.Unlock()
}

func (s *Service) observerSnapshot() pipeline.EventObserver {
	s.observerMu.RLock()
	defer s.observerMu.RUnlock()
	return s.observer
}

func (s *Service) Generate(ctx context.Context, cfg domain.AppConfig, used map[string]bool) (Result, error) {
	// Player may register a Talk reservation before this service starts its
	// RSS/LLM work. Ensure early failures release that still-unconsumed entry;
	// localtts consumes it exactly once when runtime work begins.
	reservation := generation.ReservationFromContext(ctx, generation.KindTalk)
	if reservation != nil {
		defer reservation.Release()
	}
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

	wav, err := s.local.SynthesizeWavWithObserver(ctx, cfg, script, s.observerSnapshot())
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	audioPath, err := s.writeTempAudio(wav, ".wav")
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(audioPath)
		return Result{}, err
	}

	return Result{
		AudioPath:    audioPath,
		ArticleURL:   art.Link,
		ArticleTitle: art.Title,
		FeedURL:      art.FeedURL,
	}, nil
}

func buildUserPrompt(a rss.Article) string {
	b := strings.Builder{}
	b.WriteString("以下の記事を要約してラジオトーク原稿を作ってください。文字数は200〜300。固有名詞は必要最小限。設定などは語らず、トーク原稿のみを出力してください。\n")
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
