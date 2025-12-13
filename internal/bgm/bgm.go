package bgm

import (
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrNoTracks = errors.New("no tracks")

var supportedExt = map[string]bool{
	".mp3": true,
	".wav": true,
	".m4a": true,
}

type Track struct {
	Path  string
	Title string
}

func ListGenres(root string) ([]string, error) {
	if strings.TrimSpace(root) == "" {
		return []string{}, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	genres := make([]string, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == ".git" || name == "__MACOSX" {
			continue
		}
		if strings.HasPrefix(name, ".") {
			continue
		}
		genres = append(genres, name)
	}
	sort.Strings(genres)
	return genres, nil
}

func ListTracks(root, genre string) ([]Track, error) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(genre) == "" {
		return []Track{}, nil
	}
	dir := filepath.Join(root, genre)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	tracks := make([]Track, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !supportedExt[ext] {
			continue
		}
		p := filepath.Join(dir, e.Name())
		tracks = append(tracks, Track{Path: p, Title: strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))})
	}
	return tracks, nil
}

func PickRandomTrack(tracks []Track, lastPath string) (Track, error) {
	if len(tracks) == 0 {
		return Track{}, ErrNoTracks
	}
	if len(tracks) == 1 {
		return tracks[0], nil
	}
	// Avoid immediate repeats with limited retries.
	const maxTries = 10
	for i := 0; i < maxTries; i++ {
		t := tracks[rand.IntN(len(tracks))]
		if t.Path != lastPath {
			return t, nil
		}
	}
	// Fallback after retries
	return tracks[(int(time.Now().UnixNano())%len(tracks)+len(tracks))%len(tracks)], nil
}
