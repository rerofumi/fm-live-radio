package localtts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/localtts/irodori/pipeline"
)

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
