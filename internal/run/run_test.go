// Seam-1 tests: the run model is driven the way a user drives it — a
// sequence of key messages — and asserted on what reached the copier and
// the store (issue #4's Testing Decisions). No test reaches into
// unexported state or calls internal helpers.
package run_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Tmunayyer/archivum/internal/run"
	"github.com/Tmunayyer/archivum/internal/store"
)

// --- fakes -----------------------------------------------------------------

type fakeStore struct {
	recorded [][2]string // key, value pairs in RecordValue order
	saves    int
}

func (s *fakeStore) RecordValue(key, value string) {
	s.recorded = append(s.recorded, [2]string{key, value})
}

func (s *fakeStore) Recents(key string, n int) []string { return nil }

func (s *fakeStore) Save() error {
	s.saves++
	return nil
}

// testStore is a real store on a temp path with a deterministic clock, so
// recency tests exercise the actual reorder behaviour through the seam.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Load(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	tick := time.Unix(1_700_000_000, 0)
	st.Now = func() time.Time { tick = tick.Add(time.Second); return tick }
	return st
}

type copyCall struct{ src, name string }

type fakeDest struct {
	existing map[string]bool
	copies   []copyCall
	err      error
}

func (d *fakeDest) Exists(name string) bool { return d.existing[name] }

func (d *fakeDest) Copy(src, name string) error {
	if d.err != nil {
		return d.err
	}
	d.copies = append(d.copies, copyCall{src, name})
	return nil
}

// fakePreviewer hands back a recognizable stand-in payload per file and
// records what it was asked to render, in order.
type fakePreviewer struct {
	calls []string
	err   error
}

func (p *fakePreviewer) Preview(path string) (string, error) {
	p.calls = append(p.calls, path)
	if p.err != nil {
		return "", p.err
	}
	return "PAYLOAD[" + filepath.Base(path) + "]", nil
}

// sources wraps bare source paths as run.Files; capture dates stay zero —
// only the date field type reads them (#9).
func sources(paths ...string) []run.File {
	files := make([]run.File, len(paths))
	for i, p := range paths {
		files[i] = run.File{Path: p}
	}
	return files
}

// labels wraps field keys as label-typed fields, the default for tests not
// exercising field types.
func labels(keys ...string) []run.Field {
	fields := make([]run.Field, len(keys))
	for i, k := range keys {
		fields[i] = run.Field{Key: k, Type: run.Label}
	}
	return fields
}

// --- keystroke driver --------------------------------------------------------

func typed(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	enter    = tea.Msg(tea.KeyMsg{Type: tea.KeyEnter})
	ctrlC    = tea.Msg(tea.KeyMsg{Type: tea.KeyCtrlC})
	up       = tea.Msg(tea.KeyMsg{Type: tea.KeyUp})
	down     = tea.Msg(tea.KeyMsg{Type: tea.KeyDown})
	shiftTab = tea.Msg(tea.KeyMsg{Type: tea.KeyShiftTab})
)

// answer types a value and confirms it.
func answer(s string) []tea.Msg { return []tea.Msg{typed(s), enter} }

func flatten(groups ...[]tea.Msg) []tea.Msg {
	var msgs []tea.Msg
	for _, g := range groups {
		msgs = append(msgs, g...)
	}
	return msgs
}

// drive feeds messages through Update, executing every returned command
// synchronously and feeding resulting messages back in, the way the Bubble
// Tea runtime would.
func drive(t *testing.T, m run.Model, msgs ...tea.Msg) run.Model {
	t.Helper()
	m, _ = record(t, m, msgs...)
	return m
}

// record drives msgs and also returns the lines pushed above the view via
// tea.Println — the channel previews and write reports ride (ADR-0011).
func record(t *testing.T, m run.Model, msgs ...tea.Msg) (run.Model, []string) {
	t.Helper()
	var printed []string
	for _, msg := range msgs {
		m = deliver(t, m, msg, &printed)
	}
	return m, printed
}

// start executes Init the way the runtime would on startup, returning the
// lines printed above the first prompt.
func start(t *testing.T, m run.Model) (run.Model, []string) {
	t.Helper()
	var printed []string
	m = execute(t, m, m.Init(), &printed)
	return m, printed
}

