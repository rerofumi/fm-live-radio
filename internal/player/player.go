package player

import (
	"context"
	"errors"
	"log"
	"reflect"
	"sync"
	"time"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/musicgen"
	"fm-live-radio/internal/store"
	"fm-live-radio/internal/talk"

	"github.com/google/uuid"
)

var ErrNotConfigured = errors.New("not configured")

// TalkGenerator and MusicGenerator keep Player's coordination independent of
// the concrete services. Production services satisfy these interfaces, while
// barrier-controlled fakes can exercise demand coalescing deterministically.
type TalkGenerator interface {
	Generate(context.Context, domain.AppConfig, map[string]bool) (talk.Result, error)
}

type MusicGenerator interface {
	Generate(context.Context, domain.AppConfig) (musicgen.Result, error)
	Fallback(domain.AppConfig) (musicgen.Result, error)
}

type AudioRegistrar interface {
	RegisterFile(string, time.Duration) (string, error)
	LoudnessURLForAudioURL(string) string
}

type AudioReleaser interface {
	ReleaseAudioURL(string) bool
}

type talkJob struct {
	done     chan struct{}
	result   talk.Result
	err      error
	consumed bool
}

type musicJob struct {
	done      chan struct{}
	started   chan struct{}
	entryGate *sync.Mutex
	result    musicgen.Result
	err       error
	consumed  bool
	epoch     uint64
	owner     uint64
	cancel    context.CancelFunc
	canceled  bool
}

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

	prefetchedTalk *talk.Result
	prefetching    bool
	// talkFailureBlocked records a failed Talk slot until the next selection
	// consumes that slot as BGM. It prevents a finished failure from being
	// immediately re-created by the refill hint in pickBGM.
	talkFailureBlocked bool
	talkJob            *talkJob
	talkInFlight       int
	cancelPrefetch     context.CancelFunc
	talkPrefetchOwner  uint64

	prefetchedMusic     *musicgen.Result // retained for compatibility; FIFO is musicReady
	musicReady          []musicgen.Result
	musicJobs           map[*musicJob]struct{}
	musicSvc            MusicGenerator
	musicCfg            domain.AppConfig
	musicEpoch          uint64
	musicProtected      map[string]func()
	musicSuppressRefill bool
	// musicFailureBlocked stops a failed refill from recursively retrying in
	// finishMusicJob. An explicit demand clears it at the retry boundary.
	musicFailureBlocked  bool
	musicRefillWaitTalk  bool
	musicPrefetching     bool
	musicJob             *musicJob
	musicInFlight        int
	cancelMusicPrefetch  context.CancelFunc
	musicPrefetchOwner   uint64
	nextPrefetchOwner    uint64
	localGenerationError string

	// beforeBGMFinalPublish is a deterministic test seam for the narrow race
	// between the first post-RegisterFile epoch check and the final publish
	// gate. It is intentionally unexported and nil in production.
	beforeBGMFinalPublish func()
}

func New(cfg domain.AppConfig) *Player {
	ctx, cancel := context.WithCancel(context.Background())
	cfg.StableAudio3.Genre = store.NormalizeStableAudio3Genre(cfg.StableAudio3.Genre)
	return &Player{cfg: cfg, pendingSilence: true, ownerCtx: ctx, ownerCancel: cancel, shutdownAccepted: make(chan struct{}), musicJobs: make(map[*musicJob]struct{}), musicEpoch: 1, musicProtected: make(map[string]func())}
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
	p.updateConfig(cfg, false)
}

// UpdateConfigFromSave applies the Settings SaveConfig contract. A save that
// changes no fields other than the normalized music genre keeps the current
// Talk/current/history state, including the normalized no-op case.
func (p *Player) UpdateConfigFromSave(cfg domain.AppConfig) {
	p.updateConfig(cfg, true)
}

func (p *Player) updateConfig(cfg domain.AppConfig, preserveMusicOnlyNoop bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	old := p.cfg
	cfg.StableAudio3.Genre = store.NormalizeStableAudio3Genre(cfg.StableAudio3.Genre)
	if configEqualExceptMusicGenre(old, cfg) {
		if store.NormalizeStableAudio3Genre(old.StableAudio3.Genre) != cfg.StableAudio3.Genre {
			p.updateMusicGenreLocked(cfg.StableAudio3.Genre)
			return
		}
		if !preserveMusicOnlyNoop {
			// Keep the historical direct Player.UpdateConfig behavior used by
			// lifecycle replacement callers. Settings SaveConfig uses the
			// explicit method above when a normalized no-op must be retained.
			goto fullReset
		}
		// A normalized no-op genre save is still music-only. Do not take the
		// full reset path, which would discard Talk/current/history state.
		p.cfg.StableAudio3.Genre = cfg.StableAudio3.Genre
		return
	}

fullReset:
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
	p.updateMusicGenreLocked(store.NormalizeStableAudio3Genre(genre))
}

