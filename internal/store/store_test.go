package store

import (
	"path/filepath"
	"testing"

	"fm-live-radio/internal/domain"
)

func TestNormalizeStableAudio3Genre(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", StableAudio3DefaultGenre},
		{"   ", StableAudio3DefaultGenre},
		{"unknown", StableAudio3DefaultGenre},
		{"chill lo-fi", "chill lo-fi"},
		{"smooth jazz", "smooth jazz"},
		{"minimal electronica", "minimal electronica"},
		{"ambient music", "ambient music"},
		{"  Smooth Jazz  ", "smooth jazz"},
		{"AMBIENT MUSIC", "ambient music"},
	}
	for _, c := range cases {
		got := NormalizeStableAudio3Genre(c.in)
		if got != c.want {
			t.Errorf("NormalizeStableAudio3Genre(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeStableAudio3GenreHasFourEntries(t *testing.T) {
	if got, want := len(StableAudio3AllowedGenres), 4; got != want {
		t.Fatalf("expected %d allowed genres, got %d", want, got)
	}
	want := map[string]bool{
		"chill lo-fi":         true,
		"smooth jazz":         true,
		"minimal electronica": true,
		"ambient music":       true,
	}
	for _, g := range StableAudio3AllowedGenres {
		if !want[g] {
			t.Errorf("unexpected allowed genre %q", g)
		}
	}
}

func TestIrodoriV41DefaultPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(cfg.Irodori.ModelDir); got != "irodori-v4.1" {
		t.Fatalf("new configuration default=%q, want irodori-v4.1", got)
	}
	restarted, err := NewAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	afterRestart, err := restarted.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if afterRestart.Irodori.ModelDir != cfg.Irodori.ModelDir {
		t.Fatalf("v4.1 default did not survive restart: got=%q want=%q", afterRestart.Irodori.ModelDir, cfg.Irodori.ModelDir)
	}
}

func TestIrodoriExplicitV3AndArbitraryPathsPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	paths := []struct {
		name  string
		model string
	}{
		{name: "v3", model: "E:/models/irodori-v3"},
		{name: "arbitrary", model: "E:/models/custom-irodori"},
	}
	for _, tc := range paths {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewAt(filepath.Join(dir, tc.name))
			if err != nil {
				t.Fatal(err)
			}
			cfg := DefaultConfig()
			cfg.Irodori = domain.IrodoriConfig{
				ModelDir: tc.model, NarratorDir: "E:/voice/narrators", RefWAV: "E:/voice/custom.wav",
				Seconds: -1, NumSteps: 40, SeedMode: "fixed", FixedSeed: 9,
				CfgText: 3, CfgCaption: 3, CfgSpeaker: 5, DurationScale: 1,
			}
			if err := s.SaveConfig(cfg); err != nil {
				t.Fatal(err)
			}
			restarted, err := NewAt(s.BaseDir())
			if err != nil {
				t.Fatal(err)
			}
			got, err := restarted.LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Irodori.ModelDir != cfg.Irodori.ModelDir || got.Irodori.NarratorDir != cfg.Irodori.NarratorDir || got.Irodori.RefWAV != cfg.Irodori.RefWAV {
				t.Fatalf("explicit paths were not persisted: got=%+v want=%+v", got.Irodori, cfg.Irodori)
			}
		})
	}
}
