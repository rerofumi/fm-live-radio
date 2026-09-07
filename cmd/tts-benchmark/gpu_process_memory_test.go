package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/ttseval"
)

func TestParseWDDMProcessMemoryLineMatchesPIDAndLUID(t *testing.T) {
	line := "sample\tpid_26220_luid_0x00000000_0x000185A3_phys_0\t10694348800\t77594624\t10771943424"
	sample, ok, reason := parseWDDMProcessMemoryLine(line, 26220, "0x00000000:0x000185A3")
	if !ok || reason != "" {
		t.Fatalf("valid WDDM sample rejected: ok=%v reason=%q", ok, reason)
	}
	if sample.DedicatedBytes != 10694348800 || sample.AdapterLUID != "luid_0x00000000_0x000185a3" {
		t.Fatalf("unexpected sample: %+v", sample)
	}
	if _, ok, reason = parseWDDMProcessMemoryLine(line, 26221, "0x00000000:0x000185A3"); ok || reason == "" {
		t.Fatalf("wrong PID must be rejected: ok=%v reason=%q", ok, reason)
	}
	if _, ok, reason = parseWDDMProcessMemoryLine(line, 26220, "0x00000000:0x0001C79A"); ok || reason == "" {
		t.Fatalf("wrong LUID must be rejected: ok=%v reason=%q", ok, reason)
	}
}

func TestParseWDDMProcessMemoryLineRejectsPIDPrefixCollision(t *testing.T) {
	for _, name := range []string{
		"pid_26232_luid_0x00000000_0x000185A3_phys_0",
		"pid_420_luid_0x00000000_0x000185A3_phys_0",
	} {
		line := "sample\t" + name + "\t1\t0\t1"
		if _, ok, reason := parseWDDMProcessMemoryLine(line, 26, "luid_0x00000000_0x000185a3"); ok || reason == "" {
			t.Fatalf("PID prefix collision accepted: name=%q ok=%v reason=%q", name, ok, reason)
		}
	}
}

func TestParseWDDMProcessMemoryLineFailsClosedOnMissingOrMalformedUsage(t *testing.T) {
	cases := []string{
		"sample\tpid_26220_luid_0x00000000_0x000185A3_phys_0\tN/A\t1\t2",
		"sample\tpid_26220_luid_0x00000000_0x000185A3_phys_0\t1\tN/A\t2",
		"sample\tpid_26220_luid_0x00000000_0x000185A3_phys_0\t1\t2\tN/A",
		"sample\tpid_26220_luid_0x00000000_0x000185A3_phys_0\t1",
	}
	for _, line := range cases {
		if _, ok, reason := parseWDDMProcessMemoryLine(line, 26220, "luid_0x00000000_0x000185A3"); ok || reason == "" {
			t.Fatalf("malformed sample accepted: line=%q ok=%v reason=%q", line, ok, reason)
		}
	}
}

type testGPUProcessMemoryReader struct {
	io.Reader
	closed atomic.Bool
}

func (r *testGPUProcessMemoryReader) Close() error {
	r.closed.Store(true)
	return nil
}

func TestWDDMProcessMemoryCollectorStopJoinsReaderAndConsumer(t *testing.T) {
	reader := &testGPUProcessMemoryReader{Reader: strings.NewReader("sample\tpid_42_luid_0x1_0x2_phys_0\t4096\t128\t4224\n")}
	var stopped atomic.Bool
	var waited atomic.Bool
	factory := func(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
		return gpuProcessMemoryStream{
			reader: reader,
			stop:   func() error { stopped.Store(true); return reader.Close() },
			wait:   func() error { waited.Store(true); return nil },
		}, nil
	}
	c := newWDDMProcessMemoryCollectorWithFactory(gpuProcessMemoryTarget{PID: 42, AdapterLUID: "luid_0x1_0x2", GPUUUID: "GPU-test"}, factory)
	if err := c.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	obs := c.Stop()
	if !stopped.Load() || !waited.Load() || !reader.closed.Load() {
		t.Fatalf("collector did not close and join reader: stopped=%v waited=%v closed=%v", stopped.Load(), waited.Load(), reader.closed.Load())
	}
	if !obs.Available || obs.SampleCount != 1 || obs.PeakDedicated != 4096 || obs.PID != 42 || obs.GPUUUID != "GPU-test" {
		t.Fatalf("unexpected observation: %+v", obs)
	}
	if obs.FirstSampleUTC.IsZero() || obs.LastSampleUTC.IsZero() {
		t.Fatal("sample timestamps must be recorded")
	}
	if obs.Source == "" {
		t.Fatal("source must identify CIM DedicatedUsage")
	}
}

