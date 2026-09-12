package localtts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fm-live-radio/internal/audiofmt"
	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts/irodori/pipeline"
)

type ttsRuntimeActivity struct {
	active atomic.Int32
	max    atomic.Int32
}

func (a *ttsRuntimeActivity) loaded() {
	n := a.active.Add(1)
	for {
		old := a.max.Load()
		if n <= old || a.max.CompareAndSwap(old, n) {
			return
		}
	}
}

func (a *ttsRuntimeActivity) closed() { a.active.Add(-1) }

type gatedTTSRuntime struct {
	opt       pipeline.Options
	activity  *ttsRuntimeActivity
	closeGate <-chan struct{}
	closeDone chan<- struct{}
	runErr    error
}

func (r *gatedTTSRuntime) SynthesizeWithOptions(opt pipeline.Options) error {
	if r.runErr != nil {
		return r.runErr
	}
	pcm := make([]byte, 200)
	for i := 0; i < len(pcm); i += 2 {
		pcm[i] = 0xe8
		pcm[i+1] = 0x03
	}
	wav, err := audiofmt.EncodeWavPCM16(pcm, 48000, 1)
	if err != nil {
		return err
	}
	return os.WriteFile(opt.OutputWAV, wav, 0o644)
}

func (r *gatedTTSRuntime) Close() {
	if r.closeDone != nil {
		close(r.closeDone)
	}
	if r.closeGate != nil {
		<-r.closeGate
	}
	r.activity.closed()
}

func waitTTSResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service completion (possible leaked reservation/lease)")
		return nil
	}
}

func ttsTestConfig() domain.AppConfig {
	return domain.AppConfig{
		LocalInference: domain.LocalInferenceConfig{ExecutionProvider: "cuda", DeviceID: 1, MaxWorkers: 8},
		Irodori:        domain.IrodoriConfig{ModelDir: "fake"},
	}
}

