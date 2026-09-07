package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"fm-live-radio/internal/generation"
)

// This is opt-in because it loads the multi-gigabyte v4 bundle. CI/acceptance
// can set FM_RADIO_IRODORI_MODEL to run the real public API regression.
func TestRuntimePublicSynthesizeRetainsOptions(t *testing.T) {
	modelDir := os.Getenv("FM_RADIO_IRODORI_MODEL")
	if modelDir == "" {
		t.Skip("set FM_RADIO_IRODORI_MODEL to run the real v4 Runtime regression")
	}
	lib := os.Getenv("FM_RADIO_ORT_LIB")
	if lib == "" {
		lib = generation.ResolveORTLibraryPathForEP("cpu")
	}
	if lib == "" {
		t.Skip("CPU ONNX Runtime DLL is unavailable")
	}
	if err := generation.ConfigureExecutionProvider("cpu", 0); err != nil {
		t.Fatal(err)
	}
	if err := generation.Init(lib); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	first := filepath.Join(dir, "first.wav")
	second := filepath.Join(dir, "second.wav")
	opt := DefaultOptions()
	opt.ModelDir, opt.Text, opt.OutputWAV = modelDir, "最初の公開API回帰です。", first
	opt.NumSteps, opt.Seconds, opt.RefWAV = 2, 0.5, ""
	rt, err := LoadInitialise(opt)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if err := rt.Synthesize(); err != nil {
		t.Fatalf("initial Synthesize: %v", err)
	}
	updated := opt
	updated.Text, updated.OutputWAV = "更新後の公開API回帰です。", second
	if err := rt.SynthesizeWithOptions(updated); err != nil {
		t.Fatalf("updated SynthesizeWithOptions: %v", err)
	}
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	// A zero Options value means "use the Runtime's current options"; it must
	// not erase Text/OutputWAV after the update.
	if err := rt.SynthesizeWithOptions(Options{}); err != nil {
		t.Fatalf("zero-options SynthesizeWithOptions: %v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("zero-options call did not reuse updated output: %v", err)
	}
}
