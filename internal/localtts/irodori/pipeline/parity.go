package pipeline

// parity.go provides the test-only value-bearing fixture harness used by
// cmd/tts-parity.  It deliberately enters through the product v4 runtime so
// condition batching, duration inputs, CFG, schedule/sampler, and codec
// boundaries cannot drift into a second implementation owned by the CLI.

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"fm-live-radio/internal/localtts/irodori/metadata"
	"fm-live-radio/internal/localtts/irodori/sampler"
)

const parityAtol float32 = 1e-4
const parityRtol float32 = 1e-3

type parityValue struct {
	Dtype  string          `json:"dtype"`
	Shape  []int64         `json:"shape"`
	Values json.RawMessage `json:"values"`
}

type parityFixture struct {
	Schema              int                               `json:"schema_version"`
	Source              string                            `json:"source"`
	Seed                uint32                            `json:"seed"`
	DurationFeatureText string                            `json:"duration_feature_text"`
	Graphs              map[string][]parityValue          `json:"graphs"`
	Inputs              map[string]map[string]parityValue `json:"graph_inputs"`
	Conditions          map[string]parityCondition        `json:"conditions"`
}

type parityCondition struct {
	Inputs             map[string]parityValue `json:"inputs"`
	DurationFeatures   parityValue            `json:"duration_features"`
	DurationTokenCount int                    `json:"duration_token_count"`
	DurationMaxTextLen int                    `json:"duration_max_text_len"`
	DurationHasSpeaker bool                   `json:"duration_has_speaker"`
	DurationHasCaption bool                   `json:"duration_has_caption"`
	Duration           []parityValue          `json:"duration_output"` // compatibility alias for duration_raw
	DurationRaw        []parityValue          `json:"duration_raw"`
	DurationFrames     map[string]int64       `json:"duration_frames"`
	Dit                []parityValue          `json:"dit_output"`
	Noise              parityValue            `json:"initial_noise"`
	CFG1               parityValue            `json:"trajectory_cfg1"`
	Default            parityValue            `json:"trajectory_default"`
}

// FixtureOptions controls the product-core parity inputs. Seconds is optional:
// when set, duration prediction is still checked but the final frame contract
// follows the explicit seconds branch.
type FixtureOptions struct {
	Seed          uint32
	DurationScale float64
	Seconds       float64
	Case          string
}

// CompareV4Fixture executes the official value-bearing fixture through the
// product v4 runtime.  The caller must initialize ORT with the requested EP.
func CompareV4Fixture(modelDir, fixturePath string, seed uint32) error {
	return CompareV4FixtureWithOptions(modelDir, fixturePath, FixtureOptions{Seed: seed, DurationScale: 1})
}

// CompareV4FixtureWithOptions validates the official fixture through the same
// product duration and denoising core used by synthesis.
func CompareV4FixtureWithOptions(modelDir, fixturePath string, options FixtureOptions) error {
	m, err := metadata.LoadManifest(modelDir)
	if err != nil {
		return err
	}
	data, err := m.VerifyFixture(modelDir, fixturePath)
	if err != nil {
		return fmt.Errorf("fixture verification: %w", err)
	}
	var f parityFixture
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("fixture json: %w", err)
	}
	if f.Schema != 1 || f.Source == "" || f.DurationFeatureText == "" || len(f.Graphs) != 6 || len(f.Inputs) < 6 || len(f.Conditions) != 4 {
		return fmt.Errorf("fixture is not value-bearing official fixture")
	}
	if options.Seed != f.Seed {
		return fmt.Errorf("seed %d != fixture seed %d", options.Seed, f.Seed)
	}
	if options.DurationScale <= 0 {
		options.DurationScale = 1
	}
	rawRuntime, err := loadV4Runtime(Options{ModelDir: modelDir, Seed: options.Seed, CfgText: 3, CfgSpeaker: 5, CfgCaption: 3, DurationScale: options.DurationScale, Seconds: options.Seconds})
	if err != nil {
		return err
	}
	r := rawRuntime.v4
	defer r.close()

	if err := r.compareFixtureGraphs(f); err != nil {
		return err
	}
	cases, err := fixtureCases(options.Case)
	if err != nil {
		return err
	}
	for _, name := range cases {
		c := f.Conditions[name]
		if err := r.compareDuration(name, f.DurationFeatureText, c, options); err != nil {
			return err
		}
		if err := r.compareConditionDiT(name, c.Inputs, c.Dit); err != nil {
			return err
		}
	}
	c := f.Conditions["speaker+caption"]
	if err := r.compareTrajectory("cfg=1", c, sampler.CfgConfig{ScaleText: 1, ScaleSpeaker: 1, ScaleCaption: 1, MinT: .5, MaxT: 1, UseTextCfg: true, UseSpeakerCfg: true, UseCaptionCfg: true}, c.CFG1); err != nil {
		return err
	}
	if err := r.compareTrajectory("default cfg", c, sampler.CfgConfig{ScaleText: 3, ScaleSpeaker: 5, ScaleCaption: 3, MinT: .5, MaxT: 1, UseTextCfg: true, UseSpeakerCfg: true, UseCaptionCfg: true}, c.Default); err != nil {
		return err
	}
	return nil
}

