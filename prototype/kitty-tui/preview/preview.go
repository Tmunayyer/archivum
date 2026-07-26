// Package preview extracts video frames and turns them into inline
// Kitty-graphics-protocol payloads.
//
// This is the part of the prototype worth keeping: it is pure-ish (shells out
// to ffmpeg/ffprobe/chafa, touches no terminal state of its own, and knows
// nothing about Bubble Tea). The TUI shell in ../main.go is throwaway.
package preview

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Fractions of a video's duration sampled for the preview. ADR-0005.
var Fractions = []float64{0.25, 0.50, 0.75}

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

	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	paths := make([]string, 0, len(Fractions))
	for i, f := range Fractions {
		dst := filepath.Join(outDir, fmt.Sprintf("%s-%02d.png", base, i))
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
// back something equal or smaller. A renderer that pads by the requested count
// misaligns everything below the image.
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

// DeleteAll is the Kitty escape that removes every placement and frees the
// terminal's stored image data. Used to test whether explicit lifetime
// management beats letting the renderer and the terminal fight.
const DeleteAll = "\x1b_Ga=d,d=A\x1b\\"
