package player

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/musicgen"
	"fm-live-radio/internal/talk"
)

type failingMusicGenerator struct {
	calls   atomic.Int32
	entered chan struct{}
	once    sync.Once
	err     error
}

func (f *failingMusicGenerator) Generate(context.Context, domain.AppConfig) (musicgen.Result, error) {
	f.calls.Add(1)
	if f.entered != nil {
		f.once.Do(func() { close(f.entered) })
	}
	return musicgen.Result{}, f.err
}

func (f *failingMusicGenerator) Fallback(domain.AppConfig) (musicgen.Result, error) {
	return musicgen.Result{}, f.err
}

type testAudioRegistrar struct{}

func (testAudioRegistrar) RegisterFile(path string, _ time.Duration) (string, error) {
	if path == "" {
		return "", context.Canceled
	}
	return "audio://test/" + path, nil
}

func (testAudioRegistrar) LoudnessURLForAudioURL(url string) string { return url + "/loudness" }

type testTalkGenerator struct {
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	result  talk.Result
}

type failingTalkGenerator struct {
	calls atomic.Int32
	err   error
}

func (f *failingTalkGenerator) Generate(context.Context, domain.AppConfig, map[string]bool) (talk.Result, error) {
	f.calls.Add(1)
	return talk.Result{}, f.err
}

func (f *testTalkGenerator) Generate(ctx context.Context, _ domain.AppConfig, _ map[string]bool) (talk.Result, error) {
	f.calls.Add(1)
	if f.entered != nil {
		f.once.Do(func() { close(f.entered) })
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return talk.Result{}, ctx.Err()
		}
	}
	return f.result, nil
}

type testMusicGenerator struct {
	calls     atomic.Int32
	active    atomic.Int32
	maxActive atomic.Int32
	entered   chan struct{}
	release   chan struct{}
	once      sync.Once
	result    musicgen.Result
}

// barrierTalkGenerator keeps a running Player worker in its Generate call
// after cancellation. This makes the cancellation/join/publication contract
// deterministic without a timing assumption.
type barrierTalkGenerator struct {
	calls     atomic.Int32
	started   chan int
	cancelled chan int
	release   []chan struct{}
	results   []talk.Result
}

func (f *barrierTalkGenerator) Generate(ctx context.Context, _ domain.AppConfig, _ map[string]bool) (talk.Result, error) {
	n := int(f.calls.Add(1))
	f.started <- n
	select {
	case <-f.release[n-1]:
		if ctx.Err() != nil {
			f.cancelled <- n
		}
	case <-ctx.Done():
		f.cancelled <- n
		<-f.release[n-1]
	}
	return f.results[n-1], nil
}

type barrierMusicGenerator struct {
	calls     atomic.Int32
	started   chan int
	cancelled chan int
	release   []chan struct{}
	results   []musicgen.Result
}

func (f *barrierMusicGenerator) Generate(ctx context.Context, _ domain.AppConfig) (musicgen.Result, error) {
	n := int(f.calls.Add(1))
	f.started <- n
	select {
	case <-f.release[n-1]:
	case <-ctx.Done():
		f.cancelled <- n
		<-f.release[n-1]
	}
	return f.results[n-1], nil
}

func (f *barrierMusicGenerator) Fallback(domain.AppConfig) (musicgen.Result, error) {
	return f.results[0], nil
}

type reservationMusicStart struct {
	number int
	genre  string
}

// reservationBarrierMusicGenerator follows the production service boundary:
// it consumes the reservation carried in context, holds the lease while the
// fake runtime is blocked, and releases it only after the runtime exits.
type reservationBarrierMusicGenerator struct {
	calls     atomic.Int32
	started   chan reservationMusicStart
	cancelled chan int
	release   []chan struct{}
	results   []musicgen.Result
}

func (f *reservationBarrierMusicGenerator) Generate(ctx context.Context, cfg domain.AppConfig) (musicgen.Result, error) {
	reservation := generation.ReservationFromContext(ctx, generation.KindMusic)
	if reservation == nil {
		return musicgen.Result{}, errors.New("music reservation missing")
	}
	lease, err := reservation.Acquire(ctx)
	if err != nil {
		return musicgen.Result{}, err
	}
	defer lease.Release()
	n := int(f.calls.Add(1))
	f.started <- reservationMusicStart{number: n, genre: cfg.StableAudio3.Genre}
	select {
	case <-f.release[n-1]:
	case <-ctx.Done():
		f.cancelled <- n
		<-f.release[n-1]
	}
	return f.results[n-1], nil
}

func (f *reservationBarrierMusicGenerator) Fallback(domain.AppConfig) (musicgen.Result, error) {
	return f.results[0], nil
}

func (f *testMusicGenerator) Generate(ctx context.Context, _ domain.AppConfig) (musicgen.Result, error) {
	f.calls.Add(1)
	active := f.active.Add(1)
	defer f.active.Add(-1)
	for {
		max := f.maxActive.Load()
		if active <= max || f.maxActive.CompareAndSwap(max, active) {
			break
		}
	}
	if f.entered != nil {
		f.once.Do(func() { close(f.entered) })
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return musicgen.Result{}, ctx.Err()
		}
	}
	return f.result, nil
}

func (f *testMusicGenerator) Fallback(domain.AppConfig) (musicgen.Result, error) {
	return f.result, nil
}

func testPlayerConfig() domain.AppConfig {
	return domain.AppConfig{Talk: domain.TalkConfig{Enabled: true, CycleBgmCount: 1}}
}

func preparePlayer(p *Player, cfg domain.AppConfig, bgmCount int) {
	p.mu.Lock()
	p.cfg = cfg
	p.pendingSilence = false
	p.bgmCountSinceLastTalk = bgmCount
	p.mu.Unlock()
}

func TestNextItemJoinsTalkPrefetchDemand(t *testing.T) {
	p := New(testPlayerConfig())
	talkGen := &testTalkGenerator{
		entered: make(chan struct{}), release: make(chan struct{}),
		result: talk.Result{AudioPath: "talk.wav", ArticleURL: "article", ArticleTitle: "記事"},
	}
	preparePlayer(p, testPlayerConfig(), 1)
	p.PrefetchTalk(talkGen, testPlayerConfig(), domain.History{})
	<-talkGen.entered

	itemCh := make(chan domain.PlayableItem, 1)
	errCh := make(chan error, 1)
	go func() {
		item, _, _, err := p.NextItem(testAudioRegistrar{}, talkGen, nil, domain.NextItemRequest{}, domain.History{})
		itemCh <- item
		errCh <- err
	}()
	close(talkGen.release)
	if err := <-errCh; err != nil {
		t.Fatalf("NextItem returned error: %v", err)
	}
	if got := (<-itemCh).Kind; got != domain.PlayableKindTalk {
		t.Fatalf("kind=%q, want talk", got)
	}
	if got := talkGen.calls.Load(); got != 1 {
		t.Fatalf("talk Generate calls=%d, want 1", got)
	}
	p.Shutdown()
}

