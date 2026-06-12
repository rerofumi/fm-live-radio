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
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/musicgen"
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

	prefetchedTalk *talk.Result
	prefetching    bool
	cancelPrefetch context.CancelFunc

	prefetchedMusic      *musicgen.Result
	musicPrefetching     bool
	cancelMusicPrefetch  context.CancelFunc
	localGenerationError string
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
	p.clearPrefetchLocked()
}

func (p *Player) NextItem(audioSrv *audio.Server, talkSvc *talk.Service, musicSvc *musicgen.Service, req domain.NextItemRequest, hist domain.History) (domain.PlayableItem, domain.History, bool, error) {
	p.mu.Lock()
	cfg := p.cfg
	// We mutate these; keep them protected.
	genre := req.SelectedGenre
	if genre == "" {
		genre = cfg.SelectedGenre
	}
	if cfg.BGMSource == domain.BGMSourceFiles && (cfg.BGMRootPath == "" || genre == "") {
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
	prefetched := p.prefetchedTalk
	if wantTalk && prefetched != nil {
		// Consume prefetched talk.
		p.prefetchedTalk = nil
		p.bgmCountSinceLastTalk = 0
		p.pendingSilence = true
		p.localGenerationError = generationWarning()
		p.mu.Unlock()

		url, err := audioSrv.RegisterFile(prefetched.AudioPath, 10*time.Minute)
		if err != nil {
			return domain.PlayableItem{}, hist, false, err
		}
		newHist := appendHistory(hist, prefetched.ArticleURL)
		return domain.PlayableItem{
			ID:         uuid.NewString(),
			Kind:       domain.PlayableKindTalk,
			URL:        url,
			Title:      prefetched.ArticleTitle,
			TopicTitle: prefetched.ArticleTitle,
			Source: domain.PlayableSource{
				RssURL:     prefetched.FeedURL,
				ArticleURL: prefetched.ArticleURL,
			},
		}, newHist, true, nil
	}
	p.mu.Unlock()

	if wantTalk && talkSvc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		used := buildUsedMap(hist, nil)
		res, err := talkSvc.Generate(ctx, cfg, used)
		if err == nil {
			url, err2 := audioSrv.RegisterFile(res.AudioPath, 10*time.Minute)
			if err2 == nil {
				p.mu.Lock()
				p.bgmCountSinceLastTalk = 0
				p.pendingSilence = true
				p.localGenerationError = generationWarning()
				p.mu.Unlock()
				newHist := appendHistory(hist, res.ArticleURL)
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
		return p.pickBGM(audioSrv, talkSvc, musicSvc, cfg, genre, hist)
	}

	return p.pickBGM(audioSrv, talkSvc, musicSvc, cfg, genre, hist)
}

func (p *Player) pickBGM(audioSrv *audio.Server, talkSvc *talk.Service, musicSvc *musicgen.Service, cfg domain.AppConfig, genre string, hist domain.History) (domain.PlayableItem, domain.History, bool, error) {
	if cfg.BGMSource == domain.BGMSourceStableAudio3 && musicSvc != nil {
		return p.pickGeneratedBGM(audioSrv, talkSvc, musicSvc, cfg, genre, hist)
	}
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
	count := p.bgmCountSinceLastTalk
	p.mu.Unlock()

	// Opportunistic prefetch when we are close to talk slot.
	cycle := cfg.Talk.CycleBgmCount
	if cycle <= 0 {
		cycle = 3
	}
	if talkSvc != nil && cfg.Talk.Enabled && count >= cycle-1 {
		p.PrefetchTalk(talkSvc, cfg, hist)
	}

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

func (p *Player) pickGeneratedBGM(audioSrv *audio.Server, talkSvc *talk.Service, musicSvc *musicgen.Service, cfg domain.AppConfig, genre string, hist domain.History) (domain.PlayableItem, domain.History, bool, error) {
	p.mu.Lock()
	prefetched := p.prefetchedMusic
	if prefetched != nil {
		p.prefetchedMusic = nil
	}
	p.mu.Unlock()

	var res musicgen.Result
	var err error
	if prefetched != nil {
		res = *prefetched
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		res, err = musicSvc.Generate(ctx, cfg, genre)
		if err != nil {
			if fallback, fbErr := musicSvc.Fallback(cfg); fbErr == nil {
				res = fallback
			} else {
				p.setLocalGenerationError(err)
				return domain.PlayableItem{}, hist, false, err
			}
		}
	}

	p.mu.Lock()
	p.bgmCountSinceLastTalk++
	p.pendingSilence = true
	p.localGenerationError = generationWarning()
	count := p.bgmCountSinceLastTalk
	p.mu.Unlock()

	cycle := cfg.Talk.CycleBgmCount
	if cycle <= 0 {
		cycle = 3
	}
	if talkSvc != nil && cfg.Talk.Enabled && count >= cycle-1 {
		p.PrefetchTalk(talkSvc, cfg, hist)
	}
	p.PrefetchMusic(musicSvc, cfg, genre)

	url, err := audioSrv.RegisterFile(res.AudioPath, 10*time.Minute)
	if err != nil {
		return domain.PlayableItem{}, hist, false, err
	}
	return domain.PlayableItem{
		ID:    uuid.NewString(),
		Kind:  domain.PlayableKindBGM,
		URL:   url,
		Title: res.Title,
		Source: domain.PlayableSource{
			Genre:    genre,
			FilePath: res.AudioPath,
			Provider: string(domain.BGMSourceStableAudio3),
			Prompt:   res.Prompt,
			Seed:     res.Seed,
			ModelDir: cfg.StableAudio3.ModelDir,
		},
	}, hist, false, nil
}

func (p *Player) Skip(audioSrv *audio.Server, talkSvc *talk.Service, musicSvc *musicgen.Service, req domain.SkipRequest, hist domain.History) (domain.PlayableItem, domain.History, bool, error) {
	// Skip semantics (docs/01_specification.md 5.5):
	// - If skipping BGM: advance bgmCountSinceLastTalk by 1.
	// - If talk is ready: keep it (do not discard), and do not generate a new one while ready.
	// - If talk is currently generating: cancel it.
	// - If skipping silence: consume the gap (do not keep returning silence).
	// - If skipping talk: treat as consumed (reset counter).
	p.mu.Lock()

	// Cancel in-flight generation if any.
	if p.cancelPrefetch != nil {
		p.cancelPrefetch()
		p.cancelPrefetch = nil
		p.prefetching = false
	}
	if p.cancelMusicPrefetch != nil {
		p.cancelMusicPrefetch()
		p.cancelMusicPrefetch = nil
		p.musicPrefetching = false
	}

	switch req.CurrentKind {
	case domain.PlayableKindBGM:
		p.bgmCountSinceLastTalk++
		p.pendingSilence = true
	case domain.PlayableKindTalk:
		// Skipping talk consumes the talk slot.
		p.bgmCountSinceLastTalk = 0
		p.pendingSilence = true
		// We only prefetch one talk; dropping any ready talk avoids "talk again" immediately.
		p.prefetchedTalk = nil
	case domain.PlayableKindSilence:
		// Consume the silence gap immediately.
		p.pendingSilence = false
	default:
		// Unknown kind: be conservative and just move on.
		p.pendingSilence = true
	}

	// NOTE: If talk is already ready, we intentionally keep p.prefetchedTalk.
	p.mu.Unlock()

	return p.NextItem(audioSrv, talkSvc, musicSvc, domain.NextItemRequest{SelectedGenre: req.SelectedGenre}, hist)
}

// PrefetchTalk starts generating next talk in the background if we are close to the talk slot.
// It does not mutate history; history is committed when the prefetched talk is actually consumed.
func (p *Player) PrefetchTalk(talkSvc *talk.Service, cfg domain.AppConfig, hist domain.History) {
	if talkSvc == nil {
		return
	}
	cycle := cfg.Talk.CycleBgmCount
	if cycle <= 0 {
		cycle = 3
	}

	p.mu.Lock()
	if !cfg.Talk.Enabled || p.prefetchedTalk != nil || p.prefetching {
		p.mu.Unlock()
		return
	}
	// Start prefetch when next is near: after (cycle-1) BGM played since last talk.
	if p.bgmCountSinceLastTalk < cycle-1 {
		p.mu.Unlock()
		return
	}
	p.prefetching = true
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	p.cancelPrefetch = cancel
	p.mu.Unlock()

	go func() {
		defer func() {
			p.mu.Lock()
			p.prefetching = false
			p.cancelPrefetch = nil
			p.mu.Unlock()
		}()

		used := buildUsedMap(hist, nil)
		res, err := talkSvc.Generate(ctx, cfg, used)
		if err != nil {
			log.Printf("WARN: talk prefetch failed: %v", err)
			return
		}
		p.mu.Lock()
		p.prefetchedTalk = &res
		p.localGenerationError = generationWarning()
		p.mu.Unlock()
	}()
}

func (p *Player) clearPrefetchLocked() {
	if p.cancelPrefetch != nil {
		p.cancelPrefetch()
		p.cancelPrefetch = nil
	}
	p.prefetchedTalk = nil
	p.prefetching = false
	if p.cancelMusicPrefetch != nil {
		p.cancelMusicPrefetch()
		p.cancelMusicPrefetch = nil
	}
	p.prefetchedMusic = nil
	p.musicPrefetching = false
	p.localGenerationError = ""
}

func buildUsedMap(hist domain.History, extra map[string]bool) map[string]bool {
	used := map[string]bool{}
	for _, u := range hist.UsedArticleUrls {
		used[u] = true
	}
	for k, v := range extra {
		if v {
			used[k] = true
		}
	}
	return used
}

func (p *Player) Status() domain.AppStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return domain.AppStatus{
		TalkPrefetching:      p.prefetching,
		TalkReady:            p.prefetchedTalk != nil,
		MusicGenerating:      p.musicPrefetching,
		MusicReady:           p.prefetchedMusic != nil,
		LocalGenerationError: p.localGenerationError,
	}
}

func (p *Player) PrefetchMusic(musicSvc *musicgen.Service, cfg domain.AppConfig, genre string) {
	if musicSvc == nil || cfg.BGMSource != domain.BGMSourceStableAudio3 {
		return
	}
	p.mu.Lock()
	if p.prefetchedMusic != nil || p.musicPrefetching {
		p.mu.Unlock()
		return
	}
	p.musicPrefetching = true
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	p.cancelMusicPrefetch = cancel
	p.mu.Unlock()

	go func() {
		defer func() {
			p.mu.Lock()
			p.musicPrefetching = false
			p.cancelMusicPrefetch = nil
			p.mu.Unlock()
		}()
		res, err := musicSvc.Generate(ctx, cfg, genre)
		if err != nil {
			p.setLocalGenerationError(err)
			return
		}
		p.mu.Lock()
		p.prefetchedMusic = &res
		p.localGenerationError = generationWarning()
		p.mu.Unlock()
	}()
}

func (p *Player) setLocalGenerationError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err == nil {
		p.localGenerationError = ""
		return
	}
	p.localGenerationError = err.Error()
}

func generationWarning() string {
	return generation.LastWarning()
}

func appendHistory(hist domain.History, url string) domain.History {
	if url == "" {
		return hist
	}
	newHist := hist
	newHist.UsedArticleUrls = append(newHist.UsedArticleUrls, url)
	if len(newHist.UsedArticleUrls) > 500 {
		newHist.UsedArticleUrls = newHist.UsedArticleUrls[len(newHist.UsedArticleUrls)-500:]
	}
	return newHist
}
