package musicgen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	sa3 "fm-live-radio/internal/musicgen/stableaudio/pipeline"
)

type testMusicRuntime struct{}

func (testMusicRuntime) Synthesize(_ func(step, totalSteps int)) error { return nil }
func (testMusicRuntime) Close()                                        {}

// musicRuntimeActivity records the complete runtime lifetime.  In
// particular, Close is part of the active interval; a later Load must not be
// allowed to overlap a slow Close.
type musicRuntimeActivity struct {
	active atomic.Int32
	max    atomic.Int32
}

func (a *musicRuntimeActivity) loaded() {
	n := a.active.Add(1)
	for {
		old := a.max.Load()
		if n <= old || a.max.CompareAndSwap(old, n) {
			return
		}
	}
}

func (a *musicRuntimeActivity) closed() { a.active.Add(-1) }

type gatedMusicRuntime struct {
	opt       sa3.Options
	activity  *musicRuntimeActivity
	closeGate <-chan struct{}
	closeDone chan<- struct{}
	runErr    error
}

func (r *gatedMusicRuntime) Synthesize(_ func(step, totalSteps int)) error {
	if r.runErr == nil {
		if err := os.WriteFile(r.opt.OutputWAV, []byte("fake wav"), 0o644); err != nil {
			return err
		}
	}
	return r.runErr
}

func (r *gatedMusicRuntime) Close() {
	if r.closeDone != nil {
		close(r.closeDone)
	}
	if r.closeGate != nil {
		<-r.closeGate
	}
	r.activity.closed()
}

func waitMusicResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service completion (possible leaked reservation/lease)")
		return nil
	}
}

func musicTestConfig(outDir string) domain.AppConfig {
	return domain.AppConfig{
		LocalInference: domain.LocalInferenceConfig{ExecutionProvider: "cuda", DeviceID: 1, MaxWorkers: 8},
		StableAudio3:   domain.StableAudio3Config{ModelDir: "fake", OutputDir: outDir},
	}
}

func TestServiceInstancesShareLeaseUntilSlowCloseCompletes(t *testing.T) {
	a := generation.NewArbiter()
	oldPrepare, oldLoad := prepareMusic, loadMusicRuntime
	defer func() { prepareMusic, loadMusicRuntime = oldPrepare, oldLoad }()
	outDir := t.TempDir()
	prepareMusic = func(cfg domain.AppConfig) error {
		if cfg.LocalInference.MaxWorkers <= 1 || cfg.LocalInference.ExecutionProvider != "cuda" {
			t.Fatalf("test config did not exercise provider/maxWorkers input: %+v", cfg.LocalInference)
		}
		return nil
	}
	activity := &musicRuntimeActivity{}
	firstCloseStarted := make(chan struct{})
	secondLoad := make(chan struct{})
	closeGate := make(chan struct{})
	var loads atomic.Int32
	loadMusicRuntime = func(opt sa3.Options) (musicRuntime, error) {
		n := loads.Add(1)
		activity.loaded()
		if n == 1 {
			return &gatedMusicRuntime{opt: opt, activity: activity, closeGate: closeGate, closeDone: firstCloseStarted}, nil
		}
		close(secondLoad)
		return &gatedMusicRuntime{opt: opt, activity: activity}, nil
	}

	cfg := musicTestConfig(outDir)
	firstDone := make(chan error, 1)
	go func() { _, err := NewWithArbiter(a).Generate(context.Background(), cfg); firstDone <- err }()
	<-firstCloseStarted
	secondDone := make(chan error, 1)
	go func() { _, err := NewWithArbiter(a).Generate(context.Background(), cfg); secondDone <- err }()

	// The first Close is held open. Releasing the gate establishes the only
	// transition at which the arbiter may hand the slot to the second service.
	close(closeGate)
	if err := waitMusicResult(t, firstDone); err != nil {
		t.Fatalf("first Generate: %v", err)
	}
	if err := waitMusicResult(t, secondDone); err != nil {
		t.Fatalf("second Generate: %v", err)
	}
	if got := activity.max.Load(); got != 1 {
		t.Fatalf("runtime active maximum=%d, want 1", got)
	}
	if got := loads.Load(); got != 2 {
		t.Fatalf("runtime loads=%d, want 2", got)
	}
	select {
	case <-secondLoad:
	default:
		t.Fatal("second service did not reach runtime load")
	}
}

