package player

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"fm-live-radio/internal/audio"
	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/musicgen"
	"fm-live-radio/internal/talk"

	"github.com/google/uuid"
)

var ErrNotConfigured = errors.New("not configured")

type Player struct {
	mu               sync.Mutex
	workWG           sync.WaitGroup
	prefetchWG       sync.WaitGroup
	ownerCtx         context.Context
	ownerCancel      context.CancelFunc
	shutdownAccepted chan struct{}
	closed           bool
	// generation is advanced whenever work already in flight must no longer
	// publish a result (Skip, config replacement, or Shutdown). Workers carry
	// the value they started with and publish only while it still matches.
	generation uint64

	cfg domain.AppConfig

	bgmCountSinceLastTalk int
	lastTrackPath         string
	pendingSilence        bool

	prefetchedTalk    *talk.Result
	prefetching       bool
	talkInFlight      int
	cancelPrefetch    context.CancelFunc
	talkPrefetchOwner uint64

	prefetchedMusic      *musicgen.Result
	musicPrefetching     bool
	musicInFlight        int
	cancelMusicPrefetch  context.CancelFunc
	musicPrefetchOwner   uint64
	nextPrefetchOwner    uint64
	localGenerationError string
}

func New(cfg domain.AppConfig) *Player {
	ctx, cancel := context.WithCancel(context.Background())
	return &Player{cfg: cfg, pendingSilence: true, ownerCtx: ctx, ownerCancel: cancel, shutdownAccepted: make(chan struct{})}
}

// Shutdown stops all player-owned work and waits for every Talk/BGM request
// and prefetch goroutine to leave ORT before the application destroys it.
func (p *Player) Shutdown() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		p.workWG.Wait()
		p.prefetchWG.Wait()
		return
	}
	p.closed = true
	p.generation++
	p.clearPrefetchLocked()
	cancel := p.ownerCancel
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	close(p.shutdownAccepted)
	p.prefetchWG.Wait()
	p.workWG.Wait()
}

// ShutdownAccepted returns a channel closed once Shutdown has invalidated the
// player generation and cancelled its owner context. It is used by lifecycle
// observers to distinguish an accepted shutdown request from a later join.
func (p *Player) ShutdownAccepted() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.shutdownAccepted
}

func (p *Player) beginWork() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	p.workWG.Add(1)
	return true
}

func (p *Player) generationContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	p.mu.Lock()
	owner := p.ownerCtx
	p.mu.Unlock()
	if owner == nil {
		owner = context.Background()
	}
	return context.WithTimeout(owner, timeout)
}

func (p *Player) UpdateConfig(cfg domain.AppConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.generation++
	p.cfg = cfg
	// reset cycle when config meaningfully changes
	p.bgmCountSinceLastTalk = 0
	p.lastTrackPath = ""
	p.pendingSilence = true
	p.clearPrefetchLocked()
}

// UpdateStableAudio3Genre changes only the Stable Audio 3 genre on the
// runtime config. Per FR-10, the currently playing item and any music that
// is already prefetched or generating must not be interrupted; the new genre
// is applied to subsequent BGM generations.
func (p *Player) UpdateStableAudio3Genre(genre string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg.StableAudio3.Genre = genre
}

