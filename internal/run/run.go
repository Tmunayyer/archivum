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
// field, recency recorded after each copy, persisted after each file —
// and, when the named scheme is unknown, the reads and writes of the
// composition flow (#10).
type Store interface {
	RecordValue(key, value string)
	Recents(key string, n int) []string
	Save() error

	// Composition: every known key is offered for reuse (the namespace
	// defence of ADR-0003), a finished scheme declares its new keys' types
	// and records itself.
	FieldKeys() []string
	FieldKeyType(key string) (string, bool)
	PutFieldKey(key, fieldType string)
	PutScheme(name string, keys []string)
}

// maxRecents caps the offered list at the three most recent values (#6).
const maxRecents = 3

// Dest is the destination folder: collision lookups and the copy itself.
// Nothing in it can touch a source file (ADR-0001).
type Dest interface {
	Exists(name string) bool
	Copy(srcPath, name string) error
}

// Previewer renders one source file into an inline payload — the Kitty
// escape printed above the prompt (ADR-0011). What the payload holds
// (image, three-frame strip) is the renderer's business, not the model's.
type Previewer interface {
	Preview(path string) (payload string, err error)
}

// Deps carries every side effect the model needs, injected whole so tests
// can substitute fakes. DestDir is display-only, used in progress lines.
// Now supplies "today" for date fields and defaults to time.Now. Preview
// is nil-able so seam tests can run without one; a real run always wires
// a renderer (cmd).
type Deps struct {
	Store   Store
	Dest    Dest
	DestDir string
	Now     func() time.Time
	Preview Previewer
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

// schemeSavedMsg reports the composed scheme persisted; the file loop
// begins on receipt (#10).
type schemeSavedMsg struct{}

// phase is where the model is: the file loop (the zero value, where New
// starts), or one of the scheme-composition steps an unknown scheme name
// opens with (#10).
type phase int

const (
	phaseFiles       phase = iota // prompting fields per file
	phaseCreateOffer              // unknown scheme: offering to compose it
	phaseKey                      // composing: naming the next field key
	phaseType                     // composing: a new key needs its type
)

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

	phase      phase
	schemeName string  // the scheme under composition
	draft      []Field // composed keys so far, in prompt order
	pendingKey string  // a new key awaiting its type

	copied int
	done   bool
	err    error
}

// New builds a model over the batch: files carry full source paths in
// processing order (capture order, per ADR-0010), fields the scheme's
// field keys in prompt order, each with its type (ADR-0008).
func New(files []File, fields []Field, deps Deps) Model {
	m := newModel(files, deps)
	m.fields = fields
	m.values = make([]string, len(fields))
	return m.enterField("")
}

// NewComposing builds a model for a scheme name the store does not know:
// it opens by offering to compose the scheme and enters the file loop only
// once composition completes and the scheme is saved (#10).
func NewComposing(files []File, schemeName string, deps Deps) Model {
	m := newModel(files, deps)
	m.schemeName = schemeName
	m.phase = phaseCreateOffer
	return m
}

func newModel(files []File, deps Deps) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Cursor.SetMode(cursor.CursorStatic)
	ti.Focus()
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return Model{files: files, deps: deps, input: ti}
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

// Init emits the first file's preview; each later file's rides its
// predecessor's advance (advanceFile). While a scheme is being composed
// the preview waits — it belongs to the file loop, not the composition.
func (m Model) Init() tea.Cmd {
	if m.phase != phaseFiles {
		return nil
	}
	return m.previewCurrent()
}

