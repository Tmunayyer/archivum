// Package run holds the Bubble Tea model driving a labeling run: prompt
// each field of the scheme per file, compose the name, copy, record. The
// model performs no I/O itself — reads (recents) are synchronous queries on
// Deps, and every side effect goes through Deps as a tea.Cmd — which is
// what makes this the seam most tests drive (issue #4, seam 1).
package run

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Tmunayyer/archivum/internal/compose"
	"github.com/Tmunayyer/archivum/internal/normalize"
)

// Store is the slice of the store a run touches: recents read at each
// field, recency recorded after each copy, persisted after each file.
type Store interface {
	RecordValue(key, value string)
	Recents(key string, n int) []string
	Save() error
}

// maxRecents caps the offered list at the three most recent values (#6).
const maxRecents = 3

// Dest is the destination folder: collision lookups and the copy itself.
// Nothing in it can touch a source file (ADR-0001).
type Dest interface {
	Exists(name string) bool
	Copy(srcPath, name string) error
}

// Deps carries every side effect the model needs, injected whole so tests
// can substitute fakes. DestDir is display-only, used in progress lines.
type Deps struct {
	Store   Store
	Dest    Dest
	DestDir string
}

// File is one batch member: the source path and its resolved capture date
// (seam 2, issue #7). The date field type offers the capture date first (#9).
type File struct {
	Path        string
	CaptureDate time.Time
}

type copiedMsg struct{ path string }

type failedMsg struct{ err error }

// Model prompts one field at a time across the batch. values always has
// exactly one slot per scheme field — there is no absent value (ADR-0008).
type Model struct {
	files  []File
	fields []string
	deps   Deps

	fi      int      // current file index
	fx      int      // current field index
	values  []string // one per scheme field
	input   textinput.Model
	recents []string // offered for the current field, most recent first
	rc      int      // recents cursor; -1 is the free-form state (#4 prototype)
	note    string   // rejection notice, cleared on the next keystroke

	copied int
	done   bool
	err    error
}

// New builds a model over the batch: files carry full source paths in
// processing order (capture order, per ADR-0010), fields the scheme's
// field keys in prompt order.
func New(files []File, fields []string, deps Deps) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Cursor.SetMode(cursor.CursorStatic)
	ti.Focus()
	m := Model{
		files:  files,
		fields: fields,
		deps:   deps,
		values: make([]string, len(fields)),
		input:  ti,
	}
	return m.enterField("")
}

// enterField readies the prompt for the current field: fresh recents from
// the store, the input holding prefill (the earlier answer when stepping
// back), cursor at its end. The recents cursor starts on the top recent so
// repeating a value is a bare enter; a prefill or an empty history starts
// free-form.
func (m Model) enterField(prefill string) Model {
	m.recents = m.deps.Store.Recents(m.fields[m.fx], maxRecents)
	m.input.SetValue(prefill)
	m.input.CursorEnd()
	m.rc = 0
	if prefill != "" || len(m.recents) == 0 {
		m.rc = -1
	}
	return m
}

// Copied reports how many files have been copied so far; it is what the
// end-of-run (or quit) summary states.
func (m Model) Copied() int { return m.copied }

// Err is the failure that stopped the run, if any.
func (m Model) Err() error { return m.err }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.note = ""
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			return m.confirmField()
		case tea.KeyShiftTab:
			return m.backField(), nil
		case tea.KeyUp:
			if m.rc > -1 {
				m.rc--
			}
			return m, nil
		case tea.KeyDown:
			if m.rc < len(m.recents)-1 {
				m.rc++
			}
			return m, nil
		case tea.KeyRunes, tea.KeySpace, tea.KeyBackspace:
			m.rc = -1 // typing (or editing) leaves the list
		}
	case copiedMsg:
		return m.advanceFile(msg.path)
	case failedMsg:
		m.err = msg.err
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// confirmField takes the highlighted recent, or normalizes the free-form
// entry; an entry that normalizes to nothing is refused, because a composed
// name never has an empty segment (ADR-0008). The last field's confirmation
// is what triggers the copy.
func (m Model) confirmField() (tea.Model, tea.Cmd) {
	value := normalize.Value(m.input.Value())
	if m.rc >= 0 {
		value = m.recents[m.rc] // stored values are already normalized (ADR-0009)
	}
	if value == "" {
		m.note = "a value is required — this entry normalizes to nothing"
		return m, nil
	}
	m.values[m.fx] = value
	if m.fx < len(m.fields)-1 {
		m.fx++
		// Pre-fill the next field's earlier answer, if re-advancing after
		// going back — stepping back must not discard later answers.
		return m.enterField(m.values[m.fx]), nil
	}
	return m, m.copyCurrent()
}

// backField steps to the previous field with its earlier answer pre-filled
// and editable; on the first field it does nothing — the previous file is
// already copied and out of reach (ADR-0007 keeps undo out of v1).
func (m Model) backField() Model {
	if m.fx == 0 {
		return m
	}
	m.fx--
	return m.enterField(m.values[m.fx])
}

// copyCurrent returns the command performing the whole per-file write:
// resolve collisions, copy, record each value's recency, save the store.
// Copy or save failure is fatal to the run (issue #4's failure policy).
func (m Model) copyCurrent() tea.Cmd {
	src := m.files[m.fi].Path
	values := slices.Clone(m.values)
	fields := m.fields
	deps := m.deps
	return func() tea.Msg {
		name := compose.Resolve(values, filepath.Ext(src), deps.Dest.Exists)
		if err := deps.Dest.Copy(src, name); err != nil {
			return failedMsg{fmt.Errorf("copying %s: %w", filepath.Base(src), err)}
		}
		for i, key := range fields {
			deps.Store.RecordValue(key, values[i])
		}
		if err := deps.Store.Save(); err != nil {
			return failedMsg{fmt.Errorf("saving store after %s: %w", filepath.Base(src), err)}
		}
		return copiedMsg{path: filepath.Join(deps.DestDir, name)}
	}
}

// advanceFile reports the write into scrollback (tea.Println, per
// ADR-0011) and moves to the next file or ends the run.
func (m Model) advanceFile(path string) (tea.Model, tea.Cmd) {
	m.copied++
	report := tea.Println("wrote " + path)
	m.fi++
	if m.fi == len(m.files) {
		m.done = true
		return m, tea.Sequence(report, tea.Quit)
	}
	m.fx = 0
	m.values = make([]string, len(m.fields))
	return m.enterField(""), report
}

func (m Model) View() string {
	if m.done || m.err != nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "file %d/%d — %s\n", m.fi+1, len(m.files), filepath.Base(m.files[m.fi].Path))
	for i := range m.fx {
		fmt.Fprintf(&b, "  %s: %s\n", m.fields[i], m.values[i])
	}
	fmt.Fprintf(&b, "%s (%d/%d) %s\n", m.fields[m.fx], m.fx+1, len(m.fields), m.input.View())
	for i, r := range m.recents {
		marker := "  "
		if i == m.rc {
			marker = "▸ "
		}
		fmt.Fprintf(&b, "  %s%s\n", marker, r)
	}
	if m.note != "" {
		b.WriteString(m.note + "\n")
	}
	help := "enter confirm · shift+tab back · ctrl+c quit"
	if len(m.recents) > 0 {
		help = "↑↓ recents · " + help
	}
	b.WriteString(help)
	return b.String()
}