func configEqualExceptMusicGenre(a, b domain.AppConfig) bool {
	a.StableAudio3.Genre = ""
	b.StableAudio3.Genre = ""
	return reflect.DeepEqual(a, b)
}

func (p *Player) updateMusicGenreLocked(genre string) {
	genre = store.NormalizeStableAudio3Genre(genre)
	if store.NormalizeStableAudio3Genre(p.cfg.StableAudio3.Genre) == genre {
		p.cfg.StableAudio3.Genre = genre
		return
	}
	p.cfg.StableAudio3.Genre = genre
	p.musicEpoch++
	p.musicFailureBlocked = false
	p.musicSuppressRefill = false
	p.musicRefillWaitTalk = false
	p.musicCfg.StableAudio3.Genre = genre
	p.clearMusicReadyLocked()
	for job := range p.musicJobs {
		job.canceled = true
		if job.cancel != nil {
			job.cancel()
		}
	}
	p.musicPrefetching = false
	p.musicJob = nil
	p.cancelMusicPrefetch = nil
	// A worker that belongs to the previous epoch may still be joining ORT.
	// It is intentionally excluded from the new epoch's capacity accounting.
	if p.musicSvc != nil {
		if !p.musicSuppressRefill {
			p.ensureMusicPrefetchLocked()
		}
	}
}

func (p *Player) NextItem(audioSrv AudioRegistrar, talkSvc TalkGenerator, musicSvc MusicGenerator, req domain.NextItemRequest, hist domain.History) (domain.PlayableItem, domain.History, bool, error) {
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
		if p.talkJob != nil {
			p.talkJob.consumed = true
		}
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
		p.mu.Lock()
		talkPending := p.prefetching || p.talkJob != nil
		p.mu.Unlock()
		// A pending Talk does not consume its slot merely because the current
		// boundary is slow. Let BGM carry the programme forward; when no BGM is
		// ready, pickBGM joins or starts the same Music demand. The same Talk
		// remains the first choice at the next boundary.
		if talkPending {
			return p.pickBGM(audioSrv, talkSvc, musicSvc, cfg, hist, workGeneration)
		}
		p.mu.Lock()
		if p.talkFailureBlocked {
			// The failed Talk slot is consumed exactly once. Reset the cycle
			// before selecting its BGM fallback so this boundary cannot retry.
			p.talkFailureBlocked = false
			p.bgmCountSinceLastTalk = 0
			p.pendingSilence = true
			p.mu.Unlock()
			return p.pickBGM(audioSrv, talkSvc, musicSvc, cfg, hist, workGeneration)
		}
		p.mu.Unlock()
		res, err := p.awaitTalk(talkSvc, cfg, hist, workGeneration)
		if err == nil {
			url, err2 := audioSrv.RegisterFile(res.AudioPath, 10*time.Minute)
			if err2 == nil {
				p.mu.Lock()
				if !p.publishAllowedLocked(workGeneration, nil) {
					p.mu.Unlock()
					return domain.PlayableItem{}, hist, false, generationInvalidationError(nil)
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
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return domain.PlayableItem{}, hist, false, err
			}
			log.Printf("WARN: talk generation failed, fallback to BGM: %v", err)
		}
		// Treat failed talk slot as consumed.
		p.mu.Lock()
		if !p.publishAllowedLocked(workGeneration, nil) {
			p.mu.Unlock()
			return domain.PlayableItem{}, hist, false, generationInvalidationError(nil)
		}
		p.bgmCountSinceLastTalk = 0
		p.pendingSilence = true
		p.mu.Unlock()
		return p.pickBGM(audioSrv, talkSvc, musicSvc, cfg, hist, workGeneration)
	}

	return p.pickBGM(audioSrv, talkSvc, musicSvc, cfg, hist, workGeneration)
}

