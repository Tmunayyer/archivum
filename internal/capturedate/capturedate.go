// Package capturedate resolves each file's capture date — the date its
// content was recorded, expressed in local time — in a single exiftool
// invocation over the whole batch, and orders the batch by it (ADR-0010).
// This is seam 2 (issue #4): correctness lives in the exiftool flag below,
// so the package is tested against real exiftool and real fixture media.
package capturedate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// File is one file of the run with its resolved capture date. CaptureDate
// is local wall-clock time carried in a fixed zone (UTC) — compare and
// format it, never convert it. A zero CaptureDate means nothing usable
// came back at all; such files sort first, and the date field falls
// through to today (#9).
type File struct {
	Path        string
	CaptureDate time.Time
}

// Tag priority per ADR-0010: DateTimeOriginal (images, local-naive) beats
// CreateDate (video, UTC converted to local by the flag below) beats
// FileModifyDate (weak fallback — copies and syncs rewrite it).
var tags = []string{"DateTimeOriginal", "CreateDate", "FileModifyDate"}

// One date-time shape, spelled once for exiftool (strftime) and once for
// Go's parser.
const (
	exiftoolFormat = "%Y-%m-%d %H:%M:%S"
	layout         = "2006-01-02 15:04:05"
)

// Resolve reads every file's capture date in one exiftool invocation and
// returns the files sorted into capture order, ties broken by path. A file
// with no usable tags is ordered by the weakest fallback rather than
// dropped — FileModifyDate always exists.
func Resolve(paths []string) ([]File, error) {
	// -api QuickTimeUTC=1 is load-bearing (ADR-0010): QuickTime dates are
	// UTC, EXIF dates are local-naive; without the conversion an evening
	// video lands on the next day while stills from the same session don't.
	args := []string{"-api", "QuickTimeUTC=1", "-json", "-d", exiftoolFormat}
	for _, tag := range tags {
		args = append(args, "-"+tag)
	}
	args = append(args, paths...)

	out, err := exec.Command("exiftool", args...).Output()
	if err != nil {
		// exiftool exits non-zero when any one file is unreadable while
		// still emitting entries for the rest. A file-level problem must
		// not end the run — the file is kept and dated by whatever came
		// back (issue #7: fall back, never drop). Only a run-level failure
		// — no parseable output at all — is fatal, and it carries
		// exiftool's own words rather than a bare exit status.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || !json.Valid(out) {
			detail := ""
			if exitErr != nil && len(exitErr.Stderr) > 0 {
				detail = ": " + strings.TrimSpace(string(exitErr.Stderr))
			}
			return nil, fmt.Errorf("resolving capture dates with exiftool: %w%s", err, detail)
		}
	}

	var records []map[string]string
	if err := json.Unmarshal(out, &records); err != nil {
		return nil, fmt.Errorf("parsing exiftool output: %w", err)
	}
	dates := make(map[string]time.Time, len(records))
	for _, rec := range records {
		for _, tag := range tags {
			if ts, err := time.ParseInLocation(layout, rec[tag], time.UTC); err == nil {
				dates[rec["SourceFile"]] = ts
				break
			}
		}
	}

	files := make([]File, len(paths))
	for i, p := range paths {
		files[i] = File{Path: p, CaptureDate: dates[p]}
	}
	sort.Slice(files, func(i, j int) bool {
		if !files[i].CaptureDate.Equal(files[j].CaptureDate) {
			return files[i].CaptureDate.Before(files[j].CaptureDate)
		}
		return files[i].Path < files[j].Path
	})
	return files, nil
}
