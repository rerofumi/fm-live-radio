// Package metadata loads and validates the metadata.json produced by
// the Irodori-TTS ONNX exporter. It records model dimensions, head
// sizes, sample rate, and the ONNX sub-graph I/O specifications needed
// by the Go runtime to build tensors with correct shapes.
package metadata

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExportInfo describes one ONNX sub-graph in the model directory.
type ExportInfo struct {
	File         string   `json:"file"`
	ExternalData bool     `json:"external_data"`
	ParamBytes   int64    `json:"param_bytes"`
	Inputs       []string `json:"inputs"`
	Outputs      []string `json:"outputs"`
}

// HeadDims records the RoPE head dimensions for each sub-model.
type HeadDims struct {
	Text    int `json:"text"`
	Speaker int `json:"speaker,omitempty"`
	Caption int `json:"caption,omitempty"`
	DiT     int `json:"dit"`
}

// ModelConfig records the architecture parameters baked into the
// checkpoint and used by the Go runtime for shape calculations.
type ModelConfig struct {
	TextDim             int    `json:"text_dim"`
	SpeakerDim          int    `json:"speaker_dim"`
	LatentDim           int    `json:"latent_dim"`
	LatentPatchSize     int    `json:"latent_patch_size"`
	TextTokenizerRepo   string `json:"text_tokenizer_repo"`
	TextAddBOS          bool   `json:"text_add_bos"`
	UseCaptionCondition bool   `json:"use_caption_condition"`
}

// Metadata is the top-level structure parsed from metadata.json.
type Metadata struct {
	Mode                    string                `json:"mode"`
	ModelConfig             ModelConfig           `json:"model_config"`
	HeadDims                HeadDims              `json:"head_dims"`
	SampleRate              int                   `json:"sample_rate"`
	HopLength               int                   `json:"hop_length"`
	PatchedLatentDim        int                   `json:"patched_latent_dim"`
	SpeakerPatchedLatentDim int                   `json:"speaker_patched_latent_dim"`
	UseDurationPredictor    bool                  `json:"use_duration_predictor"`
	DurationArchitecture    string                `json:"duration_architecture,omitempty"`
	UseCaptionCondition     bool                  `json:"use_caption_condition"`
	UseSpeakerCondition     bool                  `json:"use_speaker_condition,omitempty"`
	CaptionTokenizerRepo    string                `json:"caption_tokenizer_repo,omitempty"`
	CaptionAddBOS           *bool                 `json:"caption_add_bos,omitempty"`
	CaptionDim              int                   `json:"caption_dim,omitempty"`
	Exports                 map[string]ExportInfo `json:"exports"`
}

// Manifest describes the schema-v2 Irodori v4 export. It is intentionally
// separate from Metadata: metadata.json is the legacy v2/v3 contract and must
// continue to load unchanged.
type Manifest struct {
	SchemaVersion int                      `json:"schema_version"`
	ModelFamily   string                   `json:"model_family"`
	ModelRelease  string                   `json:"model_release"`
	Tokenizer     ManifestTokenizer        `json:"tokenizer"`
	Codec         ManifestCodec            `json:"codec"`
	Conditions    ManifestConditions       `json:"conditions"`
	Graphs        map[string]ManifestGraph `json:"graphs"`
	ExternalData  []ManifestExternalData   `json:"external_data"`
	Fixtures      ManifestFixtures         `json:"fixtures"`
}

type ManifestTokenizer struct {
	Path             string         `json:"path"`
	SHA256           string         `json:"sha256"`
	SpecialIDs       map[string]int `json:"special_ids"`
	TextMaxLength    int            `json:"text_max_length"`
	CaptionMaxLength int            `json:"caption_max_length"`
	MaxRefSeconds    float64        `json:"max_ref_seconds"`
}

type ManifestCodec struct {
	SampleRate int `json:"sample_rate"`
	HopLength  int `json:"hop_length"`
	LatentDim  int `json:"latent_dim"`
}

type ManifestConditions struct {
	TextDim          int `json:"text_dim"`
	CaptionDim       int `json:"caption_dim"`
	SpeakerDim       int `json:"speaker_dim"`
	SpeakerPatchSize int `json:"speaker_patch_size"`
	DurationAuxDim   int `json:"duration_aux_dim"`
	LatentDim        int `json:"latent_dim"`
	LatentPatchSize  int `json:"latent_patch_size"`
}

type ManifestTensor struct {
	Name  string `json:"name"`
	DType string `json:"dtype"`
	Shape []any  `json:"shape"`
}

