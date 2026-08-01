// Package run holds the Bubble Tea model driving a labeling run: prompt
// each field of the scheme per file, compose the name, copy, record. The
// model performs no I/O itself — every side effect goes through the
// interfaces in Deps as a tea.Cmd, which is what makes this the seam most
// tests drive (issue #4, seam 1).
package run

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Tmunayyer/archivum/internal/compose"
	"github.com/Tmunayyer/archivum/internal/normalize"
)

// Store is the slice of the store a run mutates: recency after each copy,
// persisted after each file.
type Store interface {
	RecordValue(key, value string)
	Save() error
}

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

type copiedMsg struct{ path string }

type failedMsg struct{ err error }

// Model prompts one field at a time across the batch. values always has
// exactly one slot per scheme field — there is no absent value (ADR-0008).
type Model struct {
	files  []string
	fields []string
	deps   Deps

	fi     int      // current file index
	fx     int      // current field index
	values []string // one per scheme field
	input  textinput.Model
	note   string // rejection notice, cleared on the next keystroke

	copied int
	done   bool
	err    error
}

// New builds a model over the batch: files are full source paths in
// processing order, fields the scheme's field keys in prompt order.
func New(files, fields []string, deps Deps) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Cursor.SetMode(cursor.CursorStatic)
	ti.Focus()
	return Model{
		files:  files,
		fields: fields,
		deps:   deps,
		values: make([]string, len(fields)),
		input:  ti,
	}
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
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			return m.confirmField()
		}
		m.note = ""
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

// confirmField normalizes the current entry; an entry that normalizes to
// nothing is refused, because a composed name never has an empty segment
// (ADR-0008). The last field's confirmation is what triggers the copy.
func (m Model) confirmField() (tea.Model, tea.Cmd) {
	value := normalize.Value(m.input.Value())
	if value == "" {
		m.note = "a value is required — this entry normalizes to nothing"
		return m, nil
	}
	m.note = ""
	m.values[m.fx] = value
	m.input.Reset()
	if m.fx < len(m.fields)-1 {
		m.fx++
		return m, nil
	}
	return m, m.copyCurrent()
}

// copyCurrent returns the command performing the whole per-file write:
// resolve collisions, copy, record each value's recency, save the store.
// Copy or save failure is fatal to the run (issue #4's failure policy).
func (m Model) copyCurrent() tea.Cmd {
	src := m.files[m.fi]
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
	return m, report
}

func (m Model) View() string {
	if m.done || m.err != nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "file %d/%d — %s\n", m.fi+1, len(m.files), filepath.Base(m.files[m.fi]))
	for i := range m.fx {
		fmt.Fprintf(&b, "  %s: %s\n", m.fields[i], m.values[i])
	}
	fmt.Fprintf(&b, "%s (%d/%d) %s\n", m.fields[m.fx], m.fx+1, len(m.fields), m.input.View())
	if m.note != "" {
		b.WriteString(m.note + "\n")
	}
	b.WriteString("enter confirm · ctrl+c quit")
	return b.String()
}
