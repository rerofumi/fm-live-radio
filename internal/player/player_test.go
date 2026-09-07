package player

import (
	"context"
	"testing"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/talk"
)

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