func TestNextItemJoinsMusicPrefetchDemand(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}}
	p := New(cfg)
	preparePlayer(p, cfg, 0)
	musicGen := &testMusicGenerator{
		entered: make(chan struct{}), release: make(chan struct{}),
		result: musicgen.Result{AudioPath: "music.wav", Title: "曲", Genre: "ambient"},
	}
	p.PrefetchMusic(musicGen, cfg)
	<-musicGen.entered
	itemCh := make(chan domain.PlayableItem, 1)
	errCh := make(chan error, 1)
	go func() {
		item, _, _, err := p.NextItem(testAudioRegistrar{}, nil, musicGen, domain.NextItemRequest{}, domain.History{})
		itemCh <- item
		errCh <- err
	}()
	close(musicGen.release)
	if err := <-errCh; err != nil {
		t.Fatalf("NextItem returned error: %v", err)
	}
	if got := (<-itemCh).Kind; got != domain.PlayableKindBGM {
		t.Fatalf("kind=%q, want bgm", got)
	}
	if got := musicGen.maxActive.Load(); got != 1 {
		t.Fatalf("maximum concurrent music Generate calls=%d, want 1", got)
	}
	p.Shutdown()
}

func TestPendingTalkWaitsForMusicWhenNoReadyBGM(t *testing.T) {
	cfg := testPlayerConfig()
	p := New(cfg)
	preparePlayer(p, cfg, 1)
	talkGen := &barrierTalkGenerator{
		started: make(chan int, 1), cancelled: make(chan int, 1),
		release: []chan struct{}{make(chan struct{})},
		results: []talk.Result{{AudioPath: "talk.wav", ArticleURL: "article"}},
	}
	p.PrefetchTalk(talkGen, cfg, domain.History{})
	if got := <-talkGen.started; got != 1 {
		t.Fatalf("talk worker=%d, want 1", got)
	}
	musicGen := &testMusicGenerator{result: musicgen.Result{AudioPath: "music.wav", Title: "music"}}
	item, history, historyUpdated, err := p.NextItem(testAudioRegistrar{}, talkGen, musicGen, domain.NextItemRequest{}, domain.History{})
	if err != nil {
		t.Fatalf("NextItem returned error: %v", err)
	}
	if item.Kind != domain.PlayableKindBGM || item.Title != "music" {
		t.Fatalf("item=%+v, want BGM", item)
	}
	if historyUpdated || len(history.UsedArticleUrls) != 0 {
		t.Fatalf("pending Talk changed history: updated=%t history=%+v", historyUpdated, history)
	}
	if got := talkGen.calls.Load(); got != 1 {
		t.Fatalf("Talk Generate calls=%d, want one existing job", got)
	}
	if !p.Status().TalkPrefetching {
		t.Fatal("Talk worker was not kept pending while BGM was returned")
	}
	close(talkGen.release[0])
	p.Shutdown()
}

func TestPendingTalkKeepsSlotAndHistoryUntilTalkBoundary(t *testing.T) {
	cfg := testPlayerConfig()
	p := New(cfg)
	preparePlayer(p, cfg, 1)
	talkGen := &barrierTalkGenerator{
		started: make(chan int, 1), cancelled: make(chan int, 1),
		release: []chan struct{}{make(chan struct{})},
		results: []talk.Result{{AudioPath: "talk.wav", ArticleURL: "article", ArticleTitle: "記事"}},
	}
	p.PrefetchTalk(talkGen, cfg, domain.History{})
	if got := <-talkGen.started; got != 1 {
		t.Fatalf("talk worker=%d, want 1", got)
	}
	p.mu.Lock()
	p.musicReady = []musicgen.Result{{AudioPath: "one.wav", Title: "one"}, {AudioPath: "two.wav", Title: "two"}}
	p.mu.Unlock()
	hist := domain.History{UsedArticleUrls: []string{"prior"}}
	for _, want := range []string{"one", "two"} {
		item, gotHist, updated, err := p.NextItem(testAudioRegistrar{}, talkGen, nil, domain.NextItemRequest{}, hist)
		if err != nil || item.Kind != domain.PlayableKindBGM || item.Title != want {
			t.Fatalf("item=(%+v,%v), want BGM %q", item, err, want)
		}
		if updated || len(gotHist.UsedArticleUrls) != 1 || gotHist.UsedArticleUrls[0] != "prior" {
			t.Fatalf("pending Talk changed history: updated=%t history=%+v", updated, gotHist)
		}
		p.mu.Lock()
		if p.talkJob == nil || p.prefetchedTalk != nil {
			p.mu.Unlock()
			t.Fatal("pending Talk slot was consumed before completion")
		}
		p.mu.Unlock()
	}
	if got := talkGen.calls.Load(); got != 1 {
		t.Fatalf("Talk Generate calls=%d, want one", got)
	}
	close(talkGen.release[0])
	p.prefetchWG.Wait()
	item, gotHist, updated, err := p.NextItem(testAudioRegistrar{}, talkGen, nil, domain.NextItemRequest{}, hist)
	if err != nil || item.Kind != domain.PlayableKindTalk || item.Title != "記事" {
		t.Fatalf("completed Talk=(%+v,%v), want Talk", item, err)
	}
	if !updated || len(gotHist.UsedArticleUrls) != 2 || gotHist.UsedArticleUrls[1] != "article" {
		t.Fatalf("Talk boundary history=%+v updated=%t, want one appended article", gotHist, updated)
	}
	p.Shutdown()
}