func deliver(t *testing.T, m run.Model, msg tea.Msg, printed *[]string) run.Model {
	t.Helper()
	next, cmd := m.Update(msg)
	return execute(t, next.(run.Model), cmd, printed)
}

func execute(t *testing.T, m run.Model, cmd tea.Cmd, printed *[]string) run.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = execute(t, m, c, printed)
		}
		return m
	case tea.QuitMsg:
		return m
	}
	if cmds, ok := sequenced(msg); ok {
		for _, c := range cmds {
			m = execute(t, m, c, printed)
		}
		return m
	}
	if line, ok := printLine(msg); ok {
		*printed = append(*printed, line)
		return m
	}
	return deliver(t, m, msg, printed)
}

// TestDriver pins the reflective matching below to bubbletea's actual
// types: if an upgrade renames them, this fails loudly instead of the
// driver silently dropping sequences and printed lines.
func TestDriver(t *testing.T) {
	t.Run("the driver still recognizes bubbletea's sequence and println messages", func(t *testing.T) {
		// Two cmds on purpose: Sequence compacts a single cmd to itself
		// and only wraps two or more in the sequenceMsg under test.
		if _, ok := sequenced(tea.Sequence(tea.Quit, tea.Quit)()); !ok {
			t.Fatal("tea.Sequence no longer yields the sequenceMsg the driver unpacks — update sequenced()")
		}
		if line, ok := printLine(tea.Println("x")()); !ok || line != "x" {
			t.Fatal("tea.Println no longer yields the printLineMessage the driver reads — update printLine()")
		}
	})
}

// sequenced unpacks bubbletea's unexported sequenceMsg ([]tea.Cmd), which
// the runtime executes strictly in order. Recognized reflectively by type
// name, the only handle an external test has on it.
func sequenced(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Name() != "sequenceMsg" {
		return nil, false
	}
	cmds := make([]tea.Cmd, v.Len())
	for i := range cmds {
		cmds[i] = v.Index(i).Interface().(tea.Cmd)
	}
	return cmds, true
}

// printLine reads bubbletea's unexported printLineMessage, the message
// tea.Println resolves to — the renderer consumes it, so the driver does too
// rather than feeding it back through Update.
func printLine(msg tea.Msg) (string, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Struct || v.Type().Name() != "printLineMessage" {
		return "", false
	}
	return v.Field(0).String(), true
}

// --- tests -------------------------------------------------------------------

func TestComposition(t *testing.T) {
	t.Run("values join in scheme order, extension preserved, one copy and one save per file", func(t *testing.T) {
		st := &fakeStore{}
		dest := &fakeDest{}
		m := run.New(
			sources("/src/IMG_0001.MOV", "/src/IMG_0002.jpg"),
			labels("movement", "weight", "date"),
			run.Deps{Store: st, Dest: dest},
		)
		m = drive(t, m, flatten(
			answer("Bench Press"), answer("185"), answer("2026-06-28"),
			answer("squat"), answer("225"), answer("2026-06-29"),
		)...)

		want := []copyCall{
			{"/src/IMG_0001.MOV", "bench-press_185_2026-06-28.MOV"},
			{"/src/IMG_0002.jpg", "squat_225_2026-06-29.jpg"},
		}
		if len(dest.copies) != 2 || dest.copies[0] != want[0] || dest.copies[1] != want[1] {
			t.Fatalf("copies = %v, want %v", dest.copies, want)
		}
		if st.saves != 2 {
			t.Fatalf("saves = %d, want one per file", st.saves)
		}
		if m.Copied() != 2 {
			t.Fatalf("Copied() = %d, want 2", m.Copied())
		}
	})
}

func TestCollisions(t *testing.T) {
	t.Run("existing names gain _n incrementing past taken suffixes", func(t *testing.T) {
		dest := &fakeDest{existing: map[string]bool{
			"squat.mp4":   true,
			"squat_1.mp4": true,
		}}
		m := run.New(sources("/src/a.mp4"), labels("movement"), run.Deps{Store: &fakeStore{}, Dest: dest})
		drive(t, m, flatten(answer("squat"))...)

		if len(dest.copies) != 1 || dest.copies[0].name != "squat_2.mp4" {
			t.Fatalf("copies = %v, want squat_2.mp4", dest.copies)
		}
	})
}

