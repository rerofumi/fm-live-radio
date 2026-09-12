// Package fileprotect tracks short lived ownership of generated files.
// It is deliberately small and has no knowledge of audio or music formats.
package fileprotect

import (
	"path/filepath"
	"strings"
	"sync"
)

var guard struct {
	sync.Mutex
	paths map[string]int
}

func key(path string) string {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return strings.ToLower(filepath.Clean(path))
}

// Acquire protects path until the returned release function is called.
func Acquire(path string) func() {
	k := key(path)
	guard.Lock()
	if guard.paths == nil {
		guard.paths = make(map[string]int)
	}
	guard.paths[k]++
	guard.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			guard.Lock()
			if guard.paths[k] <= 1 {
				delete(guard.paths, k)
			} else {
				guard.paths[k]--
			}
			guard.Unlock()
		})
	}
}

func Protected(path string) bool {
	k := key(path)
	guard.Lock()
	defer guard.Unlock()
	return guard.paths[k] > 0
}