func TestServiceRequestGetsMusicPriorityOverQueuedTalk(t *testing.T) {
	a := generation.NewArbiter()
	holder, err := a.Acquire(context.Background(), generation.KindTalk)
	if err != nil {
		t.Fatal(err)
	}
	oldPrepare, oldLoad := prepareMusic, loadMusicRuntime
	defer func() { prepareMusic, loadMusicRuntime = oldPrepare, oldLoad }()
	outDir := t.TempDir()
	prepareStarted := make(chan struct{})
	prepareContinue := make(chan struct{})
	prepareMusic = func(domain.AppConfig) error {
		close(prepareStarted)
		<-prepareContinue
		return nil
	}
	activity := &musicRuntimeActivity{}
	order := make(chan string, 3)
	loadMusicRuntime = func(opt sa3.Options) (musicRuntime, error) {
		activity.loaded()
		order <- "music-service"
		return &gatedMusicRuntime{opt: opt, activity: activity}, nil
	}

	// Register same-class waiters before the service request. The holder keeps
	// all three queued until the service has completed its preflight.
	talk1, err := a.Reserve(context.Background(), generation.KindTalk)
	if err != nil {
		t.Fatal(err)
	}
	talk2, err := a.Reserve(context.Background(), generation.KindTalk)
	if err != nil {
		t.Fatal(err)
	}
	acquireTalk := func(r *generation.Reservation, name string) chan error {
		done := make(chan error, 1)
		go func() {
			lease, err := r.Acquire(context.Background())
			if err == nil {
				order <- name
				lease.Release()
			}
			done <- err
		}()
		return done
	}
	talk1Done := acquireTalk(talk1, "talk-1")
	talk2Done := acquireTalk(talk2, "talk-2")
	serviceDone := make(chan error, 1)
	go func() {
		_, err := NewWithArbiter(a).Generate(context.Background(), musicTestConfig(outDir))
		serviceDone <- err
	}()
	<-prepareStarted
	close(prepareContinue)
	holder.Release()

	got := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case event := <-order:
			got = append(got, event)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for queued service requests")
		}
	}
	if got[0] != "music-service" {
		t.Fatalf("service start order=%v, want music-service first", got)
	}
	if err := waitMusicResult(t, talk1Done); err != nil {
		t.Fatalf("talk-1 acquire: %v", err)
	}
	if err := waitMusicResult(t, talk2Done); err != nil {
		t.Fatalf("talk-2 acquire: %v", err)
	}
	if err := waitMusicResult(t, serviceDone); err != nil {
		t.Fatalf("music service Generate: %v", err)
	}
}

func TestServiceRunFailureAndPreflightFailureReleaseReservations(t *testing.T) {
	a := generation.NewArbiter()
	oldPrepare, oldLoad := prepareMusic, loadMusicRuntime
	defer func() { prepareMusic, loadMusicRuntime = oldPrepare, oldLoad }()
	outDir := t.TempDir()
	var preflights, loads atomic.Int32
	var failRun atomic.Bool
	prepareMusic = func(domain.AppConfig) error {
		if preflights.Add(1) == 1 {
			return errors.New("injected preflight failure")
		}
		return nil
	}
	activity := &musicRuntimeActivity{}
	loadMusicRuntime = func(opt sa3.Options) (musicRuntime, error) {
		loads.Add(1)
		activity.loaded()
		if failRun.CompareAndSwap(false, true) {
			return &gatedMusicRuntime{opt: opt, activity: activity, runErr: errors.New("injected run failure")}, nil
		}
		return &gatedMusicRuntime{opt: opt, activity: activity}, nil
	}
	cfg := musicTestConfig(outDir)
	if _, err := NewWithArbiter(a).Generate(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "preflight") {
		t.Fatalf("first Generate error=%v, want preflight failure", err)
	}
	// Replace the preflight failure with a successful runtime whose first run
	// fails. The following request must still acquire the same arbiter.
	prepareMusic = func(domain.AppConfig) error { return nil }
	if _, err := NewWithArbiter(a).Generate(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "run failure") {
		t.Fatalf("second Generate error=%v, want run failure", err)
	}
	if _, err := NewWithArbiter(a).Generate(context.Background(), cfg); err != nil {
		t.Fatalf("third Generate after failures: %v", err)
	}
	if got := activity.active.Load(); got != 0 {
		t.Fatalf("active runtimes=%d after failure recovery, want 0", got)
	}
	if got := activity.max.Load(); got != 1 {
		t.Fatalf("runtime active maximum=%d, want 1", got)
	}
}