func TestTalkFailureConsumesSlotOnceAndFallsBackWithoutImmediateRetry(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: true, CycleBgmCount: 3}}
	p := New(cfg)
	preparePlayer(p, cfg, 3)
	talkGen := &failingTalkGenerator{err: errors.New("talk failed")}
	musicGen := &testMusicGenerator{result: musicgen.Result{AudioPath: "fallback.wav", Title: "fallback"}}
	item, hist, updated, err := p.NextItem(testAudioRegistrar{}, talkGen, musicGen, domain.NextItemRequest{}, domain.History{})
	if err != nil || item.Kind != domain.PlayableKindBGM || item.Title != "fallback" {
		t.Fatalf("fallback item=(%+v,%v), want BGM fallback", item, err)
	}
	if updated || len(hist.UsedArticleUrls) != 0 {
		t.Fatalf("Talk failure changed history: updated=%t history=%+v", updated, hist)
	}
	if got := talkGen.calls.Load(); got != 1 {
		t.Fatalf("initial Talk calls=%d, want one", got)
	}
	// The next boundary must consume the failed Talk slot as BGM again. It
	// must not start a second Talk job merely because the first fallback ended.
	item, _, _, err = p.NextItem(testAudioRegistrar{}, talkGen, musicGen, domain.NextItemRequest{}, domain.History{})
	if err != nil || item.Kind != domain.PlayableKindBGM {
		t.Fatalf("post-failure item=(%+v,%v), want BGM", item, err)
	}
	if got := talkGen.calls.Load(); got != 1 {
		t.Fatalf("post-failure Talk calls=%d, want no immediate retry", got)
	}
	p.Shutdown()
}

func TestTalkDisabledCycleOneWaitsForSameMusicDemandWhenBGMEmpty(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false, CycleBgmCount: 1}}
	p := New(cfg)
	preparePlayer(p, cfg, 0)
	gen := &barrierMusicGenerator{
		started: make(chan int, 3), cancelled: make(chan int, 1),
		release: []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})},
		results: []musicgen.Result{{AudioPath: "music.wav", Title: "music"}, {AudioPath: "refill.wav", Title: "refill"}, {AudioPath: "refill-2.wav", Title: "refill-2"}},
	}
	resultCh := make(chan struct {
		item domain.PlayableItem
		err  error
	}, 1)
	go func() {
		item, _, _, err := p.NextItem(testAudioRegistrar{}, nil, gen, domain.NextItemRequest{}, domain.History{})
		resultCh <- struct {
			item domain.PlayableItem
			err  error
		}{item: item, err: err}
	}()
	if got := <-gen.started; got != 1 {
		t.Fatalf("music worker=%d, want 1", got)
	}
	select {
	case got := <-resultCh:
		t.Fatalf("NextItem returned before same music demand completed: %+v", got)
	default:
	}
	close(gen.release[0])
	got := <-resultCh
	if got.err != nil || got.item.Kind != domain.PlayableKindBGM || got.item.Title != "music" {
		t.Fatalf("item=(%+v,%v), want music BGM", got.item, got.err)
	}
	if got := <-gen.started; got != 2 {
		t.Fatalf("refill worker=%d, want 2", got)
	}
	close(gen.release[1])
	if got := <-gen.started; got != 3 {
		t.Fatalf("second refill worker=%d, want 3", got)
	}
	close(gen.release[2])
	p.prefetchWG.Wait()
	if gen.calls.Load() != 3 {
		t.Fatalf("music Generate calls=%d, want initial demand plus bounded refills", gen.calls.Load())
	}
	p.Shutdown()
}

func TestMusicFailureStopsRefillUntilExplicitHint(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}}
	p := New(cfg)
	preparePlayer(p, cfg, 0)
	gen := &failingMusicGenerator{entered: make(chan struct{}), err: errors.New("provider failed")}
	firstEntered := gen.entered
	p.PrefetchMusic(gen, cfg)
	<-firstEntered
	p.prefetchWG.Wait()
	if got := gen.calls.Load(); got != 1 {
		t.Fatalf("failure refill calls=%d, want one without immediate retry", got)
	}
	if p.Status().MusicGenerating {
		t.Fatal("failed refill remained busy")
	}
	gen.entered = make(chan struct{})
	gen.once = sync.Once{}
	p.PrefetchMusic(gen, cfg)
	<-gen.entered
	p.prefetchWG.Wait()
	if got := gen.calls.Load(); got != 2 {
		t.Fatalf("explicit hint calls=%d, want one retry", got)
	}
	p.Shutdown()
}

func TestSaveConfigNormalizedSameGenrePreservesPlayerState(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: true, CycleBgmCount: 2}, StableAudio3: domain.StableAudio3Config{Genre: "chill lo-fi"}}
	p := New(cfg)
	preparePlayer(p, cfg, 1)
	res := talk.Result{AudioPath: "talk.wav", ArticleURL: "article"}
	p.mu.Lock()
	p.prefetchedTalk = &res
	p.musicReady = []musicgen.Result{{AudioPath: "music.wav", Genre: "chill lo-fi"}}
	p.bgmCountSinceLastTalk = 1
	gen := p.generation
	epoch := p.musicEpoch
	p.mu.Unlock()

	saved := cfg
	saved.StableAudio3.Genre = " CHILL LO-FI "
	p.UpdateConfigFromSave(saved)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.generation != gen || p.musicEpoch != epoch {
		t.Fatalf("same normalized genre reset epochs: generation=%d/%d music=%d/%d", p.generation, gen, p.musicEpoch, epoch)
	}
	if p.prefetchedTalk == nil || len(p.musicReady) != 1 || p.bgmCountSinceLastTalk != 1 {
		t.Fatalf("same normalized genre discarded state: talk=%v ready=%d count=%d", p.prefetchedTalk != nil, len(p.musicReady), p.bgmCountSinceLastTalk)
	}
}

func TestMusicPrefetchFillsTwoFIFOAndRefillsAfterConsume(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}}
	p := New(cfg)
	preparePlayer(p, cfg, 0)
	gen := &barrierMusicGenerator{
		started: make(chan int, 2), cancelled: make(chan int, 1),
		release: []chan struct{}{make(chan struct{}), make(chan struct{})},
		results: []musicgen.Result{{AudioPath: "one.wav", Title: "one"}, {AudioPath: "two.wav", Title: "two"}},
	}
	p.PrefetchMusic(gen, cfg)
	if got := <-gen.started; got != 1 {
		t.Fatalf("first worker=%d, want 1", got)
	}
	close(gen.release[0])
	if got := <-gen.started; got != 2 {
		t.Fatalf("refill worker=%d, want 2", got)
	}
	close(gen.release[1])
	p.prefetchWG.Wait()
	if status := p.Status(); !status.MusicReady || status.MusicGenerating {
		t.Fatalf("filled status=%+v, want two ready and idle", status)
	}
	// Keep this assertion focused on FIFO order; a later production hint would
	// provide the service again and exercise the consume-triggered refill.
	p.mu.Lock()
	p.musicSvc = nil
	p.mu.Unlock()
	first, _, _, err := p.NextItem(testAudioRegistrar{}, nil, nil, domain.NextItemRequest{}, domain.History{})
	if err != nil || first.Title != "one" {
		t.Fatalf("first item=(%+v,%v), want one", first, err)
	}
	second, _, _, err := p.NextItem(testAudioRegistrar{}, nil, nil, domain.NextItemRequest{}, domain.History{})
	if err != nil || second.Title != "two" {
		t.Fatalf("second item=(%+v,%v), want two", second, err)
	}
	p.Shutdown()
}