func (p *Player) pickBGM(audioSrv AudioRegistrar, talkSvc TalkGenerator, musicSvc MusicGenerator, cfg domain.AppConfig, hist domain.History, workGeneration uint64) (domain.PlayableItem, domain.History, bool, error) {
	selectionCtx, cancelSelection := p.generationContext(90 * time.Second)
	defer cancelSelection()
	const maxSelectionRetries = 16
	for attempt := 0; attempt < maxSelectionRetries; attempt++ {
		if err := selectionCtx.Err(); err != nil {
			return domain.PlayableItem{}, hist, false, err
		}
		p.mu.Lock()
		if p.closed || p.generation != workGeneration {
			p.mu.Unlock()
			return domain.PlayableItem{}, hist, false, generationInvalidationError(nil)
		}
		selectionEpoch := p.musicEpoch
		var prefetched *musicgen.Result
		if len(p.musicReady) > 0 {
			res := p.musicReady[0]
			p.musicReady = p.musicReady[1:]
			p.prefetchedMusic = nil
			// Consuming a ready item is an explicit demand boundary. Allow one
			// refill after a previous worker failed.
			p.musicFailureBlocked = false
			prefetched = &res
			if !p.musicSuppressRefill {
				p.ensureMusicPrefetchLocked()
			}
		}
		p.mu.Unlock()

		var res musicgen.Result
		var err error
		if prefetched != nil {
			res = *prefetched
		} else {
			if musicSvc == nil {
				p.mu.Lock()
				// Reuse an already registered demand when the caller omits the
				// service. After Skip the public job is cleared, so this must not
				// create an unsolicited replacement while Skip is joining.
				if p.musicJob != nil {
					musicSvc = p.musicSvc
				}
				p.mu.Unlock()
			}
			if musicSvc == nil {
				return domain.PlayableItem{}, hist, false, ErrNotConfigured
			}
			res, err = p.awaitMusic(musicSvc, cfg, workGeneration)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return domain.PlayableItem{}, hist, false, err
				}
				if fallback, fbErr := musicSvc.Fallback(cfg); fbErr == nil {
					res = fallback
				} else {
					p.mu.Lock()
					if p.publishAllowedLocked(workGeneration, nil) {
						p.localGenerationError = err.Error()
					}
					p.mu.Unlock()
					return domain.PlayableItem{}, hist, false, err
				}
			}
		}
		selectionRelease := musicgen.ProtectResult(res.AudioPath)
		url, err := audioSrv.RegisterFile(res.AudioPath, 10*time.Minute)
		if err != nil {
			selectionRelease()
			p.mu.Lock()
			p.releaseMusicResultLocked(res.AudioPath)
			p.mu.Unlock()
			return domain.PlayableItem{}, hist, false, err
		}

		// First check closes the ordinary RegisterFile -> genre update race.
		// The hook is placed after it so tests can deterministically open the
		// remaining window immediately before the final publication gate.
		p.mu.Lock()
		p.releaseMusicResultLocked(res.AudioPath)
		stale := p.musicEpoch != selectionEpoch || !p.musicResultGenreMatchesLocked(res)
		beforeFinalPublish := p.beforeBGMFinalPublish
		p.mu.Unlock()
		if stale {
			selectionRelease()
			if releaser, ok := audioSrv.(AudioReleaser); ok {
				releaser.ReleaseAudioURL(url)
			}
			continue
		}
		if beforeFinalPublish != nil {
			beforeFinalPublish()
		}

		// RegisterFile may race with Skip/UpdateConfig/Shutdown or a genre-only
		// update. Recheck every publication identity under one mutex immediately
		// before changing ready/status state or returning the item.
		p.mu.Lock()
		if !p.publishAllowedLocked(workGeneration, nil) || p.musicEpoch != selectionEpoch || !p.musicResultGenreMatchesLocked(res) {
			latestCfg := p.cfg
			retrySvc := musicSvc
			if retrySvc == nil {
				retrySvc = p.musicSvc
			}
			p.mu.Unlock()
			selectionRelease()
			if releaser, ok := audioSrv.(AudioReleaser); ok {
				releaser.ReleaseAudioURL(url)
			}
			if err := selectionCtx.Err(); err != nil {
				return domain.PlayableItem{}, hist, false, err
			}
			if retrySvc == nil {
				return domain.PlayableItem{}, hist, false, ErrNotConfigured
			}
			cfg = latestCfg
			musicSvc = retrySvc
			continue
		}
		p.bgmCountSinceLastTalk++
		p.pendingSilence = true
		p.localGenerationError = generationWarning()
		count := p.bgmCountSinceLastTalk
		p.mu.Unlock()
		selectionRelease()

		cycle := cfg.Talk.CycleBgmCount
		if cycle <= 0 {
			cycle = 3
		}
		p.mu.Lock()
		talkFailureBlocked := p.talkFailureBlocked
		p.mu.Unlock()
		if talkSvc != nil && cfg.Talk.Enabled && count >= cycle-1 && !talkFailureBlocked {
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
	return domain.PlayableItem{}, hist, false, context.Canceled
}