func TestWDDMProcessMemoryCollectorFailsClosedOnUnexpectedChildWaitError(t *testing.T) {
	reader := &testGPUProcessMemoryReader{Reader: strings.NewReader("sample\tpid_42_luid_0x1_0x2_phys_0\t4096\t128\t4224\n")}
	c := newWDDMProcessMemoryCollectorWithFactory(gpuProcessMemoryTarget{PID: 42, AdapterLUID: "luid_0x1_0x2"}, func(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
		return gpuProcessMemoryStream{
			reader: reader,
			stop:   reader.Close,
			wait:   func() error { return errors.New("child exited with failure") },
		}, nil
	})
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	obs := c.Stop()
	if obs.Available {
		t.Fatalf("child wait error must invalidate observation: %+v", obs)
	}
}

func TestWDDMProcessMemoryCollectorFailsClosedAfterValidSampleError(t *testing.T) {
	r, w := io.Pipe()
	c := newWDDMProcessMemoryCollectorWithFactory(gpuProcessMemoryTarget{PID: 42, AdapterLUID: "luid_0x1_0x2"}, func(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
		return gpuProcessMemoryStream{reader: r}, nil
	})
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "sample\tpid_42_luid_0x1_0x2_phys_0\t104857600\t0\t104857600\n"); err != nil {
		t.Fatal(err)
	}
	for c.Snapshot().SampleCount < 1 {
		time.Sleep(time.Millisecond)
	}
	if _, err := io.WriteString(w, "error\tCIM provider failed after valid sample\n"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && c.Snapshot().Available {
		time.Sleep(time.Millisecond)
	}
	if got := c.Snapshot(); got.Available {
		t.Fatalf("valid sample must not survive a later CIM error: %+v", got)
	}
	_ = w.Close()
	_ = c.Stop()
}

func TestWDDMProcessMemoryCollectorFailsClosedAfterUnexpectedEOF(t *testing.T) {
	r, w := io.Pipe()
	c := newWDDMProcessMemoryCollectorWithFactory(gpuProcessMemoryTarget{PID: 42, AdapterLUID: "luid_0x1_0x2"}, func(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
		return gpuProcessMemoryStream{reader: r}, nil
	})
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "sample\tpid_42_luid_0x1_0x2_phys_0\t104857600\t0\t104857600\n"); err != nil {
		t.Fatal(err)
	}
	for c.Snapshot().SampleCount < 1 {
		time.Sleep(time.Millisecond)
	}
	_ = w.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && c.Snapshot().Available {
		time.Sleep(time.Millisecond)
	}
	if got := c.Snapshot(); got.Available {
		t.Fatalf("valid sample must not survive unexpected reader EOF: %+v", got)
	}
	_ = c.Stop()
}

func TestWDDMProcessMemoryCollectorFailsClosedWhenSampleBecomesStale(t *testing.T) {
	r, w := io.Pipe()
	c := newWDDMProcessMemoryCollectorWithFactory(gpuProcessMemoryTarget{PID: 42, AdapterLUID: "luid_0x1_0x2", Interval: 100 * time.Millisecond}, func(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
		return gpuProcessMemoryStream{reader: r}, nil
	})
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "sample\tpid_42_luid_0x1_0x2_phys_0\t104857600\t0\t104857600\n"); err != nil {
		t.Fatal(err)
	}
	for c.Snapshot().SampleCount < 1 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(350 * time.Millisecond)
	if got := c.Snapshot(); got.Available {
		t.Fatalf("stale sample must be unavailable while reader remains alive: %+v", got)
	}
	_ = w.Close()
	_ = c.Stop()
}

