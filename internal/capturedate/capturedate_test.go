// Seam-2 tests: capture-date resolution runs against real exiftool and real
// fixture media, because per ADR-0010 the correctness lives in an exiftool
// flag — a fake proves nothing. TZ is pinned to America/Los_Angeles so the
// fixtures' UTC video dates land on known local wall-clock times everywhere.
package capturedate_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tmunayyer/archivum/internal/capturedate"
)

// requireExiftool skips loudly when exiftool is absent — a missing-tool
// signal so `go test ./...` runs on a bare machine, never a way past a
// failure (issue #7).
func requireExiftool(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("SKIPPING seam-2 capture-date tests: exiftool is not installed — install exiftool and re-run; these tests verify ADR-0010's timezone handling and must not stay skipped on a dev machine")
	}
	t.Setenv("TZ", "America/Los_Angeles")
}

func fixture(name string) string { return filepath.Join("testdata", name) }

func TestCaptureOrder(t *testing.T) {
	t.Run("two devices interleave by capture date, not filename order", func(t *testing.T) {
		requireExiftool(t)
		// Filename order: still-morning (10:00), still-noon (12:00),
		// video-late-morning (11:00 local, stored as 18:00 UTC). Capture
		// order slots the video between the stills — which only happens
		// when the QuickTime UTC date is converted to local time.
		files, err := capturedate.Resolve([]string{
			fixture("still-morning.jpg"),
			fixture("still-noon.jpg"),
			fixture("video-late-morning.mp4"),
		})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"still-morning.jpg", "video-late-morning.mp4", "still-noon.jpg"}
		if len(files) != len(want) {
			t.Fatalf("Resolve returned %d files, want %d", len(files), len(want))
		}
		for i, f := range files {
			if filepath.Base(f.Path) != want[i] {
				t.Fatalf("capture order = %v, want %v", paths(files), want)
			}
		}
	})
}

func TestEveningSession(t *testing.T) {
	t.Run("a still and a video from the same evening session agree on the date", func(t *testing.T) {
		requireExiftool(t)
		// The video's QuickTime CreateDate is 2026-06-29 03:00 UTC — 8pm on
		// the 28th in Los Angeles. Without `-api QuickTimeUTC=1` it reads as
		// the 29th and the session splits across two dates (ADR-0010).
		files, err := capturedate.Resolve([]string{
			fixture("evening-still.jpg"),
			fixture("evening-video.mp4"),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if got := f.CaptureDate.Format("2006-01-02"); got != "2026-06-28" {
				t.Fatalf("%s dated %s, want 2026-06-28 — the QuickTimeUTC conversion is missing", filepath.Base(f.Path), got)
			}
		}
	})
}

func TestTagPriority(t *testing.T) {
	// Copies with a decoy modify time, so FileModifyDate can never
	// accidentally supply the expected value.
	decoyMtime := func(t *testing.T, name string) string {
		t.Helper()
		src, err := os.ReadFile(fixture(name))
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, src, 0o644); err != nil {
			t.Fatal(err)
		}
		decoy := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(p, decoy, decoy); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("DateTimeOriginal beats CreateDate beats FileModifyDate", func(t *testing.T) {
		requireExiftool(t)
		// still-morning carries a decoy CreateDate of 2026-01-01, and both
		// copies carry a decoy modify time of 2020-01-01.
		still := decoyMtime(t, "still-morning.jpg")
		video := decoyMtime(t, "video-late-morning.mp4")
		files, err := capturedate.Resolve([]string{still, video})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]time.Time{
			still: time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
			video: time.Date(2026, 6, 28, 11, 0, 0, 0, time.UTC),
		}
		for _, f := range files {
			if !f.CaptureDate.Equal(want[f.Path]) {
				t.Fatalf("%s resolved to %v, want %v", filepath.Base(f.Path), f.CaptureDate, want[f.Path])
			}
		}
	})

	t.Run("a file with no date tags falls back to FileModifyDate rather than being dropped", func(t *testing.T) {
		requireExiftool(t)
		src, err := os.ReadFile(fixture("no-tags.png"))
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(t.TempDir(), "no-tags.png")
		if err := os.WriteFile(p, src, 0o644); err != nil {
			t.Fatal(err)
		}
		// 15:04 UTC on 2026-01-02 is 07:04 in Los Angeles (PST).
		if err := os.Chtimes(p, time.Time{}, time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}

		files, err := capturedate.Resolve([]string{p})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Fatalf("Resolve returned %d files, want the tagless file kept", len(files))
		}
		want := time.Date(2026, 1, 2, 7, 4, 5, 0, time.UTC)
		if !files[0].CaptureDate.Equal(want) {
			t.Fatalf("capture date = %v, want the modify time %v", files[0].CaptureDate, want)
		}
	})
}

func paths(files []capturedate.File) []string {
	var out []string
	for _, f := range files {
		out = append(out, filepath.Base(f.Path))
	}
	return out
}
