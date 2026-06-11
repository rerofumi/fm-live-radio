// Package metadata loads and validates the metadata.json produced by
// the Irodori-TTS ONNX exporter. It records model dimensions, head
// sizes, sample rate, and the ONNX sub-graph I/O specifications needed
// by the Go runtime to build tensors with correct shapes.
package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