func TestNormalization(t *testing.T) {
	t.Run("typed values are normalized in both the composed name and the store", func(t *testing.T) {
		st := &fakeStore{}
		dest := &fakeDest{}
		m := run.New(sources("/src/a.jpg"), labels("movement", "place"), run.Deps{Store: st, Dest: dest})
		drive(t, m, flatten(answer("Farmer's Walk"), answer("Café  Lundi"))...)

		if len(dest.copies) != 1 || dest.copies[0].name != "farmers-walk_café-lundi.jpg" {
			t.Fatalf("copies = %v, want farmers-walk_café-lundi.jpg", dest.copies)
		}
		wantRecorded := [][2]string{{"movement", "farmers-walk"}, {"place", "café-lundi"}}
		if len(st.recorded) != 2 || st.recorded[0] != wantRecorded[0] || st.recorded[1] != wantRecorded[1] {
			t.Fatalf("recorded = %v, want %v", st.recorded, wantRecorded)
		}
	})
}

func TestEmptyRejection(t *testing.T) {
	t.Run("an empty answer and one normalizing to nothing are both refused", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.jpg"), labels("movement"), run.Deps{Store: &fakeStore{}, Dest: dest})

		m = drive(t, m, enter)
		if len(dest.copies) != 0 {
			t.Fatal("empty answer must not confirm a field")
		}
		if !strings.Contains(m.View(), "required") {
			t.Fatalf("View() = %q, want a visible rejection", m.View())
		}

		m = drive(t, m, flatten(answer("!!!"))...)
		if len(dest.copies) != 0 {
			t.Fatal("an answer normalizing to nothing must not confirm a field")
		}

		drive(t, m, flatten(answer("ok"))...)
		if len(dest.copies) != 1 || dest.copies[0].name != "ok.jpg" {
			t.Fatalf("copies = %v, want ok.jpg after the rejections", dest.copies)
		}
	})
}

