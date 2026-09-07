package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"fm-live-radio/internal/localtts/irodori/tokenizer"
)

func TestV4ConsumerTrimsBeforeTokenizerAndDuration(t *testing.T) {
	modelDir := filepath.Join("..", "..", "..", "..", "model", "irodori-v4.1")
	tok, err := tokenizer.FromFile(filepath.Join(modelDir, "tokenizer", "tokenizer.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	plain := "本文。"
	spaced := "\t  本文。  \n"
	encode := func(raw string) ([]int64, []bool, []float32) {
		text := normalizeV4SynthesisText(raw)
		ids, mask := tok.EncodePadded(text, 256)
		return ids, mask, buildDurationFeatures(text, countTrue(mask), 256, true)
	}
	plainIDs, plainMask, plainFeatures := encode(plain)
	spacedIDs, spacedMask, spacedFeatures := encode(spaced)
	if !reflect.DeepEqual(plainIDs, spacedIDs) || !reflect.DeepEqual(plainMask, spacedMask) {
		t.Fatalf("trimmed text must produce identical product tokenizer inputs: ids/mask differ")
	}
	if !reflect.DeepEqual(plainFeatures, spacedFeatures) {
		t.Fatalf("trimmed text must produce identical duration features: plain=%v spaced=%v", plainFeatures, spacedFeatures)
	}
	if got := normalizeV4SynthesisText(spaced); got != plain {
		t.Fatalf("normalizeV4SynthesisText(%q)=%q, want %q", spaced, got, plain)
	}
}

func TestV4DurationFeatureShapeAndFlags(t *testing.T) {
	f := buildDurationFeatures("東京都のニュース。", 12, 256, true)
	if len(f) != 14 {
		t.Fatalf("duration feature length=%d, want 14", len(f))
	}
	if f[13] != 1 {
		t.Fatalf("speaker flag=%v, want 1", f[13])
	}
	if f[0] <= 0 || f[0] > 1 {
		t.Fatalf("token ratio=%v", f[0])
	}
	f = buildDurationFeatures("", 0, 256, false)
	if len(f) != 14 || f[13] != 0 {
		t.Fatalf("empty duration features=%v", f)
	}
}

func TestV4DurationFramesUsesOfficialOrderAndBounds(t *testing.T) {
	const sampleRate, hop = 48000, 1920
	raw := float32(5.1037364)
	base := durationFrames(raw, 1, 0, sampleRate, hop)
	low := durationFrames(raw, 0.5, 0, sampleRate, hop)
	high := durationFrames(raw, 2, 0, sampleRate, hop)
	if !(low < base && base < high) {
		t.Fatalf("duration scale order low=%d base=%d high=%d", low, base, high)
	}
	if got := durationFrames(raw, 1, 0.1, sampleRate, hop); got != 13 {
		t.Fatalf("seconds lower clamp frames=%d, want 13", got)
	}
	if got := durationFrames(raw, 1, 31, sampleRate, hop); got != 750 {
		t.Fatalf("seconds upper clamp frames=%d, want 750", got)
	}
}

func TestV4TextNormalization(t *testing.T) {
	cases := map[string]string{
		"  ＡＢＣ！　":  "ABC!",
		"（括弧）":     "括弧",
		"[n]番号...": "番号…",
		"A――～B":    "AーB",
		"♥●◯〇":     "♡○○○",
		"…… ……":    "…… ……",
	}
	for input, want := range cases {
		got := strings.TrimSpace(normalizeV4Text(input))
		if got != want {
			t.Errorf("normalize(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestV4DurationFeaturesOfficialUnicodeCounts(t *testing.T) {
	f := buildDurationFeatures("😮‍💨😮😊𠀀あA。", 10, 256, true)
	if f[9] <= 0 || f[9] >= 1 {
		t.Fatalf("emoji feature=%v, want a capped positive ratio", f[9])
	}
	if f[11] <= 0 {
		t.Fatalf("supplementary CJK must count as kanji: features=%v", f)
	}
}

func TestV4FixtureDurationFeaturesUseProductAndRejectTamper(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "..", "model", "irodori-v4.1", "go-parity-fixture.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture parityFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.DurationFeatureText == "" || len(fixture.Conditions) != 4 {
		t.Fatalf("fixture duration schema missing: text=%q conditions=%d", fixture.DurationFeatureText, len(fixture.Conditions))
	}
	for name, condition := range fixture.Conditions {
		got, err := buildAndCompareFixtureDurationFeatures(name, fixture.DurationFeatureText, condition, 14)
		if err != nil {
			t.Fatalf("condition %s: %v", name, err)
		}
		if len(got) != 14 {
			t.Fatalf("condition %s: got %d features, want 14", name, len(got))
		}
	}
	tampered := fixture.Conditions["speaker+caption"]
	want, err := parityF32(tampered.DurationFeatures)
	if err != nil {
		t.Fatal(err)
	}
	want[0] += 0.1
	tampered.DurationFeatures.Values = mustJSON(want)
	if _, err := buildAndCompareFixtureDurationFeatures("tampered", fixture.DurationFeatureText, tampered, 14); err == nil {
		t.Fatal("tampered official duration feature must be rejected")
	}
}

func TestV4ReferenceDynamicPaddingAndPatchBoundaries(t *testing.T) {
	const hop, patch = 1920, 4
	for _, frames := range []int{4, 5, 7, 8, 9, 17} {
		for _, delta := range []int{-1, 0, 1} {
			codecFrames, err := referenceCodecFrames(frames*hop+delta, hop, patch)
			if err != nil {
				t.Fatalf("frames=%d delta=%d: %v", frames, delta, err)
			}
			wantCodec := int64(frames)
			if delta > 0 {
				wantCodec++
			}
			if codecFrames != wantCodec {
				t.Errorf("frames=%d delta=%d codec=%d want=%d", frames, delta, codecFrames, wantCodec)
			}
			if got := codecFrames - codecFrames%patch; got != int64(frames/patch*patch) && delta <= 0 {
				t.Errorf("frames=%d delta=%d patch floor=%d", frames, delta, got)
			}
		}
	}
	for _, frames := range []int{1, 3} {
		if _, err := referenceCodecFrames(frames*hop, hop, patch); err == nil {
			t.Errorf("frames=%d must be rejected", frames)
		}
	}
}