func TestMusicPrefetchDemandBurstKeepsTwoAndRefillsAfterEachConsume(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}, StableAudio3: domain.StableAudio3Config{CacheLimit: 1}}
	p := New(cfg)
	preparePlayer(p, cfg, 0)
	gen := &barrierMusicGenerator{
		started: make(chan int, 4), cancelled: make(chan int, 1),
		release: []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})},
		results: []musicgen.Result{{AudioPath: "one.wav", Title: "one"}, {AudioPath: "two.wav", Title: "two"}, {AudioPath: "three.wav", Title: "three"}, {AudioPath: "four.wav", Title: "four"}},
	}
	p.PrefetchMusic(gen, cfg)
	if got := <-gen.started; got != 1 {
		t.Fatalf("first worker=%d, want 1", got)
	}
	close(gen.release[0])
	if got := <-gen.started; got != 2 {
		t.Fatalf("second worker=%d, want 2", got)
	}
	close(gen.release[1])
	// Repeated concurrent hints must observe the same two-slot state.
	var hints sync.WaitGroup
	for i := 0; i < 32; i++ {
		hints.Add(1)
		go func() {
			defer hints.Done()
			p.PrefetchMusic(gen, cfg)
		}()
	}
	hints.Wait()
	if got := gen.calls.Load(); got != 2 {
		t.Fatalf("burst calls=%d, want exactly 2", got)
	}
	if got := readyAndPendingMusic(p); got > 2 {
		t.Fatalf("ready+pending=%d, want <=2", got)
	}

	first, _, _, err := p.NextItem(testAudioRegistrar{}, nil, gen, domain.NextItemRequest{}, domain.History{})
	if err != nil || first.Title != "one" {
		t.Fatalf("first item=(%+v,%v), want one", first, err)
	}
	if got := <-gen.started; got != 3 {
		t.Fatalf("post-consume worker=%d, want 3", got)
	}
	if got := readyAndPendingMusic(p); got > 2 {
		t.Fatalf("after first consume ready+pending=%d, want <=2", got)
	}
	second, _, _, err := p.NextItem(testAudioRegistrar{}, nil, gen, domain.NextItemRequest{}, domain.History{})
	if err != nil || second.Title != "two" {
		t.Fatalf("second item=(%+v,%v), want two", second, err)
	}
	if got := gen.calls.Load(); got != 3 {
		t.Fatalf("second consume calls=%d, want no duplicate while worker pending", got)
	}
	if got := readyAndPendingMusic(p); got > 1 {
		t.Fatalf("after second consume ready+pending=%d, want <=1", got)
	}
	close(gen.release[2])
	if got := <-gen.started; got != 4 {
		t.Fatalf("third refill worker=%d, want 4", got)
	}
	close(gen.release[3])
	p.prefetchWG.Wait()
	if got := readyAndPendingMusic(p); got != 2 {
		t.Fatalf("after refills ready+pending=%d, want 2", got)
	}
	p.Shutdown()
}

func readyAndPendingMusic(p *Player) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.musicReady) + p.currentMusicJobsLocked()
}

func TestMusicGenreChangeRejectsLateOldEpochResult(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}, StableAudio3: domain.StableAudio3Config{Genre: "chill lo-fi"}}
	p := New(cfg)
	preparePlayer(p, cfg, 0)
	gen := &barrierMusicGenerator{
		started: make(chan int, 3), cancelled: make(chan int, 1),
		release: []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})},
		results: []musicgen.Result{{AudioPath: "old.wav", Title: "old", Genre: "chill lo-fi"}, {AudioPath: "new.wav", Title: "new", Genre: "smooth jazz"}, {AudioPath: "newer.wav", Title: "newer", Genre: "smooth jazz"}},
	}
	p.PrefetchMusic(gen, cfg)
	if got := <-gen.started; got != 1 {
		t.Fatalf("old worker=%d, want 1", got)
	}
	p.UpdateStableAudio3Genre("smooth jazz")
	if got := <-gen.cancelled; got != 1 {
		t.Fatalf("old worker cancellation=%d, want 1", got)
	}
	close(gen.release[0])
	if got := <-gen.started; got != 2 {
		t.Fatalf("new worker=%d, want 2", got)
	}
	close(gen.release[1])
	if got := <-gen.started; got != 3 {
		t.Fatalf("refill worker=%d, want 3", got)
	}
	close(gen.release[2])
	p.prefetchWG.Wait()
	p.mu.Lock()
	p.musicSvc = nil
	p.mu.Unlock()
	item, _, _, err := p.NextItem(testAudioRegistrar{}, nil, nil, domain.NextItemRequest{}, domain.History{})
	if err != nil || item.Title != "new" || item.Source.Genre != "smooth jazz" {
		t.Fatalf("item=(%+v,%v), want new smooth-jazz item", item, err)
	}
	p.Shutdown()
}

func TestPrefetchNextStartsMusicBeforeTalk(t *testing.T) {
	cfg := testPlayerConfig()
	p := New(cfg)
	preparePlayer(p, cfg, 1)
	order := make(chan string, 2)
	musicGen := &orderedMusicGenerator{order: order, result: musicgen.Result{AudioPath: "music.wav"}}
	talkGen := &orderedTalkGenerator{order: order, result: talk.Result{AudioPath: "talk.wav"}}
	p.PrefetchNext(talkGen, musicGen, cfg, domain.History{})
	if got := <-order; got != "music" {
		t.Fatalf("first PrefetchNext entry=%q, want music", got)
	}
	if got := <-order; got != "talk" {
		t.Fatalf("second PrefetchNext entry=%q, want talk", got)
	}
	if musicGen.calls.Load() != 1 || talkGen.calls.Load() != 1 {
		t.Fatalf("calls music=%d talk=%d, want one each", musicGen.calls.Load(), talkGen.calls.Load())
	}
	p.Shutdown()
}