// musicResultGenreMatchesLocked compares a result with the current normalized
// selection. Empty genre keeps compatibility with test/minimal generators;
// generated results that carry provenance must match exactly.
func (p *Player) musicResultGenreMatchesLocked(res musicgen.Result) bool {
	if res.Genre == "" {
		return true
	}
	return store.NormalizeStableAudio3Genre(res.Genre) == store.NormalizeStableAudio3Genre(p.cfg.StableAudio3.Genre)
}

func (p *Player) Skip(audioSrv AudioRegistrar, talkSvc TalkGenerator, musicSvc MusicGenerator, req domain.SkipRequest, hist domain.History) (domain.PlayableItem, domain.History, bool, error) {
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
		// Invalidate the public owner immediately, while retaining the
		// in-flight count until the worker's actual join. This allows a
		// replacement reservation to be registered without letting the old
		// worker clear it when its deferred finish runs.
		p.cancelPrefetch = nil
		p.prefetching = false
		p.talkJob = nil
		p.talkPrefetchOwner = 0
	}
	if p.cancelMusicPrefetch != nil {
		p.musicSuppressRefill = true
		for job := range p.musicJobs {
			if job.epoch == p.musicEpoch && !job.canceled {
				job.canceled = true
			}
		}
		p.cancelMusicPrefetch()
		p.cancelMusicPrefetch = nil
		p.musicPrefetching = false
		p.musicJob = nil
		p.musicPrefetchOwner = 0
	}
	// Keep the in-flight counts until each worker's defer has joined. The public
	// owner fields may be cleared now so a replacement can register safely;
	// Status still reports busy from the counts while ORT is running.

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

// PrefetchTalk is kept as the public App binding. One application trigger is
// coordinated here so the Music reservation is queued before Talk starts.
// History is committed only when the shared Talk job is consumed.
func (p *Player) PrefetchTalk(talkSvc TalkGenerator, cfg domain.AppConfig, hist domain.History) {
	p.prefetchNext(talkSvc, nil, cfg, hist)
}

// PrefetchNext is the coordinated application trigger. Music is registered
// first, then the near-talk decision is started under the same generation.
func (p *Player) PrefetchNext(talkSvc TalkGenerator, musicSvc MusicGenerator, cfg domain.AppConfig, hist domain.History) {
	p.prefetchNext(talkSvc, musicSvc, cfg, hist)
}

func (p *Player) prefetchNext(talkSvc TalkGenerator, musicSvc MusicGenerator, cfg domain.AppConfig, hist domain.History) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	// An explicit hint is the retry boundary after a failed refill.
	p.musicFailureBlocked = false
	if musicSvc != nil {
		musicCfg := cfg
		// A late caller (for example, pickBGM after a genre-only save) may
		// still carry an old snapshot. The Player's current config is the
		// source of truth at a new demand boundary.
		if !reflect.DeepEqual(musicCfg, p.cfg) {
			musicCfg = p.cfg
		}
		p.musicSvc, p.musicCfg = musicSvc, musicCfg
	}
	// Reserve Music synchronously before registering Talk. Reserve never waits;
	// the actual model interval remains outside p.mu in the worker.
	musicJob := p.ensureMusicPrefetchLocked()
	if talkSvc == nil || !cfg.Talk.Enabled || p.prefetchedTalk != nil || p.prefetching {
		return
	}
	cycle := cfg.Talk.CycleBgmCount
	if cycle <= 0 {
		cycle = 3
	}
	if p.bgmCountSinceLastTalk < cycle-1 {
		return
	}
	var started <-chan struct{}
	if musicJob != nil {
		started = musicJob.started
	}
	var entryGate *sync.Mutex
	if musicJob != nil {
		entryGate = musicJob.entryGate
		p.musicRefillWaitTalk = true
	}
	p.startTalkPrefetchLocked(talkSvc, cfg, hist, 240*time.Second, started, entryGate)
}

