// Package preview extracts video frames and turns media files into inline
// Kitty-graphics-protocol payloads, ready to print above the prompt.
//
// Lifted from the prototype (issue #2, ADR-0011) — it is pure-ish: shells
// out to ffmpeg/ffprobe/chafa, touches no terminal state of its own, and
// knows nothing about Bubble Tea. Emission belongs to the caller, and per
// ADR-0011 it never includes a Kitty delete escape.
package preview

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tmunayyer/archivum/internal/source"
)

// Fractions of a video's duration sampled for the preview. ADR-0005.
var Fractions = []float64{0.25, 0.50, 0.75}

// Renderer turns one source file into a Kitty payload: images directly,
// videos as a three-frame strip occupying one band of rows. It satisfies
// the run seam's Previewer.
type Renderer struct {
	Dir  string // scratch directory for extracted frames and strips
	Cols int    // terminal cells requested of chafa horizontally
	Rows int    // terminal rows requested; chafa may hand back fewer (see Rows)
}

// Preview renders path into a payload sized for the renderer's terminal.
func (r Renderer) Preview(path string) (string, error) {
	img := path
	if source.IsVideo(path) {
		frames, err := Frames(path, r.Dir)
		if err != nil {
			return "", err
		}
		img, err = Strip(frames, filepath.Join(r.Dir, stem(path)+"-strip.png"))
		if err != nil {
			return "", err
		}
	}
	return Kitty(img, r.Cols, r.Rows)
}

// stem is the filename without directory or extension — what a file's
// working frames and strip are named after.
func stem(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// Duration returns the video's length in seconds via ffprobe.
func Duration(videoPath string) (float64, error) {
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w", filepath.Base(videoPath), err)
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

// Frames extracts three stills at 25/50/75% of the video's duration and
// returns their paths in order.
func Frames(videoPath, outDir string) ([]string, error) {
	dur, err := Duration(videoPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(Fractions))
	for i, f := range Fractions {
		dst := filepath.Join(outDir, fmt.Sprintf("%s-%02d.png", stem(videoPath), i))
		cmd := exec.Command("ffmpeg",
			"-y", "-v", "error",
			"-ss", strconv.FormatFloat(dur*f, 'f', 3, 64),
			"-i", videoPath,
			"-frames:v", "1",
			dst,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("ffmpeg frame %d: %w: %s", i, err, out)
		}
		paths = append(paths, dst)
	}
	return paths, nil
}

// Strip horizontally stacks the given frames into a single image, so all three
// previews occupy one Kitty placement and one band of terminal rows.
func Strip(framePaths []string, dst string) (string, error) {
	args := []string{"-y", "-v", "error"}
	for _, p := range framePaths {
		args = append(args, "-i", p)
	}
	args = append(args,
		"-filter_complex", fmt.Sprintf("hstack=inputs=%d", len(framePaths)),
		dst,
	)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg hstack: %w: %s", err, out)
	}
	return dst, nil
}

// Kitty renders an image to a Kitty-graphics escape payload sized to fit
// cols x rows terminal cells.
func Kitty(imgPath string, cols, rows int) (string, error) {
	out, err := exec.Command("chafa",
		"--format", "kitty",
		"--size", fmt.Sprintf("%dx%d", cols, rows),
		"--animate", "off",
		"--polite", "on",
		// Explicit: chafa's auto-detection sees a pipe, not the terminal, and
		// tmux/screen passthrough would silently break rendering (ADR-0006).
		"--passthrough", "none",
		imgPath,
	).Output()
	if err != nil {
		return "", fmt.Errorf("chafa %s: %w", filepath.Base(imgPath), err)
	}
	return string(out), nil
}

// Rows reports how many terminal rows a Kitty payload will actually occupy,
// read off the `r=` key chafa put in the transmit escape. This is not the same
// as the row count requested via Kitty: chafa preserves aspect ratio and hands
// back something equal or smaller. Anything that reserves rows for an image
// must read this value back off the payload rather than trusting its own
// request, or everything below the image misaligns (ADR-0011).
func Rows(payload string) int {
	i := strings.Index(payload, ",r=")
	if i < 0 {
		return 0
	}
	rest := payload[i+3:]
	end := strings.IndexAny(rest, ",;\x1b")
	if end < 0 {
		return 0
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return n
}
