package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fm-live-radio/internal/ttseval"
)

func TestParseComputeAppsMatchesPIDAndRejectsMalformedMemory(t *testing.T) {
	got, ok, reason := parseComputeApps("123, 512\n456, 1024\n", 123)
	if !ok || got != 512 || reason != "" {
		t.Fatalf("parse valid row = %d, %v, %q", got, ok, reason)
	}
	_, ok, reason = parseComputeApps("123, N/A\n", 123)
	if ok || reason == "" {
		t.Fatalf("malformed matching row must be unavailable: ok=%v reason=%q", ok, reason)
	}
	_, ok, reason = parseComputeApps("456, 512\n", 123)
	if ok || reason == "" {
		t.Fatalf("missing PID must be unavailable: ok=%v reason=%q", ok, reason)
	}
}

func TestValidateAuditionChildConditionRejectsDiagnosticAsFormal(t *testing.T) {
	m := auditionManifest{
		EP: "cuda", ReferenceWAV: "narrator/narrator_01.wav", Steps: 2, Seconds: 0.5,
		InputSHA256: hashScripts(ttseval.Scripts()),
	}
	if err := validateAuditionChildCondition(m, "cuda", m.ReferenceWAV, 40, -1); err == nil {
		t.Fatal("diagnostic steps/seconds must not be accepted as formal merged condition")
	}
	if err := validateAuditionChildCondition(m, "cuda", m.ReferenceWAV, 2, 0.5); err != nil {
		t.Fatalf("matching child condition rejected: %v", err)
	}
}

func TestContainsSeedRequiresExactSetAtCaller(t *testing.T) {
	if !containsSeed([]uint32{0, 1, 2}, 0) || !containsSeed([]uint32{0, 1, 2}, 1) || !containsSeed([]uint32{0, 1, 2}, 2) {
		t.Fatal("expected all required seeds")
	}
	if containsSeed([]uint32{0, 1, 3}, 2) {
		t.Fatal("unexpected seed 2")
	}
}

func TestNearestRankP95UsesCeilRank(t *testing.T) {
	if got := nearestRankP95([]float64{1, 2, 3}); got != 3 {
		t.Fatalf("3-sample p95=%v, want max 3", got)
	}
	if got := nearestRankP95([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 100}); got != 100 {
		t.Fatalf("10-sample p95=%v, want max 100", got)
	}
}

func TestValidateReportGatesRejectsV4DeadlineButIgnoresVRAM(t *testing.T) {
	r := report{ModelName: "v4.1", Rows: make([]row, 0, 30), Steady: map[string]any{"vram_samples_available": true, "vram_within_512mib": true}}
	for i := 0; i < 10; i++ {
		v := int64(1000)
		r.Rows = append(r.Rows, row{Kind: "talk", ScriptID: string(rune('a' + i)), DeadlineMet: true, VRAMStatus: "available", PeakVRAMMiB: &v})
	}
	for i := 0; i < 20; i++ {
		v := int64(1000)
		r.Rows = append(r.Rows, row{Kind: "steady", Repeat: i + 1, DeadlineMet: true, VRAMStatus: "available", PeakVRAMMiB: &v})
	}
	if err := validateReportGates(r); err != nil {
		t.Fatalf("valid gate rejected: %v", err)
	}
	r.Rows[9].DeadlineMet = false
	if err := validateReportGates(r); err == nil {
		t.Fatal("deadline miss must fail")
	}
	r.Rows[9].DeadlineMet = true
	r.Rows[0].PeakVRAMMiB = nil
	if err := validateReportGates(r); err != nil {
		t.Fatalf("VRAM missing is diagnostic only: %v", err)
	}
}

func TestValidateReportGatesKeepsVramFalseAndUnavailableDiagnosticOnly(t *testing.T) {
	r := report{ModelName: "v4.1", Rows: make([]row, 0, 30), Steady: map[string]any{
		"vram_samples_available": false,
		"vram_within_512mib":     false,
	}}
	for i := 0; i < 10; i++ {
		r.Rows = append(r.Rows, row{Kind: "talk", ScriptID: string(rune('a' + i)), DeadlineMet: true, VRAMStatus: "unavailable"})
	}
	for i := 0; i < 20; i++ {
		r.Rows = append(r.Rows, row{Kind: "steady", Repeat: i + 1, DeadlineMet: true, VRAMStatus: "unavailable"})
	}
	if err := validateReportGates(r); err != nil {
		t.Fatalf("resource diagnostics alone must not fail adoption: %v", err)
	}
	assessment := adoptionAssessment(r, validateReportGates(r))
	if got, ok := assessment["passed"].(bool); !ok || !got {
		t.Fatalf("adoption assessment must pass without resource observations: %+v", assessment)
	}
	if got := assessment["vram_status"]; got != "unavailable" {
		t.Fatalf("VRAM unavailable must remain explicit: %v", got)
	}
}

