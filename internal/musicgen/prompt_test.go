package musicgen

import (
	"strings"
	"testing"

	"fm-live-radio/internal/domain"
)

func TestBuildPromptIncludesGenreAndPromptBase(t *testing.T) {
	cfg := domain.AppConfig{
		StableAudio3: domain.StableAudio3Config{
			PromptBase: "instrumental background music for a radio show",
			Genre:      "smooth jazz",
		},
	}
	got := BuildPrompt(cfg)
	if !strings.HasPrefix(got, "smooth jazz ensemble feel,") {
		t.Errorf("expected prompt to start with selected genre description, got %q", got)
	}
	for _, want := range []string{"warm electric piano", "brushed drums", "relaxed sophisticated groove"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected prompt to include smooth jazz descriptor %q, got %q", want, got)
		}
	}
	if !strings.Contains(got, "instrumental background music for a radio show") {
		t.Errorf("expected prompt to retain promptBase, got %q", got)
	}
	if !strings.Contains(got, "no vocals") {
		t.Errorf("expected prompt to retain no vocals, got %q", got)
	}
}

func TestBuildPromptNormalizesInvalidGenreToDefault(t *testing.T) {
	cfg := domain.AppConfig{
		StableAudio3: domain.StableAudio3Config{
			PromptBase: "instrumental background music for a radio show",
			Genre:      "j-pop",
		},
	}
	got := BuildPrompt(cfg)
	if !strings.HasPrefix(got, "chill lo-fi hip hop texture,") {
		t.Errorf("expected invalid genre to normalize to default, got %q", got)
	}
	if !strings.Contains(got, "dusty drums") {
		t.Errorf("expected default lo-fi descriptor, got %q", got)
	}
}

func TestBuildPromptFallsBackWhenGenreEmpty(t *testing.T) {
	cfg := domain.AppConfig{
		StableAudio3: domain.StableAudio3Config{
			PromptBase: "instrumental background music for a radio show",
		},
	}
	got := BuildPrompt(cfg)
	if !strings.HasPrefix(got, "chill lo-fi hip hop texture,") {
		t.Errorf("expected empty genre to fall back to default, got %q", got)
	}
}

func TestSelectedGenreReturnsNormalized(t *testing.T) {
	cfg := domain.AppConfig{
		StableAudio3: domain.StableAudio3Config{Genre: "  Ambient Music "},
	}
	if got := SelectedGenre(cfg); got != "ambient music" {
		t.Errorf("expected normalized genre, got %q", got)
	}
}

func TestGenrePromptDescriptionByGenre(t *testing.T) {
	cases := []struct {
		genre string
		want  []string
	}{
		{"chill lo-fi", []string{"chill lo-fi hip hop texture", "dusty drums", "mellow keys"}},
		{"smooth jazz", []string{"smooth jazz ensemble feel", "warm electric piano", "brushed drums"}},
		{"minimal electronica", []string{"minimal electronic composition", "sparse synth patterns", "restrained bass"}},
		{"ambient music", []string{"ambient soundscape", "slow evolving pads", "no strong beat"}},
	}

	for _, c := range cases {
		got := GenrePromptDescription(c.genre)
		for _, want := range c.want {
			if !strings.Contains(got, want) {
				t.Errorf("GenrePromptDescription(%q) missing %q in %q", c.genre, want, got)
			}
		}
	}
}

func TestGenrePromptDescriptionNormalizesUnknownGenre(t *testing.T) {
	got := GenrePromptDescription("unknown")
	if !strings.Contains(got, "chill lo-fi hip hop texture") {
		t.Errorf("expected unknown genre to use default descriptor, got %q", got)
	}
}