type orderedTalkGenerator struct {
	calls  atomic.Int32
	order  chan<- string
	result talk.Result
}

func (f *orderedTalkGenerator) Generate(context.Context, domain.AppConfig, map[string]bool) (talk.Result, error) {
	f.calls.Add(1)
	f.order <- "talk"
	return f.result, nil
}

type orderedMusicGenerator struct {
	calls  atomic.Int32
	order  chan<- string
	result musicgen.Result
}

func (f *orderedMusicGenerator) Generate(context.Context, domain.AppConfig) (musicgen.Result, error) {
	f.calls.Add(1)
	f.order <- "music"
	return f.result, nil
}

func (f *orderedMusicGenerator) Fallback(domain.AppConfig) (musicgen.Result, error) {
	return f.result, nil
}

type genreMusicGenerator struct {
	calls   atomic.Int32
	started chan string
}

func (f *genreMusicGenerator) Generate(_ context.Context, cfg domain.AppConfig) (musicgen.Result, error) {
	f.calls.Add(1)
	genre := cfg.StableAudio3.Genre
	if f.started != nil {
		select {
		case f.started <- genre:
		default:
		}
	}
	return musicgen.Result{AudioPath: genre + ".wav", Title: genre, Genre: genre}, nil
}

func (f *genreMusicGenerator) Fallback(cfg domain.AppConfig) (musicgen.Result, error) {
	return musicgen.Result{AudioPath: cfg.StableAudio3.Genre + ".wav", Title: cfg.StableAudio3.Genre, Genre: cfg.StableAudio3.Genre}, nil
}

type blockingAudioRegistrar struct {
	entered    chan struct{}
	release    chan struct{}
	releaseURL atomic.Int32
	calls      atomic.Int32
}

func (r *blockingAudioRegistrar) RegisterFile(path string, _ time.Duration) (string, error) {
	if r.calls.Add(1) == 1 {
		close(r.entered)
		<-r.release
	}
	return "audio://registered/" + path, nil
}

func (r *blockingAudioRegistrar) LoudnessURLForAudioURL(url string) string { return url + "/loudness" }

func (r *blockingAudioRegistrar) ReleaseAudioURL(string) bool {
	r.releaseURL.Add(1)
	return true
}

func TestMusicGenreChangeAtRegisterFileBoundaryRechoosesLatestGenre(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}, StableAudio3: domain.StableAudio3Config{Genre: "chill lo-fi"}}
	p := New(cfg)
	preparePlayer(p, cfg, 0)
	p.mu.Lock()
	p.musicReady = []musicgen.Result{{AudioPath: "old-a.wav", Title: "old-a", Genre: "chill lo-fi"}}
	p.mu.Unlock()
	gen := &genreMusicGenerator{started: make(chan string, 1)}
	audio := &blockingAudioRegistrar{entered: make(chan struct{}), release: make(chan struct{})}
	resultCh := make(chan struct {
		item domain.PlayableItem
		err  error
	}, 1)
	go func() {
		item, _, _, err := p.NextItem(audio, nil, gen, domain.NextItemRequest{}, domain.History{})
		resultCh <- struct {
			item domain.PlayableItem
			err  error
		}{item: item, err: err}
	}()
	<-audio.entered
	p.UpdateStableAudio3Genre("smooth jazz")
	close(audio.release)
	got := <-resultCh
	if got.err != nil || got.item.Source.Genre != "smooth jazz" || got.item.Title != "smooth jazz" {
		t.Fatalf("boundary item=(%+v,%v), want current B", got.item, got.err)
	}
	if audio.releaseURL.Load() != 1 {
		t.Fatalf("stale registered URL releases=%d, want 1", audio.releaseURL.Load())
	}
	if gen.calls.Load() < 1 {
		t.Fatalf("new genre Generate calls=%d, want at least one", gen.calls.Load())
	}
	p.Shutdown()
}

func TestMusicGenreChangeAfterFirstEpochCheckReleasesAndRechooses(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}, StableAudio3: domain.StableAudio3Config{Genre: "chill lo-fi"}}
	p := New(cfg)
	preparePlayer(p, cfg, 0)
	p.mu.Lock()
	p.musicReady = []musicgen.Result{{AudioPath: "old-a.wav", Title: "old-a", Genre: "chill lo-fi"}}
	p.mu.Unlock()
	gen := &genreMusicGenerator{}
	audio := &blockingAudioRegistrar{entered: make(chan struct{}), release: make(chan struct{})}
	close(audio.release)
	firstCheckPassed := make(chan struct{})
	resume := make(chan struct{})
	var hookOnce sync.Once
	p.beforeBGMFinalPublish = func() {
		hookOnce.Do(func() {
			close(firstCheckPassed)
			<-resume
		})
	}
	resultCh := make(chan struct {
		item domain.PlayableItem
		err  error
	}, 1)
	go func() {
		item, _, _, err := p.NextItem(audio, nil, gen, domain.NextItemRequest{}, domain.History{})
		resultCh <- struct {
			item domain.PlayableItem
			err  error
		}{item: item, err: err}
	}()
	<-firstCheckPassed
	p.UpdateStableAudio3Genre("smooth jazz")
	close(resume)
	got := <-resultCh
	if got.err != nil || got.item.Source.Genre != "smooth jazz" || got.item.Title != "smooth jazz" {
		t.Fatalf("final-gate item=(%+v,%v), want current smooth-jazz item", got.item, got.err)
	}
	if audio.releaseURL.Load() != 1 {
		t.Fatalf("stale registered URL releases=%d, want 1", audio.releaseURL.Load())
	}
	p.Shutdown()
}

