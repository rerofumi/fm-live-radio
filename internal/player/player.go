package player

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"fm-live-radio/internal/audio"
	"fm-live-radio/internal/bgm"
	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/talk"

	"github.com/google/uuid"
)

var ErrNotConfigured = errors.New("not configured")

type Player struct {
	mu sync.Mutex

	cfg domain.AppConfig

	bgmCountSinceLastTalk int
	lastTrackPath         string
	pendingSilence        bool
}

func New(cfg domain.AppConfig) *Player {
	return &Player{cfg: cfg, pendingSilence: true}
}

func (p *Player) UpdateConfig(cfg domain.AppConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg
	// reset cycle when config meaningfully changes
	p.bgmCountSinceLastTalk = 0
	p.lastTrackPath = ""
	p.pendingSilence = true
}

func (p *Player) NextItem(audioSrv *audio.Server, talkSvc *talk.Service, req domain.NextItemRequest, hist domain.History) (domain.PlayableItem, domain.History, bool, error) {
	p.mu.Lock()
	cfg := p.cfg
	// We mutate these; keep them protected.
	genre := req.SelectedGenre
	if genre == "" {
		genre = cfg.SelectedGenre
	}
	if cfg.BGMRootPath == "" || genre == "" {
		p.mu.Unlock()
		return domain.PlayableItem{}, hist, false, ErrNotConfigured
	}

	// Insert a "radio-like" gap between items.
	if p.pendingSilence && cfg.Talk.SilenceGapMinMs > 0 {
		gap := cfg.Talk.SilenceGapMinMs
		if cfg.Talk.SilenceGapMaxMs > cfg.Talk.SilenceGapMinMs {
			gap += int(time.Now().UnixNano() % int64(cfg.Talk.SilenceGapMaxMs-cfg.Talk.SilenceGapMinMs+1))
		}
		p.pendingSilence = false
		p.mu.Unlock()
		return domain.PlayableItem{
			ID:             uuid.NewString(),
			Kind:           domain.PlayableKindSilence,
			Title:          "(間)",
			DurationHintMs: gap,
		}, hist, false, nil
	}

	cycle := cfg.Talk.CycleBgmCount
	if cycle <= 0 {
		cycle = 3
	}

	// Decide next kind.
	wantTalk := cfg.Talk.Enabled && (p.bgmCountSinceLastTalk >= cycle)
	p.mu.Unlock()

	if wantTalk && talkSvc != nil {
		// TTS can be slow; keep this longer than the HTTP client's timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		res, newHist, err := talkSvc.Generate(ctx, cfg, hist)
		if err == nil {
			url, err2 := audioSrv.RegisterFile(res.AudioPath, 10*time.Minute)
			if err2 == nil {
				p.mu.Lock()
				p.bgmCountSinceLastTalk = 0
				p.pendingSilence = true
				p.mu.Unlock()
				return domain.PlayableItem{
					ID:         uuid.NewString(),
					Kind:       domain.PlayableKindTalk,
					URL:        url,
					Title:      res.ArticleTitle,
					TopicTitle: res.ArticleTitle,
					Source: domain.PlayableSource{
						RssURL:     res.FeedURL,
						ArticleURL: res.ArticleURL,
					},
				}, newHist, true, nil
			}
		}
		log.Printf("WARN: talk generation failed, fallback to BGM: %v", err)
		// Treat failed talk slot as consumed.
		p.mu.Lock()
		p.bgmCountSinceLastTalk = 0
		p.pendingSilence = true
		p.mu.Unlock()
		return p.pickBGM(audioSrv, cfg, genre, hist)
	}

	return p.pickBGM(audioSrv, cfg, genre, hist)
}

func (p *Player) pickBGM(audioSrv *audio.Server, cfg domain.AppConfig, genre string, hist domain.History) (domain.PlayableItem, domain.History, bool, error) {
	tracks, err := bgm.ListTracks(cfg.BGMRootPath, genre)
	if err != nil {
		return domain.PlayableItem{}, hist, false, err
	}
	t, err := bgm.PickRandomTrack(tracks, p.lastTrackPath)
	if err != nil {
		return domain.PlayableItem{}, hist, false, err
	}

	p.mu.Lock()
	p.lastTrackPath = t.Path
	p.bgmCountSinceLastTalk++
	p.pendingSilence = true
	p.mu.Unlock()

	url, err := audioSrv.RegisterFile(t.Path, 10*time.Minute)
	if err != nil {
		return domain.PlayableItem{}, hist, false, err
	}

	return domain.PlayableItem{
		ID:    uuid.NewString(),
		Kind:  domain.PlayableKindBGM,
		URL:   url,
		Title: t.Title,
		Source: domain.PlayableSource{
			Genre:    genre,
			FilePath: t.Path,
		},
	}, hist, false, nil
}

func (p *Player) Skip(audioSrv *audio.Server, talkSvc *talk.Service, req domain.NextItemRequest, hist domain.History) (domain.PlayableItem, domain.History, bool, error) {
	// Skipping consumes current slot; we just ask for the next.
	return p.NextItem(audioSrv, talkSvc, req, hist)
}

func (p *Player) PrefetchTalk() {
	// noop for now
}