func fixtureCases(name string) ([]string, error) {
	if name == "" || name == "all" {
		return []string{"null", "speaker", "caption", "speaker+caption"}, nil
	}
	switch name {
	case "null", "speaker", "caption", "speaker+caption":
		return []string{name}, nil
	case "reference":
		return []string{"speaker"}, nil
	case "reference+caption":
		return []string{"speaker+caption"}, nil
	default:
		return nil, fmt.Errorf("unknown fixture case %q", name)
	}
}

// FixtureConditionCount is used by the CLI report so a selected case cannot
// claim that all four conditions were executed.
func FixtureConditionCount(name string) (int, error) {
	cases, err := fixtureCases(name)
	if err != nil {
		return 0, err
	}
	return len(cases), nil
}

func (r *v4Runtime) compareFixtureGraphs(f parityFixture) error {
	textIn := f.Inputs["text_caption_encoder"]
	textIDs, err := parityI64(textIn["text_input_ids"])
	if err != nil {
		return err
	}
	textMask, err := parityBool(textIn["text_mask"])
	if err != nil {
		return err
	}
	captionIDs, err := parityI64(textIn["caption_input_ids"])
	if err != nil {
		return err
	}
	captionMask, err := parityBool(textIn["caption_mask"])
	if err != nil {
		return err
	}
	textState, captionState, err := r.runTextCaption(textIDs, textMask, captionIDs, captionMask)
	if err != nil {
		return fmt.Errorf("text_caption_encoder: %w", err)
	}
	if err := compareParity("text_caption_encoder.text_state", f.Graphs["text_caption_encoder"][0], textState); err != nil {
		return err
	}
	if err := compareParity("text_caption_encoder.caption_state", f.Graphs["text_caption_encoder"][1], captionState); err != nil {
		return err
	}

	spkIn := f.Inputs["speaker_encoder"]
	refLatent, err := parityF32(spkIn["ref_latent"])
	if err != nil {
		return err
	}
	refMask, err := parityBool(spkIn["ref_mask"])
	if err != nil {
		return err
	}
	speakerState, speakerMask, err := r.runSpeaker(refLatent, refMask)
	if err != nil {
		return fmt.Errorf("speaker_encoder: %w", err)
	}
	if err := compareParity("speaker_encoder.speaker_state", f.Graphs["speaker_encoder"][0], speakerState); err != nil {
		return err
	}
	if err := compareParityBool("speaker_encoder.speaker_mask", f.Graphs["speaker_encoder"][1], speakerMask); err != nil {
		return err
	}

	codecIn := f.Inputs["codec_encoder"]["waveform"]
	waveform, err := parityF32(codecIn)
	if err != nil {
		return err
	}
	codecExpected := f.Graphs["codec_encoder"][0]
	codecShape, err := parityShape(codecExpected, 3)
	if err != nil {
		return err
	}
	codecLatent, err := r.runCodecEncoder(waveform, codecShape[1])
	if err != nil {
		return fmt.Errorf("codec_encoder: %w", err)
	}
	if err := compareParity("codec_encoder.latent", codecExpected, codecLatent); err != nil {
		return err
	}

	codecDecIn := f.Inputs["codec_decoder"]["latent"]
	latent, err := parityF32(codecDecIn)
	if err != nil {
		return err
	}
	latentShape, err := parityShape(codecDecIn, 3)
	if err != nil {
		return err
	}
	decoded, err := r.runCodecDecoder(latent, latentShape[1])
	if err != nil {
		return fmt.Errorf("codec_decoder: %w", err)
	}
	if err := compareParity("codec_decoder.waveform", f.Graphs["codec_decoder"][0], decoded); err != nil {
		return err
	}

	ditIn := f.Inputs["dit_step"]
	if err := r.compareConditionDiT("graph", ditIn, f.Graphs["dit_step"]); err != nil {
		return err
	}
	return nil
}