func TestMusicGenreMultiEpochIdentityRejectsLateResults(t *testing.T) {
	cases := []struct {
		name   string
		wantID string
	}{
		{name: "A-B-C", wantID: "C-latest"},
		{name: "A-B-A", wantID: "A-latest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const genreA = "chill lo-fi"
			const genreB = "smooth jazz"
			const genreC = "ambient music"
			finalGenre := genreC
			if tc.name == "A-B-A" {
				finalGenre = genreA
			}
			cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}, StableAudio3: domain.StableAudio3Config{Genre: genreA}}
			p := New(cfg)
			preparePlayer(p, cfg, 0)
			gen := &reservationBarrierMusicGenerator{
				started: make(chan reservationMusicStart, 3), cancelled: make(chan int, 3),
				release: []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})},
				results: []musicgen.Result{
					{AudioPath: "A-old.wav", Title: "A-old", Genre: genreA},
					{AudioPath: "B-old.wav", Title: "B-old", Genre: genreB},
					{AudioPath: tc.wantID + ".wav", Title: tc.wantID, Genre: finalGenre},
				},
			}
			p.PrefetchMusic(gen, cfg)
			if got := <-gen.started; got.number != 1 || got.genre != genreA {
				t.Fatalf("first worker=%+v, want %s", got, genreA)
			}

			p.UpdateStableAudio3Genre(genreB)
			if got := <-gen.cancelled; got != 1 {
				t.Fatalf("cancelled A worker=%d, want 1", got)
			}
			close(gen.release[0])
			if got := <-gen.started; got.number != 2 || got.genre != genreB {
				t.Fatalf("second worker=%+v, want %s", got, genreB)
			}

			p.UpdateStableAudio3Genre(finalGenre)
			if got := <-gen.cancelled; got != 2 {
				t.Fatalf("cancelled B worker=%d, want 2", got)
			}
			close(gen.release[1])
			if got := <-gen.started; got.number != 3 || got.genre != finalGenre {
				t.Fatalf("latest worker=%+v, want %s", got, finalGenre)
			}
			// Prevent the successful latest worker's completion from starting a
			// second refill; this leaves exactly the identity under test ready.
			p.mu.Lock()
			p.musicSvc = nil
			p.mu.Unlock()
			close(gen.release[2])
			p.prefetchWG.Wait()

			status := p.Status()
			if status.MusicGenerating || !status.MusicReady {
				t.Fatalf("latest status=%+v, want idle with one ready item", status)
			}
			p.mu.Lock()
			if len(p.musicReady) != 1 || p.musicReady[0].Title != tc.wantID {
				got := append([]musicgen.Result(nil), p.musicReady...)
				p.mu.Unlock()
				t.Fatalf("ready results=%+v, want only %s", got, tc.wantID)
			}
			p.mu.Unlock()

			item, _, _, err := p.NextItem(testAudioRegistrar{}, nil, nil, domain.NextItemRequest{}, domain.History{})
			if err != nil || item.Title != tc.wantID || item.Source.Genre != finalGenre {
				t.Fatalf("latest item=(%+v,%v), want %s/%s", item, err, tc.wantID, finalGenre)
			}
			p.Shutdown()
		})
	}
}

func TestRunningTalkCancellationJoinsAndProtectsReplacement(t *testing.T) {
	for _, action := range []string{"Skip", "UpdateConfig"} {
		t.Run(action, func(t *testing.T) {
			cfg := testPlayerConfig()
			p := New(cfg)
			preparePlayer(p, cfg, 1)
			gen := &barrierTalkGenerator{
				started: make(chan int, 2), cancelled: make(chan int, 1),
				release: []chan struct{}{make(chan struct{}), make(chan struct{})},
				results: []talk.Result{{AudioPath: "old-talk.wav", ArticleURL: "old-url", ArticleTitle: "old-talk"}, {AudioPath: "new-talk.wav", ArticleURL: "new-url", ArticleTitle: "new-talk"}},
			}
			p.PrefetchTalk(gen, cfg, domain.History{})
			if got := <-gen.started; got != 1 {
				t.Fatalf("initial worker=%d, want 1", got)
			}

			if action == "Skip" {
				done := make(chan struct{})
				go func() {
					_, _, _, _ = p.Skip(nil, nil, nil, domain.SkipRequest{CurrentKind: domain.PlayableKindSilence}, domain.History{})
					close(done)
				}()
				if got := <-gen.cancelled; got != 1 {
					t.Fatalf("cancelled worker=%d, want 1", got)
				}
				<-done
			} else {
				p.UpdateConfig(cfg)
				// UpdateConfig resets the cycle counter as part of its normal
				// behavior; restore a talk boundary so the replacement result can
				// be consumed through NextItem below.
				preparePlayer(p, cfg, 1)
				if got := <-gen.cancelled; got != 1 {
					t.Fatalf("cancelled worker=%d, want 1", got)
				}
			}
			if !p.Status().TalkPrefetching {
				t.Fatal("Talk busy cleared before cancelled worker joined")
			}

			if action == "Skip" {
				// Skip keeps the in-flight reservation owner until its worker
				// joins. Verify that join clears busy before the next owner starts.
				close(gen.release[0])
				p.prefetchWG.Wait()
				if status := p.Status(); status.TalkPrefetching || status.TalkReady {
					t.Fatalf("after Skip join status=%+v, want no busy/ready", status)
				}
			}
			p.PrefetchTalk(gen, cfg, domain.History{})
			p.mu.Lock()
			replacement := p.talkJob
			p.mu.Unlock()
			if replacement == nil {
				t.Fatal("replacement Talk worker was not registered")
			}
			if action != "Skip" {
				close(gen.release[0])
			}
			if got := <-gen.started; got != 2 {
				t.Fatalf("replacement worker=%d, want 2", got)
			}
			close(gen.release[1])
			p.prefetchWG.Wait()
			status := p.Status()
			if status.TalkPrefetching || !status.TalkReady {
				t.Fatalf("after join status=%+v, want busy=false ready=true", status)
			}

			item, history, _, err := p.NextItem(testAudioRegistrar{}, gen, nil, domain.NextItemRequest{}, domain.History{})
			if err != nil {
				t.Fatalf("NextItem returned error: %v", err)
			}
			if item.Title != "new-talk" || len(history.UsedArticleUrls) != 1 || history.UsedArticleUrls[0] != "new-url" {
				t.Fatalf("stale Talk was exposed: item=%+v history=%+v", item, history)
			}
			p.Shutdown()
		})
	}
}

