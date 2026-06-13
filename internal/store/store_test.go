package store

import "testing"

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
		"chill lo-fi":          true,
		"smooth jazz":          true,
		"minimal electronica": true,
		"ambient music":        true,
	}
	for _, g := range StableAudio3AllowedGenres {
		if !want[g] {
			t.Errorf("unexpected allowed genre %q", g)
		}
	}
}