func TestWDDMProcessMemoryCollectorLatchesRecoveredSampleGap(t *testing.T) {
	r, w := io.Pipe()
	c := newWDDMProcessMemoryCollectorWithFactory(gpuProcessMemoryTarget{PID: 42, AdapterLUID: "luid_0x1_0x2", Interval: 100 * time.Millisecond}, func(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
		return gpuProcessMemoryStream{reader: r, stop: w.Close}, nil
	})
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	emit := func(value uint64) {
		t.Helper()
		if _, err := fmt.Fprintf(w, "sample\tpid_42_luid_0x1_0x2_phys_0\t%d\t0\t%d\n", value, value); err != nil {
			t.Fatal(err)
		}
	}
	emit(100)
	deadline := time.Now().Add(time.Second)
	for c.Snapshot().SampleCount < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(350 * time.Millisecond)
	if got := c.Snapshot(); got.Available {
		t.Fatalf("stale interval must be unavailable: %+v", got)
	}
	emit(1)
	deadline = time.Now().Add(time.Second)
	for c.Snapshot().SampleCount < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := c.Snapshot(); got.Available {
		t.Fatalf("fresh sample must not erase latched gap: %+v", got)
	}
}

func TestWDDMProcessMemoryCollectorRejectsImpossibleSampleClock(t *testing.T) {
	now := time.Now().UTC()
	for _, samples := range [][]gpuProcessMemorySample{
		{{PID: 42, AdapterLUID: "luid_0x1_0x2", DedicatedBytes: 1, ObservedAt: now.Add(time.Hour)}},
		{{PID: 42, AdapterLUID: "luid_0x1_0x2", DedicatedBytes: 1, ObservedAt: now.Add(time.Hour)}, {PID: 42, AdapterLUID: "luid_0x1_0x2", DedicatedBytes: 1, ObservedAt: now}},
	} {
		c := newWDDMProcessMemoryCollector(gpuProcessMemoryTarget{PID: 42, AdapterLUID: "luid_0x1_0x2"})
		c.samples = samples
		if got := c.Snapshot(); got.Available {
			t.Fatalf("impossible timestamp geometry accepted: %+v", got)
		}
	}
}

func TestWDDMProcessMemoryCollectorOnlyAllowsProvenWaitError(t *testing.T) {
	r, w := io.Pipe()
	waitErr := errors.New("child exited with failure")
	c := newWDDMProcessMemoryCollectorWithFactory(gpuProcessMemoryTarget{PID: 42, AdapterLUID: "luid_0x1_0x2"}, func(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
		return gpuProcessMemoryStream{reader: r, stop: w.Close, wait: func() error { return waitErr }, plannedStopWaitErrorOK: true}, nil
	})
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(w, "sample\tpid_42_luid_0x1_0x2_phys_0\t1\t0\t1"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for c.Snapshot().SampleCount < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := c.Stop(); got.Available {
		t.Fatalf("compatibility flag must not allow arbitrary wait errors: %+v", got)
	}
}

func TestWDDMProcessMemoryCollectorIntervalsDoNotReusePriorPeak(t *testing.T) {
	r, w := io.Pipe()
	c := newWDDMProcessMemoryCollectorWithFactory(gpuProcessMemoryTarget{PID: 42, AdapterLUID: "luid_0x1_0x2"}, func(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
		return gpuProcessMemoryStream{reader: r}, nil
	})
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "sample\tpid_42_luid_0x1_0x2_phys_0\t104857600\t0\t104857600\n"); err != nil {
		t.Fatal(err)
	}
	for c.Snapshot().SampleCount < 1 {
		time.Sleep(time.Millisecond)
	}
	first := c.Snapshot()
	if _, err := io.WriteString(w, "sample\tpid_42_luid_0x1_0x2_phys_0\t1048576\t0\t1048576\n"); err != nil {
		t.Fatal(err)
	}
	for c.Snapshot().SampleCount < 2 {
		time.Sleep(time.Millisecond)
	}
	second, _ := c.snapshotSince(first.SampleCount)
	if !second.Available || second.PeakDedicated != 1048576 {
		t.Fatalf("second interval reused prior peak: first=%+v second=%+v", first, second)
	}
	_ = w.Close()
	_ = c.Stop()
}

func TestWDDMProcessMemoryCollectorRequiresPIDAndLUID(t *testing.T) {
	factory := func(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
		return gpuProcessMemoryStream{}, nil
	}
	for _, target := range []gpuProcessMemoryTarget{{AdapterLUID: "luid_0x1"}, {PID: 1}} {
		c := newWDDMProcessMemoryCollectorWithFactory(target, factory)
		if err := c.Start(); err == nil {
			t.Fatalf("invalid target accepted: %+v", target)
		}
	}
}