func TestRunningMusicCancellationJoinsAndProtectsReplacement(t *testing.T) {
	for _, action := range []string{"Skip", "UpdateConfig"} {
		t.Run(action, func(t *testing.T) {
			cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}}
			p := New(cfg)
			preparePlayer(p, cfg, 0)
			gen := &barrierMusicGenerator{
				started: make(chan int, 2), cancelled: make(chan int, 1),
				release: []chan struct{}{make(chan struct{}), make(chan struct{})},
				results: []musicgen.Result{{AudioPath: "old-music.wav", Title: "old-music"}, {AudioPath: "new-music.wav", Title: "new-music"}},
			}
			p.PrefetchMusic(gen, cfg)
			if got := <-gen.started; got != 1 {
				t.Fatalf("initial worker=%d, want 1", got)
			}

			if action == "Skip" {
				done := make(chan struct{})
				go func() {
					_, _, _, _ = p.Skip(nil, nil, nil, domain.SkipRequest{CurrentKind: domain.PlayableKindSilence}, domain.History{})
					close(done)
				}()
				if got := <-gen.cancelled; got != 1 {
					t.Fatalf("cancelled worker=%d, want 1", got)
				}
				<-done
			} else {
				p.UpdateConfig(cfg)
				if got := <-gen.cancelled; got != 1 {
					t.Fatalf("cancelled worker=%d, want 1", got)
				}
			}
			if !p.Status().MusicGenerating {
				t.Fatal("Music busy cleared before cancelled worker joined")
			}

			if action == "Skip" {
				close(gen.release[0])
				p.prefetchWG.Wait()
				if status := p.Status(); status.MusicGenerating || status.MusicReady {
					t.Fatalf("after Skip join status=%+v, want no busy/ready", status)
				}
			}
			p.PrefetchMusic(gen, cfg)
			p.mu.Lock()
			replacement := p.musicJob
			p.mu.Unlock()
			if replacement == nil {
				t.Fatal("replacement Music worker was not registered")
			}
			if action != "Skip" {
				close(gen.release[0])
			}
			if got := <-gen.started; got != 2 {
				t.Fatalf("replacement worker=%d, want 2", got)
			}
			close(gen.release[1])
			p.prefetchWG.Wait()
			status := p.Status()
			if status.MusicGenerating || !status.MusicReady {
				t.Fatalf("after join status=%+v, want busy=false ready=true", status)
			}

			item, history, _, err := p.NextItem(testAudioRegistrar{}, nil, nil, domain.NextItemRequest{}, domain.History{})
			if err != nil {
				t.Fatalf("NextItem returned error: %v", err)
			}
			if item.Title != "new-music" || len(history.UsedArticleUrls) != 0 {
				t.Fatalf("stale Music was exposed: item=%+v history=%+v", item, history)
			}
			p.Shutdown()
		})
	}
}

func TestRunningTalkAndMusicShutdownCancelJoinAndRejectOldResults(t *testing.T) {
	t.Run("Talk", func(t *testing.T) {
		cfg := testPlayerConfig()
		p := New(cfg)
		preparePlayer(p, cfg, 1)
		gen := &barrierTalkGenerator{started: make(chan int, 1), cancelled: make(chan int, 1), release: []chan struct{}{make(chan struct{})}, results: []talk.Result{{AudioPath: "old-talk.wav", ArticleURL: "old-url", ArticleTitle: "old-talk"}}}
		p.PrefetchTalk(gen, cfg, domain.History{})
		<-gen.started
		done := make(chan struct{})
		go func() { p.Shutdown(); close(done) }()
		<-gen.cancelled
		if !p.Status().TalkPrefetching {
			t.Fatal("Talk busy cleared before Shutdown worker joined")
		}
		close(gen.release[0])
		<-done
		status := p.Status()
		if status.TalkPrefetching || status.TalkReady {
			t.Fatalf("after Shutdown status=%+v, want no busy/ready", status)
		}
	})

	t.Run("Music", func(t *testing.T) {
		cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}}
		p := New(cfg)
		preparePlayer(p, cfg, 0)
		gen := &barrierMusicGenerator{started: make(chan int, 1), cancelled: make(chan int, 1), release: []chan struct{}{make(chan struct{})}, results: []musicgen.Result{{AudioPath: "old-music.wav", Title: "old-music"}}}
		p.PrefetchMusic(gen, cfg)
		<-gen.started
		done := make(chan struct{})
		go func() { p.Shutdown(); close(done) }()
		<-gen.cancelled
		if !p.Status().MusicGenerating {
			t.Fatal("Music busy cleared before Shutdown worker joined")
		}
		close(gen.release[0])
		<-done
		status := p.Status()
		if status.MusicGenerating || status.MusicReady {
			t.Fatalf("after Shutdown status=%+v, want no busy/ready", status)
		}
	})
}

func TestShutdownWaitsForPrefetchAndClearsBusy(t *testing.T) {
	cfg := domain.AppConfig{Talk: domain.TalkConfig{Enabled: false}}
	p := New(cfg)
	preparePlayer(p, cfg, 0)
	musicGen := &testMusicGenerator{entered: make(chan struct{}), release: make(chan struct{})}
	p.PrefetchMusic(musicGen, cfg)
	<-musicGen.entered
	p.Shutdown()
	status := p.Status()
	if status.MusicGenerating || status.TalkPrefetching {
		t.Fatalf("busy remains after Shutdown: %+v", status)
	}
}

func TestTalkPrefetchOldWorkerCannotClearReplacement(t *testing.T) {
	p := New(domain.AppConfig{})
	oldCancel := func() {}
	newCancel := func() {}
	p.mu.Lock()
	oldGeneration := p.generation
	p.nextPrefetchOwner++
	oldOwner := p.nextPrefetchOwner
	p.talkPrefetchOwner = oldOwner
	p.prefetching = true
	p.talkInFlight = 1
	p.cancelPrefetch = oldCancel
	p.mu.Unlock()

	p.UpdateConfig(domain.AppConfig{})
	p.mu.Lock()
	p.nextPrefetchOwner++
	newOwner := p.nextPrefetchOwner
	p.talkPrefetchOwner = newOwner
	p.prefetching = true
	p.talkInFlight = 2
	p.cancelPrefetch = newCancel
	p.mu.Unlock()
	// This is the old worker's actual completion path. It must only decrement
	// its accounting; it must not release the replacement reservation/handle.
	p.finishTalkPrefetch(oldGeneration, oldOwner)
	p.mu.Lock()
	if !p.prefetching || p.cancelPrefetch == nil || p.talkPrefetchOwner != newOwner {
		t.Fatalf("old talk worker cleared replacement: active=%t cancel=%v owner=%d", p.prefetching, p.cancelPrefetch != nil, p.talkPrefetchOwner)
	}
	if p.talkInFlight != 1 {
		t.Fatalf("talk in-flight count=%d, want 1", p.talkInFlight)
	}
	p.mu.Unlock()
}