func TestCopySemantics(t *testing.T) {
	t.Run("files are copied once each through the real dest and sources are untouched", func(t *testing.T) {
		srcDir, destDir := t.TempDir(), t.TempDir()
		srcs := []string{filepath.Join(srcDir, "a.jpg"), filepath.Join(srcDir, "b.jpg")}
		for _, p := range srcs {
			if err := os.WriteFile(p, []byte("original bytes of "+p), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		m := run.New(sources(srcs...), labels("movement"), run.Deps{Store: &fakeStore{}, Dest: run.DirDest{Dir: destDir}})
		m = drive(t, m, flatten(answer("squat"), answer("squat"))...)
		if err := m.Err(); err != nil {
			t.Fatal(err)
		}

		for _, p := range srcs {
			raw, err := os.ReadFile(p)
			if err != nil || string(raw) != "original bytes of "+p {
				t.Fatalf("source %s was disturbed (err=%v)", p, err)
			}
		}
		for _, name := range []string{"squat.jpg", "squat_1.jpg"} {
			if _, err := os.Stat(filepath.Join(destDir, name)); err != nil {
				t.Fatalf("expected %s in destination: %v", name, err)
			}
		}
		entries, err := os.ReadDir(destDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("destination has %d entries, want exactly one copy per file", len(entries))
		}
	})
}

func TestStoreDurability(t *testing.T) {
	t.Run("the store is written after each file, so quitting keeps completed files' values", func(t *testing.T) {
		st := &fakeStore{}
		dest := &fakeDest{}
		m := run.New(
			sources("/src/a.jpg", "/src/b.jpg"),
			labels("movement", "weight"),
			run.Deps{Store: st, Dest: dest},
		)
		m = drive(t, m, flatten(answer("squat"), answer("185"))...)
		m = drive(t, m, typed("half"), ctrlC)

		if st.saves != 1 {
			t.Fatalf("saves = %d, want exactly one — after the completed file", st.saves)
		}
		want := [][2]string{{"movement", "squat"}, {"weight", "185"}}
		if len(st.recorded) != 2 || st.recorded[0] != want[0] || st.recorded[1] != want[1] {
			t.Fatalf("recorded = %v, want only the completed file's values %v", st.recorded, want)
		}
		if m.Copied() != 1 {
			t.Fatalf("Copied() = %d, want 1 to report at quit", m.Copied())
		}
	})
}

// assertOrder fails unless every substring appears in view, in the order given.
func assertOrder(t *testing.T, view string, subs ...string) {
	t.Helper()
	last := -1
	for _, s := range subs {
		i := strings.Index(view, s)
		if i < 0 {
			t.Fatalf("View() = %q, missing %q", view, s)
		}
		if i < last {
			t.Fatalf("View() = %q, want %v in this order", view, subs)
		}
		last = i
	}
}

func TestRecents(t *testing.T) {
	// seeded history for "movement", oldest first: dip is the most recent.
	seed := func(t *testing.T) *store.Store {
		st := testStore(t)
		for _, v := range []string{"curl", "row", "squat", "dip"} {
			st.RecordValue("movement", v)
		}
		return st
	}

	t.Run("a field with history lists the top three most-recent-first", func(t *testing.T) {
		m := run.New(sources("/src/a.mp4"), labels("movement"), run.Deps{Store: seed(t), Dest: &fakeDest{}})
		view := m.View()
		assertOrder(t, view, "dip", "squat", "row")
		if strings.Contains(view, "curl") {
			t.Fatalf("View() = %q, want the fourth-most-recent value capped off the list", view)
		}
	})

	t.Run("enter alone confirms the top recent", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.mp4"), labels("movement"), run.Deps{Store: seed(t), Dest: dest})
		drive(t, m, enter)
		if len(dest.copies) != 1 || dest.copies[0].name != "dip.mp4" {
			t.Fatalf("copies = %v, want dip.mp4 from a single keystroke", dest.copies)
		}
	})

	t.Run("arrows move the highlight and enter confirms it, clamping at the ends", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.mp4"), labels("movement"), run.Deps{Store: seed(t), Dest: dest})
		drive(t, m, down, down, down, enter) // third down clamps at the last entry
		if len(dest.copies) != 1 || dest.copies[0].name != "row.mp4" {
			t.Fatalf("copies = %v, want row.mp4 (the third recent)", dest.copies)
		}
	})

	t.Run("typing at any point leaves the list for free-form entry", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.mp4"), labels("movement"), run.Deps{Store: seed(t), Dest: dest})
		drive(t, m, flatten([]tea.Msg{down}, answer("kick"))...)
		if len(dest.copies) != 1 || dest.copies[0].name != "kick.mp4" {
			t.Fatalf("copies = %v, want kick.mp4 from free-form entry", dest.copies)
		}
	})

	t.Run("arrowing up past the top returns to free-form entry", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.mp4"), labels("movement"), run.Deps{Store: seed(t), Dest: dest})
		m = drive(t, m, up, enter)
		if len(dest.copies) != 0 {
			t.Fatalf("copies = %v, want none — enter above the list is an empty free-form entry", dest.copies)
		}
		if !strings.Contains(m.View(), "required") {
			t.Fatalf("View() = %q, want the empty-entry rejection", m.View())
		}
	})

	t.Run("backspace also leaves the list, editing the typed text", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.mp4"), labels("movement"), run.Deps{Store: seed(t), Dest: dest})
		backspace := tea.Msg(tea.KeyMsg{Type: tea.KeyBackspace})
		drive(t, m, typed("xx"), down, backspace, enter)
		if len(dest.copies) != 1 || dest.copies[0].name != "x.mp4" {
			t.Fatalf("copies = %v, want x.mp4 — enter must confirm the edited text, not the highlight", dest.copies)
		}
	})

	t.Run("a field with no history shows no list", func(t *testing.T) {
		m := run.New(sources("/src/a.mp4"), labels("movement"), run.Deps{Store: &fakeStore{}, Dest: &fakeDest{}})
		if strings.Contains(m.View(), "▸") {
			t.Fatalf("View() = %q, want no recents list on an empty history", m.View())
		}
	})

	t.Run("a typed value tops the list on the very next file", func(t *testing.T) {
		st := testStore(t)
		for _, v := range []string{"curl", "row"} {
			st.RecordValue("movement", v)
		}
		m := run.New(sources("/src/a.jpg", "/src/b.jpg"), labels("movement"), run.Deps{Store: st, Dest: &fakeDest{}})
		m = drive(t, m, flatten(answer("squat"))...)
		assertOrder(t, m.View(), "▸ squat", "row", "curl")
	})

	t.Run("a selected value tops the list on the very next file", func(t *testing.T) {
		st := testStore(t)
		for _, v := range []string{"curl", "row", "squat"} {
			st.RecordValue("movement", v)
		}
		m := run.New(sources("/src/a.jpg", "/src/b.jpg"), labels("movement"), run.Deps{Store: st, Dest: &fakeDest{}})
		m = drive(t, m, down, enter) // select "row", the second recent
		assertOrder(t, m.View(), "▸ row", "squat", "curl")
	})

	t.Run("recents are global per field key, shared across schemes", func(t *testing.T) {
		st := testStore(t)
		a := run.New(sources("/src/a.jpg"), labels("movement", "weight-lb"), run.Deps{Store: st, Dest: &fakeDest{}})
		drive(t, a, flatten(answer("squat"), answer("185"))...)

		b := run.New(sources("/src/b.jpg"), labels("movement"), run.Deps{Store: st, Dest: &fakeDest{}})
		if !strings.Contains(b.View(), "▸ squat") {
			t.Fatalf("View() = %q, want squat offered under a different scheme", b.View())
		}
	})
}