func (p *Player) clearPrefetchLocked() {
	if p.cancelPrefetch != nil {
		p.cancelPrefetch()
		p.cancelPrefetch = nil
	}
	p.prefetchedTalk = nil
	p.prefetching = false
	p.talkFailureBlocked = false
	p.talkJob = nil
	p.musicSuppressRefill = true
	p.musicFailureBlocked = false
	p.musicRefillWaitTalk = false
	for job := range p.musicJobs {
		job.canceled = true
		if job.cancel != nil {
			job.cancel()
		}
	}
	p.cancelMusicPrefetch = nil
	p.clearMusicReadyLocked()
	p.musicPrefetching = false
	p.musicJob = nil
	p.localGenerationError = ""
}

func (p *Player) clearMusicReadyLocked() {
	for path, release := range p.musicProtected {
		if release != nil {
			release()
		}
		delete(p.musicProtected, path)
	}
	p.musicReady = nil
	p.prefetchedMusic = nil
}

func (p *Player) protectMusicResultLocked(res musicgen.Result) {
	if res.AudioPath == "" {
		return
	}
	if p.musicProtected == nil {
		p.musicProtected = make(map[string]func())
	}
	if _, ok := p.musicProtected[res.AudioPath]; !ok {
		p.musicProtected[res.AudioPath] = musicgen.ProtectResult(res.AudioPath)
	}
}

func (p *Player) releaseMusicResultLocked(path string) {
	if release := p.musicProtected[path]; release != nil {
		release()
		delete(p.musicProtected, path)
	}
}

// awaitTalk coalesces synchronous NextItem with an existing prefetch job. The
// wait is deliberately outside p.mu so cancellation and Skip can progress.
func (p *Player) awaitTalk(svc TalkGenerator, cfg domain.AppConfig, hist domain.History, workGeneration uint64) (talk.Result, error) {
	ctx, cancel := p.generationContext(180 * time.Second)
	defer cancel()
	for {
		p.mu.Lock()
		if p.generation != workGeneration || p.closed {
			p.mu.Unlock()
			return talk.Result{}, context.Canceled
		}
		if p.prefetchedTalk != nil {
			res := *p.prefetchedTalk
			p.prefetchedTalk = nil
			if p.talkJob != nil {
				p.talkJob.consumed = true
			}
			p.mu.Unlock()
			return res, nil
		}
		job := p.talkJob
		if job == nil || (job.consumed && job.err == nil) {
			job = p.startTalkPrefetchLocked(svc, cfg, hist, 180*time.Second, nil, nil)
		}
		if job == nil {
			p.mu.Unlock()
			return talk.Result{}, context.Canceled
		}
		p.mu.Unlock()
		select {
		case <-job.done:
			if job.err != nil {
				return talk.Result{}, job.err
			}
			// The worker publishes under p.mu before closing done. Loop to
			// consume the one ready result.
		case <-ctx.Done():
			return talk.Result{}, ctx.Err()
		}
	}
}

func (p *Player) awaitMusic(svc MusicGenerator, cfg domain.AppConfig, workGeneration uint64) (musicgen.Result, error) {
	ctx, cancel := p.generationContext(90 * time.Second)
	defer cancel()
	for {
		p.mu.Lock()
		if p.generation != workGeneration || p.closed {
			p.mu.Unlock()
			return musicgen.Result{}, context.Canceled
		}
		if len(p.musicReady) > 0 {
			res := p.musicReady[0]
			p.musicReady = p.musicReady[1:]
			p.prefetchedMusic = nil
			// This is a consume-triggered refill boundary.
			p.musicFailureBlocked = false
			if !p.musicSuppressRefill {
				p.ensureMusicPrefetchLocked()
			}
			p.mu.Unlock()
			return res, nil
		}
		if !reflect.DeepEqual(cfg, p.cfg) {
			cfg = p.cfg
		}
		p.musicSvc, p.musicCfg = svc, cfg
		job := p.musicJob
		if job == nil {
			// NextItem is an explicit demand and therefore the retry boundary
			// after a failed refill.
			p.musicFailureBlocked = false
			p.musicSvc, p.musicCfg = svc, cfg
			job = p.ensureMusicPrefetchLocked()
		}
		if job == nil {
			p.mu.Unlock()
			return musicgen.Result{}, context.Canceled
		}
		p.mu.Unlock()
		select {
		case <-job.done:
			if job.err != nil {
				return musicgen.Result{}, job.err
			}
		case <-ctx.Done():
			return musicgen.Result{}, ctx.Err()
		}
	}
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
		MusicReady:           len(p.musicReady) > 0,
		LocalGenerationError: p.localGenerationError,
	}
}