func TestServiceInstancesShareLeaseUntilSlowCloseCompletes(t *testing.T) {
	a := generation.NewArbiter()
	oldPreflight, oldLoad := preflightTTS, loadTTSRuntime
	defer func() { preflightTTS, loadTTSRuntime = oldPreflight, oldLoad }()
	preflightTTS = func(cfg domain.AppConfig, _ pipeline.EventObserver) error {
		if cfg.LocalInference.MaxWorkers <= 1 || cfg.LocalInference.ExecutionProvider != "cuda" {
			t.Fatalf("test config did not exercise provider/maxWorkers input: %+v", cfg.LocalInference)
		}
		return nil
	}
	activity := &ttsRuntimeActivity{}
	firstCloseStarted := make(chan struct{})
	secondLoad := make(chan struct{})
	closeGate := make(chan struct{})
	var loads atomic.Int32
	loadTTSRuntime = func(opt pipeline.Options) (ttsRuntime, error) {
		n := loads.Add(1)
		activity.loaded()
		if n == 1 {
			return &gatedTTSRuntime{opt: opt, activity: activity, closeGate: closeGate, closeDone: firstCloseStarted}, nil
		}
		close(secondLoad)
		return &gatedTTSRuntime{opt: opt, activity: activity}, nil
	}

	cfg := ttsTestConfig()
	firstDone := make(chan error, 1)
	go func() {
		_, err := NewWithArbiter(a).SynthesizeWav(context.Background(), cfg, "最初の発話。")
		firstDone <- err
	}()
	<-firstCloseStarted
	secondDone := make(chan error, 1)
	go func() {
		_, err := NewWithArbiter(a).SynthesizeWav(context.Background(), cfg, "次の発話。")
		secondDone <- err
	}()

	// The slot stays occupied until the first Runtime.Close returns.
	close(closeGate)
	if err := waitTTSResult(t, firstDone); err != nil {
		t.Fatalf("first SynthesizeWav: %v", err)
	}
	if err := waitTTSResult(t, secondDone); err != nil {
		t.Fatalf("second SynthesizeWav: %v", err)
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

func TestServiceRunFailureAndPreflightFailureReleaseReservations(t *testing.T) {
	a := generation.NewArbiter()
	oldPreflight, oldLoad := preflightTTS, loadTTSRuntime
	defer func() { preflightTTS, loadTTSRuntime = oldPreflight, oldLoad }()
	var preflights atomic.Int32
	var failRun atomic.Bool
	preflightTTS = func(domain.AppConfig, pipeline.EventObserver) error {
		if preflights.Add(1) == 1 {
			return errors.New("injected preflight failure")
		}
		return nil
	}
	activity := &ttsRuntimeActivity{}
	var loads atomic.Int32
	loadTTSRuntime = func(opt pipeline.Options) (ttsRuntime, error) {
		loads.Add(1)
		activity.loaded()
		if failRun.CompareAndSwap(false, true) {
			return &gatedTTSRuntime{opt: opt, activity: activity, runErr: errors.New("injected run failure")}, nil
		}
		return &gatedTTSRuntime{opt: opt, activity: activity}, nil
	}
	cfg := ttsTestConfig()
	if _, err := NewWithArbiter(a).SynthesizeWav(context.Background(), cfg, "preflight failure。"); err == nil || !strings.Contains(err.Error(), "preflight failure") {
		t.Fatalf("first SynthesizeWav error=%v, want preflight failure", err)
	}
	if _, err := NewWithArbiter(a).SynthesizeWav(context.Background(), cfg, "run failure。"); !errors.Is(err, ErrAllSentencesFailed) {
		t.Fatalf("second SynthesizeWav error=%v, want all-sentences runtime failure", err)
	}
	if _, err := NewWithArbiter(a).SynthesizeWav(context.Background(), cfg, "recovery。"); err != nil {
		t.Fatalf("third SynthesizeWav after failures: %v", err)
	}
	if got := activity.active.Load(); got != 0 {
		t.Fatalf("active runtimes=%d after failure recovery, want 0", got)
	}
	if got := activity.max.Load(); got != 1 {
		t.Fatalf("runtime active maximum=%d, want 1", got)
	}
	if got := loads.Load(); got != 2 {
		t.Fatalf("runtime loads=%d, want 2 (preflight failure must not load)", got)
	}
}

func TestServiceConsumesTransferredReservation(t *testing.T) {
	a := generation.NewArbiter()
	holder, err := a.Acquire(context.Background(), generation.KindMusic)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	oldPreflight, oldLoad := preflightTTS, loadTTSRuntime
	defer func() { preflightTTS, loadTTSRuntime = oldPreflight, oldLoad }()
	preflightTTS = func(domain.AppConfig, pipeline.EventObserver) error { return nil }
	var loads atomic.Int32
	loadTTSRuntime = func(opt pipeline.Options) (ttsRuntime, error) {
		loads.Add(1)
		activity := &ttsRuntimeActivity{}
		activity.loaded()
		return &gatedTTSRuntime{opt: opt, activity: activity}, nil
	}
	r, err := a.Reserve(context.Background(), generation.KindTalk)
	if err != nil {
		t.Fatal(err)
	}
	ctx := generation.WithReservation(context.Background(), r)
	done := make(chan error, 1)
	go func() {
		_, err := NewWithArbiter(a).SynthesizeWav(ctx, ttsTestConfig(), "転送予約。")
		done <- err
	}()
	holder.Release()
	if err := waitTTSResult(t, done); err != nil {
		t.Fatalf("SynthesizeWav with transferred reservation: %v", err)
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("runtime loads=%d, want one reservation consumed once", got)
	}
}

func TestSplitSentencesKeepsExistingBoundaries(t *testing.T) {
	got := splitSentences("  一文目です。\n二文目です！  三文目？\n")
	want := []string{"一文目です。", "二文目です！", "三文目？"}
	if len(got) != len(want) {
		t.Fatalf("split count=%d, want %d (%q)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sentence %d=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestServiceCancelledWhileWaitingDoesNotLoadRuntime(t *testing.T) {
	a := generation.NewArbiter()
	holder, err := a.Acquire(context.Background(), generation.KindMusic)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()

	oldPreflight, oldLoad := preflightTTS, loadTTSRuntime
	defer func() { preflightTTS, loadTTSRuntime = oldPreflight, oldLoad }()
	preflightStarted := make(chan struct{})
	preflightContinue := make(chan struct{})
	var loads atomic.Int32
	preflightTTS = func(domain.AppConfig, pipeline.EventObserver) error {
		close(preflightStarted)
		<-preflightContinue
		return nil
	}
	loadTTSRuntime = func(pipeline.Options) (ttsRuntime, error) {
		loads.Add(1)
		return nil, errors.New("load should not be reached")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewWithArbiter(a).SynthesizeWav(ctx, domain.AppConfig{}, "待機中の取消。")
		done <- err
	}()
	<-preflightStarted
	cancel()
	close(preflightContinue)
	holder.Release()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}
	if got := loads.Load(); got != 0 {
		t.Fatalf("runtime loads=%d, want 0", got)
	}
}

// Opt-in real-ORT regression: cancellation must join/close the first Talk so
// the same Service can load and synthesize a later Talk in this process.
func TestServiceCancelThenRegenerate(t *testing.T) {
	modelDir := os.Getenv("FM_RADIO_IRODORI_MODEL")
	if modelDir == "" {
		t.Skip("set FM_RADIO_IRODORI_MODEL to run the real cancellation regression")
	}
	lib := os.Getenv("FM_RADIO_ORT_LIB")
	if lib == "" {
		lib = "third_party/onnxruntime/onnxruntime-win-x64-1.26.0/lib/onnxruntime.dll"
	}
	ref := os.Getenv("FM_RADIO_IRODORI_REF")
	if ref == "" {
		t.Skip("set FM_RADIO_IRODORI_REF to run the real cancellation regression")
	}
	cfg := domain.AppConfig{Irodori: domain.IrodoriConfig{
		ModelDir: modelDir, RefWAV: ref, Seconds: 0.5,
		NumSteps: 40, SeedMode: "fixed", CfgText: 3, CfgCaption: 3, CfgSpeaker: 5,
		DurationScale: 1,
	}, LocalInference: domain.LocalInferenceConfig{ORTLibraryPath: lib, ExecutionProvider: "cpu"}}
	svc := New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	startLoads := pipeline.RuntimeLoadCount()
	if _, err := svc.SynthesizeWav(ctx, cfg, "取消中の実推論を確認します。"); !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled synthesis error=%v", err)
	}
	if got := pipeline.RuntimeLoadCount() - startLoads; got != 1 {
		t.Fatalf("cancelled Talk load count=%d, want 1", got)
	}
	cfg.Irodori.NumSteps = 2
	regen, err := svc.SynthesizeWav(context.Background(), cfg, "取消後の後続生成です。")
	if err != nil {
		t.Fatalf("same-Service regeneration: %v", err)
	}
	if len(regen) == 0 {
		t.Fatal("same-Service regeneration returned empty WAV")
	}
	if got := pipeline.RuntimeLoadCount() - startLoads; got != 2 {
		t.Fatalf("two Talk load count=%d, want 2", got)
	}
}

func TestPipelineOptionsKeepExplicitReferenceAndSeed(t *testing.T) {
	cfg := domain.AppConfig{Irodori: domain.IrodoriConfig{
		ModelDir: "E:/models/irodori-v4.1", RefWAV: "E:/voice/ref.wav",
		Seconds: 2, NumSteps: 7, SeedMode: "fixed", FixedSeed: 42,
		CfgText: 3, CfgCaption: 2, CfgSpeaker: 5, DurationScale: 0.75,
	}}
	o := pipelineOptions(cfg)
	if o.ModelDir != cfg.Irodori.ModelDir || o.RefWAV != cfg.Irodori.RefWAV || o.Seed != 42 {
		t.Fatalf("explicit v4 settings were not preserved: %+v", o)
	}
	if o.Seconds != 2 || o.NumSteps != 7 || o.DurationScale != 0.75 {
		t.Fatalf("inference settings were not preserved: %+v", o)
	}
}

func TestValidateModelAssetsRejectsMissingLegacyBundle(t *testing.T) {
	err := validateModelAssets(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected missing model/tokenizer to be rejected before inference")
	}
}