func TestValidateReportGatesKeepsV3DeadlineAsComparisonDiagnostic(t *testing.T) {
	r := report{ModelName: "v3", Rows: make([]row, 0, 30)}
	for i := 0; i < 10; i++ {
		r.Rows = append(r.Rows, row{Kind: "talk", ScriptID: string(rune('a' + i)), DeadlineMet: false})
	}
	for i := 0; i < 20; i++ {
		r.Rows = append(r.Rows, row{Kind: "steady", Repeat: i + 1, DeadlineMet: false})
	}
	if err := validateReportGates(r); err != nil {
		t.Fatalf("v3 deadline is comparison diagnostic only: %v", err)
	}
}

func TestAdoptionAssessmentRejectsFormalGenerationFailure(t *testing.T) {
	r := report{ModelName: "v4.1", Rows: make([]row, 0, 30)}
	for i := 0; i < 10; i++ {
		r.Rows = append(r.Rows, row{Kind: "talk", ScriptID: string(rune('a' + i)), DeadlineMet: true})
	}
	for i := 0; i < 20; i++ {
		r.Rows = append(r.Rows, row{Kind: "steady", Repeat: i + 1, DeadlineMet: true})
	}
	r.Rows[0].Error = "synthesis failed"
	err := validateReportGates(r)
	if err == nil {
		t.Fatal("generation failure must fail adoption")
	}
	assessment := adoptionAssessment(r, err)
	if passed, _ := assessment["passed"].(bool); passed {
		t.Fatalf("failed generation was marked adopted: %+v", assessment)
	}
}

func TestValidateReportGatesRejectsSentenceGenerationFailure(t *testing.T) {
	r := report{ModelName: "v4.1", Rows: make([]row, 0, 31)}
	for i := 0; i < 10; i++ {
		r.Rows = append(r.Rows, row{Kind: "talk", ScriptID: string(rune('a' + i)), DeadlineMet: true})
	}
	for i := 0; i < 20; i++ {
		r.Rows = append(r.Rows, row{Kind: "steady", Repeat: i + 1, DeadlineMet: true})
	}
	r.Rows = append(r.Rows, row{Kind: "sentence", ScriptID: "a", Repeat: 1, DeadlineMet: true, Error: "sentence synthesis failed"})
	gateErr := validateReportGates(r)
	if gateErr == nil {
		t.Fatal("sentence generation failure must fail adoption")
	}
	assessment := adoptionAssessment(r, gateErr)
	if passed, _ := assessment["passed"].(bool); passed {
		t.Fatalf("failed sentence generation was marked adopted: %+v", assessment)
	}
}

func TestCombinedAdoptionRejectsChildFailure(t *testing.T) {
	comparison := map[string]any{"adoption_failed": false}
	if combinedAdoptionPassed(comparison, []string{"v4.1 child process: exit status 1"}) {
		t.Fatal("child process failure must fail the combined adoption decision")
	}
	if !combinedAdoptionPassed(comparison, nil) {
		t.Fatal("successful comparison without child failures must pass")
	}
}

func TestCompareReportsKeepsP95RatioDiagnosticOutsideAdoptionGate(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, elapsed float64) {
		r := report{ModelName: name, Rows: make([]row, 0, 30)}
		for i := 0; i < 10; i++ {
			r.Rows = append(r.Rows, row{Kind: "talk", ScriptID: string(rune('a' + i)), ElapsedMS: elapsed, DeadlineMet: true})
		}
		for i := 0; i < 20; i++ {
			r.Rows = append(r.Rows, row{Kind: "steady", Repeat: i + 1, ElapsedMS: elapsed, DeadlineMet: true})
		}
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "benchmark-cuda-"+name+"-seed0.json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("v3", 100)
	write("v4.1", 200)
	got, err := compareReports(dir, "cuda", 0)
	if err != nil {
		t.Fatal(err)
	}
	if failed, _ := got["adoption_failed"].(bool); failed {
		t.Fatalf("resource-independent p95 ratio must not fail adoption: %+v", got)
	}
	if within, _ := got["p95_ratio_within_limit"].(bool); within {
		t.Fatalf("p95 ratio diagnostic was turned into a success value: %+v", got)
	}
	if got["p95_ratio_gate"] != "diagnostic_only" {
		t.Fatalf("comparison/adoption policy distinction missing: %+v", got)
	}
}