// previewCurrent returns the command rendering the current file's preview
// and printing it above the prompt — tea.Println, never the view, and no
// delete escape ever (ADR-0011). A failure prints its reason in the same
// place: one unpreviewable file never ends the batch (#8).
func (m Model) previewCurrent() tea.Cmd {
	if m.deps.Preview == nil {
		return nil
	}
	path := m.files[m.fi].Path
	render := m.deps.Preview
	return func() tea.Msg {
		payload, err := render.Preview(path)
		if err != nil {
			return tea.Println(fmt.Sprintf("no preview for %s: %v", filepath.Base(path), err))()
		}
		return tea.Println(payload)()
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.note = ""
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.phase != phaseFiles {
			return m.updateComposing(msg)
		}
		switch msg.Type {
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
	case schemeSavedMsg:
		return m.beginFiles()
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

// updateComposing routes keys while a scheme is being composed (#10). The
// offer list is the interaction recents established: ↑↓ move the cursor,
// typing drops to free-form, enter confirms; esc finishes the scheme —
// or, on the type prompt, abandons the pending key.
func (m Model) updateComposing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.phase == phaseCreateOffer {
		if msg.Type == tea.KeyEnter {
			m.phase = phaseKey
			return m.enterKeyPrompt(), nil
		}
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEnter:
		if m.phase == phaseType {
			return m.confirmType()
		}
		return m.confirmKey()
	case tea.KeyEsc:
		if m.phase == phaseType {
			m.pendingKey = ""
			m.phase = phaseKey
			return m.enterKeyPrompt(), nil
		}
		return m.finishComposing()
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
		m.rc = -1
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// enterKeyPrompt readies the next key name: every field key the store
// knows is offered for reuse, tagged with its type — the namespace defence
// of ADR-0003 — minus keys already in the draft. The cursor starts on the
// top offer so reuse is the easy path; no offers starts free-form.
func (m Model) enterKeyPrompt() Model {
	m.offers = nil
	for _, key := range m.deps.Store.FieldKeys() {
		if m.drafted(key) {
			continue
		}
		m.offers = append(m.offers, offer{value: key, tag: string(m.storedType(key))})
	}
	m.input.SetValue("")
	m.rc = 0
	if len(m.offers) == 0 {
		m.rc = -1
	}
	return m
}

// enterTypePrompt readies the type choice for a new key: the three types
// of ADR-0008, label first as the common case.
func (m Model) enterTypePrompt() Model {
	m.offers = []offer{
		{value: string(Label), tag: "recents + free-form"},
		{value: string(Number), tag: "digits, at most one decimal point"},
		{value: string(Date), tag: "capture date, today, or YYYY-MM-DD"},
	}
	m.input.SetValue("")
	m.rc = 0
	return m
}

// drafted reports whether a key is already in the scheme under composition.
func (m Model) drafted(key string) bool {
	return slices.ContainsFunc(m.draft, func(f Field) bool { return f.Key == key })
}

// storedType resolves an existing key's declared type. A key with no
// parseable declaration — the hand-seeded store shape — is a label, the
// behaviour undeclared keys have always had (cmd applies the same default).
func (m Model) storedType(key string) FieldType {
	if declared, ok := m.deps.Store.FieldKeyType(key); ok {
		if t, err := ParseFieldType(declared); err == nil {
			return t
		}
	}
	return Label
}

// confirmKey takes the highlighted existing key, or normalizes the typed
// name by the same rule values get (ADR-0009) — so "Weight LB" and
// "weight-lb" cannot mint two keys. A key the store knows keeps its type
// and is never asked again (ADR-0008); a new key goes on to the type
// prompt.
func (m Model) confirmKey() (tea.Model, tea.Cmd) {
	key := normalize.Value(m.input.Value())
	if m.rc >= 0 {
		key = m.offers[m.rc].value
	}
	if key == "" {
		m.note = "a field key name is required"
		return m, nil
	}
	if m.drafted(key) {
		m.note = key + " is already in this scheme"
		return m, nil
	}
	if slices.Contains(m.deps.Store.FieldKeys(), key) {
		m.draft = append(m.draft, Field{Key: key, Type: m.storedType(key)})
		return m.enterKeyPrompt(), nil
	}
	m.pendingKey = key
	m.phase = phaseType
	return m.enterTypePrompt(), nil
}

// confirmType fixes the pending key's type — permanent, and global to
// every scheme that will ever use the key (ADR-0008).
func (m Model) confirmType() (tea.Model, tea.Cmd) {
	value := normalize.Value(m.input.Value())
	if m.rc >= 0 {
		value = m.offers[m.rc].value
	}
	t, err := ParseFieldType(value)
	if err != nil {
		m.note = "choose label, number, or date"
		return m, nil
	}
	m.draft = append(m.draft, Field{Key: m.pendingKey, Type: t})
	m.pendingKey = ""
	m.phase = phaseKey
	return m.enterKeyPrompt(), nil
}

// finishComposing closes the key loop: the draft becomes the scheme,
// persisted before the first file is prompted (#10).
func (m Model) finishComposing() (tea.Model, tea.Cmd) {
	if len(m.draft) == 0 {
		m.note = "a scheme needs at least one field key"
		return m, nil
	}
	return m, m.saveScheme()
}

// saveScheme returns the command persisting the composed scheme: declare
// each still-undeclared key's type, record the scheme, save — the moment
// composition completes, not after the first file (#10). Save failure is
// fatal to the run (issue #4's failure policy).
func (m Model) saveScheme() tea.Cmd {
	st := m.deps.Store
	name := m.schemeName
	fields := slices.Clone(m.draft)
	return func() tea.Msg {
		keys := make([]string, len(fields))
		for i, f := range fields {
			keys[i] = f.Key
			if _, ok := st.FieldKeyType(f.Key); !ok {
				st.PutFieldKey(f.Key, string(f.Type))
			}
		}
		st.PutScheme(name, keys)
		if err := st.Save(); err != nil {
			return failedMsg{fmt.Errorf("saving scheme %s: %w", name, err)}
		}
		return schemeSavedMsg{}
	}
}

// beginFiles enters the file loop with the freshly composed scheme,
// printing its keys and types into scrollback first so the user sees what
// they built before the first file is prompted (#10).
func (m Model) beginFiles() (tea.Model, tea.Cmd) {
	m.fields = m.draft
	m.values = make([]string, len(m.fields))
	m.phase = phaseFiles
	parts := make([]string, len(m.fields))
	for i, f := range m.fields {
		parts[i] = fmt.Sprintf("%s (%s)", f.Key, f.Type)
	}
	summary := fmt.Sprintf("scheme %q saved: %s", m.schemeName, strings.Join(parts, ", "))
	m = m.enterField("")
	return m, tea.Sequence(tea.Println(summary), m.previewCurrent())
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
// ADR-0011) and moves to the next file or ends the run. The next file's
// preview is sequenced after the report, so the scrollback reads: preview,
// values written, next preview.
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
	m = m.enterField("")
	return m, tea.Sequence(report, m.previewCurrent())
}

func (m Model) View() string {
	if m.done || m.err != nil {
		return ""
	}
	if m.phase != phaseFiles {
		return m.composingView()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "file %d/%d — %s\n", m.fi+1, len(m.files), filepath.Base(m.files[m.fi].Path))
	for i := range m.fx {
		fmt.Fprintf(&b, "  %s: %s\n", m.fields[i].Key, m.values[i])
	}
	fmt.Fprintf(&b, "%s (%d/%d) %s\n", m.fields[m.fx].Key, m.fx+1, len(m.fields), m.input.View())
	m.writeOffers(&b)
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

// composingView is the scheme-composition screen: the draft so far, then
// the current prompt — a key name with the store's keys offered for reuse,
// or a new key's type (#10).
func (m Model) composingView() string {
	var b strings.Builder
	fmt.Fprintf(&b, "new scheme %q\n", m.schemeName)
	if m.phase == phaseCreateOffer {
		b.WriteString("not in the store yet — compose it now, naming its field keys in prompt order\n")
		b.WriteString("enter compose · ctrl+c quit")
		return b.String()
	}
	for i, f := range m.draft {
		fmt.Fprintf(&b, "  %d. %s (%s)\n", i+1, f.Key, f.Type)
	}
	if m.phase == phaseType {
		fmt.Fprintf(&b, "%s is new — choose its type, fixed forever %s\n", m.pendingKey, m.input.View())
	} else {
		fmt.Fprintf(&b, "field key %d %s\n", len(m.draft)+1, m.input.View())
	}
	m.writeOffers(&b)
	if m.note != "" {
		b.WriteString(m.note + "\n")
	}
	if m.phase == phaseType {
		b.WriteString("↑↓ types · enter confirm · esc back · ctrl+c quit")
	} else {
		help := "enter add · esc done · ctrl+c quit"
		if len(m.offers) > 0 {
			help = "↑↓ existing keys · " + help
		}
		b.WriteString(help)
	}
	return b.String()
}

// writeOffers renders the offer list under the prompt, cursor marked.
func (m Model) writeOffers(b *strings.Builder) {
	for i, o := range m.offers {
		marker := "  "
		if i == m.rc {
			marker = "▸ "
		}
		tag := ""
		if o.tag != "" {
			tag = " · " + o.tag
		}
		fmt.Fprintf(b, "  %s%s%s\n", marker, o.value, tag)
	}
}
