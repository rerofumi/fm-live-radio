package musicgen

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fm-live-radio/internal/fileprotect"
	"fm-live-radio/internal/store"
)

type CacheEntry struct {
	Path    string
	ModTime time.Time
}

func ListCache(dir string) ([]CacheEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]CacheEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".wav") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, CacheEntry{
			Path:    filepath.Join(dir, entry.Name()),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime.Before(out[j].ModTime)
	})
	return out, nil
}

func PickFallback(dir string, genre ...string) (string, error) {
	g := ""
	if len(genre) > 0 {
		g = store.NormalizeStableAudio3Genre(genre[0])
	}
	return pickFallback(dir, g)
}

// PickFallbackForGenre returns the newest-safe fallback whose generated
// provenance matches genre. WAV files without a valid sidecar are ignored.
func PickFallbackForGenre(dir, genre string) (string, error) {
	return pickFallback(dir, store.NormalizeStableAudio3Genre(genre))
}

func pickFallback(dir, genre string) (string, error) {
	files, err := ListCache(dir)
	if err != nil {
		return "", err
	}
	if genre != "" {
		filtered := files[:0]
		for _, f := range files {
			if metadata, ok := readMetadata(f.Path); ok && strings.EqualFold(metadata.Genre, genre) {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}
	if len(files) == 0 {
		return "", os.ErrNotExist
	}
	return files[len(files)/2].Path, nil
}

// TrimCache keeps all protected paths and their sidecars. Multiple keep paths
// are accepted for compatibility with the old single keepPath call.
func TrimCache(dir string, limit int, keepPaths ...string) error {
	if limit <= 0 {
		limit = 20
	}
	files, err := ListCache(dir)
	if err != nil {
		return err
	}
	for len(files) > limit {
		idx := -1
		var target CacheEntry
		for i, candidate := range files {
			if fileprotect.Protected(candidate.Path) {
				continue
			}
			keep := false
			for _, path := range keepPaths {
				if path != "" && strings.EqualFold(filepath.Clean(candidate.Path), filepath.Clean(path)) {
					keep = true
					break
				}
			}
			if !keep {
				idx, target = i, candidate
				break
			}
		}
		if idx < 0 {
			break
		}
		files = append(files[:idx], files[idx+1:]...)
		keep := fileprotect.Protected(target.Path)
		for _, path := range keepPaths {
			if path != "" && strings.EqualFold(filepath.Clean(target.Path), filepath.Clean(path)) {
				keep = true
			}
		}
		if keep {
			// A protected old file must count toward the limit but cannot be
			// removed. Continue inspecting other entries while available.
			continue
		}
		if err := os.Remove(target.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			continue
		}
		_ = os.Remove(MetadataPath(target.Path))
	}
	return nil
}

// MetadataPath is the atomic sidecar path for a generated WAV.
func MetadataPath(wavPath string) string { return wavPath + ".json" }

type Metadata struct {
	Genre string `json:"genre"`
}

func readMetadata(path string) (Metadata, bool) {
	b, err := os.ReadFile(MetadataPath(path))
	if err != nil {
		return Metadata{}, false
	}
	var m Metadata
	if json.Unmarshal(b, &m) != nil || strings.TrimSpace(m.Genre) == "" {
		return Metadata{}, false
	}
	return m, true
}