type ManifestGraph struct {
	Path    string   `json:"path"`
	Bytes   int64    `json:"bytes"`
	SHA256  string   `json:"sha256"`
	Inputs  []string `json:"inputs"`
	Outputs []string `json:"outputs"`
	IO      struct {
		Inputs  []ManifestTensor `json:"inputs"`
		Outputs []ManifestTensor `json:"outputs"`
	} `json:"io"`
}

type ManifestExternalData struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type ManifestFixtures struct {
	Path       string   `json:"path"`
	SHA256     string   `json:"sha256"`
	Seed       uint32   `json:"seed"`
	Conditions []string `json:"conditions"`
}

var v4GraphNames = []string{"text_caption_encoder", "speaker_encoder", "duration_predictor", "dit_step", "codec_encoder", "codec_decoder"}

// LoadManifest loads and validates the schema-v2 manifest. Validation is
// deliberately strict: runtime code may only consume the six graph contract
// that was independently exported and parity-checked.
func LoadManifest(modelDir string) (*Manifest, error) {
	p := filepath.Join(modelDir, "manifest.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("manifest: read %s: %w", p, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest: parse %s: %w", p, err)
	}
	if err := m.Validate(modelDir); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) Validate(modelDir string) error {
	if m.SchemaVersion != 2 {
		return fmt.Errorf("manifest: unsupported schema_version %d (want 2)", m.SchemaVersion)
	}
	if m.ModelFamily != "irodori-v4" || m.ModelRelease == "" {
		return fmt.Errorf("manifest: unsupported model %q release %q", m.ModelFamily, m.ModelRelease)
	}
	if m.Tokenizer.Path == "" || len(m.Tokenizer.SHA256) != sha256.Size*2 || m.Tokenizer.TextMaxLength <= 0 || m.Tokenizer.CaptionMaxLength <= 0 {
		return fmt.Errorf("manifest: incomplete tokenizer contract")
	}
	if _, err := os.Stat(filepath.Join(modelDir, filepath.FromSlash(m.Tokenizer.Path))); err != nil {
		return fmt.Errorf("manifest: tokenizer %q: %w", m.Tokenizer.Path, err)
	}
	if m.Codec.SampleRate != 48000 || m.Codec.HopLength <= 0 || m.Codec.LatentDim <= 0 {
		return fmt.Errorf("manifest: invalid codec contract: sample_rate=%d hop_length=%d latent_dim=%d", m.Codec.SampleRate, m.Codec.HopLength, m.Codec.LatentDim)
	}
	if m.Conditions.TextDim != 512 || m.Conditions.CaptionDim != 512 || m.Conditions.SpeakerDim != 768 || m.Conditions.SpeakerPatchSize != 4 || m.Conditions.LatentDim != m.Codec.LatentDim || m.Conditions.LatentPatchSize != 1 || m.Conditions.DurationAuxDim != 14 {
		return fmt.Errorf("manifest: unsupported v4 condition dimensions: %+v", m.Conditions)
	}
	if len(m.Graphs) != len(v4GraphNames) {
		return fmt.Errorf("manifest: graph count %d, want %d", len(m.Graphs), len(v4GraphNames))
	}
	for _, name := range v4GraphNames {
		g, ok := m.Graphs[name]
		if !ok {
			return fmt.Errorf("manifest: missing graph %q", name)
		}
		if err := validateGraph(name, g); err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(modelDir, filepath.FromSlash(g.Path))); err != nil {
			return fmt.Errorf("manifest: graph %q asset %q: %w", name, g.Path, err)
		}
	}
	external := make(map[string]ManifestExternalData, len(m.ExternalData))
	for _, e := range m.ExternalData {
		external[e.Path] = e
	}
	for _, name := range v4GraphNames {
		g := m.Graphs[name]
		if _, ok := external[g.Path+".data"]; !ok {
			return fmt.Errorf("manifest: graph %q is missing declared external data %q", name, g.Path+".data")
		}
	}
	for _, e := range m.ExternalData {
		if e.Path == "" || e.Bytes <= 0 || len(e.SHA256) != sha256.Size*2 {
			return fmt.Errorf("manifest: invalid external data entry %+v", e)
		}
		p := filepath.Join(modelDir, filepath.FromSlash(e.Path))
		st, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("manifest: external data %q: %w", e.Path, err)
		}
		if st.Size() != e.Bytes {
			return fmt.Errorf("manifest: external data %q size=%d, want %d", e.Path, st.Size(), e.Bytes)
		}
	}
	if m.Fixtures.Path != "" {
		if len(m.Fixtures.SHA256) != sha256.Size*2 {
			return fmt.Errorf("manifest: fixtures %q has invalid sha256", m.Fixtures.Path)
		}
	}
	return nil
}