func TestBackAField(t *testing.T) {
	t.Run("shift+tab pre-fills the previous answer editable with the cursor at the end", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.jpg"), labels("movement", "weight-lb"), run.Deps{Store: &fakeStore{}, Dest: dest})
		drive(t, m, flatten(
			answer("bench"),
			[]tea.Msg{shiftTab, typed("-press"), enter}, // appending proves prefill and cursor-at-end
			answer("185"),
		)...)
		if len(dest.copies) != 1 || dest.copies[0].name != "bench-press_185.jpg" {
			t.Fatalf("copies = %v, want bench-press_185.jpg after editing the earlier field", dest.copies)
		}
	})

	t.Run("shift+tab on the first field is a no-op", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.jpg"), labels("movement", "weight-lb"), run.Deps{Store: &fakeStore{}, Dest: dest})
		m = drive(t, m, shiftTab)
		if !strings.Contains(m.View(), "movement (1/2)") {
			t.Fatalf("View() = %q, want to stay on the first field", m.View())
		}
		drive(t, m, flatten(answer("squat"), answer("185"))...)
		if len(dest.copies) != 1 || dest.copies[0].name != "squat_185.jpg" {
			t.Fatalf("copies = %v, want the run to proceed normally", dest.copies)
		}
	})

	t.Run("re-advancing after going back keeps the later answers pre-filled", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.jpg"), labels("movement", "weight-lb", "date"), run.Deps{Store: &fakeStore{}, Dest: dest})
		drive(t, m, flatten(
			answer("one"), answer("two"),
			[]tea.Msg{shiftTab, shiftTab}, // back to the first field
			[]tea.Msg{enter, enter},       // re-confirm the pre-filled first and second answers
			answer("three"),
		)...)
		if len(dest.copies) != 1 || dest.copies[0].name != "one_two_three.jpg" {
			t.Fatalf("copies = %v, want one_two_three.jpg — going back must not discard later answers", dest.copies)
		}
	})

	t.Run("enter after going back confirms the pre-filled value, not a recent", func(t *testing.T) {
		st := testStore(t)
		st.RecordValue("movement", "row")
		dest := &fakeDest{}
		m := run.New(sources("/src/a.jpg"), labels("movement", "weight-lb"), run.Deps{Store: st, Dest: dest})
		drive(t, m, flatten(
			answer("bench"),
			[]tea.Msg{shiftTab, enter}, // straight back and re-confirm
			answer("185"),
		)...)
		if len(dest.copies) != 1 || dest.copies[0].name != "bench_185.jpg" {
			t.Fatalf("copies = %v, want the pre-filled bench, not the recent row", dest.copies)
		}
	})
}

func TestParseFieldType(t *testing.T) {
	t.Run("the three declared types parse; anything else errors", func(t *testing.T) {
		for _, s := range []string{"label", "number", "date"} {
			if ft, err := run.ParseFieldType(s); err != nil || string(ft) != s {
				t.Fatalf("ParseFieldType(%q) = %v, %v", s, ft, err)
			}
		}
		if _, err := run.ParseFieldType("color"); err == nil {
			t.Fatal("want an error for an undeclared type")
		}
	})
}