func TestServiceConsumesTransferredReservation(t *testing.T) {
	a := generation.NewArbiter()
	holder, err := a.Acquire(context.Background(), generation.KindTalk)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	oldPrepare, oldLoad := prepareMusic, loadMusicRuntime
	defer func() { prepareMusic, loadMusicRuntime = oldPrepare, oldLoad }()
	outDir := t.TempDir()
	prepareMusic = func(domain.AppConfig) error { return nil }
	var loads atomic.Int32
	loadMusicRuntime = func(opt sa3.Options) (musicRuntime, error) {
		loads.Add(1)
		return &gatedMusicRuntime{opt: opt, activity: &musicRuntimeActivity{}}, nil
	}
	r, err := a.Reserve(context.Background(), generation.KindMusic)
	if err != nil {
		t.Fatal(err)
	}
	ctx := generation.WithReservation(context.Background(), r)
	done := make(chan error, 1)
	go func() { _, err := NewWithArbiter(a).Generate(ctx, musicTestConfig(outDir)); done <- err }()
	holder.Release()
	if err := waitMusicResult(t, done); err != nil {
		t.Fatalf("Generate with transferred reservation: %v", err)
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("runtime loads=%d, want one reservation consumed once", got)
	}
}

func TestServiceCancelledWhileWaitingDoesNotLoadRuntime(t *testing.T) {
	a := generation.NewArbiter()
	holder, err := a.Acquire(context.Background(), generation.KindTalk)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()

	oldPrepare, oldLoad := prepareMusic, loadMusicRuntime
	defer func() { prepareMusic, loadMusicRuntime = oldPrepare, oldLoad }()
	prepareStarted := make(chan struct{})
	prepareContinue := make(chan struct{})
	var loads atomic.Int32
	prepareMusic = func(domain.AppConfig) error {
		close(prepareStarted)
		<-prepareContinue
		return nil
	}
	loadMusicRuntime = func(sa3.Options) (musicRuntime, error) {
		loads.Add(1)
		return nil, errors.New("load should not be reached")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewWithArbiter(a).Generate(ctx, domain.AppConfig{})
		done <- err
	}()
	<-prepareStarted
	cancel()
	close(prepareContinue)
	holder.Release()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}
	if got := loads.Load(); got != 0 {
		t.Fatalf("runtime loads=%d, want 0", got)
	}
}

func TestServiceLoadFailureReleasesArbiterForNextRequest(t *testing.T) {
	a := generation.NewArbiter()
	oldPrepare, oldLoad := prepareMusic, loadMusicRuntime
	defer func() { prepareMusic, loadMusicRuntime = oldPrepare, oldLoad }()
	outDir := t.TempDir()
	prepareMusic = func(domain.AppConfig) error { return os.MkdirAll(outDir, 0o755) }
	var loads atomic.Int32
	loadMusicRuntime = func(opt sa3.Options) (musicRuntime, error) {
		if loads.Add(1) == 1 {
			return nil, errors.New("injected load failure")
		}
		if err := os.WriteFile(filepath.Join(outDir, filepath.Base(opt.OutputWAV)), []byte("fake wav"), 0o644); err != nil {
			return nil, err
		}
		return testMusicRuntime{}, nil
	}
	svc := NewWithArbiter(a)
	cfg := domain.AppConfig{StableAudio3: domain.StableAudio3Config{ModelDir: "fake", OutputDir: outDir}}
	if _, err := svc.Generate(context.Background(), cfg); err == nil {
		t.Fatal("first Generate unexpectedly succeeded")
	}
	if _, err := svc.Generate(context.Background(), cfg); err != nil {
		t.Fatalf("second Generate after load failure: %v", err)
	}
	if got := loads.Load(); got != 2 {
		t.Fatalf("runtime loads=%d, want 2", got)
	}
}