func TestTalkPrefetchOldWorkerCannotClearReplacementAfterSkip(t *testing.T) {
	p := New(domain.AppConfig{})
	p.mu.Lock()
	oldGeneration := p.generation
	p.nextPrefetchOwner++
	oldOwner := p.nextPrefetchOwner
	p.talkPrefetchOwner, p.prefetching, p.talkInFlight = oldOwner, true, 1
	p.cancelPrefetch = func() {}
	p.mu.Unlock()
	_, _, _, _ = p.Skip(nil, nil, nil, domain.SkipRequest{CurrentKind: domain.PlayableKindSilence}, domain.History{})
	p.mu.Lock()
	p.nextPrefetchOwner++
	newOwner := p.nextPrefetchOwner
	p.talkPrefetchOwner, p.prefetching, p.talkInFlight = newOwner, true, 2
	p.cancelPrefetch = func() {}
	p.mu.Unlock()
	p.finishTalkPrefetch(oldGeneration, oldOwner)
	p.mu.Lock()
	if !p.prefetching || p.talkPrefetchOwner != newOwner || p.talkInFlight != 1 {
		t.Fatalf("skip replacement reservation changed by old worker: active=%t owner=%d inflight=%d", p.prefetching, p.talkPrefetchOwner, p.talkInFlight)
	}
	p.mu.Unlock()
}

func TestTalkPrefetchCleanupRequiresGenerationAndOwner(t *testing.T) {
	p := New(domain.AppConfig{})
	p.mu.Lock()
	workerGeneration := p.generation
	p.nextPrefetchOwner++
	owner := p.nextPrefetchOwner
	p.talkPrefetchOwner, p.prefetching, p.talkInFlight = owner, true, 1
	p.cancelPrefetch = func() {}
	p.mu.Unlock()

	// A reservation can retain its owner handle while a newer generation is
	// being installed.  Completion from that old generation must not release
	// the current reservation, even if the owner token happens to match.
	p.mu.Lock()
	p.generation++
	p.talkInFlight = 2
	p.mu.Unlock()
	p.finishTalkPrefetch(workerGeneration, owner)

	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.prefetching || p.cancelPrefetch == nil || p.talkPrefetchOwner != owner {
		t.Fatalf("generation-mismatched cleanup cleared reservation: active=%t cancel=%v owner=%d", p.prefetching, p.cancelPrefetch != nil, p.talkPrefetchOwner)
	}
	if p.talkInFlight != 1 {
		t.Fatalf("talk in-flight count=%d, want 1 replacement worker", p.talkInFlight)
	}
}

func TestMusicPrefetchOldWorkerCannotClearReplacement(t *testing.T) {
	p := New(domain.AppConfig{})
	oldCancel := func() {}
	newCancel := func() {}
	p.mu.Lock()
	oldGeneration := p.generation
	p.nextPrefetchOwner++
	oldOwner := p.nextPrefetchOwner
	p.musicPrefetchOwner = oldOwner
	p.musicPrefetching = true
	p.musicInFlight = 1
	p.cancelMusicPrefetch = oldCancel
	p.mu.Unlock()

	p.UpdateConfig(domain.AppConfig{})
	p.mu.Lock()
	p.nextPrefetchOwner++
	newOwner := p.nextPrefetchOwner
	p.musicPrefetchOwner = newOwner
	p.musicPrefetching = true
	p.musicInFlight = 2
	p.cancelMusicPrefetch = newCancel
	p.mu.Unlock()
	p.finishMusicPrefetch(oldGeneration, oldOwner)
	p.mu.Lock()
	if !p.musicPrefetching || p.cancelMusicPrefetch == nil || p.musicPrefetchOwner != newOwner {
		t.Fatalf("old music worker cleared replacement: active=%t cancel=%v owner=%d", p.musicPrefetching, p.cancelMusicPrefetch != nil, p.musicPrefetchOwner)
	}
	if p.musicInFlight != 1 {
		t.Fatalf("music in-flight count=%d, want 1", p.musicInFlight)
	}
	p.mu.Unlock()
}

func TestMusicPrefetchOldWorkerCannotClearReplacementAfterSkip(t *testing.T) {
	p := New(domain.AppConfig{})
	p.mu.Lock()
	oldGeneration := p.generation
	p.nextPrefetchOwner++
	oldOwner := p.nextPrefetchOwner
	p.musicPrefetchOwner, p.musicPrefetching, p.musicInFlight = oldOwner, true, 1
	p.cancelMusicPrefetch = func() {}
	p.mu.Unlock()
	_, _, _, _ = p.Skip(nil, nil, nil, domain.SkipRequest{CurrentKind: domain.PlayableKindSilence}, domain.History{})
	p.mu.Lock()
	p.nextPrefetchOwner++
	newOwner := p.nextPrefetchOwner
	p.musicPrefetchOwner, p.musicPrefetching, p.musicInFlight = newOwner, true, 2
	p.cancelMusicPrefetch = func() {}
	p.mu.Unlock()
	p.finishMusicPrefetch(oldGeneration, oldOwner)
	p.mu.Lock()
	if !p.musicPrefetching || p.musicPrefetchOwner != newOwner || p.musicInFlight != 1 {
		t.Fatalf("skip replacement reservation changed by old worker: active=%t owner=%d inflight=%d", p.musicPrefetching, p.musicPrefetchOwner, p.musicInFlight)
	}
	p.mu.Unlock()
}

func TestPrefetchPublishGateRejectsInvalidatedGeneration(t *testing.T) {
	p := New(domain.AppConfig{})
	ctx := context.Background()

	p.mu.Lock()
	generation := p.generation
	p.mu.Unlock()

	// UpdateConfig represents a config-generation replacement. A worker that
	// was already running must not be able to repopulate ready after it lands.
	p.UpdateConfig(domain.AppConfig{})
	p.mu.Lock()
	ok := p.publishAllowedLocked(generation, ctx)
	if ok {
		result := talk.Result{ArticleTitle: "stale"}
		p.prefetchedTalk = &result
	}
	p.mu.Unlock()
	if ok {
		t.Fatal("stale generation was allowed to publish")
	}
	if got := p.Status(); got.TalkReady {
		t.Fatal("stale talk result became ready")
	}
}

func TestPrefetchPublishGateRejectsCancellationAndShutdown(t *testing.T) {
	p := New(domain.AppConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	generation := p.generation
	p.mu.Unlock()
	cancel()
	p.mu.Lock()
	if p.publishAllowedLocked(generation, ctx) {
		p.mu.Unlock()
		t.Fatal("cancelled worker was allowed to publish")
	}
	p.mu.Unlock()

	// Shutdown advances the same generation token and waits for owned work;
	// its post-close publication gate must remain closed deterministically.
	p.Shutdown()
	p.mu.Lock()
	ok := p.publishAllowedLocked(generation, context.Background())
	p.mu.Unlock()
	if ok {
		t.Fatal("shutdown worker was allowed to publish")
	}
}