func (r *v4Runtime) compareDuration(name, featureText string, c parityCondition, options FixtureOptions) error {
	in := c.Inputs
	expected := c.DurationRaw
	if len(expected) == 0 {
		expected = c.Duration
	}
	features, err := r.fixtureDurationFeatures(name, featureText, c)
	if err != nil {
		return err
	}
	if in["duration_features"].Dtype != "float32" || len(in["duration_features"].Shape) != 2 || in["duration_features"].Shape[1] != int64(r.manifest.Conditions.DurationAuxDim) {
		return fmt.Errorf("condition %s duration features input: dtype/shape=%s/%v, want float32/[1 %d]", name, in["duration_features"].Dtype, in["duration_features"].Shape, r.manifest.Conditions.DurationAuxDim)
	}
	if err := compareParity("condition "+name+" duration input features", in["duration_features"], features); err != nil {
		return err
	}
	hasSpeaker, err := parityConditionFlag(in["has_speaker"], c.DurationHasSpeaker)
	if err != nil {
		return fmt.Errorf("condition %s has_speaker: %w", name, err)
	}
	hasCaption, err := parityConditionFlag(in["has_caption"], c.DurationHasCaption)
	if err != nil {
		return fmt.Errorf("condition %s has_caption: %w", name, err)
	}
	got, err := r.runDurationFixture(in, features, hasSpeaker, hasCaption)
	if err != nil {
		return fmt.Errorf("condition %s duration: %w", name, err)
	}
	if len(expected) != 1 {
		return fmt.Errorf("condition %s duration: expected %d outputs", name, len(expected))
	}
	if err := compareParity("condition "+name+" duration", expected[0], got); err != nil {
		return err
	}
	if len(got) != 1 {
		return fmt.Errorf("condition %s duration: expected one scalar", name)
	}
	// Compare all official scale expectations, not only ordering. This proves
	// raw -> expm1 -> scale -> round/clamp with the same shared product core.
	if len(c.DurationFrames) < 3 {
		return fmt.Errorf("condition %s duration: fixture is missing scale frame expectations", name)
	}
	for _, scale := range []float64{0.5, 1, 2} {
		key := strconv.FormatFloat(scale, 'g', -1, 64)
		want, ok := c.DurationFrames[key]
		if !ok {
			return fmt.Errorf("condition %s duration: fixture missing scale %s frame", name, key)
		}
		gotFrame := durationFrames(got[0], scale, 0, r.manifest.Codec.SampleRate, r.manifest.Codec.HopLength)
		if delta := gotFrame - want; delta < -1 || delta > 1 {
			return fmt.Errorf("condition %s duration: scale %s frame=%d want=%d delta=%d", name, key, gotFrame, want, delta)
		}
	}
	frame := durationFrames(got[0], options.DurationScale, options.Seconds, r.manifest.Codec.SampleRate, r.manifest.Codec.HopLength)
	if frame < 1 {
		return fmt.Errorf("condition %s duration: invalid final frame count %d", name, frame)
	}
	return nil
}

// fixtureDurationFeatures reconstructs the auxiliary duration vector through
// the product implementation. The expected vector is an independent official
// value in the fixture; it is never used as the NN input.
func (r *v4Runtime) fixtureDurationFeatures(name, text string, c parityCondition) ([]float32, error) {
	return buildAndCompareFixtureDurationFeatures(name, text, c, r.manifest.Conditions.DurationAuxDim)
}