func (p *Player) NextItem(audioSrv *audio.Server, talkSvc *talk.Service, musicSvc *musicgen.Service, req domain.NextItemRequest, hist domain.History) (domain.PlayableItem, domain.History, bool, error) {
	if !p.beginWork() {
		return domain.PlayableItem{}, hist, false, ErrNotConfigured
	}
	defer p.workWG.Done()
	p.mu.Lock()
	cfg := p.cfg
	workGeneration := p.generation

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
			ID:          uuid.NewString(),
			Kind:        domain.PlayableKindTalk,
			URL:         url,
			LoudnessURL: audioSrv.LoudnessURLForAudioURL(url),
			Title:       prefetched.ArticleTitle,
			TopicTitle:  prefetched.ArticleTitle,
			Source: domain.PlayableSource{
				RssURL:     prefetched.FeedURL,
				ArticleURL: prefetched.ArticleURL,
			},
		}, newHist, true, nil
	}
	p.mu.Unlock()

	if wantTalk && talkSvc != nil {
		ctx, cancel := p.generationContext(180 * time.Second)
		defer cancel()

		used := buildUsedMap(hist, nil)
		res, err := talkSvc.Generate(ctx, cfg, used)
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		if err == nil {
			url, err2 := audioSrv.RegisterFile(res.AudioPath, 10*time.Minute)
			if err2 == nil {
				p.mu.Lock()
				if !p.publishAllowedLocked(workGeneration, ctx) {
					p.mu.Unlock()
					return domain.PlayableItem{}, hist, false, generationInvalidationError(ctx)
				}
				p.bgmCountSinceLastTalk = 0
				p.pendingSilence = true
				p.localGenerationError = generationWarning()
				p.mu.Unlock()
				newHist := appendHistory(hist, res.ArticleURL)
				return domain.PlayableItem{
					ID:          uuid.NewString(),
					Kind:        domain.PlayableKindTalk,
					URL:         url,
					LoudnessURL: audioSrv.LoudnessURLForAudioURL(url),
					Title:       res.ArticleTitle,
					TopicTitle:  res.ArticleTitle,
					Source: domain.PlayableSource{
						RssURL:     res.FeedURL,
						ArticleURL: res.ArticleURL,
					},
				}, newHist, true, nil
			}
		}
		if ctx.Err() != nil {
			return domain.PlayableItem{}, hist, false, ctx.Err()
		}
		log.Printf("WARN: talk generation failed, fallback to BGM: %v", err)
		// Treat failed talk slot as consumed.
		p.mu.Lock()
		if !p.publishAllowedLocked(workGeneration, ctx) {
			p.mu.Unlock()
			return domain.PlayableItem{}, hist, false, generationInvalidationError(ctx)
		}
		p.bgmCountSinceLastTalk = 0
		p.pendingSilence = true
		p.mu.Unlock()
		return p.pickBGM(audioSrv, talkSvc, musicSvc, cfg, hist, workGeneration)
	}

	return p.pickBGM(audioSrv, talkSvc, musicSvc, cfg, hist, workGeneration)
}

