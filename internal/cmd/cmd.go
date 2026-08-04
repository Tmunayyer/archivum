// Package cmd is the Cobra wiring: flag parsing, preflight checks, and
// dependency construction. Everything here is configuration; behaviour
// lives behind the run model's seam.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/Tmunayyer/archivum/internal/capturedate"
	"github.com/Tmunayyer/archivum/internal/preview"
	"github.com/Tmunayyer/archivum/internal/run"
	"github.com/Tmunayyer/archivum/internal/source"
	"github.com/Tmunayyer/archivum/internal/store"
)

// stripRows is the band of terminal rows requested for each preview; chafa
// may hand back fewer to keep aspect ratio (ADR-0011).
const stripRows = 12

var rootCmd = &cobra.Command{
	Use:           "archivum",
	Short:         "Batch-label media by copying files under composed names",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	var srcDir, destDir, schemeName string

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Label every eligible file in a source folder into a destination folder",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBatch(srcDir, destDir, schemeName)
		},
	}
	runCmd.Flags().StringVar(&srcDir, "source", "", "folder of media to label (required)")
	runCmd.Flags().StringVar(&destDir, "dest", "", "folder the labeled copies land in (required)")
	runCmd.Flags().StringVar(&schemeName, "scheme", "", "name of the scheme to prompt with (required)")
	for _, f := range []string{"source", "dest", "scheme"} {
		cobra.CheckErr(runCmd.MarkFlagRequired(f))
	}
	rootCmd.AddCommand(runCmd)
}

// Execute runs the CLI and exits non-zero on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "archivum:", err)
		os.Exit(1)
	}
}

// previewCols is the width requested of chafa: the terminal's, less a
// two-cell margin — the prototype's choice, as is the 100-column default
// when stdout is not a measurable terminal (or too narrow for the margin
// to leave anything).
func previewCols() int {
	if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 4 {
		return w - 2
	}
	return 100
}

func storePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating the store: %w", err)
	}
	return filepath.Join(home, ".config", "archivum", "store.json"), nil
}

// preflight fails the run at startup when a required external tool is
// missing, so the failure is one clear message rather than a mid-batch
// surprise (issue #4's failure policy). tmux is refused outright: Kitty
// escapes do not pass through it, so previews would fail silently
// (ADR-0006). Parameterized on the lookups so tests need no real PATH.
func preflight(lookPath func(string) (string, error), getenv func(string) string) error {
	if getenv("TMUX") != "" {
		return fmt.Errorf("refusing to start inside tmux — Kitty graphics escapes do not pass through it and previews would silently fail (ADR-0006); use a plain pane")
	}
	for _, tool := range []struct{ bin, job, pkg string }{
		{"exiftool", "resolve capture dates", "exiftool"},
		{"ffmpeg", "extract video preview frames", "ffmpeg"},
		{"ffprobe", "read video durations", "ffmpeg"},
		{"chafa", "render previews inline", "chafa"},
	} {
		if _, err := lookPath(tool.bin); err != nil {
			return fmt.Errorf("%s is required to %s but was not found on PATH — install it (e.g. `brew install %s`) and re-run", tool.bin, tool.job, tool.pkg)
		}
	}
	return nil
}

func runBatch(srcDir, destDir, schemeName string) error {
	if err := preflight(exec.LookPath, os.Getenv); err != nil {
		return err
	}
	path, err := storePath()
	if err != nil {
		return err
	}
	st, err := store.Load(path)
	if err != nil {
		return err
	}
	keys, ok := st.Scheme(schemeName)
	if !ok {
		return fmt.Errorf("scheme %q not found in %s — seed it by hand for now; inline scheme creation is a later ticket", schemeName, path)
	}
	fields := make([]run.Field, len(keys))
	for i, key := range keys {
		// A key with no fieldKeys entry predates types (hand-seeded store);
		// label is the behaviour those keys have always had.
		fieldType := run.Label
		if declared, ok := st.FieldKeyType(key); ok {
			fieldType, err = run.ParseFieldType(declared)
			if err != nil {
				return fmt.Errorf("field key %q in %s: %w", key, path, err)
			}
		}
		fields[i] = run.Field{Key: key, Type: fieldType}
	}

	files, err := source.List(srcDir)
	if err != nil {
		return fmt.Errorf("reading source folder: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no eligible media files in %s — nothing to label", srcDir)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating destination folder: %w", err)
	}

	// One metadata pass over the whole batch, before the first prompt, so
	// files arrive in capture order and the wait is paid once (ADR-0010).
	resolved, err := capturedate.Resolve(files)
	if err != nil {
		return err
	}
	ordered := make([]run.File, len(resolved))
	for i, f := range resolved {
		ordered[i] = run.File{Path: f.Path, CaptureDate: f.CaptureDate}
	}

	fmt.Printf("%d file(s) to label from %s\n", len(ordered), srcDir)

	// Frames and strips are working files, gone when the run is; only the
	// labeled copies persist.
	scratch, err := os.MkdirTemp("", "archivum-preview-")
	if err != nil {
		return fmt.Errorf("creating preview scratch dir: %w", err)
	}
	defer os.RemoveAll(scratch)

	// Inline on purpose: no alt-screen, so previews and progress lines
	// scroll up into terminal history (ADR-0011).
	model := run.New(ordered, fields, run.Deps{
		Store:   st,
		Dest:    run.DirDest{Dir: destDir},
		DestDir: destDir,
		Preview: preview.Renderer{Dir: scratch, Cols: previewCols(), Rows: stripRows},
	})
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return err
	}

	m := final.(run.Model)
	fmt.Printf("copied %d of %d file(s) to %s\n", m.Copied(), len(files), destDir)
	return m.Err()
}