func validateGraph(name string, g ManifestGraph) error {
	if g.Path == "" || g.Bytes <= 0 || len(g.SHA256) != sha256.Size*2 || len(g.Inputs) == 0 || len(g.Outputs) == 0 || len(g.IO.Inputs) != len(g.Inputs) || len(g.IO.Outputs) != len(g.Outputs) {
		return fmt.Errorf("manifest: graph %q has incomplete I/O contract", name)
	}
	for i, n := range g.Inputs {
		if n == "" || g.IO.Inputs[i].Name != n || !validDType(g.IO.Inputs[i].DType) {
			return fmt.Errorf("manifest: graph %q input %d name/type mismatch", name, i)
		}
	}
	for i, n := range g.Outputs {
		if n == "" || g.IO.Outputs[i].Name != n || !validDType(g.IO.Outputs[i].DType) {
			return fmt.Errorf("manifest: graph %q output %d name/type mismatch", name, i)
		}
	}
	expected := expectedGraphIO[name]
	if !sameStrings(g.Inputs, expected[0]) || !sameStrings(g.Outputs, expected[1]) {
		return fmt.Errorf("manifest: graph %q has unexpected named I/O: inputs=%v outputs=%v", name, g.Inputs, g.Outputs)
	}
	actual := append(append([]ManifestTensor{}, g.IO.Inputs...), g.IO.Outputs...)
	expectedTensors, ok := expectedGraphTensors[name]
	if !ok || len(expectedTensors) != len(actual) {
		return fmt.Errorf("manifest: graph %q tensor contract is incomplete", name)
	}
	for i, t := range actual {
		if !sameShape(t.Shape, expectedTensors[i].shape) {
			return fmt.Errorf("manifest: graph %q tensor %q shape=%v, want %v", name, t.Name, t.Shape, expectedTensors[i].shape)
		}
		if strings.ToUpper(t.DType) != expectedTensors[i].dtype {
			return fmt.Errorf("manifest: graph %q tensor %q dtype=%q, want %q", name, t.Name, t.DType, expectedTensors[i].dtype)
		}
	}
	return nil
}

type expectedTensor struct {
	dtype string
	shape []any
}

var expectedGraphTensors = map[string][]expectedTensor{
	"text_caption_encoder": {{"INT64", []any{"batch", "text_seq"}}, {"BOOL", []any{"batch", "text_seq"}}, {"INT64", []any{"batch", "caption_seq"}}, {"BOOL", []any{"batch", "caption_seq"}}, {"FLOAT", []any{"batch", "text_seq", float64(512)}}, {"FLOAT", []any{"batch", "caption_seq", float64(512)}}},
	"speaker_encoder":      {{"FLOAT", []any{"batch", "ref_seq", float64(32)}}, {"BOOL", []any{"batch", "ref_seq"}}, {"FLOAT", []any{"batch", "speaker_seq", float64(768)}}, {"BOOL", []any{"batch", "speaker_seq"}}},
	"duration_predictor":   {{"FLOAT", []any{"batch", "text_seq", float64(512)}}, {"BOOL", []any{"batch", "text_seq"}}, {"FLOAT", []any{"batch", "speaker_seq", float64(768)}}, {"BOOL", []any{"batch", "speaker_seq"}}, {"FLOAT", []any{"batch", "caption_seq", float64(512)}}, {"BOOL", []any{"batch", "caption_seq"}}, {"FLOAT", []any{"batch", float64(14)}}, {"BOOL", []any{"batch"}}, {"BOOL", []any{"batch"}}, {"FLOAT", []any{"batch"}}},
	"dit_step":             {{"FLOAT", []any{"batch", "latent_seq", float64(32)}}, {"FLOAT", []any{"batch"}}, {"FLOAT", []any{"batch", "text_seq", float64(512)}}, {"BOOL", []any{"batch", "text_seq"}}, {"FLOAT", []any{"batch", "speaker_seq", float64(768)}}, {"BOOL", []any{"batch", "speaker_seq"}}, {"FLOAT", []any{"batch", "caption_seq", float64(512)}}, {"BOOL", []any{"batch", "caption_seq"}}, {"BOOL", []any{"batch", "latent_seq"}}, {"FLOAT", []any{"batch", "latent_seq", float64(32)}}},
	"codec_encoder":        {{"FLOAT", []any{"batch", float64(1), "samples"}}, {"FLOAT", []any{"batch", "latent_seq", "Transposelatent_dim_2"}}},
	"codec_decoder":        {{"FLOAT", []any{"batch", "latent_seq", float64(32)}}, {"FLOAT", []any{"batch", float64(1), "samples"}}},
}

