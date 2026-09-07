// tts-benchmark measures v3/v4 under one explicit process/EP/seed condition.
// It writes machine-readable JSON and CSV and never turns a failed synthesis
// into a pass.  Quality (reading, voice and naturalness) is intentionally not
// judged here; the generated WAV manifest is for human audition.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fm-live-radio/internal/audiofmt"
	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts"
	"fm-live-radio/internal/localtts/irodori/pipeline"
	"fm-live-radio/internal/ttseval"
)

type row struct {
	Model       string  `json:"model"`
	EP          string  `json:"execution_provider"`
	Seed        uint32  `json:"seed"`
	ScriptID    string  `json:"script_id"`
	Kind        string  `json:"kind"`
	Repeat      int     `json:"repeat"`
	TextRunes   int     `json:"text_runes"`
	ElapsedMS   float64 `json:"elapsed_ms"`
	ColdLoadMS  float64 `json:"cold_load_ms"`
	RTF         float64 `json:"rtf"`
	DeadlineMS  float64 `json:"deadline_ms"`
	DeadlineMet bool    `json:"deadline_met"`
	PeakVRAMMiB *int64  `json:"peak_vram_mib,omitempty"`
	VRAMStatus  string  `json:"vram_status,omitempty"`
	VRAMReason  string  `json:"vram_reason,omitempty"`
	VRAMSource  string  `json:"vram_source,omitempty"`
	GPUIndex    string  `json:"gpu_index,omitempty"`
	GPUUUID     string  `json:"gpu_uuid,omitempty"`
	PreflightMS float64 `json:"preflight_ms,omitempty"`
	LoadMS      float64 `json:"load_ms,omitempty"`
	CloseMS     float64 `json:"close_ms,omitempty"`
	CombineMS   float64 `json:"combine_ms,omitempty"`
	GapCount    int     `json:"gap_count,omitempty"`
	Output      string  `json:"output,omitempty"`
	SHA256      string  `json:"sha256,omitempty"`
	SampleRate  int     `json:"sample_rate,omitempty"`
	Frames      int     `json:"frames,omitempty"`
	Error       string  `json:"error,omitempty"`
}

type report struct {
	Version            string             `json:"version"`
	GeneratedAt        string             `json:"generated_at"`
	Host               string             `json:"host"`
	GOOS               string             `json:"goos"`
	EP                 string             `json:"execution_provider"`
	Seed               uint32             `json:"seed"`
	Ref                string             `json:"reference_wav"`
	Models             []string           `json:"models"`
	ModelName          string             `json:"model_name,omitempty"`
	DeadlineMS         float64            `json:"talk_deadline_ms"`
	DeadlineSource     string             `json:"talk_deadline_source"`
	Steps              int                `json:"steps"`
	Seconds            float64            `json:"seconds"`
	CFG                map[string]float64 `json:"cfg"`
	DurationScale      float64            `json:"duration_scale"`
	Snapshot           string             `json:"snapshot"`
	Parent             string             `json:"parent"`
	WorkingTreeSHA256  string             `json:"working_tree_sha256"`
	RefSHA256          string             `json:"reference_sha256,omitempty"`
	ModelSHA256        string             `json:"model_manifest_sha256,omitempty"`
	InputSHA256        string             `json:"input_sha256"`
	ORTLibrary         string             `json:"ort_library"`
	ORTLibrarySHA256   string             `json:"ort_library_sha256"`
	GPU                string             `json:"gpu"`
	Driver             string             `json:"driver"`
	ModelAssets        map[string]string  `json:"model_assets"`
	VRAMSampling       string             `json:"vram_sampling"`
	VRAMHostCondition  string             `json:"vram_host_condition"`
	GPUIndex           string             `json:"gpu_index"`
	GPUUUID            string             `json:"gpu_uuid"`
	VRAMBaselineMiB    *int64             `json:"vram_baseline_mib,omitempty"`
	VRAMBaselineSource string             `json:"vram_baseline_source"`
	Scripts            []ttseval.Script   `json:"scripts"`
	Rows               []row              `json:"rows"`
	Steady             map[string]any     `json:"steady_summary,omitempty"`
	Adoption           map[string]any     `json:"adoption,omitempty"`
	Comparison         map[string]any     `json:"comparison,omitempty"`
}

// childReference is copied into parent/merged reports so a combined result is
// auditable without guessing which child condition produced it.
type childReference struct {
	Path              string             `json:"report"`
	SHA256            string             `json:"report_sha256"`
	Snapshot          string             `json:"snapshot"`
	Parent            string             `json:"parent"`
	WorkingTreeSHA256 string             `json:"working_tree_sha256"`
	Steps             int                `json:"steps"`
	Seconds           float64            `json:"seconds"`
	CFG               map[string]float64 `json:"cfg"`
	DurationScale     float64            `json:"duration_scale"`
	ReferenceSHA256   string             `json:"reference_sha256"`
	InputSHA256       string             `json:"input_sha256"`
	ModelManifestSHA  string             `json:"model_manifest_sha256"`
	ModelAssets       map[string]string  `json:"model_assets"`
	ORTLibrary        string             `json:"ort_library"`
	ORTLibrarySHA256  string             `json:"ort_library_sha256"`
	EP                string             `json:"execution_provider"`
	GPU               string             `json:"gpu"`
	Driver            string             `json:"driver"`
}

// runProvenance is captured once by the parent benchmark process and passed
// to both model children. Reports are written below the output directory, so
// the working-tree digest deliberately excludes only those generated files.
// Product changes outside that directory remain visible and are rejected by
// verifyProvenanceUnchanged before the combined report is accepted.
type runProvenance struct {
	Snapshot          string
	Parent            string
	WorkingTreeSHA256 string
}