func (p *Player) pickBGM(audioSrv *audio.Server, talkSvc *talk.Service, musicSvc *musicgen.Service, cfg domain.AppConfig, hist domain.History, workGeneration uint64) (domain.PlayableItem, domain.History, bool, error) {
	p.mu.Lock()
	prefetched := p.prefetchedMusic
	if prefetched != nil {
		p.prefetchedMusic = nil
	}
	p.mu.Unlock()

	var res musicgen.Result
	var err error
	var ctx context.Context
	if prefetched != nil {
		res = *prefetched
	} else {
		var cancel context.CancelFunc
		ctx, cancel = p.generationContext(90 * time.Second)
		defer cancel()
		if musicSvc == nil {
			return domain.PlayableItem{}, hist, false, ErrNotConfigured
		}
		res, err = musicSvc.Generate(ctx, cfg)
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		if err != nil {
			if ctx.Err() != nil {
				return domain.PlayableItem{}, hist, false, ctx.Err()
			}
			if fallback, fbErr := musicSvc.Fallback(cfg); fbErr == nil {
				res = fallback
			} else {
				p.mu.Lock()
				if p.publishAllowedLocked(workGeneration, ctx) {
					p.localGenerationError = err.Error()
				}
				p.mu.Unlock()
				return domain.PlayableItem{}, hist, false, err
			}
		}
	}

	url, err := audioSrv.RegisterFile(res.AudioPath, 10*time.Minute)
	if err != nil {
		return domain.PlayableItem{}, hist, false, err
	}

	// RegisterFile may race with Skip/UpdateConfig/Shutdown. Make the final
	// state transition only after registration and under the same mutex-bound
	// generation/context gate used by the worker publication paths.
	p.mu.Lock()
	if !p.publishAllowedLocked(workGeneration, ctx) {
		p.mu.Unlock()
		return domain.PlayableItem{}, hist, false, generationInvalidationError(ctx)
	}
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
	p.PrefetchMusic(musicSvc, cfg)

	return domain.PlayableItem{
		ID:          uuid.NewString(),
		Kind:        domain.PlayableKindBGM,
		URL:         url,
		LoudnessURL: audioSrv.LoudnessURLForAudioURL(url),
		Title:       res.Title,
		Source: domain.PlayableSource{
			FilePath: res.AudioPath,
			Provider: "stable_audio_3",
			Genre:    res.Genre,
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
	p.generation++

	// Cancel in-flight generation if any.
	if p.cancelPrefetch != nil {
		p.cancelPrefetch()
	}
	if p.cancelMusicPrefetch != nil {
		p.cancelMusicPrefetch()
	}
	// Keep the in-flight flags until each worker's defer has joined. Clearing
	// them here would report a reservation as idle while ORT is still running.

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

	return p.NextItem(audioSrv, talkSvc, musicSvc, domain.NextItemRequest{}, hist)
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
	workGeneration := p.generation
	if p.closed || !cfg.Talk.Enabled || p.prefetchedTalk != nil || p.prefetching {
		p.mu.Unlock()
		return
	}
	// Start prefetch when next is near: after (cycle-1) BGM played since last talk.
	if p.bgmCountSinceLastTalk < cycle-1 {
		p.mu.Unlock()
		return
	}
	p.prefetching = true
	p.talkInFlight++
	p.nextPrefetchOwner++
	ownerToken := p.nextPrefetchOwner
	p.talkPrefetchOwner = ownerToken
	owner := p.ownerCtx
	if owner == nil {
		owner = context.Background()
	}
	ctx, cancel := context.WithTimeout(owner, 240*time.Second)
	p.cancelPrefetch = cancel
	p.prefetchWG.Add(1)
	p.mu.Unlock()

	go func() {
		defer p.prefetchWG.Done()
		defer p.finishTalkPrefetch(workGeneration, ownerToken)

		used := buildUsedMap(hist, nil)
		res, err := talkSvc.Generate(ctx, cfg, used)
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		if err != nil {
			log.Printf("WARN: talk prefetch failed: %v", err)
			return
		}
		p.mu.Lock()
		if p.publishAllowedLocked(workGeneration, ctx) {
			p.prefetchedTalk = &res
			p.localGenerationError = generationWarning()
		}
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
		TalkPrefetching:      p.talkInFlight > 0,
		TalkReady:            p.prefetchedTalk != nil,
		MusicGenerating:      p.musicInFlight > 0,
		MusicReady:           p.prefetchedMusic != nil,
		LocalGenerationError: p.localGenerationError,
	}
}

func (p *Player) PrefetchMusic(musicSvc *musicgen.Service, cfg domain.AppConfig) {
	if musicSvc == nil {
		return
	}
	p.mu.Lock()
	workGeneration := p.generation
	if p.closed || p.prefetchedMusic != nil || p.musicPrefetching {
		p.mu.Unlock()
		return
	}
	p.musicPrefetching = true
	p.musicInFlight++
	p.nextPrefetchOwner++
	ownerToken := p.nextPrefetchOwner
	p.musicPrefetchOwner = ownerToken
	owner := p.ownerCtx
	if owner == nil {
		owner = context.Background()
	}
	ctx, cancel := context.WithTimeout(owner, 180*time.Second)
	p.cancelMusicPrefetch = cancel
	p.prefetchWG.Add(1)
	p.mu.Unlock()

	go func() {
		defer p.prefetchWG.Done()
		defer p.finishMusicPrefetch(workGeneration, ownerToken)
		res, err := musicSvc.Generate(ctx, cfg)
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		if err != nil {
			p.mu.Lock()
			if p.publishAllowedLocked(workGeneration, ctx) {
				p.localGenerationError = err.Error()
			}
			p.mu.Unlock()
			return
		}
		p.mu.Lock()
		if p.publishAllowedLocked(workGeneration, ctx) {
			p.prefetchedMusic = &res
			p.localGenerationError = generationWarning()
		}
		p.mu.Unlock()
	}()
}

// finishTalkPrefetch releases only the reservation owned by this worker. A
// replacement worker may be running after Skip/UpdateConfig while the old
// worker is still joining ORT, so generation and ownership must not be used to
// clear the replacement's active flag or cancellation handle.
func (p *Player) finishTalkPrefetch(workGeneration, ownerToken uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.talkInFlight > 0 {
		p.talkInFlight--
	}
	// Both values belong to the reservation.  The owner token prevents an old
	// worker from clearing a replacement reservation; the generation check also
	// protects the cleanup path if a reservation handle is reused while a
	// previous generation is still joining.
	if p.generation == workGeneration && p.talkPrefetchOwner == ownerToken {
		p.prefetching = false
		p.talkPrefetchOwner = 0
		p.cancelPrefetch = nil
	}
}

func (p *Player) finishMusicPrefetch(workGeneration, ownerToken uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.musicInFlight > 0 {
		p.musicInFlight--
	}
	if p.generation == workGeneration && p.musicPrefetchOwner == ownerToken {
		p.musicPrefetching = false
		p.musicPrefetchOwner = 0
		p.cancelMusicPrefetch = nil
	}
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

// publishAllowedLocked is the final publication gate. The caller must hold
// p.mu; checking the context, lifecycle, and generation together closes the
// race where Skip/UpdateConfig/Shutdown lands between the worker's last
// context check and its ready-state write.
func (p *Player) publishAllowedLocked(workGeneration uint64, ctx context.Context) bool {
	return !p.closed && p.generation == workGeneration && (ctx == nil || ctx.Err() == nil)
}

func generationInvalidationError(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return context.Canceled
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