func TestValidateTalkObservationRejectsOverlappingTrace(t *testing.T) {
	now := time.Now()
	valid := talkObservation{
		preflightStart: now, preflightEnd: now.Add(time.Millisecond),
		loadStart: now.Add(2 * time.Millisecond), loaded: now.Add(3 * time.Millisecond), loadEnd: now.Add(4 * time.Millisecond),
		sentenceStarts: []time.Time{now.Add(5 * time.Millisecond), now.Add(8 * time.Millisecond)},
		sentenceEnds:   []time.Time{now.Add(7 * time.Millisecond), now.Add(10 * time.Millisecond)},
		gapCount:       1, combineStart: now.Add(11 * time.Millisecond), combineEnd: now.Add(12 * time.Millisecond), closeStart: now.Add(13 * time.Millisecond), closeEnd: now.Add(14 * time.Millisecond),
	}
	if err := validateTalkObservation(valid, 2); err != nil {
		t.Fatalf("valid trace rejected: %v", err)
	}
	valid.combineStart = now.Add(9 * time.Millisecond)
	if err := validateTalkObservation(valid, 2); err == nil {
		t.Fatal("overlapping combine must fail")
	}
}

func TestValidateChildReferencePairRejectsConditionAndProvenanceMismatch(t *testing.T) {
	base := childReference{
		Snapshot: strings.Repeat("a", 40), Parent: strings.Repeat("b", 40), WorkingTreeSHA256: strings.Repeat("c", 64),
		Steps: 40, Seconds: -1, CFG: map[string]float64{"text": 3, "caption": 3, "speaker": 5}, DurationScale: 1,
		ReferenceSHA256: strings.Repeat("d", 64), InputSHA256: strings.Repeat("e", 64), EP: "cuda", ORTLibrarySHA256: strings.Repeat("f", 64),
	}
	if err := validateProvenance(base.Snapshot, base.Parent, base.WorkingTreeSHA256); err != nil {
		t.Fatalf("valid provenance rejected: %v", err)
	}
	other := base
	other.Steps = 2
	if err := validateChildReferencePair(base, other); err == nil {
		t.Fatal("condition mismatch must be rejected")
	}
	other = base
	other.WorkingTreeSHA256 = strings.Repeat("0", 64)
	if err := validateChildReferencePair(base, other); err == nil {
		t.Fatal("provenance mismatch must be rejected")
	}
	if err := validateProvenance("working-tree", base.Parent, base.WorkingTreeSHA256); err == nil {
		t.Fatal("non-commit snapshot must be rejected")
	}
}

func TestWorkingTreePathFilterExcludesOnlyOutputSubtree(t *testing.T) {
	excluded := []string{"evidence/tts-benchmark"}
	if !isExcludedWorkingTreePath("evidence/tts-benchmark/benchmark-cuda-v3-seed0.json", excluded) {
		t.Fatal("generated benchmark report must be excluded from product hash")
	}
	if isExcludedWorkingTreePath("evidence/tts-benchmark-e2e/README.md", excluded) {
		t.Fatal("similar prefix outside output subtree must remain in product hash")
	}
	if isExcludedWorkingTreePath("cmd/tts-benchmark/main.go", excluded) {
		t.Fatal("product source must remain in product hash")
	}
	if isOwnedBenchmarkOutputPath("internal/localtts/service.go", []string{"internal"}) {
		t.Fatal("product source must never be excluded merely because --out covers its parent")
	}
	if !isOwnedBenchmarkOutputPath("internal/benchmark-cuda-v3-seed0.json", []string{"internal"}) {
		t.Fatal("owned benchmark JSON should be excluded from output hash")
	}
	if isOwnedBenchmarkOutputPath("internal/README.md", []string{"internal"}) {
		t.Fatal("unowned output-directory files must remain in product hash")
	}
}

func TestValidateBenchmarkOutputDirRejectsProductSourcePath(t *testing.T) {
	if err := validateBenchmarkOutputDir("cmd/tts-benchmark"); err == nil {
		t.Fatal("output overlapping product source must be rejected")
	}
	if err := validateBenchmarkOutputDir("internal/cmd/tts-benchmark"); err == nil {
		t.Fatal("nested product output path must be rejected")
	}
	if err := validateBenchmarkOutputDir("evidence/tts-benchmark-test"); err != nil {
		t.Fatalf("evidence output rejected: %v", err)
	}
	if err := validateBenchmarkOutputDir("build/bin/acceptance/formal-benchmark-cuda"); err != nil {
		t.Fatalf("ignored build/bin output rejected: %v", err)
	}
	if err := validateBenchmarkOutputDir(`C:\Users\rero2\AppData\Local\Temp\fm-radio-formal`); err != nil {
		t.Fatalf("external output on another volume rejected: %v", err)
	}
}