func buildAndCompareFixtureDurationFeatures(name, text string, c parityCondition, featureDim int) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("condition %s duration feature text is missing", name)
	}
	if c.DurationTokenCount <= 0 || c.DurationMaxTextLen <= 0 {
		return nil, fmt.Errorf("condition %s duration feature metadata is invalid: token_count=%d max_text_len=%d", name, c.DurationTokenCount, c.DurationMaxTextLen)
	}
	if c.DurationFeatures.Dtype != "float32" || len(c.DurationFeatures.Shape) != 2 || c.DurationFeatures.Shape[0] != 1 || c.DurationFeatures.Shape[1] != int64(featureDim) {
		return nil, fmt.Errorf("condition %s official duration features: dtype/shape=%s/%v, want float32/[1 %d]", name, c.DurationFeatures.Dtype, c.DurationFeatures.Shape, featureDim)
	}
	features := buildDurationFeatures(text, c.DurationTokenCount, c.DurationMaxTextLen, c.DurationHasSpeaker)
	if len(features) != featureDim {
		return nil, fmt.Errorf("condition %s product duration features: got %d values, want %d", name, len(features), featureDim)
	}
	for i, value := range features {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return nil, fmt.Errorf("condition %s product duration features: nonfinite at %d", name, i)
		}
	}
	if err := compareParity("condition "+name+" product duration features", c.DurationFeatures, features); err != nil {
		return nil, err
	}
	return features, nil
}

func parityConditionFlag(v parityValue, want bool) (bool, error) {
	got, err := parityBool(v)
	if err != nil {
		return false, err
	}
	if len(got) != 1 {
		return false, fmt.Errorf("expected one value, got %d", len(got))
	}
	if got[0] != want {
		return false, fmt.Errorf("fixture=%t metadata=%t", got[0], want)
	}
	return want, nil
}

func (r *v4Runtime) runDurationFixture(in map[string]parityValue, features []float32, hasSpeaker, hasCaption bool) ([]float32, error) {
	textState, err := parityF32(in["text_state"])
	if err != nil {
		return nil, err
	}
	textMask, err := parityBool(in["text_mask"])
	if err != nil {
		return nil, err
	}
	speakerState, err := parityF32(in["speaker_state"])
	if err != nil {
		return nil, err
	}
	speakerMask, err := parityBool(in["speaker_mask"])
	if err != nil {
		return nil, err
	}
	captionState, err := parityF32(in["caption_state"])
	if err != nil {
		return nil, err
	}
	captionMask, err := parityBool(in["caption_mask"])
	if err != nil {
		return nil, err
	}
	return r.runDurationRaw(durationInput{
		textState: textState, textMask: textMask,
		speakerState: speakerState, speakerMask: speakerMask,
		captionState: captionState, captionMask: captionMask,
		features: features, hasSpeaker: hasSpeaker, hasCaption: hasCaption,
	})
}

func (r *v4Runtime) compareConditionDiT(name string, in map[string]parityValue, expected []parityValue) error {
	// Condition fixtures store the shared states; the product comparator injects
	// the same fixed zero latent, t=.5, and all-true latent mask used by the
	// official condition graph fixture.
	if len(in["x_t"].Values) == 0 {
		latentLen := int64(8)
		in = cloneParityInputs(in)
		in["x_t"] = parityValue{Dtype: "float32", Shape: []int64{1, latentLen, int64(r.manifest.Conditions.LatentDim)}, Values: mustJSON(make([]float32, latentLen*int64(r.manifest.Conditions.LatentDim)))}
		in["t"] = parityValue{Dtype: "float32", Shape: []int64{1}, Values: mustJSON([]float32{.5})}
		in["latent_mask"] = parityValue{Dtype: "bool", Shape: []int64{1, latentLen}, Values: mustJSON([]bool{true, true, true, true, true, true, true, true})}
	}
	x, err := parityF32(in["x_t"])
	if err != nil {
		return err
	}
	textState, err := parityF32(in["text_state"])
	if err != nil {
		return err
	}
	textMask, err := parityBool(in["text_mask"])
	if err != nil {
		return err
	}
	speakerState, err := parityF32(in["speaker_state"])
	if err != nil {
		return err
	}
	speakerMask, err := parityBool(in["speaker_mask"])
	if err != nil {
		return err
	}
	captionState, err := parityF32(in["caption_state"])
	if err != nil {
		return err
	}
	captionMask, err := parityBool(in["caption_mask"])
	if err != nil {
		return err
	}
	latentShape, err := parityShape(in["x_t"], 3)
	if err != nil {
		return err
	}
	t, err := parityF32(in["t"])
	if err != nil {
		return err
	}
	got, err := r.runDiTBatch(x, textState, textMask, speakerState, speakerMask, captionState, captionMask, latentShape[1], []sampler.CfgBundle{sampler.BundleCond}, t[0])
	if err != nil {
		return fmt.Errorf("condition %s dit: %w", name, err)
	}
	if len(expected) != 1 {
		return fmt.Errorf("condition %s dit: expected one output", name)
	}
	return compareParity("condition "+name+" dit", expected[0], got)
}