func TestNumberField(t *testing.T) {
	weight := []run.Field{{Key: "weight-lb", Type: run.Number}}

	t.Run("letters and symbols are rejected as they are typed, with visible feedback", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.mp4"), weight, run.Deps{Store: &fakeStore{}, Dest: dest})
		for _, bad := range []string{"a", "-", "+"} {
			m = drive(t, m, typed("1"), typed(bad))
			if !strings.Contains(m.View(), "digit") {
				t.Fatalf("View() = %q, want a visible rejection of %q", m.View(), bad)
			}
		}
		m = drive(t, m, typed("8"), enter)
		if len(dest.copies) != 1 || dest.copies[0].name != "1118.mp4" {
			t.Fatalf("copies = %v, want 1118.mp4 — no rejected keystroke may enter the value", dest.copies)
		}
	})

	t.Run("a second decimal point is rejected; the first composes as typed", func(t *testing.T) {
		st := &fakeStore{}
		dest := &fakeDest{}
		m := run.New(sources("/src/a.mp4"), weight, run.Deps{Store: st, Dest: dest})
		m = drive(t, m, typed("185.5"), typed("."))
		if !strings.Contains(m.View(), "decimal") {
			t.Fatalf("View() = %q, want a visible rejection of the second point", m.View())
		}
		m = drive(t, m, enter)
		if len(dest.copies) != 1 || dest.copies[0].name != "185.5.mp4" {
			t.Fatalf("copies = %v, want 185.5.mp4 — a number is stored as typed, never dash-normalized", dest.copies)
		}
		if len(st.recorded) != 1 || st.recorded[0] != [2]string{"weight-lb", "185.5"} {
			t.Fatalf("recorded = %v, want the decimal recorded as typed", st.recorded)
		}
	})

	t.Run("recents are offered and selectable like any field", func(t *testing.T) {
		st := testStore(t)
		for _, v := range []string{"135", "185"} {
			st.RecordValue("weight-lb", v)
		}
		dest := &fakeDest{}
		m := run.New(sources("/src/a.mp4"), weight, run.Deps{Store: st, Dest: dest})
		assertOrder(t, m.View(), "185", "135")
		drive(t, m, enter) // bare enter takes the top recent
		if len(dest.copies) != 1 || dest.copies[0].name != "185.mp4" {
			t.Fatalf("copies = %v, want 185.mp4 from a single keystroke", dest.copies)
		}
	})

	t.Run("space is rejected", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.mp4"), weight, run.Deps{Store: &fakeStore{}, Dest: dest})
		space := tea.Msg(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
		m = drive(t, m, typed("18"), space)
		if !strings.Contains(m.View(), "digit") {
			t.Fatalf("View() = %q, want a visible rejection of the space", m.View())
		}
		drive(t, m, enter)
		if len(dest.copies) != 1 || dest.copies[0].name != "18.mp4" {
			t.Fatalf("copies = %v, want 18.mp4 with the space kept out", dest.copies)
		}
	})
}

