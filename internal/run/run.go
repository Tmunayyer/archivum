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
// Now supplies "today" for date fields and defaults to time.Now.
type Deps struct {
	Store   Store
	Dest    Dest
	DestDir string
	Now     func() time.Time
}

// dateLayout is the one accepted shape of a date value (ADR-0008).
const dateLayout = "2006-01-02"

// File is one file of the run: the source path and its resolved capture
// date (seam 2, issue #7). The date field type offers the capture date
// first (#9).
type File struct {
	Path        string
	CaptureDate time.Time
}

// FieldType is the behaviour of a field key, fixed at the key's creation
// and global thereafter (ADR-0008).
type FieldType string

const (
	Label  FieldType = "label"  // recents plus unrestricted free-form entry
	Number FieldType = "number" // digits with at most one decimal point; recents
	Date   FieldType = "date"   // capture date, then today, then YYYY-MM-DD free-form
)

// Field is one slot of the scheme: the field key and its type.
type Field struct {
	Key  string
	Type FieldType
}

// ParseFieldType maps a stored type string onto a FieldType, so the one
// place the vocabulary is defined is also the one place it is parsed.
func ParseFieldType(s string) (FieldType, error) {
	switch t := FieldType(s); t {
	case Label, Number, Date:
		return t, nil
	}
	return "", fmt.Errorf("unknown field type %q — expected label, number, or date", s)
}

// offer is one entry of the list shown above free-form entry: the value,
// and for date fields a display-only tag naming where it came from.
type offer struct {
	value string
	tag   string
}

type copiedMsg struct{ path string }

type failedMsg struct{ err error }

// Model prompts one field at a time across the batch. values always has
// exactly one slot per scheme field — there is no absent value (ADR-0008).
type Model struct {
	files  []File
	fields []Field
	deps   Deps

	fi     int      // current file index
	fx     int      // current field index
	values []string // one per scheme field
	input  textinput.Model
	offers []offer // the list for the current field: recents, or dates (#9)
	rc     int     // offer cursor; -1 is the free-form state (#4 prototype)
	note   string  // rejection notice, cleared on the next keystroke

	copied int
	done   bool
	err    error
}

// New builds a model over the batch: files carry full source paths in
// processing order (capture order, per ADR-0010), fields the scheme's
// field keys in prompt order, each with its type (ADR-0008).
func New(files []File, fields []Field, deps Deps) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Cursor.SetMode(cursor.CursorStatic)
	ti.Focus()
	if deps.Now == nil {
		deps.Now = time.Now
	}
	m := Model{
		files:  files,
		fields: fields,
		deps:   deps,
		values: make([]string, len(fields)),
		input:  ti,
	}
	return m.enterField("")
}

// enterField readies the prompt for the current field: the offer list —
// recents from the store, or capture date and today for a date field — and
// the input holding prefill (the earlier answer when stepping back), cursor
// at its end. The cursor starts on the top offer so the common case is a
// bare enter; a prefill or an empty list starts free-form.
func (m Model) enterField(prefill string) Model {
	if m.fields[m.fx].Type == Date {
		m.offers = dateOffers(m.files[m.fi].CaptureDate, m.deps.Now())
	} else {
		recents := m.deps.Store.Recents(m.fields[m.fx].Key, maxRecents)
		m.offers = make([]offer, len(recents))
		for i, r := range recents {
			m.offers[i] = offer{value: r}
		}
	}
	m.input.SetValue(prefill)
	m.input.CursorEnd()
	m.rc = 0
	if prefill != "" || len(m.offers) == 0 {
		m.rc = -1
	}
	return m
}

// dateOffers is the date field's list: the file's capture date first, today
// second (ADR-0008) — one entry when they agree or the capture date is
// missing (a zero capture date falls through to today, per seam 2).
func dateOffers(capture, now time.Time) []offer {
	today := offer{value: now.Format(dateLayout), tag: "today"}
	if capture.IsZero() {
		return []offer{today}
	}
	shot := offer{value: capture.Format(dateLayout), tag: "capture date"}
	if shot.value == today.value {
		return []offer{shot}
	}
	return []offer{shot, today}
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
			if m.rc < len(m.offers)-1 {
				m.rc++
			}
			return m, nil
		case tea.KeyRunes, tea.KeySpace, tea.KeyBackspace:
			if msg.Type != tea.KeyBackspace && m.fields[m.fx].Type == Number && !numberAccepts(m.input.Value(), msg) {
				m.note = "a number is digits with at most one decimal point"
				return m, nil
			}
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

// numberAccepts reports whether typing msg into a number field holding
// current keeps the grammar of ADR-0008: digits with at most one decimal
// point, nothing else — rejection happens per keystroke, on the keystroke.
func numberAccepts(current string, msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes {
		return false // space is the only other key routed here
	}
	dots := strings.Count(current, ".")
	for _, r := range msg.Runes {
		switch {
		case r >= '0' && r <= '9':
		case r == '.':
			dots++
			if dots > 1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// confirmField takes the highlighted recent, or normalizes the free-form
// entry; an entry that normalizes to nothing is refused, because a composed
// name never has an empty segment (ADR-0008). The last field's confirmation
// is what triggers the copy.
func (m Model) confirmField() (tea.Model, tea.Cmd) {
	value := normalize.Value(m.input.Value())
	switch {
	case m.rc >= 0:
		value = m.offers[m.rc].value // offered values are already canonical (ADR-0009)
	case m.fields[m.fx].Type == Number:
		// The grammar is enforced per keystroke, and normalizing would turn
		// the decimal point into a dash — a number is taken as typed. A bare
		// or trailing "." carries no digits and trims away.
		value = strings.TrimSuffix(m.input.Value(), ".")
	case m.fields[m.fx].Type == Date && value != "":
		// The shape cannot be judged mid-entry, so a free-form date is
		// validated here, on the normalized form (ADR-0008).
		if _, err := time.Parse(dateLayout, value); err != nil {
			m.note = "a date must be YYYY-MM-DD"
			return m, nil
		}
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
		for i, f := range fields {
			deps.Store.RecordValue(f.Key, values[i])
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
		fmt.Fprintf(&b, "  %s: %s\n", m.fields[i].Key, m.values[i])
	}
	fmt.Fprintf(&b, "%s (%d/%d) %s\n", m.fields[m.fx].Key, m.fx+1, len(m.fields), m.input.View())
	for i, o := range m.offers {
		marker := "  "
		if i == m.rc {
			marker = "▸ "
		}
		tag := ""
		if o.tag != "" {
			tag = " · " + o.tag
		}
		fmt.Fprintf(&b, "  %s%s%s\n", marker, o.value, tag)
	}
	if m.note != "" {
		b.WriteString(m.note + "\n")
	}
	help := "enter confirm · shift+tab back · ctrl+c quit"
	if len(m.offers) > 0 {
		list := "recents"
		if m.fields[m.fx].Type == Date {
			list = "dates"
		}
		help = "↑↓ " + list + " · " + help
	}
	b.WriteString(help)
	return b.String()
}
