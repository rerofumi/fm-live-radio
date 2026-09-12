package musicgen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPickFallbackForGenreRequiresValidMatchingSidecar(t *testing.T) {
	dir := t.TempDir()
	matching := filepath.Join(dir, "matching.wav")
	wrong := filepath.Join(dir, "wrong.wav")
	unknown := filepath.Join(dir, "unknown.wav")
	for _, path := range []string{matching, wrong, unknown} {
		if err := os.WriteFile(path, []byte("wav"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeMetadata(matching, Metadata{Genre: "ambient music"}); err != nil {
		t.Fatal(err)
	}
	if err := writeMetadata(wrong, Metadata{Genre: "smooth jazz"}); err != nil {
		t.Fatal(err)
	}
	if got, err := PickFallback(dir, "ambient music"); err != nil || got != matching {
		t.Fatalf("fallback=(%q,%v), want matching file", got, err)
	}
	if got, err := PickFallback(dir, "minimal electronica"); !os.IsNotExist(err) || got != "" {
		t.Fatalf("fallback=(%q,%v), want no matching file", got, err)
	}
	if err := os.WriteFile(MetadataPath(matching), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := PickFallback(dir, "ambient music"); !os.IsNotExist(err) || got != "" {
		t.Fatalf("corrupt metadata fallback=(%q,%v), want no file", got, err)
	}
}

func TestTrimCacheSkipsProtectedWAVAndRemovesSidecarTogether(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "protected.wav")
	removable := filepath.Join(dir, "removable.wav")
	for _, path := range []string{protected, removable} {
		if err := os.WriteFile(path, []byte("wav"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeMetadata(path, Metadata{Genre: "ambient music"}); err != nil {
			t.Fatal(err)
		}
	}
	release := ProtectResult(protected)
	defer release()
	if err := TrimCache(dir, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(protected); err != nil {
		t.Fatalf("protected WAV removed: %v", err)
	}
	if _, err := os.Stat(removable); !os.IsNotExist(err) {
		t.Fatalf("removable WAV still exists: %v", err)
	}
	if _, err := os.Stat(MetadataPath(removable)); !os.IsNotExist(err) {
		t.Fatalf("removable sidecar still exists: %v", err)
	}
}

func TestTrimCacheDefersWhenAllProtectedThenRemovesAfterRelease(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "old.wav"),
		filepath.Join(dir, "middle.wav"),
		filepath.Join(dir, "new.wav"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("wav"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeMetadata(path, Metadata{Genre: "ambient music"}); err != nil {
			t.Fatal(err)
		}
	}
	nonWAV := filepath.Join(dir, "keep.mp3")
	unrelatedSidecar := filepath.Join(dir, "unrelated.wav.json")
	if err := os.WriteFile(nonWAV, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelatedSidecar, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Keep all cache entries protected so an over-limit cache is deferred.
	releases := make([]func(), 0, len(paths))
	for _, path := range paths {
		releases = append(releases, ProtectResult(path))
	}
	if err := TrimCache(dir, 1); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected WAV %s removed before release: %v", path, err)
		}
	}
	for _, release := range releases {
		release()
	}
	if err := TrimCache(dir, 1); err != nil {
		t.Fatal(err)
	}
	entries, err := ListCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("remaining WAV count=%d, want 1", len(entries))
	}
	for _, path := range paths {
		if path == entries[0].Path {
			continue
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("released old WAV %s remains: %v", path, err)
		}
		if _, err := os.Stat(MetadataPath(path)); !os.IsNotExist(err) {
			t.Fatalf("released old sidecar %s remains: %v", MetadataPath(path), err)
		}
	}
	if _, err := os.Stat(nonWAV); err != nil {
		t.Fatalf("unrelated non-WAV removed: %v", err)
	}
	if _, err := os.Stat(unrelatedSidecar); err != nil {
		t.Fatalf("unrelated sidecar removed: %v", err)
	}
}
