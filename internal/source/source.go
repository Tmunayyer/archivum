// Package source selects the files a run will process: a flat walk of one
// directory, filtered by a case-insensitive extension allowlist, skipping
// dotfiles and camera sidecar files. Nested directories are never entered.
package source

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var images = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".heic": true, ".heif": true,
	".webp": true, ".gif": true,
}

var videos = map[string]bool{
	".mov": true, ".mp4": true, ".m4v": true,
}

// IsVideo reports whether path names a video, by extension — the split that
// decides whether a preview is the file itself or a three-frame strip
// (ADR-0005).
func IsVideo(path string) bool {
	return videos[strings.ToLower(filepath.Ext(path))]
}

// List returns the full paths of eligible files directly inside dir, in
// filename order. An empty result is not an error — the caller decides
// whether an empty batch may start.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if !images[ext] && !videos[ext] {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files, nil
}
