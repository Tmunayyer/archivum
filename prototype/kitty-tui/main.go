// PROTOTYPE — throwaway. See README.md.
//
// The question (issue #2): do inline Kitty-protocol images survive a Bubble
// Tea redraw? Archivum's per-file loop is "show a 3-frame preview, then prompt
// for N fields". Bubble Tea repaints on every keystroke; Kitty images live in
// the terminal's own image store, which the renderer knows nothing about. If
// the frames get clobbered or scrolled away on the first repaint, the whole
// TUI design in #1 (back-a-field, pre-filled editing, recents lists) has to be
// rethought as a plain stdin script.
//
// So this is NOT a state-model prototype. The prompt below is deliberately
// fake — three fields, hardcoded recents, no store, no file copying. The thing
// under test is what is still *visible on screen* after 20+ keystrokes and 3
// file transitions, under five different image-emission strategies you can
// cycle through at runtime with ctrl+s.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"archivum-prototype/kitty-tui/preview"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Terminal rows the 3-frame strip is rendered into.
const stripRows = 12

// ---------------------------------------------------------------------------
// Emission strategies — the actual variable under test
// ---------------------------------------------------------------------------

type strategy int

const (
	inlinePrintln strategy = iota // image via tea.Println, above the viewport
	inlineReemit                  // image inside View(), re-sent every frame
	inlineRaw                     // image written straight to stdout, once per file
	altReemit                     // alt-screen + image inside View()
	altRaw                        // alt-screen + raw stdout write, once per file
	numStrategies
)

var strategyNames = [numStrategies]string{
	"inline/println — tea.Println, once per file, above the viewport",
	"inline/reemit  — payload inside View(), every frame",
	"inline/raw     — raw stdout write, once per file",
	"alt/reemit     — alt-screen, payload inside View(), every frame",
	"alt/raw        — alt-screen, raw stdout write, once per file",
}

func (s strategy) altScreen() bool { return s == altReemit || s == altRaw }
func (s strategy) reemits() bool   { return s == inlineReemit || s == altReemit }

// ---------------------------------------------------------------------------
// Fake prompt — stands in for a real scheme, using CONTEXT.md vocabulary
// ---------------------------------------------------------------------------

type field struct {
	key     string
	kind    string
	recents []string // top 3 most recent, hardcoded (ADR-0003 shape, no store)
}

var scheme = []field{
	{"movement", "label", []string{"bench-press", "back-squat", "deadlift"}},
	{"weight-lb", "number", []string{"185", "225", "135"}},
	{"date", "date", []string{"2026-07-26", "2026-07-19", "2026-07-12"}},
}