func (p *Player) PrefetchMusic(musicSvc MusicGenerator, cfg domain.AppConfig) {
	if musicSvc == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	if !reflect.DeepEqual(cfg, p.cfg) {
		cfg = p.cfg
	}
	p.musicSvc, p.musicCfg = musicSvc, cfg
	// An explicit prefetch hint is the retry boundary after a failed refill.
	p.musicFailureBlocked = false
	p.ensureMusicPrefetchLocked()
	p.mu.Unlock()
}

func (p *Player) startTalkPrefetchLocked(talkSvc TalkGenerator, cfg domain.AppConfig, hist domain.History, timeout time.Duration, waitFor <-chan struct{}, entryGate *sync.Mutex) *talkJob {
	owner := p.ownerCtx
	if owner == nil {
		owner = context.Background()
	}
	reservation, err := generation.Reserve(owner, generation.KindTalk)
	if err != nil {
		return nil
	}
	workGeneration := p.generation
	p.prefetching = true
	p.talkInFlight++
	p.nextPrefetchOwner++
	ownerToken := p.nextPrefetchOwner
	p.talkPrefetchOwner = ownerToken
	ctx, cancel := context.WithTimeout(owner, timeout)
	p.cancelPrefetch = cancel
	job := &talkJob{done: make(chan struct{})}
	p.talkFailureBlocked = false
	p.talkJob = job
	p.prefetchWG.Add(1)
	go func() {
		defer p.prefetchWG.Done()
		defer reservation.Release()
		if waitFor != nil {
			select {
			case <-waitFor:
			case <-ctx.Done():
			}
		}
		if entryGate != nil {
			entryGate.Lock()
			defer entryGate.Unlock()
		}
		used := buildUsedMap(hist, nil)
		res, runErr := talkSvc.Generate(generation.WithReservation(ctx, reservation), cfg, used)
		p.mu.Lock()
		if p.generation == workGeneration {
			p.musicRefillWaitTalk = false
		}
		p.mu.Unlock()
		if runErr == nil && ctx.Err() != nil {
			runErr = ctx.Err()
		}
		p.mu.Lock()
		job.result, job.err = res, runErr
		if runErr != nil && p.generation == workGeneration {
			p.talkFailureBlocked = true
		}
		if runErr == nil && p.publishAllowedLocked(workGeneration, ctx) {
			p.prefetchedTalk = &res
			p.localGenerationError = generationWarning()
		}
		close(job.done)
		p.mu.Unlock()
		if runErr != nil {
			log.Printf("WARN: talk prefetch failed: %v", runErr)
		}
		p.finishTalkPrefetch(workGeneration, ownerToken)
	}()
	return job
}

func (p *Player) startMusicPrefetchLocked(musicSvc MusicGenerator, cfg domain.AppConfig, reservation *generation.Reservation, timeout time.Duration) *musicJob {
	workGeneration := p.generation
	if p.musicJobs == nil {
		p.musicJobs = make(map[*musicJob]struct{})
	}
	workEpoch := p.musicEpoch
	p.musicPrefetching = true
	p.musicInFlight++
	p.nextPrefetchOwner++
	ownerToken := p.nextPrefetchOwner
	p.musicPrefetchOwner = ownerToken
	owner := p.ownerCtx
	if owner == nil {
		owner = context.Background()
	}
	ctx, cancel := context.WithTimeout(owner, timeout)
	p.cancelMusicPrefetch = cancel
	job := &musicJob{done: make(chan struct{}), started: make(chan struct{}), entryGate: &sync.Mutex{}, epoch: workEpoch, owner: ownerToken, cancel: cancel}
	p.musicJob = job
	p.musicJobs[job] = struct{}{}
	p.prefetchWG.Add(1)
	go func() {
		defer p.prefetchWG.Done()
		defer reservation.Release()
		job.entryGate.Lock()
		close(job.started)
		res, runErr := musicSvc.Generate(generation.WithReservation(ctx, reservation), cfg)
		job.entryGate.Unlock()
		if runErr == nil && ctx.Err() != nil {
			runErr = ctx.Err()
		}
		p.mu.Lock()
		job.result, job.err = res, runErr
		if runErr == nil && p.publishAllowedLocked(workGeneration, ctx) && p.musicEpoch == workEpoch {
			p.musicReady = append(p.musicReady, res)
			p.prefetchedMusic = &p.musicReady[0]
			p.protectMusicResultLocked(res)
			p.localGenerationError = generationWarning()
		} else if runErr == nil {
			musicgen.RemoveResult(res.AudioPath)
		}
		close(job.done)
		p.mu.Unlock()
		if runErr != nil {
			p.mu.Lock()
			if p.publishAllowedLocked(workGeneration, ctx) && p.musicEpoch == workEpoch {
				p.localGenerationError = runErr.Error()
			}
			p.mu.Unlock()
		}
		p.finishMusicJob(job, workGeneration, ownerToken)
	}()
	return job
}

