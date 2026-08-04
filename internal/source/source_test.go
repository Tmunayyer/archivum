package source

import (
	"os"
	"path/filepath"
	"testing"
)

func seed(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func basenames(files []string) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = filepath.Base(f)
	}
	return out
}

func TestList(t *testing.T) {
	t.Run("keeps allowlisted extensions case-insensitively, in filename order", func(t *testing.T) {
		dir := seed(t, "b.MOV", "a.jpg", "c.HeIc", "d.mp4", "e.webp", "f.gif", "g.m4v", "h.jpeg", "i.png", "j.heif")
		got, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"a.jpg", "b.MOV", "c.HeIc", "d.mp4", "e.webp", "f.gif", "g.m4v", "h.jpeg", "i.png", "j.heif"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", basenames(got), want)
		}
		for i := range want {
			if filepath.Base(got[i]) != want[i] {
				t.Fatalf("got %v, want %v", basenames(got), want)
			}
		}
	})

	t.Run("skips non-media, dotfiles and sidecars", func(t *testing.T) {
		dir := seed(t, "a.jpg", "notes.txt", "archive.zip", ".DS_Store", ".hidden.jpg", "a.AAE", "b.xmp", "c.THM", "d.lrv", "noext")
		got, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || filepath.Base(got[0]) != "a.jpg" {
			t.Fatalf("got %v, want just a.jpg", basenames(got))
		}
	})

	t.Run("stays flat, ignoring subdirectories", func(t *testing.T) {
		dir := seed(t, "a.jpg")
		if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "nested", "b.jpg"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || filepath.Base(got[0]) != "a.jpg" {
			t.Fatalf("got %v, want just a.jpg", basenames(got))
		}
	})

	t.Run("returns full paths under the source dir", func(t *testing.T) {
		dir := seed(t, "a.jpg")
		got, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got[0] != filepath.Join(dir, "a.jpg") {
			t.Fatalf("got %q, want path under %q", got[0], dir)
		}
	})

	t.Run("empty of eligible files returns an empty list", func(t *testing.T) {
		dir := seed(t, "notes.txt")
		got, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("got %v, want none", basenames(got))
		}
	})

	t.Run("missing directory errors", func(t *testing.T) {
		if _, err := List(filepath.Join(t.TempDir(), "nope")); err == nil {
			t.Fatal("want error for missing directory")
		}
	})
}

func TestIsVideo(t *testing.T) {
	t.Run("video extensions are videos case-insensitively, images and strangers are not", func(t *testing.T) {
		for _, p := range []string{"/src/a.mov", "/src/b.MP4", "/src/c.m4v"} {
			if !IsVideo(p) {
				t.Fatalf("IsVideo(%q) = false, want true", p)
			}
		}
		for _, p := range []string{"/src/a.jpg", "/src/b.HEIC", "/src/c.txt", "/src/noext"} {
			if IsVideo(p) {
				t.Fatalf("IsVideo(%q) = true, want false", p)
			}
		}
	})
}