type auditionManifest struct {
	Version           string                       `json:"version"`
	GeneratedAt       string                       `json:"generated_at"`
	EP                string                       `json:"ep"`
	ReferenceWAV      string                       `json:"reference_wav"`
	ReferenceSHA256   string                       `json:"reference_sha256"`
	Snapshot          string                       `json:"snapshot"`
	Parent            string                       `json:"parent"`
	WorkingTreeSHA256 string                       `json:"working_tree_sha256"`
	Steps             int                          `json:"steps"`
	Seconds           float64                      `json:"seconds"`
	CFG               map[string]float64           `json:"cfg"`
	DurationScale     float64                      `json:"duration_scale"`
	Model             string                       `json:"model"`
	ModelAssets       map[string]map[string]string `json:"model_assets"`
	InputSHA256       string                       `json:"input_sha256"`
	ORTLibrary        string                       `json:"ort_library"`
	ORTLibrarySHA256  string                       `json:"ort_library_sha256"`
	GPU               string                       `json:"gpu"`
	Driver            string                       `json:"driver"`
	Rows              []auditionRow                `json:"rows"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	modelV3 := flag.String("model-v3", "model/irodori-v3", "v3 model directory")
	modelV4 := flag.String("model-v4", "model/irodori-v4.1", "v4.1 model directory")
	singleModel := flag.String("model", "", "run only this model in this process; parent mode launches v3/v4 separately")
	singleName := flag.String("model-name", "", "label for --model (v3 or v4.1)")
	ep := flag.String("ep", "cpu", "cpu, cuda, or auto; run each EP in a separate process")
	seed := flag.Uint("seed", 0, "fixed seed")
	ref := flag.String("ref", "narrator/narrator_01.wav", "shared reference WAV")
	outDir := flag.String("out", filepath.Join("evidence", "tts-benchmark"), "WAV and report directory")
	deadline := flag.Float64("deadline-ms", 60000, "next Talk deadline used for the recorded condition")
	steps := flag.Int("steps", 40, "denoising steps")
	seconds := flag.Float64("seconds", -1, "duration; -1 uses duration predictor")
	scriptLimit := flag.Int("scripts", 10, "number of fixed scripts (must be 10 for REQ-08)")
	steadyRepeats := flag.Int("steady-repeats", 20, "steady-state repeats (must be 20 for REQ-08)")
	auditionOnly := flag.Bool("audition-only", false, "generate the REQ-09 60-WAV human-audition set instead of timing benchmark")
	auditionSeeds := flag.String("audition-seeds", "0,1,2", "comma-separated seeds for --audition-only")
	provenanceChild := flag.Bool("provenance-child", false, "internal: parent-launched child process")
	provenanceSnapshot := flag.String("provenance-snapshot", "", "parent-captured commit snapshot (child mode)")
	provenanceParent := flag.String("provenance-parent", "", "parent-captured parent commit (child mode)")
	provenanceWorkingTree := flag.String("provenance-working-tree-sha256", "", "parent-captured product working-tree hash (child mode)")
	flag.Parse()
	if err := ttseval.ValidateAll(); err != nil {
		return err
	}
	if !validBenchmarkCounts(*scriptLimit, *steadyRepeats) {
		return errors.New("benchmark requires --scripts=10 and --steady-repeats=20, or diagnostic --scripts=1 and --steady-repeats=1")
	}
	if *ep != "cpu" && *ep != "cuda" && *ep != "auto" {
		return fmt.Errorf("unsupported EP %q", *ep)
	}
	if *deadline <= 0 || *steps <= 0 {
		return errors.New("deadline-ms and steps must be positive")
	}
	if err := validateBenchmarkOutputDir(*outDir); err != nil {
		return err
	}
	if err := validateProvenanceInvocation(*singleModel, *provenanceChild, *provenanceSnapshot, *provenanceParent, *provenanceWorkingTree); err != nil {
		return err
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	if *singleModel == "" {
		return runChildren(*modelV3, *modelV4, *ep, uint32(*seed), *ref, *outDir, *deadline, *steps, *seconds, *scriptLimit, *steadyRepeats, *auditionOnly, *auditionSeeds)
	}
	if *singleName == "" {
		*singleName = "model"
	}
	if err := validateBenchmarkLabel(*singleName); err != nil {
		return err
	}
	provenance, err := resolveRunProvenance(*provenanceSnapshot, *provenanceParent, *provenanceWorkingTree, *outDir)
	if err != nil {
		return err
	}
	lib := generation.ResolveORTLibraryPathForEP(*ep)
	if lib == "" {
		return fmt.Errorf("matching %s ORT DLL not found", *ep)
	}
	if err := generation.ConfigureExecutionProvider(*ep, 0); err != nil {
		return err
	}
	if err := generation.Init(lib); err != nil {
		return err
	}
	if *auditionOnly {
		if *singleName == "v3" {
			return runAudition(*singleModel, "", *singleName, *ep, *ref, *steps, *seconds, *outDir, *auditionSeeds, provenance)
		}
		return runAudition("", *singleModel, *singleName, *ep, *ref, *steps, *seconds, *outDir, *auditionSeeds, provenance)
	}

	scripts := selectBenchmarkScripts(*scriptLimit)
	assets, assetsErr := hashModelAssets(*singleModel)
	if assetsErr != nil {
		return fmt.Errorf("model asset provenance: %w", assetsErr)
	}
	gpuIndex, gpuUUID := gpuIdentity()
	baseline, _, _, baselineOK, _ := queryDeviceUsedMemory(gpuUUID)
	var baselinePtr *int64
	if baselineOK {
		baselinePtr = &baseline
	}
	baselineSource := "unavailable"
	if baselineOK {
		baselineSource = "device-total"
	}
	r := report{Version: "working-tree", GeneratedAt: time.Now().UTC().Format(time.RFC3339), Host: hostname(), GOOS: runtime.GOOS, EP: *ep, Seed: uint32(*seed), Ref: *ref, Models: []string{*singleModel}, ModelName: *singleName, DeadlineMS: *deadline, DeadlineSource: deadlineSource(*deadline), Steps: *steps, Seconds: *seconds, DurationScale: 1, CFG: map[string]float64{"text": 3, "caption": 3, "speaker": 5}, Snapshot: provenance.Snapshot, Parent: provenance.Parent, WorkingTreeSHA256: provenance.WorkingTreeSHA256, RefSHA256: hashFile(*ref), ModelSHA256: hashFile(filepath.Join(*singleModel, "manifest.json")), InputSHA256: hashScripts(scripts), ORTLibrary: lib, ORTLibrarySHA256: hashFile(lib), GPU: gpuInfo(), Driver: driverInfo(), GPUIndex: gpuIndex, GPUUUID: gpuUUID, VRAMBaselineMiB: baselinePtr, VRAMBaselineSource: baselineSource, ModelAssets: assets, VRAMSampling: "nvidia-smi process used_memory; Windows WDDM DedicatedUsage process fallback; device-total is baseline only; poll interval=100ms", VRAMHostCondition: "process-scoped values are diagnostic only; device-total baseline is diagnostic and never substitutes for process memory", Scripts: scripts}
	for _, model := range []struct{ name, path string }{{*singleName, *singleModel}} {
		cfg := benchmarkConfig(model.path, *ref, *seconds, *steps, uint32(*seed), *ep, lib)
		for _, script := range scripts {
			rows, err := measureTalk(context.Background(), cfg, model.name, *ep, script, *outDir, *deadline)
			if err != nil {
				r.Rows = append(r.Rows, row{Model: model.name, EP: *ep, Seed: uint32(*seed), ScriptID: script.ID, Kind: "talk", Error: err.Error()})
			} else {
				r.Rows = append(r.Rows, rows...)
			}
		}
		steady := make([]row, 0, *steadyRepeats)
		for i := 0; i < *steadyRepeats; i++ {
			rows, err := measureTalk(context.Background(), cfg, model.name, *ep, scripts[0], "", *deadline)
			if err != nil {
				steady = append(steady, row{Model: model.name, EP: *ep, Seed: uint32(*seed), ScriptID: scripts[0].ID, Kind: "steady", Repeat: i + 1, Error: err.Error()})
			} else {
				for _, x := range rows {
					if x.Kind == "talk" {
						x.Kind, x.Repeat = "steady", i+1
						steady = append(steady, x)
						break
					}
				}
			}
		}
		r.Rows = append(r.Rows, steady...)
		r.Steady = summarizeSteady(r.Steady, model.name, steady)
	}

	suffix := ""
	if *singleName != "" {
		suffix = "-" + *singleName
	}
	jsonPath := filepath.Join(*outDir, fmt.Sprintf("benchmark-%s%s-seed%d.json", *ep, suffix, *seed))
	csvPath := filepath.Join(*outDir, fmt.Sprintf("benchmark-%s%s-seed%d.csv", *ep, suffix, *seed))
	gateErr := validateReportGates(r)
	r.Adoption = adoptionAssessment(r, gateErr)
	// Include the adoption decision before the first report write, then keep
	// provenance verification as the final write boundary for this child.
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, b, 0o600); err != nil {
		return err
	}
	if err := writeCSV(csvPath, r); err != nil {
		return err
	}
	if err := verifyProvenanceUnchanged(provenance, *outDir); err != nil {
		return fmt.Errorf("benchmark provenance changed during child run: %w", err)
	}
	failed := 0
	for _, x := range r.Rows {
		if x.Error != "" {
			failed++
		}
	}
	fmt.Printf("benchmark json=%s csv=%s rows=%d failed=%d\n", jsonPath, csvPath, len(r.Rows), failed)
	if failed > 0 {
		return fmt.Errorf("benchmark completed with %d failed measurements; see %s", failed, jsonPath)
	}
	if err := gateErr; err != nil {
		return fmt.Errorf("benchmark gates failed: %w; see %s", err, jsonPath)
	}
	return nil
}

func validBenchmarkCounts(scripts, steady int) bool {
	return (scripts == 10 && steady == 20) || (scripts == 1 && steady == 1)
}

func selectBenchmarkScripts(limit int) []ttseval.Script {
	scripts := ttseval.Scripts()
	if limit < len(scripts) {
		return scripts[:limit]
	}
	return scripts
}

func validateBenchmarkLabel(label string) error {
	if strings.TrimSpace(label) == "" || label == "." || label == ".." || strings.ContainsAny(label, `/\\`) {
		return fmt.Errorf("benchmark model name %q is not a safe output label", label)
	}
	return nil
}

func runChildren(v3, v4, ep string, seed uint32, ref, out string, deadline float64, steps int, seconds float64, scripts, repeats int, audition bool, auditionSeeds string) error {
	models := []struct{ name, path string }{{"v3", v3}, {"v4.1", v4}}
	provenance, err := captureRunProvenance(out)
	if err != nil {
		return fmt.Errorf("parent benchmark provenance: %w", err)
	}
	var childFailures []string
	for _, model := range models {
		args := []string{"--model", model.path, "--model-name", model.name, "--ep", ep, "--seed", strconv.FormatUint(uint64(seed), 10), "--ref", ref, "--out", out, "--deadline-ms", strconv.FormatFloat(deadline, 'f', -1, 64), "--steps", strconv.Itoa(steps), "--seconds", strconv.FormatFloat(seconds, 'f', -1, 64), "--scripts", strconv.Itoa(scripts), "--steady-repeats", strconv.Itoa(repeats)}
		args = append(args, "--provenance-child")
		args = appendProvenanceArgs(args, provenance)
		if audition {
			args = append(args, "--audition-only", "--audition-seeds", auditionSeeds)
		}
		cmd := exec.Command(os.Args[0], args...)
		cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
		if err := cmd.Run(); err != nil {
			// Gate failure is recorded in the child report. Continue so v4 is
			// still measured and the parent can aggregate both reports.
			childFailures = append(childFailures, fmt.Sprintf("%s child process: %v", model.name, err))
		}
	}
	if err := verifyProvenanceUnchanged(provenance, out); err != nil {
		return fmt.Errorf("benchmark provenance changed during child runs: %w", err)
	}
	if audition {
		all := make([]auditionRow, 0, 60)
		children := make([]auditionManifest, 0, len(models))
		for _, model := range models {
			p := filepath.Join(out, fmt.Sprintf("audition-manifest-%s-%s.json", ep, model.name))
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			var x auditionManifest
			if err := json.Unmarshal(b, &x); err != nil {
				return err
			}
			if err := validateAuditionChildCondition(x, ep, ref, steps, seconds); err != nil {
				return fmt.Errorf("%s child condition: %w", model.name, err)
			}
			if err := validateProvenance(x.Snapshot, x.Parent, x.WorkingTreeSHA256); err != nil {
				return fmt.Errorf("%s child provenance: %w", model.name, err)
			}
			if err := validateRunProvenance(provenance, runProvenance{Snapshot: x.Snapshot, Parent: x.Parent, WorkingTreeSHA256: x.WorkingTreeSHA256}); err != nil {
				return fmt.Errorf("%s child provenance inheritance: %w", model.name, err)
			}
			children = append(children, x)
			all = append(all, x.Rows...)
		}
		childRefs := make(map[string]childReference, len(children))
		for _, child := range children {
			path := filepath.Join(out, fmt.Sprintf("audition-manifest-%s-%s.json", ep, child.Model))
			childRefs[child.Model] = childReference{Path: path, SHA256: hashFile(path), Snapshot: child.Snapshot, Parent: child.Parent, WorkingTreeSHA256: child.WorkingTreeSHA256, Steps: child.Steps, Seconds: child.Seconds, CFG: child.CFG, DurationScale: child.DurationScale, ReferenceSHA256: child.ReferenceSHA256, InputSHA256: child.InputSHA256, ModelAssets: flattenAssets(child.ModelAssets), ORTLibrary: child.ORTLibrary, ORTLibrarySHA256: child.ORTLibrarySHA256, EP: child.EP, GPU: child.GPU, Driver: child.Driver}
		}
		if err := validateChildReferencePair(childRefs["v3"], childRefs["v4.1"]); err != nil {
			return fmt.Errorf("merged audition condition: %w", err)
		}
		condition := childRefs["v3"]
		merged := map[string]any{"version": "working-tree", "generated_at": time.Now().UTC().Format(time.RFC3339), "ep": ep, "reference_wav": ref, "reference_sha256": condition.ReferenceSHA256, "snapshot": provenance.Snapshot, "parent": provenance.Parent, "working_tree_sha256": provenance.WorkingTreeSHA256, "steps": condition.Steps, "seconds": condition.Seconds, "cfg": condition.CFG, "duration_scale": condition.DurationScale, "input_sha256": condition.InputSHA256, "child_reports": childRefs, "rows": all}
		b, err := json.MarshalIndent(merged, "", "  ")
		if err != nil {
			return err
		}
		p := filepath.Join(out, fmt.Sprintf("audition-manifest-%s.json", ep))
		if err := os.WriteFile(p, b, 0o600); err != nil {
			return err
		}
		if err := verifyProvenanceUnchanged(provenance, out); err != nil {
			return fmt.Errorf("benchmark provenance changed after merged audition: %w", err)
		}
		if len(all) != 60 {
			return fmt.Errorf("merged audition manifest has %d rows, want 60", len(all))
		}
		fmt.Printf("audition manifest=%s rows=%d\n", p, len(all))
		if len(childFailures) > 0 {
			return fmt.Errorf("audition child failures: %s", strings.Join(childFailures, "; "))
		}
		return nil
	}
	comparison, err := compareReports(out, ep, seed)
	if err != nil {
		return fmt.Errorf("compare reports unavailable after child runs (%s): %w", strings.Join(childFailures, "; "), err)
	}
	children, err := loadChildReferences(out, ep, seed, steps, seconds, ref, scripts)
	if err != nil {
		return fmt.Errorf("child provenance mismatch: %w", err)
	}
	for name, child := range children {
		if err := validateRunProvenance(provenance, runProvenance{Snapshot: child.Snapshot, Parent: child.Parent, WorkingTreeSHA256: child.WorkingTreeSHA256}); err != nil {
			return fmt.Errorf("%s child provenance inheritance: %w", name, err)
		}
	}
	if err := validateChildReferencePair(children["v3"], children["v4.1"]); err != nil {
		return fmt.Errorf("child condition/provenance mismatch: %w", err)
	}
	condition := children["v3"]
	adoptionPassed := combinedAdoptionPassed(comparison, childFailures)
	combined := map[string]any{"version": "working-tree", "generated_at": time.Now().UTC().Format(time.RFC3339), "ep": ep, "seed": seed, "adoption": map[string]any{"passed": adoptionPassed, "comparison_diagnostics_excluded": true}, "snapshot": provenance.Snapshot, "parent": provenance.Parent, "working_tree_sha256": provenance.WorkingTreeSHA256, "steps": condition.Steps, "seconds": condition.Seconds, "cfg": condition.CFG, "duration_scale": condition.DurationScale, "input_sha256": condition.InputSHA256, "child_reports": children, "v3_report": children["v3"].Path, "v4_report": children["v4.1"].Path, "comparison_diagnostics": comparison, "child_failures": childFailures}
	b, err := json.MarshalIndent(combined, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(out, fmt.Sprintf("benchmark-%s-seed%d.json", ep, seed))
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	if err := verifyProvenanceUnchanged(provenance, out); err != nil {
		return fmt.Errorf("benchmark provenance changed after merged report: %w", err)
	}
	if !adoptionPassed {
		return fmt.Errorf("benchmark comparison failed; see %s", path)
	}
	return nil
}

func validateAuditionChildCondition(m auditionManifest, ep, ref string, steps int, seconds float64) error {
	if m.EP != ep {
		return fmt.Errorf("execution provider=%q want %q", m.EP, ep)
	}
	if m.Steps != steps || math.Abs(m.Seconds-seconds) > 1e-9 {
		return fmt.Errorf("steps/seconds=%d/%g want %d/%g", m.Steps, m.Seconds, steps, seconds)
	}
	if m.ReferenceWAV != ref {
		return fmt.Errorf("reference WAV=%q want %q", m.ReferenceWAV, ref)
	}
	if m.InputSHA256 != hashScripts(ttseval.Scripts()) {
		return fmt.Errorf("input hash=%q does not match current scripts", m.InputSHA256)
	}
	return nil
}

func loadChildReferences(out, ep string, seed uint32, steps int, seconds float64, ref string, scriptLimit int) (map[string]childReference, error) {
	refs := make(map[string]childReference, 2)
	for _, name := range []string{"v3", "v4.1"} {
		path := filepath.Join(out, fmt.Sprintf("benchmark-%s-%s-seed%d.json", ep, name, seed))
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var child report
		if err := json.Unmarshal(b, &child); err != nil {
			return nil, fmt.Errorf("decode %s: %w", name, err)
		}
		if child.Steps != steps || math.Abs(child.Seconds-seconds) > 1e-9 {
			return nil, fmt.Errorf("%s steps/seconds=%d/%g want %d/%g", name, child.Steps, child.Seconds, steps, seconds)
		}
		if child.Ref != ref {
			return nil, fmt.Errorf("%s reference=%q want %q", name, child.Ref, ref)
		}
		expectedScripts := selectBenchmarkScripts(scriptLimit)
		if len(child.Scripts) != len(expectedScripts) || child.InputSHA256 != hashScripts(expectedScripts) {
			return nil, fmt.Errorf("%s scripts/input hash does not match requested count %d", name, scriptLimit)
		}
		if err := validateProvenance(child.Snapshot, child.Parent, child.WorkingTreeSHA256); err != nil {
			return nil, fmt.Errorf("%s provenance: %w", name, err)
		}
		refs[name] = childReference{Path: path, SHA256: hashBytes(b), Snapshot: child.Snapshot, Parent: child.Parent, WorkingTreeSHA256: child.WorkingTreeSHA256, Steps: child.Steps, Seconds: child.Seconds, CFG: child.CFG, DurationScale: child.DurationScale, ReferenceSHA256: child.RefSHA256, InputSHA256: child.InputSHA256, ModelManifestSHA: child.ModelSHA256, ModelAssets: child.ModelAssets, ORTLibrary: child.ORTLibrary, ORTLibrarySHA256: child.ORTLibrarySHA256, EP: child.EP, GPU: child.GPU, Driver: child.Driver}
	}
	return refs, nil
}

func validateProvenance(snapshotID, parentID, workingTreeSHA string) error {
	if !regexp.MustCompile(`^[0-9a-fA-F]{40}$`).MatchString(snapshotID) {
		return fmt.Errorf("snapshot must be a 40-digit commit id, got %q", snapshotID)
	}
	if !regexp.MustCompile(`^[0-9a-fA-F]{40}$`).MatchString(parentID) {
		return fmt.Errorf("parent must be a 40-digit commit id, got %q", parentID)
	}
	if !regexp.MustCompile(`^[0-9a-fA-F]{64}$`).MatchString(workingTreeSHA) {
		return fmt.Errorf("working_tree_sha256 must be a 64-digit hash, got %q", workingTreeSHA)
	}
	return nil
}

func validateCapturedVCSIDs(snapshotID, parentID string) error {
	if err := validateProvenance(snapshotID, parentID, strings.Repeat("0", 64)); err != nil {
		// validateProvenance also checks the commit-id shape. The working-tree
		// value is deliberately a known-valid placeholder for this helper.
		return err
	}
	actualSnapshot, err := jjCommitID(snapshotID)
	if err != nil {
		return fmt.Errorf("snapshot is not a known jj commit: %w", err)
	}
	if actualSnapshot != strings.ToLower(snapshotID) {
		return fmt.Errorf("jj snapshot identity mismatch: got %s want %s", actualSnapshot, strings.ToLower(snapshotID))
	}
	actualParent, err := jjCommitID(snapshotID + "-")
	if err != nil {
		return fmt.Errorf("snapshot parent is not a known jj commit: %w", err)
	}
	if actualParent != strings.ToLower(parentID) {
		return fmt.Errorf("jj snapshot parent mismatch: got %s want %s", actualParent, strings.ToLower(parentID))
	}
	return nil
}

func jjCommitID(rev string) (string, error) {
	b, err := exec.Command("jj", "log", "-r", rev, "-T", "commit_id", "--no-graph").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(string(b)))
	}
	id := strings.ToLower(strings.TrimSpace(string(b)))
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(id) {
		return "", fmt.Errorf("invalid commit id %q", id)
	}
	return id, nil
}

func validateRunProvenance(expected, actual runProvenance) error {
	if expected != actual {
		return fmt.Errorf("inherited provenance differs: expected snapshot=%s parent=%s working_tree_sha256=%s, got snapshot=%s parent=%s working_tree_sha256=%s", expected.Snapshot, expected.Parent, expected.WorkingTreeSHA256, actual.Snapshot, actual.Parent, actual.WorkingTreeSHA256)
	}
	return nil
}

func validateChildReferencePair(a, b childReference) error {
	if a.Steps != b.Steps || math.Abs(a.Seconds-b.Seconds) > 1e-9 {
		return fmt.Errorf("steps/seconds differ: v3=%d/%g v4.1=%d/%g", a.Steps, a.Seconds, b.Steps, b.Seconds)
	}
	if a.DurationScale != b.DurationScale {
		return fmt.Errorf("duration scale differs: v3=%g v4.1=%g", a.DurationScale, b.DurationScale)
	}
	if !equalCFG(a.CFG, b.CFG) {
		return fmt.Errorf("CFG differs: v3=%v v4.1=%v", a.CFG, b.CFG)
	}
	for field, values := range map[string][2]string{
		"snapshot":            {a.Snapshot, b.Snapshot},
		"parent":              {a.Parent, b.Parent},
		"working_tree_sha256": {a.WorkingTreeSHA256, b.WorkingTreeSHA256},
		"reference_sha256":    {a.ReferenceSHA256, b.ReferenceSHA256},
		"input_sha256":        {a.InputSHA256, b.InputSHA256},
		"execution_provider":  {a.EP, b.EP},
		"ort_library_sha256":  {a.ORTLibrarySHA256, b.ORTLibrarySHA256},
	} {
		if values[0] != values[1] {
			return fmt.Errorf("%s differs: %q vs %q", field, values[0], values[1])
		}
	}
	return nil
}

func validateProvenanceInvocation(singleModel string, childMode bool, snapshot, parent, workingTree string) error {
	provided := snapshot != "" || parent != "" || workingTree != ""
	if childMode {
		if singleModel == "" {
			return errors.New("provenance-child requires single-model child mode")
		}
		if !provided {
			return errors.New("provenance-child requires parent-captured provenance")
		}
		return nil
	}
	if provided {
		return errors.New("external provenance is accepted only for a parent-launched child")
	}
	return nil
}

func equalCFG(a, b map[string]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func flattenAssets(nested map[string]map[string]string) map[string]string {
	for _, assets := range nested {
		return assets
	}
	return map[string]string{}
}

func compareReports(out, ep string, seed uint32) (map[string]any, error) {
	load := func(name string) (report, error) {
		var r report
		b, err := os.ReadFile(filepath.Join(out, fmt.Sprintf("benchmark-%s-%s-seed%d.json", ep, name, seed)))
		if err != nil {
			return r, err
		}
		err = json.Unmarshal(b, &r)
		return r, err
	}
	v3, err := load("v3")
	if err != nil {
		return nil, err
	}
	v4, err := load("v4.1")
	if err != nil {
		return nil, err
	}
	p95 := func(r report) (float64, bool) {
		xs := []float64{}
		deadlineOK := true
		for _, x := range r.Rows {
			if x.Kind == "talk" && x.Repeat == 0 {
				if x.Error != "" {
					return 0, false
				}
				xs = append(xs, x.ElapsedMS)
				deadlineOK = deadlineOK && x.DeadlineMet
			}
		}
		if len(xs) == 0 {
			return 0, false
		}
		for i := 1; i < len(xs); i++ {
			for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
				xs[j], xs[j-1] = xs[j-1], xs[j]
			}
		}
		return nearestRankP95(xs), deadlineOK
	}
	v3p, v3Deadline := p95(v3)
	v4p, v4Deadline := p95(v4)
	ratio := 0.0
	if v3p > 0 {
		ratio = v4p / v3p
	}
	v3Gate := validateReportGates(v3)
	v4Gate := validateReportGates(v4)
	// p95 ratio, v3 deadline, and VRAM observations are retained as raw
	// comparison diagnostics. The v4.1 adoption decision only requires both
	// formal result sets, successful generation, and the v4.1 deadline.
	adoptionFailed := v3p == 0 || v4p == 0 || !v4Deadline || v3Gate != nil || v4Gate != nil
	return map[string]any{
		"diagnostic_kind":        "v3-v4.1-comparison",
		"v3_p95_ms":              v3p,
		"v4_p95_ms":              v4p,
		"v4_v3_p95_ratio":        ratio,
		"p95_ratio_limit":        1.2,
		"limit":                  1.2,
		"p95_ratio_within_limit": v3p > 0 && ratio <= 1.2,
		"p95_ratio_gate":         "diagnostic_only",
		"v3_deadline_met":        v3Deadline,
		"v4_deadline_met":        v4Deadline,
		"v3_gate_error":          errorText(v3Gate),
		"v4_gate_error":          errorText(v4Gate),
		"vram_gate":              "diagnostic_only",
		"adoption_failed":        adoptionFailed,
	}, nil
}

func nearestRankP95(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	idx := int(math.Ceil(0.95*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type auditionRow struct {
	File            string `json:"file"`
	SHA256          string `json:"sha256"`
	Text            string `json:"text"`
	ExpectedReading string `json:"expected_reading"`
	Model           string `json:"model"`
	Seed            uint32 `json:"seed"`
	Error           string `json:"error,omitempty"`
}

func runAudition(v3, v4, label, ep, ref string, steps int, seconds float64, outDir, seedText string, provenance runProvenance) error {
	var seeds []uint32
	for _, part := range strings.Split(seedText, ",") {
		n, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32)
		if err != nil {
			return fmt.Errorf("invalid audition seed %q: %w", part, err)
		}
		seeds = append(seeds, uint32(n))
	}
	if len(seeds) != 3 || !(containsSeed(seeds, 0) && containsSeed(seeds, 1) && containsSeed(seeds, 2)) {
		return errors.New("REQ-09 requires the exact unique seed set {0,1,2}")
	}
	scripts := ttseval.Scripts()
	rows := make([]auditionRow, 0, 60)
	for _, model := range []struct{ name, path string }{{"v3", v3}, {"v4.1", v4}} {
		if strings.TrimSpace(model.path) == "" {
			continue
		}
		for _, seed := range seeds {
			cfg := benchmarkConfig(model.path, ref, seconds, steps, seed, ep, generation.ResolveORTLibraryPathForEP(ep))
			for _, script := range scripts {
				p := filepath.Join(outDir, fmt.Sprintf("audition-%s-seed%d-script%s.wav", model.name, seed, script.ID))
				x := auditionRow{File: p, Text: script.Text, ExpectedReading: script.ExpectedReading, Model: model.name, Seed: seed}
				tmp, tempErr := os.MkdirTemp("", "fm-radio-audition-")
				if tempErr != nil {
					x.Error = tempErr.Error()
					rows = append(rows, x)
					continue
				}
				cfg.Irodori.RefWAV = ref
				wav, err := localtts.New().SynthesizeWav(context.Background(), cfg, script.Text)
				_ = os.RemoveAll(tmp)
				if err != nil {
					x.Error = err.Error()
				} else if err = os.WriteFile(p, wav, 0o600); err != nil {
					x.Error = err.Error()
				} else {
					x.SHA256 = hashBytes(wav)
				}
				rows = append(rows, x)
			}
		}
	}
	assets := map[string]map[string]string{}
	if v3 != "" {
		assets["v3"], _ = hashModelAssets(v3)
	}
	if v4 != "" {
		assets["v4.1"], _ = hashModelAssets(v4)
	}
	b, err := json.MarshalIndent(map[string]any{"version": "working-tree", "generated_at": time.Now().UTC().Format(time.RFC3339), "ep": ep, "reference_wav": ref, "reference_sha256": hashFile(ref), "snapshot": provenance.Snapshot, "parent": provenance.Parent, "working_tree_sha256": provenance.WorkingTreeSHA256, "steps": steps, "seconds": seconds, "cfg": map[string]float64{"text": 3, "caption": 3, "speaker": 5}, "duration_scale": 1, "model": label, "model_assets": assets, "input_sha256": hashScripts(scripts), "ort_library": generation.ResolveORTLibraryPathForEP(ep), "ort_library_sha256": hashFile(generation.ResolveORTLibraryPathForEP(ep)), "gpu": gpuInfo(), "driver": driverInfo(), "rows": rows}, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(outDir, fmt.Sprintf("audition-manifest-%s-%s.json", ep, label))
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	if err := verifyProvenanceUnchanged(provenance, outDir); err != nil {
		return fmt.Errorf("audition provenance changed during child run: %w", err)
	}
	failed := 0
	for _, x := range rows {
		if x.Error != "" {
			failed++
		}
	}
	fmt.Printf("audition manifest=%s rows=%d failed=%d\n", path, len(rows), failed)
	if len(rows) != 30 || failed > 0 {
		return fmt.Errorf("audition set incomplete: rows=%d failed=%d", len(rows), failed)
	}
	return nil
}

func containsSeed(seeds []uint32, want uint32) bool {
	for _, seed := range seeds {
		if seed == want {
			return true
		}
	}
	return false
}

type vramObservation struct {
	Peak     *int64
	Status   string
	Reason   string
	Source   string
	GPUIndex string
	GPUUUID  string
}

type talkObservation struct {
	started, preflightStart, preflightEnd, loadStart, loaded, loadEnd, closeStart, closeEnd time.Time
	combineStart, combineEnd                                                                time.Time
	gapCount                                                                                int
	sentenceStarts                                                                          []time.Time
	sentenceEnds                                                                            []time.Time
	vram                                                                                    []vramObservation
	vramCursors                                                                             []vramSamplerCursor
	finalVRAM                                                                               *vramObservation
	events                                                                                  []pipeline.RuntimeEvent
}

func benchmarkConfig(model, ref string, seconds float64, steps int, seed uint32, ep, lib string) domain.AppConfig {
	return domain.AppConfig{Irodori: domain.IrodoriConfig{ModelDir: model, RefWAV: ref, NarratorDir: filepath.Dir(ref), Seconds: seconds, NumSteps: steps, SeedMode: "fixed", FixedSeed: seed, CfgText: 3, CfgCaption: 3, CfgSpeaker: 5, DurationScale: 1}, LocalInference: domain.LocalInferenceConfig{ORTLibraryPath: lib, ExecutionProvider: ep}}
}

func measureTalk(ctx context.Context, cfg domain.AppConfig, model, ep string, script ttseval.Script, outDir string, deadline float64) ([]row, error) {
	tmp, err := os.MkdirTemp("", "fm-radio-benchmark-talk-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	obs := talkObservation{started: time.Now()}
	var sampler *vramSampler
	observer := func(event pipeline.RuntimeEvent) {
		now := time.Now()
		obs.events = append(obs.events, event)
		switch event {
		case pipeline.EventPreflightStart:
			obs.preflightStart = now
		case pipeline.EventPreflightEnd:
			obs.preflightEnd = now
		case pipeline.EventLoadStart:
			obs.loadStart = now
			// Start WDDM polling before the first inference so the initial CIM
			// query latency does not erase a short sentence's sample window.
			if sampler == nil {
				sampler = newVRAMSampler(ep)
			}
		case pipeline.EventLoaded:
			obs.loaded = now
		case pipeline.EventLoadEnd:
			obs.loadEnd = now
		case pipeline.EventSentenceStart:
			obs.sentenceStarts = append(obs.sentenceStarts, now)
		case pipeline.EventInferenceStart:
			if sampler == nil {
				sampler = newVRAMSampler(ep)
			}
			obs.vramCursors = append(obs.vramCursors, sampler.cursor())
		case pipeline.EventInferenceEnd:
			if sampler != nil {
				cursor := vramSamplerCursor{}
				if len(obs.vram) < len(obs.vramCursors) {
					cursor = obs.vramCursors[len(obs.vram)]
				}
				obs.vram = append(obs.vram, sampler.snapshotSince(cursor))
			}
		case pipeline.EventSentenceEnd:
			obs.sentenceEnds = append(obs.sentenceEnds, now)
		case pipeline.EventCloseStart:
			obs.closeStart = now
		case pipeline.EventCloseEnd:
			obs.closeEnd = now
		case pipeline.EventCombineStart:
			obs.combineStart = now
		case pipeline.EventCombineEnd:
			obs.combineEnd = now
		case pipeline.EventGapStart:
			obs.gapCount++
		}
	}
	wav, synthErr := localtts.New().SynthesizeWavWithObserver(ctx, cfg, script.Text, observer)
	if sampler != nil {
		finalVRAM := sampler.stop()
		obs.finalVRAM = &finalVRAM
	}
	end := time.Now()
	if err := validateTalkObservation(obs, len(splitSentences(script.Text))); err != nil {
		if synthErr == nil {
			synthErr = err
		}
	}
	x := row{Model: model, EP: ep, Seed: cfg.Irodori.FixedSeed, ScriptID: script.ID, Kind: "talk", TextRunes: len([]rune(script.Text)), ElapsedMS: ms(obs.started, end), DeadlineMS: deadline}
	x.PreflightMS = ms(obs.preflightStart, obs.preflightEnd)
	if !obs.loadStart.IsZero() {
		x.LoadMS = ms(obs.loadStart, obs.loadEnd)
	}
	x.ColdLoadMS = x.LoadMS
	x.CloseMS = ms(obs.closeStart, obs.closeEnd)
	x.CombineMS, x.GapCount = ms(obs.combineStart, obs.combineEnd), obs.gapCount
	x.DeadlineMet = synthErr == nil && x.ElapsedMS <= deadline
	if obs.finalVRAM != nil {
		x.PeakVRAMMiB, x.VRAMStatus, x.VRAMReason, x.VRAMSource, x.GPUIndex, x.GPUUUID = observationFields(*obs.finalVRAM)
	} else {
		x.PeakVRAMMiB, x.VRAMStatus, x.VRAMReason, x.VRAMSource, x.GPUIndex, x.GPUUUID = mergeVRAM(obs.vram)
	}
	rows := []row{x}
	for i, start := range obs.sentenceStarts {
		endAt := end
		if i < len(obs.sentenceEnds) {
			endAt = obs.sentenceEnds[i]
		}
		s := row{Model: model, EP: ep, Seed: cfg.Irodori.FixedSeed, ScriptID: script.ID, Kind: "sentence", Repeat: i + 1, TextRunes: len([]rune(splitSentences(script.Text)[min(i, len(splitSentences(script.Text))-1)])), ElapsedMS: ms(start, endAt), DeadlineMS: deadline, DeadlineMet: ms(start, endAt) <= deadline}
		if i < len(obs.vram) {
			s.PeakVRAMMiB, s.VRAMStatus, s.VRAMReason = obs.vram[i].Peak, obs.vram[i].Status, obs.vram[i].Reason
			s.VRAMSource, s.GPUIndex, s.GPUUUID = obs.vram[i].Source, obs.vram[i].GPUIndex, obs.vram[i].GPUUUID
		}
		rows = append(rows, s)
	}
	if synthErr != nil {
		rows[0].Error = synthErr.Error()
		return rows, synthErr
	}
	decoded, err := audiofmt.DecodeWavPCM16(wav)
	if err != nil {
		rows[0].Error = err.Error()
		return rows, err
	}
	rows[0].Frames, rows[0].SampleRate = len(decoded.PCM)/2, decoded.SampleRate
	if decoded.SampleRate > 0 {
		rows[0].RTF = (rows[0].ElapsedMS / 1000) / (float64(rows[0].Frames) / float64(decoded.SampleRate))
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			rows[0].Error = err.Error()
			return rows, err
		}
		p := filepath.Join(outDir, fmt.Sprintf("%s-script%s-seed%d.wav", model, script.ID, cfg.Irodori.FixedSeed))
		if err := os.WriteFile(p, wav, 0o600); err != nil {
			rows[0].Error = err.Error()
			return rows, err
		}
		rows[0].Output, rows[0].SHA256 = p, hashBytes(wav)
	}
	return rows, nil
}

// validateTalkObservation proves the event trace has disjoint lifecycle
// boundaries.  It is intentionally independent of elapsed-time thresholds.
func validateTalkObservation(obs talkObservation, sentenceCount int) error {
	if obs.preflightStart.IsZero() || obs.preflightEnd.IsZero() || obs.preflightEnd.Before(obs.preflightStart) {
		return errors.New("missing/invalid preflight span")
	}
	if obs.loadStart.IsZero() || obs.loadEnd.IsZero() || obs.loadEnd.Before(obs.loadStart) {
		return errors.New("missing/invalid runtime load span")
	}
	if obs.preflightEnd.After(obs.loadStart) {
		return errors.New("preflight overlaps runtime load")
	}
	if len(obs.sentenceStarts) != sentenceCount || len(obs.sentenceEnds) != sentenceCount {
		return fmt.Errorf("sentence boundary count=%d/%d want %d", len(obs.sentenceStarts), len(obs.sentenceEnds), sentenceCount)
	}
	for i := range obs.sentenceStarts {
		if obs.sentenceEnds[i].Before(obs.sentenceStarts[i]) {
			return fmt.Errorf("sentence %d end precedes start", i+1)
		}
	}
	if obs.combineStart.IsZero() || obs.combineEnd.IsZero() || obs.combineEnd.Before(obs.combineStart) {
		return errors.New("missing/invalid combine span")
	}
	if obs.combineStart.Before(obs.sentenceEnds[len(obs.sentenceEnds)-1]) {
		return errors.New("combine overlaps sentence")
	}
	if obs.closeStart.IsZero() || obs.closeEnd.IsZero() || obs.closeEnd.Before(obs.closeStart) {
		return errors.New("missing/invalid close span")
	}
	if obs.closeStart.Before(obs.combineEnd) {
		return errors.New("close overlaps combine")
	}
	if obs.gapCount != sentenceCount-1 {
		return fmt.Errorf("gap count=%d want %d", obs.gapCount, sentenceCount-1)
	}
	return nil
}

func ms(start, end time.Time) float64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return float64(end.Sub(start).Microseconds()) / 1000
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func summarizeSteady(_ map[string]any, model string, rows []row) map[string]any {
	first, last := rows, rows
	if len(rows) > 5 {
		first, last = rows[:5], rows[len(rows)-5:]
	}
	median := func(xs []row) float64 {
		v := make([]float64, 0, len(xs))
		for _, x := range xs {
			if x.Error == "" {
				v = append(v, x.ElapsedMS)
			}
		}
		if len(v) == 0 {
			return 0
		}
		for i := 1; i < len(v); i++ {
			for j := i; j > 0 && v[j] < v[j-1]; j-- {
				v[j], v[j-1] = v[j-1], v[j]
			}
		}
		return v[len(v)/2]
	}
	vr := func(xs []row) (int64, int64, int64, int64, bool) {
		var vals []int64
		for _, x := range xs {
			if x.PeakVRAMMiB != nil {
				vals = append(vals, *x.PeakVRAMMiB)
			}
		}
		if len(vals) == 0 || len(vals) != len(xs) {
			return 0, 0, 0, 0, false
		}
		for i := 1; i < len(vals); i++ {
			for j := i; j > 0 && vals[j] < vals[j-1]; j-- {
				vals[j], vals[j-1] = vals[j-1], vals[j]
			}
		}
		minV, maxV := vals[0], vals[len(vals)-1]
		return vals[len(vals)/2], minV, maxV, maxV - minV, true
	}
	firstMedian, _, _, _, firstOK := vr(first)
	_, lastMin, lastMax, lastRange, lastOK := vr(last)
	threshold := int64(0)
	if firstOK {
		threshold = firstMedian + 512
	}
	return map[string]any{"model": model, "repeats": len(rows), "first5_median_ms": median(first), "last5_median_ms": median(last), "first5_vram_median_mib": firstMedian, "last5_vram_min_mib": lastMin, "last5_vram_max_mib": lastMax, "last5_vram_range_mib": lastRange, "vram_limit_mib": threshold, "vram_within_512mib": firstOK && lastOK && lastMax <= threshold, "vram_range_diagnostic_mib": lastRange, "vram_samples_available": firstOK && lastOK, "vram_evaluation": "diagnostic_only", "first5": first, "last5": last}
}

func validateReportGates(r report) error {
	var failures []string
	var talks, steady []row
	for _, x := range r.Rows {
		// Every generated phase is part of the same adoption decision. In
		// particular, sentence rows must not be silently ignored when their
		// parent Talk row happened to be recorded successfully.
		if x.Error != "" {
			failures = append(failures, x.Kind+" error="+x.Error)
		}
		switch x.Kind {
		case "talk":
			if x.Repeat == 0 {
				talks = append(talks, x)
			}
		case "steady":
			steady = append(steady, x)
		}
	}
	if len(talks) != 10 {
		failures = append(failures, fmt.Sprintf("talk rows=%d want 10", len(talks)))
	}
	if len(steady) != 20 {
		failures = append(failures, fmt.Sprintf("steady rows=%d want 20", len(steady)))
	}
	check := func(name string, xs []row) {
		for _, x := range xs {
			if r.ModelName != "v3" && !x.DeadlineMet {
				failures = append(failures, name+" deadline miss "+x.ScriptID)
			}
		}
	}
	check("talk", talks)
	check("steady", steady)
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func combinedAdoptionPassed(comparison map[string]any, childFailures []string) bool {
	adoptionFailed, ok := comparison["adoption_failed"].(bool)
	return ok && !adoptionFailed && len(childFailures) == 0
}

func adoptionAssessment(r report, gateErr error) map[string]any {
	return map[string]any{
		"passed":                      gateErr == nil,
		"gate_error":                  errorText(gateErr),
		"formal_counts_required":      "talk=10, steady=20",
		"v4_deadline_required":        r.ModelName != "v3",
		"comparison_diagnostics_only": true,
		"vram_status":                 vramAssessmentStatus(r),
		"p95_ratio_status":            "diagnostic_only",
	}
}

func vramAssessmentStatus(r report) string {
	if r.Steady == nil {
		return "unavailable"
	}
	if available, ok := r.Steady["vram_samples_available"].(bool); ok && available {
		return "diagnostic"
	}
	return "unavailable"
}

func writeCSV(path string, r report) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"model", "ep", "seed", "script_id", "kind", "repeat", "text_runes", "elapsed_ms", "cold_load_ms", "preflight_ms", "load_ms", "close_ms", "combine_ms", "gap_count", "rtf", "deadline_ms", "deadline_met", "peak_vram_mib", "vram_status", "vram_source", "gpu_index", "gpu_uuid", "vram_reason", "output", "sha256", "sample_rate", "frames", "error", "snapshot", "parent", "working_tree_sha256", "steps", "seconds", "cfg_text", "cfg_caption", "cfg_speaker", "duration_scale", "reference_sha256", "model_manifest_sha256", "input_sha256", "ort_library_sha256", "gpu", "driver"}); err != nil {
		return err
	}
	for _, x := range r.Rows {
		vram := ""
		if x.PeakVRAMMiB != nil {
			vram = strconv.FormatInt(*x.PeakVRAMMiB, 10)
		}
		if err := w.Write([]string{x.Model, x.EP, strconv.FormatUint(uint64(x.Seed), 10), x.ScriptID, x.Kind, strconv.Itoa(x.Repeat), strconv.Itoa(x.TextRunes), fmt.Sprintf("%.3f", x.ElapsedMS), fmt.Sprintf("%.3f", x.ColdLoadMS), fmt.Sprintf("%.3f", x.PreflightMS), fmt.Sprintf("%.3f", x.LoadMS), fmt.Sprintf("%.3f", x.CloseMS), fmt.Sprintf("%.3f", x.CombineMS), strconv.Itoa(x.GapCount), fmt.Sprintf("%.5f", x.RTF), fmt.Sprintf("%.3f", x.DeadlineMS), strconv.FormatBool(x.DeadlineMet), vram, x.VRAMStatus, x.VRAMSource, x.GPUIndex, x.GPUUUID, x.VRAMReason, x.Output, x.SHA256, strconv.Itoa(x.SampleRate), strconv.Itoa(x.Frames), x.Error, r.Snapshot, r.Parent, r.WorkingTreeSHA256, strconv.Itoa(r.Steps), strconv.FormatFloat(r.Seconds, 'f', -1, 64), strconv.FormatFloat(r.CFG["text"], 'f', -1, 64), strconv.FormatFloat(r.CFG["caption"], 'f', -1, 64), strconv.FormatFloat(r.CFG["speaker"], 'f', -1, 64), strconv.FormatFloat(r.DurationScale, 'f', -1, 64), r.RefSHA256, r.ModelSHA256, r.InputSHA256, r.ORTLibrarySHA256, r.GPU, r.Driver}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func splitSentences(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if t := strings.TrimSpace(b.String()); t != "" {
			out = append(out, t)
		}
		b.Reset()
	}
	for _, r := range s {
		b.WriteRune(r)
		if strings.ContainsRune("。！？!?", r) {
			flush()
		}
	}
	flush()
	return out
}
func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func hostname() string          { h, _ := os.Hostname(); return h }

type vramSampler struct {
	done     chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	values   []int64
	reason   string
	source   string
	gpuIndex string
	gpuUUID  string
	wddm     *wddmProcessMemoryCollector
	ep       string
}

type vramSamplerCursor struct {
	values int
	wddm   int
}

func newVRAMSampler(ep ...string) *vramSampler {
	s := &vramSampler{done: make(chan struct{})}
	if len(ep) > 0 {
		s.ep = ep[0]
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.sample()
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.sample()
			case <-s.done:
				return
			}
		}
	}()
	return s
}
func (s *vramSampler) sample() {
	s.mu.Lock()
	if s.wddm != nil {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	cmd := exec.Command("nvidia-smi", "--query-compute-apps=pid,gpu_uuid,used_memory", "--format=csv,noheader,nounits")
	b, err := cmd.CombinedOutput()
	if err != nil {
		s.mu.Lock()
		if s.reason == "" {
			s.reason = strings.TrimSpace(string(b))
			if s.reason == "" {
				s.reason = err.Error()
			}
		}
		s.mu.Unlock()
		return
	}
	m, uuid, ok, reason := parseComputeAppsWithDevice(string(b), os.Getpid())
	s.mu.Lock()
	if ok {
		s.values = append(s.values, m)
		s.source = "process"
		if s.gpuUUID == "" {
			s.gpuUUID = uuid
		}
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if runtime.GOOS == "windows" && s.ep == "cuda" {
		if s.startWDDMProcessSampler(uuid, reason) {
			return
		}
	}
	// Device-total is not process memory. Keep it out of formal VRAM rows so a
	// missing process sample fails closed on every platform.
	s.mu.Lock()
	if s.reason == "" {
		s.reason = reason
	}
	s.mu.Unlock()
}

func (s *vramSampler) cursor() vramSamplerCursor {
	s.mu.Lock()
	wddm, values := s.wddm, len(s.values)
	s.mu.Unlock()
	c := vramSamplerCursor{values: values}
	if wddm != nil {
		c.wddm = wddm.Snapshot().SampleCount
	}
	return c
}

func (s *vramSampler) startWDDMProcessSampler(uuid, nvidiaReason string) bool {
	s.mu.Lock()
	if s.wddm != nil {
		s.mu.Unlock()
		return true
	}
	index := s.gpuIndex
	if index == "" {
		index, s.gpuUUID = gpuIdentity()
	}
	if uuid != "" && s.gpuUUID == "" {
		s.gpuUUID = uuid
	}
	gpuUUID := s.gpuUUID
	s.mu.Unlock()
	luid, err := queryWDDMAdapterLUID(index)
	if err != nil {
		s.mu.Lock()
		if s.reason == "" {
			s.reason = nvidiaReason + "; WDDM adapter LUID unavailable: " + err.Error()
		}
		s.mu.Unlock()
		return false
	}
	target := gpuProcessMemoryTarget{PID: os.Getpid(), GPUIndex: index, AdapterLUID: luid, GPUUUID: gpuUUID, Interval: defaultGPUProcessMemoryInterval}
	collector := newWDDMProcessMemoryCollector(target)
	if err := collector.Start(); err != nil {
		s.mu.Lock()
		if s.reason == "" {
			s.reason = nvidiaReason + "; WDDM process collector unavailable: " + err.Error()
		}
		s.mu.Unlock()
		return false
	}
	s.mu.Lock()
	if s.wddm == nil {
		s.wddm = collector
		s.source = "wddm-process-dedicated"
	}
	s.mu.Unlock()
	return true
}

func (s *vramSampler) snapshot() vramObservation {
	return s.snapshotSince(vramSamplerCursor{})
}

func (s *vramSampler) snapshotSince(cursor vramSamplerCursor) vramObservation {
	s.mu.Lock()
	wddm := s.wddm
	if wddm == nil {
		if cursor.values < 0 || cursor.values > len(s.values) {
			cursor.values = len(s.values)
		}
		values := append([]int64(nil), s.values[cursor.values:]...)
		reason, source, gpuIndex, gpuUUID := s.reason, s.source, s.gpuIndex, s.gpuUUID
		s.mu.Unlock()
		if len(values) == 0 {
			return vramObservation{Status: "unavailable", Reason: "no process VRAM sample in interval; " + reason}
		}
		peak := values[0]
		for _, v := range values[1:] {
			if v > peak {
				peak = v
			}
		}
		return vramObservation{Peak: &peak, Status: "available", Source: source, GPUIndex: gpuIndex, GPUUUID: gpuUUID, Reason: fmt.Sprintf("samples=%d; source=%s", len(values), source)}
	}
	s.mu.Unlock()
	obs, _ := wddm.snapshotSince(cursor.wddm)
	return wddmObservationToVRAM(obs)
}

func observationFields(x vramObservation) (*int64, string, string, string, string, string) {
	return x.Peak, x.Status, x.Reason, x.Source, x.GPUIndex, x.GPUUUID
}

func wddmObservationToVRAM(obs gpuProcessMemoryObservation) vramObservation {
	if obs.Available {
		peak := int64((obs.PeakDedicated + (1 << 20) - 1) / (1 << 20))
		return vramObservation{Peak: &peak, Status: "available", Source: "wddm-process-dedicated", GPUIndex: obs.GPUIndex, GPUUUID: obs.GPUUUID, Reason: fmt.Sprintf("pid=%d; luid=%s; samples=%d; first=%s; last=%s; DedicatedUsage peak=%d bytes", obs.PID, obs.AdapterLUID, obs.SampleCount, obs.FirstSampleUTC.Format(time.RFC3339Nano), obs.LastSampleUTC.Format(time.RFC3339Nano), obs.PeakDedicated)}
	}
	return vramObservation{Status: "unavailable", Source: "wddm-process-dedicated", GPUIndex: obs.GPUIndex, GPUUUID: obs.GPUUUID, Reason: obs.Reason}
}

func (s *vramSampler) stop() vramObservation {
	close(s.done)
	s.wg.Wait()
	s.mu.Lock()
	wddm := s.wddm
	s.mu.Unlock()
	if wddm != nil {
		obs := wddm.Stop()
		return wddmObservationToVRAM(obs)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.values) == 0 {
		reason := s.reason
		if reason == "" {
			reason = "nvidia-smi returned no matching process row"
		}
		return vramObservation{Status: "unavailable", Reason: reason}
	}
	peak := s.values[0]
	for _, v := range s.values[1:] {
		if v > peak {
			peak = v
		}
	}
	source := s.source
	if source == "" {
		source = "process"
	}
	return vramObservation{Peak: &peak, Status: "available", Source: source, GPUIndex: s.gpuIndex, GPUUUID: s.gpuUUID, Reason: fmt.Sprintf("samples=%d; source=%s", len(s.values), source)}
}
func parseComputeApps(output string, pid int) (int64, bool, string) {
	m, _, ok, reason := parseComputeAppsWithDevice(output, pid)
	return m, ok, reason
}
func parseComputeAppsWithDevice(output string, pid int) (int64, string, bool, string) {
	var total int64
	var gpuUUID string
	found := false
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			continue
		}
		p, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}
		if p != pid {
			continue
		}
		memField := fields[len(fields)-1]
		if len(fields) >= 3 {
			gpuUUID = strings.TrimSpace(fields[1])
		}
		mem := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(memField), "MiB"))
		m, err := strconv.ParseInt(mem, 10, 64)
		if err != nil || m < 0 {
			return 0, gpuUUID, false, "matching PID has invalid used_memory"
		}
		total += m
		found = true
	}
	if !found {
		return 0, gpuUUID, false, fmt.Sprintf("PID %d absent from nvidia-smi compute-app output", pid)
	}
	return total, gpuUUID, true, ""
}
func queryDeviceUsedMemory(preferredUUID string) (int64, string, string, bool, string) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=index,uuid,memory.used", "--format=csv,noheader,nounits")
	b, err := cmd.CombinedOutput()
	if err != nil {
		return 0, "", "", false, strings.TrimSpace(string(b))
	}
	var selected int64
	var selectedIndex string
	var selectedUUID string
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, ",")
		if len(f) < 3 {
			continue
		}
		index, uuid := strings.TrimSpace(f[0]), strings.TrimSpace(f[1])
		if preferredUUID != "" && uuid != preferredUUID {
			continue
		}
		v, e := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(f[2]), "MiB")), 10, 64)
		if e != nil || v < 0 {
			return 0, index, uuid, false, "device memory.used is N/A or invalid"
		}
		selected, selectedIndex, selectedUUID, found = v, index, uuid, true
		break
	}
	if !found {
		return 0, "", "", false, "matching device UUID absent from nvidia-smi device output"
	}
	return selected, selectedIndex, selectedUUID, true, ""
}
func mergeVRAM(xs []vramObservation) (*int64, string, string, string, string, string) {
	if len(xs) == 0 {
		return nil, "unavailable", "no sentence VRAM observations", "", "", ""
	}
	var peak *int64
	reason := ""
	source, gpuIndex, gpuUUID := "", "", ""
	for _, x := range xs {
		if x.Peak == nil || x.Status != "available" {
			if x.Reason != "" {
				reason = x.Reason
			}
			return nil, "unavailable", reason, x.Source, x.GPUIndex, x.GPUUUID
		}
		if x.Peak != nil && (peak == nil || *x.Peak > *peak) {
			v := *x.Peak
			peak = &v
		}
		if x.Reason != "" {
			reason = x.Reason
		}
		if source == "" {
			source = x.Source
		}
		if gpuIndex == "" {
			gpuIndex = x.GPUIndex
		}
		if gpuUUID == "" {
			gpuUUID = x.GPUUUID
		}
	}
	if peak == nil {
		return nil, "unavailable", reason, source, gpuIndex, gpuUUID
	}
	return peak, "available", reason, source, gpuIndex, gpuUUID
}
func hashFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "unavailable:" + err.Error()
	}
	return hashBytes(b)
}
func hashScripts(s []ttseval.Script) string { b, _ := json.Marshal(s); return hashBytes(b) }

func captureRunProvenance(outputDir string) (runProvenance, error) {
	snapshotID, parentID := vcsIDs()
	if err := validateCapturedVCSIDs(snapshotID, parentID); err != nil {
		return runProvenance{}, fmt.Errorf("working-tree provenance identity: %w", err)
	}
	capturedSHA, err := snapshotWorkingTreeSHA256(snapshotID, parentID, outputDir)
	if err != nil {
		return runProvenance{}, fmt.Errorf("captured snapshot working-tree provenance: %w", err)
	}
	current, err := currentRunProvenance(outputDir)
	if err != nil {
		return runProvenance{}, err
	}
	if current.Parent != parentID || current.WorkingTreeSHA256 != capturedSHA {
		return runProvenance{}, fmt.Errorf("captured snapshot does not match current working tree: snapshot=%s parent=%s captured_hash=%s current_parent=%s current_hash=%s", snapshotID, parentID, capturedSHA, current.Parent, current.WorkingTreeSHA256)
	}
	return runProvenance{Snapshot: strings.ToLower(snapshotID), Parent: strings.ToLower(parentID), WorkingTreeSHA256: capturedSHA}, nil
}

func resolveRunProvenance(snapshotID, parentID, workingTreeSHA, outputDir string) (runProvenance, error) {
	provided := snapshotID != "" || parentID != "" || workingTreeSHA != ""
	if !provided {
		return captureRunProvenance(outputDir)
	}
	if snapshotID == "" || parentID == "" || workingTreeSHA == "" {
		return runProvenance{}, errors.New("child provenance requires snapshot, parent, and working-tree hash")
	}
	if err := validateProvenance(snapshotID, parentID, workingTreeSHA); err != nil {
		return runProvenance{}, fmt.Errorf("child provenance: %w", err)
	}
	if err := validateCapturedVCSIDs(snapshotID, parentID); err != nil {
		return runProvenance{}, fmt.Errorf("child provenance identity: %w", err)
	}
	expected := runProvenance{Snapshot: strings.ToLower(snapshotID), Parent: strings.ToLower(parentID), WorkingTreeSHA256: strings.ToLower(workingTreeSHA)}
	current, err := currentRunProvenance(outputDir)
	if err != nil {
		return runProvenance{}, fmt.Errorf("child current provenance: %w", err)
	}
	if current.Parent != expected.Parent || current.WorkingTreeSHA256 != expected.WorkingTreeSHA256 {
		return runProvenance{}, fmt.Errorf("child provenance does not match current working tree: expected parent=%s hash=%s, got parent=%s hash=%s", expected.Parent, expected.WorkingTreeSHA256, current.Parent, current.WorkingTreeSHA256)
	}
	capturedSHA, err := snapshotWorkingTreeSHA256(expected.Snapshot, expected.Parent, outputDir)
	if err != nil {
		return runProvenance{}, fmt.Errorf("child captured snapshot provenance: %w", err)
	}
	if capturedSHA != expected.WorkingTreeSHA256 {
		return runProvenance{}, fmt.Errorf("child provenance hash does not match captured snapshot diff: expected=%s captured=%s", expected.WorkingTreeSHA256, capturedSHA)
	}
	return expected, nil
}

func appendProvenanceArgs(args []string, p runProvenance) []string {
	return append(args,
		"--provenance-snapshot", p.Snapshot,
		"--provenance-parent", p.Parent,
		"--provenance-working-tree-sha256", p.WorkingTreeSHA256,
	)
}

func verifyProvenanceUnchanged(expected runProvenance, outputDir string) error {
	if err := validateCapturedVCSIDs(expected.Snapshot, expected.Parent); err != nil {
		return fmt.Errorf("captured jj identity is no longer valid: %w", err)
	}
	current, err := currentRunProvenance(outputDir)
	if err != nil {
		return err
	}
	if current.Parent != expected.Parent {
		return fmt.Errorf("current jj parent changed: start=%s current=%s (snapshot=%s)", expected.Parent, current.Parent, current.Snapshot)
	}
	if current.WorkingTreeSHA256 != expected.WorkingTreeSHA256 {
		return fmt.Errorf("product working-tree hash changed: start=%s current=%s", expected.WorkingTreeSHA256, current.WorkingTreeSHA256)
	}
	capturedSHA, err := snapshotWorkingTreeSHA256(expected.Snapshot, expected.Parent, outputDir)
	if err != nil {
		return fmt.Errorf("captured snapshot working-tree provenance: %w", err)
	}
	if capturedSHA != expected.WorkingTreeSHA256 {
		return fmt.Errorf("captured snapshot diff hash changed: expected=%s captured=%s", expected.WorkingTreeSHA256, capturedSHA)
	}
	return nil
}

func currentRunProvenance(outputDir string) (runProvenance, error) {
	snapshotID, parentID := vcsIDs()
	workingTreeSHA := workingTreeSHA256(outputDir)
	if err := validateProvenance(snapshotID, parentID, workingTreeSHA); err != nil {
		return runProvenance{}, fmt.Errorf("current working-tree provenance: %w", err)
	}
	return runProvenance{Snapshot: snapshotID, Parent: parentID, WorkingTreeSHA256: workingTreeSHA}, nil
}

func vcsIDs() (string, string) {
	read := func(rev string) string {
		b, err := exec.Command("jj", "log", "-r", rev, "-T", "commit_id", "--no-graph").CombinedOutput()
		if err != nil {
			return "unavailable:" + strings.TrimSpace(string(b))
		}
		v := strings.TrimSpace(string(b))
		if !regexp.MustCompile(`^[0-9a-fA-F]{40}$`).MatchString(v) {
			return "unavailable:jj commit id is not 40 hex"
		}
		return strings.ToLower(v)
	}
	return read("@"), read("@-")
}

func workingTreeSHA256(excluded ...string) string {
	args := []string{"diff", "--git"}
	if len(excluded) > 0 {
		paths, err := changedWorkingTreePaths()
		if err != nil {
			return "unavailable:" + err.Error()
		}
		excludedPaths := make([]string, 0, len(excluded))
		for _, path := range excluded {
			if normalized, ok := workingTreeRelativePath(path); ok {
				excludedPaths = append(excludedPaths, normalized)
			}
		}
		for _, path := range paths {
			if !isOwnedBenchmarkOutputPath(path, excludedPaths) {
				args = append(args, path)
			}
		}
		if len(args) == 2 {
			return hashBytes(nil)
		}
	}
	b, err := exec.Command("jj", args...).CombinedOutput()
	if err != nil {
		return "unavailable:" + strings.TrimSpace(string(b))
	}
	return hashBytes(b)
}

func snapshotWorkingTreeSHA256(snapshotID, parentID, outputDir string) (string, error) {
	paths, err := snapshotDiffPaths(snapshotID, parentID)
	if err != nil {
		return "", err
	}
	args := []string{"diff", "--ignore-working-copy", "--git", "--from", parentID, "--to", snapshotID}
	excludedPaths := make([]string, 0, 1)
	if normalized, ok := workingTreeRelativePath(outputDir); ok {
		excludedPaths = append(excludedPaths, normalized)
	}
	for _, path := range paths {
		if !isOwnedBenchmarkOutputPath(path, excludedPaths) {
			args = append(args, path)
		}
	}
	if len(args) == 7 {
		return hashBytes(nil), nil
	}
	b, err := exec.Command("jj", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("snapshot diff: %s: %w", strings.TrimSpace(string(b)), err)
	}
	return hashBytes(b), nil
}

func snapshotDiffPaths(snapshotID, parentID string) ([]string, error) {
	b, err := exec.Command("jj", "diff", "--ignore-working-copy", "--name-only", "--from", parentID, "--to", snapshotID).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("snapshot paths: %s: %w", strings.TrimSpace(string(b)), err)
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if path := normalizeWorkingTreePath(line); path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func validateBenchmarkOutputDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("benchmark output directory must not be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("benchmark output directory: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("benchmark output directory: %w", err)
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil {
		// A different volume has no repository-relative files to exclude. It is
		// an external output location and the product hash keeps all source
		// paths visible.
		return nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		// An external temporary directory cannot affect the repository hash.
		return nil
	}
	rel = normalizeWorkingTreePath(rel)
	if rel == "build/bin" || strings.HasPrefix(rel, "build/bin/") {
		// build/bin is an existing ignored artifact area used by formal runs.
		// Product files outside the owned output patterns remain in the hash.
		return nil
	}
	if rel != "evidence" && !strings.HasPrefix(rel, "evidence/") {
		return fmt.Errorf("benchmark output directory %q is inside the repository outside evidence/; choose evidence/<run> or an external directory", path)
	}
	return nil
}

func isOwnedBenchmarkOutputPath(path string, outputRoots []string) bool {
	path = normalizeWorkingTreePath(path)
	for _, root := range outputRoots {
		root = normalizeWorkingTreePath(root)
		if root == "" || path == root || !strings.HasPrefix(path, root+"/") {
			continue
		}
		name := strings.TrimPrefix(path, root+"/")
		if strings.Contains(name, "/") {
			continue
		}
		if regexp.MustCompile(`^benchmark-[^/]+-seed[0-9]+\.(json|csv)$`).MatchString(name) ||
			regexp.MustCompile(`^audition-manifest-[^/]+\.json$`).MatchString(name) ||
			regexp.MustCompile(`^audition-(v3|v4\.1)-seed[0-9]+-script[^/]+\.wav$`).MatchString(name) ||
			regexp.MustCompile(`^(v3|v4\.1)-script[^/]+-seed[0-9]+\.wav$`).MatchString(name) {
			return true
		}
	}
	return false
}

func changedWorkingTreePaths() ([]string, error) {
	b, err := exec.Command("jj", "diff", "--name-only").CombinedOutput()
	if err != nil {
		return nil, errors.New(strings.TrimSpace(string(b)))
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if path := normalizeWorkingTreePath(line); path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func normalizeWorkingTreePath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	return strings.TrimPrefix(path, "/")
}

func workingTreeRelativePath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return normalizeWorkingTreePath(rel), true
}

func isExcludedWorkingTreePath(path string, excluded []string) bool {
	path = normalizeWorkingTreePath(path)
	for _, root := range excluded {
		root = normalizeWorkingTreePath(root)
		if root == "" {
			continue
		}
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func deadlineSource(deadline float64) string {
	if math.Abs(deadline-60000) < 1e-9 {
		return "product next-Talk scheduling budget: default StableAudio3 30s x 2 remaining BGM slots = 60000ms (gap excluded)"
	}
	return "user-specified --deadline-ms; product next-Talk scheduling budget"
}

func gpuInfo() string {
	b, err := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader").CombinedOutput()
	if err != nil {
		return "unavailable:" + strings.TrimSpace(string(b))
	}
	return strings.TrimSpace(string(b))
}
func gpuIdentity() (string, string) {
	b, err := exec.Command("nvidia-smi", "--query-gpu=index,uuid", "--format=csv,noheader").CombinedOutput()
	if err != nil {
		return "unavailable", "unavailable"
	}
	fields := strings.Split(strings.TrimSpace(strings.Split(string(b), "\n")[0]), ",")
	if len(fields) < 2 {
		return "unavailable", "unavailable"
	}
	return strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
}
func driverInfo() string {
	b, err := exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader").CombinedOutput()
	if err != nil {
		return "unavailable:" + strings.TrimSpace(string(b))
	}
	return strings.TrimSpace(string(b))
}
func hashModelAssets(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		base := strings.ToLower(filepath.Base(path))
		if base == "metadata.json" || base == "manifest.json" || base == "tokenizer.json" || ext == ".onnx" || ext == ".data" {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			out[filepath.ToSlash(rel)] = hashFile(path)
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	if len(out) == 0 {
		return out, errors.New("no metadata/tokenizer/graph assets found")
	}
	return out, nil
}
func snapshot() string {
	if v := strings.TrimSpace(os.Getenv("FM_RADIO_SNAPSHOT")); v != "" {
		return v
	}
	return "working-tree"
}