// ensureMusicPrefetchLocked keeps the current epoch's ready plus outstanding
// reservations at two. The arbiter serializes actual runtime execution; up to
// two reservations are intentionally allowed to wait there, so no duplicate
// demand is created by hint bursts.
func (p *Player) ensureMusicPrefetchLocked() *musicJob {
	if p.musicSvc == nil || p.closed || p.musicRefillWaitTalk {
		return nil
	}
	if p.musicJobs == nil {
		p.musicJobs = make(map[*musicJob]struct{})
	}
	var first *musicJob
	// Generation is deliberately one-at-a-time. Once a result is published,
	// finishMusicJob invokes this method again to fill the second slot. This
	// keeps the reservation queue bounded while preserving FIFO completion.
	if len(p.musicReady) >= 2 || p.currentMusicJobsLocked() > 0 {
		return nil
	}
	for len(p.musicReady)+p.currentMusicJobsLocked() < 2 {
		owner := p.ownerCtx
		if owner == nil {
			owner = context.Background()
		}
		reservation, err := generation.Reserve(owner, generation.KindMusic)
		if err != nil {
			break
		}
		job := p.startMusicPrefetchLocked(p.musicSvc, p.musicCfg, reservation, 180*time.Second)
		if first == nil {
			first = job
		}
		break
	}
	return first
}

func (p *Player) currentMusicJobsLocked() int {
	n := 0
	for job := range p.musicJobs {
		if job.epoch == p.musicEpoch && !job.canceled {
			n++
		}
	}
	return n
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
		p.talkJob = nil
	}
}

func (p *Player) finishMusicPrefetch(workGeneration, ownerToken uint64) {
	p.mu.Lock()
	var target *musicJob
	for job := range p.musicJobs {
		if job.owner == ownerToken {
			target = job
			break
		}
	}
	p.mu.Unlock()
	if target != nil {
		p.finishMusicJob(target, workGeneration, ownerToken)
		return
	}
	// Compatibility for narrow lifecycle tests that exercise this cleanup
	// method with a synthetic owner rather than a registered job.
	p.mu.Lock()
	if p.musicInFlight > 0 {
		p.musicInFlight--
	}
	if p.generation == workGeneration && p.musicPrefetchOwner == ownerToken {
		p.musicPrefetching = false
		p.musicPrefetchOwner = 0
		p.cancelMusicPrefetch = nil
		p.musicJob = nil
	}
	p.mu.Unlock()
}

func (p *Player) finishMusicJob(job *musicJob, workGeneration, ownerToken uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.musicInFlight > 0 {
		p.musicInFlight--
	}
	delete(p.musicJobs, job)
	if p.generation == workGeneration && p.musicEpoch == job.epoch && p.musicPrefetchOwner == ownerToken {
		if job.err != nil {
			// Record the failure and stop this refill cycle. Calling ensure here
			// would immediately retry forever when the provider keeps failing.
			p.musicFailureBlocked = true
		}
		p.musicPrefetching = p.currentMusicJobsLocked() > 0
		if p.musicJob == job {
			p.musicJob = nil
			p.cancelMusicPrefetch = nil
		}
		if !p.musicPrefetching {
			p.musicPrefetchOwner = 0
		}
		// Completion itself opens the next refill slot. The first completed
		// result was already published above, so callers can consume it without
		// waiting for this refill.
		if job.err == nil && !p.musicFailureBlocked && !p.musicSuppressRefill {
			p.ensureMusicPrefetchLocked()
		}
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