func TestWDDMProcessMemoryCollectorShortCUDARealPID(t *testing.T) {
	if os.Getenv("FM_RADIO_GPU_COLLECTOR_E2E") != "1" {
		t.Skip("set FM_RADIO_GPU_COLLECTOR_E2E=1 to run the explicit CUDA process probe")
	}
	if runtime.GOOS != "windows" {
		t.Skip("WDDM process memory is Windows-only")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	binary := filepath.Join(repoRoot, "build", "bin", "acceptance", "acceptance-gpu-smoke.exe")
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("short CUDA smoke binary is unavailable: %v", err)
	}
	index, uuid := gpuIdentity()
	luid, err := queryWDDMAdapterLUID(index)
	if err != nil {
		t.Fatalf("resolve target adapter LUID: %v", err)
	}
	cmd := exec.Command(binary, "--service", "--model", "model/irodori-v4.1", "--ep", "cuda", "--seed", "0", "--steps", "1", "--seconds", "0.2", "--out", filepath.Join(os.TempDir(), "fm-radio-gpu-collector-test.wav"))
	cmd.Dir = repoRoot
	if err := cmd.Start(); err != nil {
		t.Fatalf("start short CUDA smoke: %v", err)
	}
	collector := newWDDMProcessMemoryCollector(gpuProcessMemoryTarget{PID: cmd.Process.Pid, GPUIndex: index, AdapterLUID: luid, GPUUUID: uuid})
	if err := collector.Start(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("start WDDM collector: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && collector.Snapshot().SampleCount < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	obs := collector.Stop()
	cmdErr := cmd.Wait()
	if cmdErr != nil {
		t.Fatalf("short CUDA smoke failed: %v", cmdErr)
	}
	if !obs.Available || obs.SampleCount == 0 || obs.PeakDedicated == 0 {
		t.Fatalf("real PID WDDM observation unavailable: %+v", obs)
	}
	t.Logf("pid=%d luid=%s uuid=%s source=%s samples=%d peak_dedicated_bytes=%d", obs.PID, obs.AdapterLUID, obs.GPUUUID, obs.Source, obs.SampleCount, obs.PeakDedicated)
}

func TestVRAMSamplerShortCUDAIntegration(t *testing.T) {
	if os.Getenv("FM_RADIO_GPU_SAMPLER_E2E") != "1" {
		t.Skip("set FM_RADIO_GPU_SAMPLER_E2E=1 to run one integrated benchmark Talk")
	}
	if runtime.GOOS != "windows" {
		t.Skip("WDDM process memory is Windows-only")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	lib := filepath.Join(repoRoot, "third_party", "onnxruntime-gpu", "onnxruntime-win-x64-gpu-1.26.0", "lib", "onnxruntime.dll")
	if err := generation.ConfigureExecutionProvider("cuda", 0); err != nil {
		t.Fatalf("configure CUDA: %v", err)
	}
	if err := generation.Init(lib); err != nil {
		t.Fatalf("initialize ORT: %v", err)
	}
	cfg := benchmarkConfig(filepath.Join(repoRoot, "model", "irodori-v4.1"), filepath.Join(repoRoot, "narrator", "narrator_01.wav"), 0.2, 1, 0, "cuda", lib)
	rows, err := measureTalk(context.Background(), cfg, "v4.1", "cuda", ttseval.Scripts()[0], "", 60000)
	if err != nil {
		t.Fatalf("integrated short Talk failed: %v", err)
	}
	if len(rows) == 0 || rows[0].VRAMStatus != "available" || rows[0].VRAMSource != "wddm-process-dedicated" || rows[0].PeakVRAMMiB == nil {
		t.Fatalf("integrated WDDM VRAM observation unavailable: %+v", rows)
	}
	t.Logf("source=%s gpu_index=%s gpu_uuid=%s peak_vram_mib=%d reason=%s", rows[0].VRAMSource, rows[0].GPUIndex, rows[0].GPUUUID, *rows[0].PeakVRAMMiB, rows[0].VRAMReason)
}