func TestDateField(t *testing.T) {
	date := []run.Field{{Key: "date", Type: run.Date}}
	// Capture dates arrive as local wall-clock in a fixed zone (seam 2) —
	// an evening time must offer as its own day, not the next.
	shot := time.Date(2026, 6, 28, 20, 15, 0, 0, time.UTC)
	today := func() time.Time { return time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC) }
	file := func(d time.Time) []run.File { return []run.File{{Path: "/src/a.mp4", CaptureDate: d}} }

	t.Run("capture date is offered first, today second; bare enter takes capture", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(file(shot), date, run.Deps{Store: &fakeStore{}, Dest: dest, Now: today})
		assertOrder(t, m.View(), "2026-06-28", "capture", "2026-08-02", "today")
		drive(t, m, enter)
		if len(dest.copies) != 1 || dest.copies[0].name != "2026-06-28.mp4" {
			t.Fatalf("copies = %v, want the capture date from a single keystroke", dest.copies)
		}
	})

	t.Run("one keystroke down selects today", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(file(shot), date, run.Deps{Store: &fakeStore{}, Dest: dest, Now: today})
		drive(t, m, down, enter)
		if len(dest.copies) != 1 || dest.copies[0].name != "2026-08-02.mp4" {
			t.Fatalf("copies = %v, want today from down+enter", dest.copies)
		}
	})

	t.Run("free-form entry is rejected on confirm unless it is YYYY-MM-DD", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(file(shot), date, run.Deps{Store: &fakeStore{}, Dest: dest, Now: today})
		m = drive(t, m, flatten(answer("june 28"))...)
		if len(dest.copies) != 0 {
			t.Fatalf("copies = %v, want a malformed date refused on confirm", dest.copies)
		}
		if !strings.Contains(m.View(), "YYYY-MM-DD") {
			t.Fatalf("View() = %q, want a visible rejection naming the shape", m.View())
		}
	})

	t.Run("a valid free-form date is accepted and recorded in the store", func(t *testing.T) {
		st := &fakeStore{}
		dest := &fakeDest{}
		m := run.New(file(shot), date, run.Deps{Store: st, Dest: dest, Now: today})
		drive(t, m, flatten(answer("2026-12-25"))...)
		if len(dest.copies) != 1 || dest.copies[0].name != "2026-12-25.mp4" {
			t.Fatalf("copies = %v, want the typed date", dest.copies)
		}
		if len(st.recorded) != 1 || st.recorded[0] != [2]string{"date", "2026-12-25"} {
			t.Fatalf("recorded = %v, want the confirmed date recorded like any value", st.recorded)
		}
	})

	t.Run("separator variants normalize to the canonical date and are accepted", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(file(shot), date, run.Deps{Store: &fakeStore{}, Dest: dest, Now: today})
		drive(t, m, flatten(answer("2026/12/25"))...)
		if len(dest.copies) != 1 || dest.copies[0].name != "2026-12-25.mp4" {
			t.Fatalf("copies = %v, want the canonical form of a real date (ADR-0009)", dest.copies)
		}
	})

	t.Run("history is never offered on a date field", func(t *testing.T) {
		st := testStore(t)
		st.RecordValue("date", "2025-01-01")
		m := run.New(file(shot), date, run.Deps{Store: st, Dest: &fakeDest{}, Now: today})
		if strings.Contains(m.View(), "2025-01-01") {
			t.Fatalf("View() = %q, want no recents on a date field", m.View())
		}
	})

	t.Run("a zero capture date offers only today", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(file(time.Time{}), date, run.Deps{Store: &fakeStore{}, Dest: dest, Now: today})
		if strings.Contains(m.View(), "capture") {
			t.Fatalf("View() = %q, want no capture offer without a capture date", m.View())
		}
		drive(t, m, enter)
		if len(dest.copies) != 1 || dest.copies[0].name != "2026-08-02.mp4" {
			t.Fatalf("copies = %v, want today as the one-keystroke answer", dest.copies)
		}
	})

	t.Run("capture date equal to today is offered once", func(t *testing.T) {
		shotToday := time.Date(2026, 8, 2, 7, 30, 0, 0, time.UTC)
		m := run.New(file(shotToday), date, run.Deps{Store: &fakeStore{}, Dest: &fakeDest{}, Now: today})
		if got := strings.Count(m.View(), "2026-08-02"); got != 1 {
			t.Fatalf("View() = %q, want the shared date listed once, got %d", m.View(), got)
		}
	})
}

