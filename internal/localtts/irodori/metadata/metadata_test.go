package metadata

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestVerifyHashesRejectsSizePreservingMutation(t *testing.T) {
	dir := t.TempDir()
	tokenizer := []byte("tokenizer")
	graph := []byte("graph")
	external := []byte("external")
	for name, data := range map[string][]byte{"tokenizer.json": tokenizer, "graph.onnx": graph, "graph.onnx.data": external} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	hash := func(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
	m := &Manifest{
		Tokenizer: ManifestTokenizer{Path: "tokenizer.json", SHA256: hash(tokenizer)},
		Graphs: map[string]ManifestGraph{
			"test": {Path: "graph.onnx", SHA256: hash(graph)},
		},
		ExternalData: []ManifestExternalData{{Path: "graph.onnx.data", Bytes: int64(len(external)), SHA256: hash(external)}},
	}
	if err := m.VerifyHashes(dir); err != nil {
		t.Fatal(err)
	}
	// Keep the size unchanged while changing content: size-only preflight must
	// not accept this bundle.
	if err := os.WriteFile(filepath.Join(dir, "graph.onnx"), []byte("grapH"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.VerifyHashes(dir); err == nil {
		t.Fatal("expected size-preserving graph mutation to fail hash verification")
	}
}

func TestV4ManifestContract(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "model", "irodori-v4.1")
	m, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if m.SchemaVersion != 2 || len(m.Graphs) != 6 {
		t.Fatalf("unexpected manifest: schema=%d graphs=%d", m.SchemaVersion, len(m.Graphs))
	}
	for _, name := range v4GraphNames {
		g := m.Graphs[name]
		if len(g.Inputs) != len(g.IO.Inputs) || len(g.Outputs) != len(g.IO.Outputs) {
			t.Fatalf("%s I/O metadata mismatch", name)
		}
	}
}

func TestV4ManifestRejectsUnknownGraphContract(t *testing.T) {
	m := &Manifest{SchemaVersion: 2, ModelFamily: "irodori-v4", ModelRelease: "test", Tokenizer: ManifestTokenizer{Path: "tokenizer/tokenizer.json", TextMaxLength: 256, CaptionMaxLength: 512}, Codec: ManifestCodec{SampleRate: 48000, HopLength: 1920, LatentDim: 32}, Conditions: ManifestConditions{TextDim: 512, CaptionDim: 512, SpeakerDim: 768, SpeakerPatchSize: 4, DurationAuxDim: 14, LatentDim: 32, LatentPatchSize: 1}, Graphs: map[string]ManifestGraph{}}
	for _, name := range v4GraphNames {
		m.Graphs[name] = ManifestGraph{Path: name + ".onnx", Inputs: expectedGraphIO[name][0], Outputs: expectedGraphIO[name][1], IO: struct {
			Inputs  []ManifestTensor `json:"inputs"`
			Outputs []ManifestTensor `json:"outputs"`
		}{Inputs: make([]ManifestTensor, len(expectedGraphIO[name][0])), Outputs: make([]ManifestTensor, len(expectedGraphIO[name][1]))}}
	}
	m.Graphs["dit_step"] = m.Graphs["dit_step"]
	m.Graphs["dit_step"] = ManifestGraph{Path: "dit_step.onnx", Inputs: []string{"bad"}, Outputs: []string{"v_pred"}, IO: m.Graphs["dit_step"].IO}
	if err := m.Validate(t.TempDir()); err == nil {
		t.Fatal("expected malformed graph contract to be rejected")
	}
}

func TestV4ManifestRejectsGraphMutationFromNormalBundle(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "model", "irodori-v4.1")
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	// Start from the complete normal manifest and break only one graph I/O
	// contract; this must fail before any hash/external-data preflight.
	g := m.Graphs["dit_step"]
	g.Inputs[0] = "wrong_x_t"
	m.Graphs["dit_step"] = g
	err = m.Validate(root)
	if err == nil || !strings.Contains(err.Error(), `graph "dit_step"`) {
		t.Fatalf("expected mutated graph contract error, got %v", err)
	}
}

func TestV4ManifestLoadDoesNotRequireParityFixture(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "model", "irodori-v4.1")
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.Fixtures.Path = "missing/go-parity-fixture.json"
	if err := m.Validate(root); err != nil {
		t.Fatalf("normal bundle validation must not require parity fixture: %v", err)
	}
	if _, err := m.VerifyFixture(root, ""); err == nil {
		t.Fatal("parity validation must fail when the explicit fixture is missing")
	}
}

func TestV4ParityFixtureRejectsCorruptPayload(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "model", "irodori-v4.1")
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyFixture(root, path); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected corrupt fixture hash failure, got %v", err)
	}
}
