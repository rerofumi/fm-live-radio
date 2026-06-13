package musicgen

import (
	"strings"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/store"
)

func BuildPrompt(cfg domain.AppConfig) string {
	parts := []string{
		GenrePromptDescription(SelectedGenre(cfg)),
		strings.TrimSpace(cfg.StableAudio3.PromptBase),
		"instrumental",
		"background music",
		"no vocals",
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

// SelectedGenre returns the effective genre used to build the Stable Audio 3
// prompt. The value is normalized to one of store.StableAudio3AllowedGenres.
func SelectedGenre(cfg domain.AppConfig) string {
	return store.NormalizeStableAudio3Genre(cfg.StableAudio3.Genre)
}

// GenrePromptDescription expands a short UI genre value into concrete musical
// instructions for Stable Audio 3. Config still stores the short genre name.
func GenrePromptDescription(genre string) string {
	switch store.NormalizeStableAudio3Genre(genre) {
	case "smooth jazz":
		return "smooth jazz ensemble feel, warm electric piano, clean guitar or sax-like lead, brushed drums, relaxed sophisticated groove"
	case "minimal electronica":
		return "minimal electronic composition, sparse synth patterns, precise soft pulses, restrained bass, clean modern atmosphere"
	case "ambient music":
		return "ambient soundscape, slow evolving pads, airy textures, no strong beat, spacious calm immersive atmosphere"
	default:
		return "chill lo-fi hip hop texture, dusty drums, mellow keys, soft vinyl noise, warm tape saturation, relaxed late-night mood"
	}
}