func TestValidateBenchmarkLabelRejectsPathTraversal(t *testing.T) {
	for _, label := range []string{"../cmd", `..\cmd`, ".", ""} {
		if err := validateBenchmarkLabel(label); err == nil {
			t.Fatalf("unsafe model label %q accepted", label)
		}
	}
	if err := validateBenchmarkLabel("v4.1"); err != nil {
		t.Fatalf("normal model label rejected: %v", err)
	}
}

func TestValidateCapturedVCSIDsRejectsFabricatedHex(t *testing.T) {
	if err := validateCapturedVCSIDs(strings.Repeat("a", 40), strings.Repeat("b", 40)); err == nil {
		t.Fatal("fabricated commit ids must not pass provenance validation")
	}
	snapshot, parent := vcsIDs()
	if strings.HasPrefix(snapshot, "unavailable:") || strings.HasPrefix(parent, "unavailable:") {
		t.Skipf("jj identity unavailable: snapshot=%s parent=%s", snapshot, parent)
	}
	if err := validateCapturedVCSIDs(snapshot, parent); err != nil {
		t.Fatalf("actual jj commit pair rejected: %v", err)
	}
}

func TestValidateProvenanceInvocationRejectsExternalSingleModel(t *testing.T) {
	ids := []string{strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 64)}
	if err := validateProvenanceInvocation("model/irodori-v4.1", false, ids[0], ids[1], ids[2]); err == nil {
		t.Fatal("single-model mode must reject externally supplied provenance")
	}
	if err := validateProvenanceInvocation("model/irodori-v4.1", true, ids[0], ids[1], ids[2]); err != nil {
		t.Fatalf("parent-launched child provenance rejected: %v", err)
	}
}

func TestResolveRunProvenanceRejectsFabricatedWorkingTreeHash(t *testing.T) {
	snapshot, parent := vcsIDs()
	if strings.HasPrefix(snapshot, "unavailable:") || strings.HasPrefix(parent, "unavailable:") {
		t.Skipf("jj identity unavailable: snapshot=%s parent=%s", snapshot, parent)
	}
	fake := strings.Repeat("c", 64)
	if _, err := resolveRunProvenance(snapshot, parent, fake, "evidence/tts-benchmark"); err == nil {
		t.Fatal("fabricated inherited working-tree hash must be rejected against current and captured content")
	}
}

func TestCapturedSnapshotDiffMatchesCurrentWorkingTree(t *testing.T) {
	snapshot, parent := vcsIDs()
	if strings.HasPrefix(snapshot, "unavailable:") || strings.HasPrefix(parent, "unavailable:") {
		t.Skipf("jj identity unavailable: snapshot=%s parent=%s", snapshot, parent)
	}
	captured, err := snapshotWorkingTreeSHA256(snapshot, parent, "evidence/tts-benchmark")
	if err != nil {
		t.Fatalf("captured snapshot diff hash failed: %v", err)
	}
	current := workingTreeSHA256("evidence/tts-benchmark")
	if strings.HasPrefix(current, "unavailable:") {
		t.Fatalf("current working-tree hash unavailable: %s", current)
	}
	if captured != current {
		t.Fatalf("captured/current provenance hashes differ: captured=%s current=%s", captured, current)
	}
}

func TestAuditionCompletionProvenanceBoundary(t *testing.T) {
	outputDir := t.TempDir()
	provenance, err := captureRunProvenance(outputDir)
	if err != nil {
		t.Fatalf("capture provenance failed: %v", err)
	}
	if err := verifyProvenanceUnchanged(provenance, outputDir); err != nil {
		t.Fatalf("unchanged audition provenance rejected at completion: %v", err)
	}

	changedTree := provenance
	changedTree.WorkingTreeSHA256 = strings.Repeat("0", 64)
	if err := verifyProvenanceUnchanged(changedTree, outputDir); err == nil || !strings.Contains(err.Error(), "product working-tree hash changed") {
		t.Fatalf("changed audition working tree must fail at completion: %v", err)
	}

	changedParent := provenance
	changedParent.Parent = strings.Repeat("0", 40)
	if err := verifyProvenanceUnchanged(changedParent, outputDir); err == nil || !strings.Contains(err.Error(), "captured jj identity is no longer valid") {
		t.Fatalf("changed audition parent must fail at completion: %v", err)
	}
}