func sameShape(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if fmt.Sprint(a[i]) != fmt.Sprint(b[i]) {
			return false
		}
	}
	return true
}

var expectedGraphIO = map[string][2][]string{
	"text_caption_encoder": {{"text_input_ids", "text_mask", "caption_input_ids", "caption_mask"}, {"text_state", "caption_state"}},
	"speaker_encoder":      {{"ref_latent", "ref_mask"}, {"speaker_state", "speaker_mask"}},
	"duration_predictor":   {{"text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "duration_features", "has_speaker", "has_caption"}, {"log_frames"}},
	"dit_step":             {{"x_t", "t", "text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "latent_mask"}, {"v_pred"}},
	"codec_encoder":        {{"waveform"}, {"latent"}},
	"codec_decoder":        {{"latent"}, {"waveform"}},
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validDType(v string) bool {
	switch strings.ToUpper(v) {
	case "FLOAT", "FLOAT32", "INT64", "BOOL":
		return true
	default:
		return false
	}
}

// VerifyHashes is intentionally opt-in: loading a model validates graph
// contract, file presence, and declared sizes; callers doing a preflight can
// additionally request SHA-256 verification without changing legacy startup.
func (m *Manifest) VerifyHashes(modelDir string) error {
	verify := func(rel, expected string) error {
		if expected == "" {
			return nil
		}
		f, err := os.Open(filepath.Join(modelDir, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		if got := fmt.Sprintf("%x", h.Sum(nil)); !strings.EqualFold(got, expected) {
			return fmt.Errorf("manifest: sha256 mismatch for %q: got %s want %s", rel, got, expected)
		}
		return nil
	}
	if err := verify(m.Tokenizer.Path, m.Tokenizer.SHA256); err != nil {
		return err
	}
	for _, g := range m.Graphs {
		if err := verify(g.Path, g.SHA256); err != nil {
			return err
		}
	}
	for _, e := range m.ExternalData {
		if err := verify(e.Path, e.SHA256); err != nil {
			return err
		}
	}
	return nil
}

// VerifyFixture validates the optional, verification-only value fixture.
// Product bundle loading and graph preflight intentionally do not call this:
// the fixture is a development/acceptance asset and is not part of the
// shipped bundle. Parity callers must opt in and therefore get an explicit
// missing/hash/provenance failure.
func (m *Manifest) VerifyFixture(modelDir, fixturePath string) ([]byte, error) {
	if fixturePath == "" {
		fixturePath = filepath.Join(modelDir, filepath.FromSlash(m.Fixtures.Path))
	} else if !filepath.IsAbs(fixturePath) {
		candidate := filepath.Join(modelDir, filepath.FromSlash(fixturePath))
		if _, statErr := os.Stat(candidate); statErr == nil {
			fixturePath = candidate
		}
	}
	if m.Fixtures.Path == "" || m.Fixtures.SHA256 == "" {
		return nil, fmt.Errorf("manifest: verification fixture is not declared")
	}
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("manifest: fixture %q: %w", fixturePath, err)
	}
	h := sha256.Sum256(data)
	got := fmt.Sprintf("%x", h[:])
	if !strings.EqualFold(got, m.Fixtures.SHA256) {
		return nil, fmt.Errorf("manifest: fixture sha256 mismatch for %q: got %s want %s", fixturePath, got, m.Fixtures.SHA256)
	}
	var doc struct {
		Provenance map[string]any `json:"provenance"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("manifest: fixture parse: %w", err)
	}
	// Provenance is deliberately checked here, rather than by normal bundle
	// validation, so an official fixture cannot silently become an arbitrary
	// value dump while product startup remains fixture-independent.
	required := []string{
		"source_repository", "source_revision", "source_tree_sha256",
		"model_revision", "model_file_sha256", "tokenizer_revision",
		"tokenizer_file_sha256", "codec_repository", "codec_weights_revision",
		"codec_code_revision", "codec_weights_file_sha256", "lock_path",
		"lock_sha256", "generator_path", "generator_sha256",
	}
	if len(doc.Provenance) == 0 {
		return nil, fmt.Errorf("manifest: fixture provenance is missing")
	}
	for _, key := range required {
		value, ok := doc.Provenance[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("manifest: fixture provenance %q is missing", key)
		}
	}
	for _, key := range []string{"source_tree_sha256", "model_file_sha256", "tokenizer_file_sha256", "codec_weights_file_sha256", "lock_sha256", "generator_sha256"} {
		value := doc.Provenance[key].(string)
		if len(value) != sha256.Size*2 {
			return nil, fmt.Errorf("manifest: fixture provenance %q has invalid sha256", key)
		}
	}
	return data, nil
}

// Defaults returns sensible defaults that align with the v2-VoiceDesign
// checkpoint when a metadata.json field is missing or zero.
func Defaults() *Metadata {
	trueVal := true
	return &Metadata{
		Mode: "caption",
		ModelConfig: ModelConfig{
			TextDim:             1280,
			SpeakerDim:          1280,
			LatentDim:           128,
			LatentPatchSize:     1,
			TextAddBOS:          true,
			UseCaptionCondition: true,
		},
		HeadDims: HeadDims{
			Text:    128,
			Speaker: 128,
			DiT:     128,
		},
		SampleRate:              48000,
		HopLength:               1920,
		PatchedLatentDim:        32,
		SpeakerPatchedLatentDim: 32,
		UseCaptionCondition:     true,
		CaptionAddBOS:           &trueVal,
		CaptionDim:              1280,
	}
}

// Load reads metadata.json from the given model directory, merges it
// with Defaults for any zero-valued fields, and validates consistency.
func Load(modelDir string) (*Metadata, error) {
	p := filepath.Join(modelDir, "metadata.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("metadata: read %s: %w", p, err)
	}
	md := Defaults()
	if err := json.Unmarshal(data, md); err != nil {
		return nil, fmt.Errorf("metadata: parse %s: %w", p, err)
	}
	if err := md.Validate(); err != nil {
		return nil, err
	}
	return md, nil
}

// Validate checks the metadata for internal consistency and that all
// required exports for the declared mode are present.
func (m *Metadata) Validate() error {
	if m.SampleRate <= 0 {
		return fmt.Errorf("metadata: sample_rate must be positive, got %d", m.SampleRate)
	}
	if m.HopLength <= 0 {
		return fmt.Errorf("metadata: hop_length must be positive, got %d", m.HopLength)
	}
	if m.PatchedLatentDim <= 0 {
		return fmt.Errorf("metadata: patched_latent_dim must be positive, got %d", m.PatchedLatentDim)
	}
	if m.HeadDims.Text <= 0 {
		return fmt.Errorf("metadata: head_dims.text must be positive, got %d", m.HeadDims.Text)
	}
	if m.HeadDims.DiT <= 0 {
		return fmt.Errorf("metadata: head_dims.dit must be positive, got %d", m.HeadDims.DiT)
	}
	required := []string{"text_encoder", "dit_step", "dacvae_decoder"}
	if m.Mode == "caption" || m.UseCaptionCondition {
		required = append(required, "caption_encoder")
	}
	if m.Mode == "speaker" || m.UseSpeakerCondition {
		required = append(required, "speaker_encoder")
		required = append(required, "dacvae_encoder")
	}
	if m.UseDurationPredictor {
		required = append(required, "duration_predictor")
	}
	for _, name := range required {
		ei, ok := m.Exports[name]
		if !ok {
			return fmt.Errorf("metadata: missing required export %q", name)
		}
		if ei.File == "" {
			return fmt.Errorf("metadata: export %q has empty file field", name)
		}
	}
	return nil
}

// FilePath returns the full path to the ONNX file for the named export.
func (m *Metadata) FilePath(modelDir, exportName string) string {
	ei, ok := m.Exports[exportName]
	if !ok {
		return ""
	}
	return filepath.Join(modelDir, ei.File)
}

// EffectivePatchedLatentDim returns the stored
// patched_latent_dim, falling back to latent_dim * latent_patch_size.
func (m *Metadata) EffectivePatchedLatentDim() int {
	if m.PatchedLatentDim > 0 {
		return m.PatchedLatentDim
	}
	patch := m.ModelConfig.LatentPatchSize
	if patch <= 0 {
		patch = 1
	}
	return m.ModelConfig.LatentDim * patch
}

// IsCaptionMode returns true when the model uses caption conditioning.
func (m *Metadata) IsCaptionMode() bool {
	return m.Mode == "caption" || m.UseCaptionCondition
}

// CaptionAddBOSSafe returns whether the caption tokenizer should prepend
// BOS. Returns true by default when caption_add_bos is not set.
func (m *Metadata) CaptionAddBOSSafe() bool {
	if m.CaptionAddBOS != nil {
		return *m.CaptionAddBOS
	}
	return true
}