func TestPreviews(t *testing.T) {
	t.Run("startup emits the first file's preview, and the prompt still asks its field", func(t *testing.T) {
		p := &fakePreviewer{}
		m := run.New(sources("/src/a.jpg", "/src/b.jpg"), labels("movement"),
			run.Deps{Store: &fakeStore{}, Dest: &fakeDest{}, Preview: p})
		m, printed := start(t, m)
		if len(printed) != 1 || printed[0] != "PAYLOAD[a.jpg]" {
			t.Fatalf("printed = %q, want the first file's payload above the prompt", printed)
		}
		if !strings.Contains(m.View(), "movement") {
			t.Fatalf("View() = %q, want the first field prompted under the preview", m.View())
		}
	})

	t.Run("a finished file reports its write, then the next file's preview, in that order", func(t *testing.T) {
		p := &fakePreviewer{}
		dest := &fakeDest{}
		m := run.New(sources("/src/a.jpg", "/src/b.jpg"), labels("movement"),
			run.Deps{Store: &fakeStore{}, Dest: dest, DestDir: "/dst", Preview: p})
		m, _ = start(t, m)
		_, printed := record(t, m, flatten(answer("squat"))...)
		want := []string{"wrote /dst/squat.jpg", "PAYLOAD[b.jpg]"}
		if len(printed) != 2 || printed[0] != want[0] || printed[1] != want[1] {
			t.Fatalf("printed = %q, want %q", printed, want)
		}
		wantCalls := []string{"/src/a.jpg", "/src/b.jpg"}
		if len(p.calls) != 2 || p.calls[0] != wantCalls[0] || p.calls[1] != wantCalls[1] {
			t.Fatalf("previewed = %q, want each file once in batch order", p.calls)
		}
	})

	t.Run("typing, recents navigation, and back-a-field never re-render the preview", func(t *testing.T) {
		st := testStore(t)
		st.RecordValue("movement", "squat")
		p := &fakePreviewer{}
		m := run.New(sources("/src/a.jpg"), labels("movement", "weight-lb"),
			run.Deps{Store: st, Dest: &fakeDest{}, Preview: p})
		m, _ = start(t, m)
		m, printed := record(t, m, flatten(
			[]tea.Msg{down, up, typed("bench"), enter},
			[]tea.Msg{shiftTab, enter}, // back to movement and re-confirm
			answer("185"),
		)...)
		if len(p.calls) != 1 {
			t.Fatalf("previewed = %q, want exactly one render for the file", p.calls)
		}
		if len(printed) != 1 || !strings.HasPrefix(printed[0], "wrote ") {
			t.Fatalf("printed = %q, want only the write report after the startup preview", printed)
		}
	})

	t.Run("a preview failure prints the reason in its place and the file is still prompted", func(t *testing.T) {
		p := &fakePreviewer{err: errors.New("ffprobe: no duration")}
		dest := &fakeDest{}
		m := run.New(sources("/src/a.jpg"), labels("movement"),
			run.Deps{Store: &fakeStore{}, Dest: dest, Preview: p})
		m, printed := start(t, m)
		if len(printed) != 1 || !strings.Contains(printed[0], "a.jpg") || !strings.Contains(printed[0], "no duration") {
			t.Fatalf("printed = %q, want the reason naming the file", printed)
		}
		drive(t, m, flatten(answer("squat"))...)
		if len(dest.copies) != 1 || dest.copies[0].name != "squat.jpg" {
			t.Fatalf("copies = %v, want the unpreviewable file still labeled and copied", dest.copies)
		}
	})

	t.Run("a run without a previewer emits nothing and still works", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.jpg"), labels("movement"), run.Deps{Store: &fakeStore{}, Dest: dest})
		m, printed := start(t, m)
		if len(printed) != 0 {
			t.Fatalf("printed = %q, want nothing without a previewer", printed)
		}
		drive(t, m, flatten(answer("squat"))...)
		if len(dest.copies) != 1 {
			t.Fatalf("copies = %v, want the run unaffected", dest.copies)
		}
	})
}

func TestProgress(t *testing.T) {
	t.Run("the prompt shows field position, total, and the values already given", func(t *testing.T) {
		m := run.New(
			sources("/src/a.jpg"),
			labels("movement", "weight-lb", "date"),
			run.Deps{Store: &fakeStore{}, Dest: &fakeDest{}},
		)
		m = drive(t, m, flatten(answer("squat"), answer("185"))...)
		view := m.View()
		for _, want := range []string{"file 1/1", "movement: squat", "weight-lb: 185", "date (3/3)"} {
			if !strings.Contains(view, want) {
				t.Fatalf("View() = %q, missing %q", view, want)
			}
		}
	})
}

func TestCopyFailure(t *testing.T) {
	t.Run("a copy failure stops the run naming the file it was on", func(t *testing.T) {
		st := &fakeStore{}
		dest := &fakeDest{err: errors.New("read-only file system")}
		m := run.New(sources("/src/IMG_0042.jpg"), labels("movement"), run.Deps{Store: st, Dest: dest})
		m = drive(t, m, flatten(answer("squat"))...)

		err := m.Err()
		if err == nil {
			t.Fatal("want an error after a failed copy")
		}
		if !strings.Contains(err.Error(), "IMG_0042.jpg") {
			t.Fatalf("error %q does not name the file", err)
		}
		if st.saves != 0 {
			t.Fatalf("saves = %d, want none after a failed copy", st.saves)
		}
	})
}