func TestDiagnosticBenchmarkCountsAreAllowedButFormalCountsAreSeparate(t *testing.T) {
	if !validBenchmarkCounts(1, 1) || !validBenchmarkCounts(10, 20) {
		t.Fatal("expected diagnostic and formal count pairs")
	}
	if validBenchmarkCounts(1, 20) || validBenchmarkCounts(10, 1) || validBenchmarkCounts(2, 2) {
		t.Fatal("unexpected partial count pair accepted")
	}
	r := report{Rows: []row{{Kind: "talk", ScriptID: "01"}, {Kind: "steady", Repeat: 1}}}
	if err := validateReportGates(r); err == nil || !strings.Contains(err.Error(), "want 10") || !strings.Contains(err.Error(), "want 20") {
		t.Fatalf("diagnostic report must fail formal gate, got %v", err)
	}
}

func TestAppendProvenanceArgsCarriesParentSnapshotToChild(t *testing.T) {
	p := runProvenance{Snapshot: strings.Repeat("a", 40), Parent: strings.Repeat("b", 40), WorkingTreeSHA256: strings.Repeat("c", 64)}
	args := appendProvenanceArgs([]string{"--model", "model/irodori-v3"}, p)
	want := []string{"--provenance-snapshot", p.Snapshot, "--provenance-parent", p.Parent, "--provenance-working-tree-sha256", p.WorkingTreeSHA256}
	if got := args[len(args)-len(want):]; len(got) != len(want) {
		t.Fatalf("provenance args length=%d want %d", len(got), len(want))
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("provenance arg[%d]=%q want %q", i, got[i], want[i])
			}
		}
	}
	if err := validateRunProvenance(p, p); err != nil {
		t.Fatalf("identical parent provenance rejected: %v", err)
	}
	changed := p
	changed.WorkingTreeSHA256 = strings.Repeat("d", 64)
	if err := validateRunProvenance(p, changed); err == nil {
		t.Fatal("changed product hash must be rejected")
	}
}

func TestDeadlineSourceDistinguishesProductDefault(t *testing.T) {
	if got := deadlineSource(60000); !strings.Contains(got, "30s x 2") {
		t.Fatalf("default deadline provenance=%q", got)
	}
	if got := deadlineSource(1234); !strings.Contains(got, "user-specified") {
		t.Fatalf("custom deadline provenance=%q", got)
	}
}

func TestSummarizeSteadyUsesAllFiveVRAMSamplesAnd512Boundary(t *testing.T) {
	rows := make([]row, 0, 10)
	for i := 0; i < 5; i++ {
		v := int64(1000)
		rows = append(rows, row{PeakVRAMMiB: &v})
	}
	for i := 0; i < 5; i++ {
		v := int64(1512)
		rows = append(rows, row{PeakVRAMMiB: &v})
	}
	s := summarizeSteady(nil, "v4.1", rows)
	if ok, _ := s["vram_within_512mib"].(bool); !ok {
		t.Fatal("exact +512 boundary should pass")
	}
	v := int64(1513)
	rows[9].PeakVRAMMiB = &v
	s = summarizeSteady(nil, "v4.1", rows)
	if ok, _ := s["vram_within_512mib"].(bool); ok {
		t.Fatal("+513 boundary must fail")
	}
	rows[9].PeakVRAMMiB = nil
	s = summarizeSteady(nil, "v4.1", rows)
	if ok, _ := s["vram_samples_available"].(bool); ok {
		t.Fatal("one missing last5 sample must be unverified")
	}
}

func TestSummarizeSteadyDoesNotGateOnLastFiveRange(t *testing.T) {
	rows := make([]row, 0, 10)
	for i := 0; i < 5; i++ {
		v := int64(1000)
		rows = append(rows, row{PeakVRAMMiB: &v})
	}
	for i, value := range []int64{1000, 1512, 1001, 1511, 1002} {
		v := value
		rows = append(rows, row{PeakVRAMMiB: &v, Repeat: i + 6})
	}
	s := summarizeSteady(nil, "v4.1", rows)
	if ok, _ := s["vram_within_512mib"].(bool); !ok {
		t.Fatal("last5 range is diagnostic and must not fail a max<=+512 sample")
	}
	if got, _ := s["vram_range_diagnostic_mib"].(int64); got <= 0 {
		t.Fatalf("expected diagnostic range, got %v", s["vram_range_diagnostic_mib"])
	}
}