type mediaFile struct {
	name    string
	payload string // Kitty escape sequence for the hstacked 3-frame strip
	rows    int    // rows the payload actually occupies, per its own r= key
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type repaintMsg struct{}

type model struct {
	files []mediaFile
	fi    int // current file index

	values []string
	fx     int // current field index

	input textinput.Model
	rc    int // recents cursor; -1 = free-form typing

	strat       strategy
	deleteFirst bool // send the Kitty delete-all escape before each emit
	keystrokes  int
	transitions int
	w, h        int
	status      string
	finished    bool
}

func newModel(files []mediaFile, s strategy) model {
	in := textinput.New()
	in.Prompt = ""
	in.Focus()
	return model{
		files:       files,
		values:      make([]string, len(scheme)),
		input:       in,
		rc:          -1,
		strat:       s,
		deleteFirst: true,
		status:      "ready",
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if m.strat.altScreen() {
		cmds = append(cmds, tea.EnterAltScreen)
	}
	cmds = append(cmds, m.emitCmd())
	return tea.Batch(cmds...)
}

// emitCmd pushes the current file's image out-of-band. Nil for the reemit
// strategies, which put the payload in View() instead.
func (m model) emitCmd() tea.Cmd {
	if m.strat.reemits() || len(m.files) == 0 {
		return nil
	}
	payload := m.files[m.fi].payload
	if m.deleteFirst {
		payload = preview.DeleteAll + payload
	}
	switch m.strat {
	case inlinePrintln:
		return tea.Println(payload)
	case inlineRaw:
		return func() tea.Msg {
			os.Stdout.WriteString(payload + "\n")
			return repaintMsg{}
		}
	case altRaw:
		return func() tea.Msg {
			// Home the cursor first — in alt-screen there is no "above the
			// viewport" to print into.
			os.Stdout.WriteString("\x1b[H" + payload)
			return repaintMsg{}
		}
	}
	return nil
}

func (m *model) loadField() {
	m.input.SetValue(m.values[m.fx])
	m.input.CursorEnd()
	m.rc = -1
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case repaintMsg:
		return m, nil

	case tea.KeyMsg:
		m.keystrokes++

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "ctrl+s": // cycle emission strategy
			old := m.strat
			m.strat = (m.strat + 1) % numStrategies
			m.status = "strategy → " + strings.TrimSpace(strategyNames[m.strat])
			cmds := []tea.Cmd{func() tea.Msg {
				os.Stdout.WriteString(preview.DeleteAll)
				return repaintMsg{}
			}}
			if old.altScreen() != m.strat.altScreen() {
				if m.strat.altScreen() {
					cmds = append(cmds, tea.EnterAltScreen)
				} else {
					cmds = append(cmds, tea.ExitAltScreen)
				}
			}
			cmds = append(cmds, tea.ClearScreen, m.emitCmd())
			return m, tea.Batch(cmds...)

		case "ctrl+d": // toggle delete-all-before-emit
			m.deleteFirst = !m.deleteFirst
			m.status = fmt.Sprintf("delete-before-emit → %v", m.deleteFirst)
			return m, nil

		case "ctrl+r": // force a clean repaint + re-emit
			m.status = "forced repaint"
			return m, tea.Batch(tea.ClearScreen, m.emitCmd())

		case "up":
			if m.rc > 0 {
				m.rc--
			} else {
				m.rc = len(scheme[m.fx].recents) - 1
			}
			m.input.SetValue(scheme[m.fx].recents[m.rc])
			m.input.CursorEnd()
			return m, nil

		case "down":
			m.rc = (m.rc + 1) % len(scheme[m.fx].recents)
			m.input.SetValue(scheme[m.fx].recents[m.rc])
			m.input.CursorEnd()
			return m, nil

		case "shift+tab": // back a field, pre-filled (ADR-0007)
			if m.fx > 0 {
				m.values[m.fx] = m.input.Value()
				m.fx--
				m.loadField()
				m.status = "back a field"
			} else {
				m.status = "already at first field"
			}
			return m, nil

		case "enter":
			m.values[m.fx] = m.input.Value()
			if m.fx < len(scheme)-1 {
				m.fx++
				m.loadField()
				return m, nil
			}
			// Last field committed → advance to the next file.
			m.transitions++
			m.status = "composed " + strings.Join(m.values, "_") +
				filepath.Ext(m.files[m.fi].name)
			m.fi++
			if m.fi >= len(m.files) {
				m.finished = true
				return m, nil
			}
			m.fx = 0
			for i := range m.values {
				m.values[i] = ""
			}
			m.loadField()
			return m, m.emitCmd()
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.rc = -1 // typing leaves the recents list
	return m, cmd
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

const (
	bold  = "\x1b[1m"
	dim   = "\x1b[2m"
	green = "\x1b[32m"
	reset = "\x1b[0m"
)

func (m model) View() string {
	var b strings.Builder

	// The whole experiment, in the reemit strategies: put the raw Kitty
	// payload inside the frame Bubble Tea repaints, padded with the rows the
	// image physically occupies (the renderer counts newlines, not pixels).
	if m.strat.reemits() && !m.finished && m.fi < len(m.files) {
		if m.deleteFirst {
			b.WriteString(preview.DeleteAll)
		}
		b.WriteString(m.files[m.fi].payload)
		b.WriteString(strings.Repeat("\n", m.files[m.fi].rows))
	}

	check := func(ok bool, s string) string {
		if ok {
			return green + s + " ✓" + reset
		}
		return s
	}

	fmt.Fprintf(&b, "%sarchivum · kitty-under-bubbletea prototype%s %s(issue #2)%s\n",
		bold, reset, dim, reset)
	fmt.Fprintf(&b, "%sstrategy%s %s   %sdelete-first%s %v   %sterm%s %dx%d\n",
		dim, reset, strategyNames[m.strat], dim, reset, m.deleteFirst, dim, reset, m.w, m.h)
	fmt.Fprintf(&b, "%skeystrokes%s %s   %stransitions%s %s\n",
		dim, reset, check(m.keystrokes >= 20, fmt.Sprintf("%d/20", m.keystrokes)),
		dim, reset, check(m.transitions >= 3, fmt.Sprintf("%d/3", m.transitions)))

	if m.finished {
		fmt.Fprintf(&b, "\n%sall files done.%s ctrl+s to try another strategy, ctrl+c to quit.\n",
			bold, reset)
		b.WriteString(verdictChecklist())
		return b.String()
	}

	f := scheme[m.fx]
	fmt.Fprintf(&b, "\n%sfile%s %d/%d %s   %sfield%s %d/%d %s%s%s %s(%s)%s\n",
		dim, reset, m.fi+1, len(m.files), m.files[m.fi].name,
		dim, reset, m.fx+1, len(scheme), bold, f.key, reset, dim, f.kind, reset)
	fmt.Fprintf(&b, "  %s>%s %s\n", bold, reset, m.input.View())

	b.WriteString("  " + dim + "recents" + reset + " ")
	for i, r := range f.recents {
		if i == m.rc {
			fmt.Fprintf(&b, " %s▸ %s%s ", bold, r, reset)
		} else {
			fmt.Fprintf(&b, "   %s ", r)
		}
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "\n%s%s%s\n", dim, m.status, reset)
	fmt.Fprintf(&b, "%senter next · shift+tab back · ↑↓ recents · ctrl+s strategy · "+
		"ctrl+d delete-first · ctrl+r repaint · ctrl+c quit%s\n", dim, reset)
	return b.String()
}

func verdictChecklist() string {
	return dim + `
  Look at the screen, not at this text:
    · are all three frames still visible, for the current file only?
    · any flicker while typing?
    · any orphaned/duplicated strips drifting up the scrollback?
` + reset
}

// ---------------------------------------------------------------------------
// Setup — frame extraction happens up front so transitions are instant
// ---------------------------------------------------------------------------

func main() {
	dir := flag.String("dir", "", "directory of videos to preview (default: generate 3 samples)")
	stratFlag := flag.Int("strategy", 0, "starting emission strategy 0..4")
	flag.Parse()

	for _, bin := range []string{"ffmpeg", "ffprobe", "chafa"} {
		if _, err := exec.LookPath(bin); err != nil {
			fmt.Fprintf(os.Stderr, "missing %s — run: brew install ffmpeg chafa\n", bin)
			os.Exit(1)
		}
	}
	if os.Getenv("TMUX") != "" {
		fmt.Fprintln(os.Stderr,
			"refusing to run inside tmux: Kitty escapes don't pass through and images "+
				"silently fail, which would be a false negative (ADR-0006). Use a plain pane.")
		os.Exit(1)
	}

	scratch := filepath.Join(os.TempDir(), "archivum-prototype-scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		fatal(err)
	}

	videos, err := sourceVideos(*dir, scratch)
	if err != nil {
		fatal(err)
	}

	cols := 100
	if w, _, err := term(); err == nil && w > 4 {
		cols = w - 2
	}

	files := make([]mediaFile, 0, len(videos))
	for i, v := range videos {
		fmt.Printf("\rpreparing %d/%d %s…", i+1, len(videos), filepath.Base(v))
		frames, err := preview.Frames(v, filepath.Join(scratch, "frames"))
		if err != nil {
			fatal(err)
		}
		strip, err := preview.Strip(frames,
			filepath.Join(scratch, "frames",
				strings.TrimSuffix(filepath.Base(v), filepath.Ext(v))+"-strip.png"))
		if err != nil {
			fatal(err)
		}
		payload, err := preview.Kitty(strip, cols, stripRows)
		if err != nil {
			fatal(err)
		}
		files = append(files, mediaFile{
			name:    filepath.Base(v),
			payload: payload,
			rows:    preview.Rows(payload),
		})
	}
	fmt.Print("\r\x1b[K")

	s := strategy(*stratFlag % int(numStrategies))
	if _, err := tea.NewProgram(newModel(files, s)).Run(); err != nil {
		fatal(err)
	}
	// Leave the terminal clean whichever strategy was last active.
	os.Stdout.WriteString(preview.DeleteAll)
}

// sourceVideos returns the videos to preview, generating three visually
// distinct samples if no directory was given.
func sourceVideos(dir, scratch string) ([]string, error) {
	if dir != "" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		var out []string
		for _, e := range entries {
			switch strings.ToLower(filepath.Ext(e.Name())) {
			case ".mp4", ".mov", ".m4v", ".avi", ".mkv":
				out = append(out, filepath.Join(dir, e.Name()))
			}
		}
		sort.Strings(out)
		if len(out) == 0 {
			return nil, fmt.Errorf("no videos in %s", dir)
		}
		return out, nil
	}

	hues := []int{0, 120, 240}
	var out []string
	for i, h := range hues {
		dst := filepath.Join(scratch, fmt.Sprintf("sample-%d.mp4", i+1))
		out = append(out, dst)
		if _, err := os.Stat(dst); err == nil {
			continue // cached from a previous run
		}
		cmd := exec.Command("ffmpeg", "-y", "-v", "error",
			"-f", "lavfi", "-i", "testsrc=size=640x360:rate=15:duration=8",
			"-vf", fmt.Sprintf("hue=h=%d", h),
			"-pix_fmt", "yuv420p", dst)
		if o, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("generating sample %d: %w: %s", i+1, err, o)
		}
	}
	return out, nil
}

func term() (int, int, error) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin // stty reads the tty from stdin, which exec won't inherit
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}
	var rows, cols int
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d", &rows, &cols)
	return cols, rows, err
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
