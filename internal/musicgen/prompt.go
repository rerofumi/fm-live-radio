package musicgen

import (
	"strings"

	"fm-live-radio/internal/domain"
)

func BuildPrompt(cfg domain.AppConfig, genre string) string {
	parts := []string{strings.TrimSpace(cfg.StableAudio3.PromptBase)}
	if genre = strings.TrimSpace(genre); genre != "" {
		parts = append(parts, genre)
	}
	parts = append(parts, "instrumental", "background music", "no vocals")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}
