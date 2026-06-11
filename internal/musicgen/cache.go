package musicgen

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

func PickFallback(dir string) (string, error) {
	files, err := ListCache(dir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", os.ErrNotExist
	}
	return files[len(files)/2].Path, nil
}

func TrimCache(dir string, limit int, keepPath string) error {
	if limit <= 0 {
		limit = 20
	}
	files, err := ListCache(dir)
	if err != nil {
		return err
	}
	for len(files) > limit {
		target := files[0]
		files = files[1:]
		if keepPath != "" && strings.EqualFold(target.Path, keepPath) {
			continue
		}
		_ = os.Remove(target.Path)
	}
	return nil
}