func (r *v4Runtime) compareTrajectory(name string, c parityCondition, cfg sampler.CfgConfig, expected parityValue) error {
	x, err := parityF32(c.Noise)
	if err != nil {
		return err
	}
	shape, err := parityShape(c.Noise, 3)
	if err != nil {
		return err
	}
	textState, err := parityF32(c.Inputs["text_state"])
	if err != nil {
		return err
	}
	textMask, err := parityBool(c.Inputs["text_mask"])
	if err != nil {
		return err
	}
	speakerState, err := parityF32(c.Inputs["speaker_state"])
	if err != nil {
		return err
	}
	speakerMask, err := parityBool(c.Inputs["speaker_mask"])
	if err != nil {
		return err
	}
	captionState, err := parityF32(c.Inputs["caption_state"])
	if err != nil {
		return err
	}
	captionMask, err := parityBool(c.Inputs["caption_mask"])
	if err != nil {
		return err
	}
	if err := r.runDenoising(x, textState, textMask, speakerState, speakerMask, captionState, captionMask, shape[1], cfg, defaultV4Steps); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return compareParity(name, expected, x)
}

func parityShape(v parityValue, want int) ([]int64, error) {
	if len(v.Shape) != want {
		return nil, fmt.Errorf("shape=%v want rank %d", v.Shape, want)
	}
	return v.Shape, nil
}

func cloneParityInputs(in map[string]parityValue) map[string]parityValue {
	out := make(map[string]parityValue, len(in)+3)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func parityF32(v parityValue) ([]float32, error) {
	if len(v.Values) == 0 {
		return nil, fmt.Errorf("fixture value is missing (dtype=%q shape=%v)", v.Dtype, v.Shape)
	}
	var out []float32
	if err := json.Unmarshal(v.Values, &out); err != nil {
		return nil, fmt.Errorf("decode %s shape=%v: %w", v.Dtype, v.Shape, err)
	}
	return out, nil
}
func parityI64(v parityValue) ([]int64, error) {
	if len(v.Values) == 0 {
		return nil, fmt.Errorf("fixture value is missing (dtype=%q shape=%v)", v.Dtype, v.Shape)
	}
	var out []int64
	if err := json.Unmarshal(v.Values, &out); err != nil {
		return nil, fmt.Errorf("decode %s shape=%v: %w", v.Dtype, v.Shape, err)
	}
	return out, nil
}
func parityBool(v parityValue) ([]bool, error) {
	if len(v.Values) == 0 {
		return nil, fmt.Errorf("fixture value is missing (dtype=%q shape=%v)", v.Dtype, v.Shape)
	}
	var out []bool
	if err := json.Unmarshal(v.Values, &out); err != nil {
		return nil, fmt.Errorf("decode %s shape=%v: %w", v.Dtype, v.Shape, err)
	}
	return out, nil
}
func compareParity(name string, want parityValue, got []float32) error {
	expected, err := parityF32(want)
	if err != nil {
		return err
	}
	if len(expected) != len(got) {
		return fmt.Errorf("%s: length %d != %d", name, len(got), len(expected))
	}
	for i := range expected {
		if math.IsNaN(float64(got[i])) || math.IsInf(float64(got[i]), 0) {
			return fmt.Errorf("%s: nonfinite at %d", name, i)
		}
		d := float32(math.Abs(float64(expected[i] - got[i])))
		if d > parityAtol+parityRtol*float32(math.Abs(float64(expected[i]))) {
			return fmt.Errorf("%s: numeric mismatch at %d abs=%g", name, i, d)
		}
	}
	return nil
}

func compareParityBool(name string, want parityValue, got []bool) error {
	expected, err := parityBool(want)
	if err != nil {
		return err
	}
	if len(expected) != len(got) {
		return fmt.Errorf("%s: length %d != %d", name, len(got), len(expected))
	}
	for i := range expected {
		if expected[i] != got[i] {
			return fmt.Errorf("%s: bool mismatch at %d", name, i)
		}
	}
	return nil
}
