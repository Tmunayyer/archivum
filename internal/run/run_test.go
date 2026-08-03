// Seam-1 tests: the run model is driven the way a user drives it — a
// sequence of key messages — and asserted on what reached the copier and
// the store (issue #4's Testing Decisions). No test reaches into
// unexported state or calls internal helpers.
package run_test

import (
	"errors"
	"os"
	"path/filepath"
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
	for _, msg := range msgs {
		m = deliver(t, m, msg)
	}
	return m
}

func deliver(t *testing.T, m run.Model, msg tea.Msg) run.Model {
	t.Helper()
	next, cmd := m.Update(msg)
	return execute(t, next.(run.Model), cmd)
}

func execute(t *testing.T, m run.Model, cmd tea.Cmd) run.Model {
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
			m = execute(t, m, c)
		}
		return m
	case tea.QuitMsg:
		return m
	}
	return deliver(t, m, msg)
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

func TestNumberField(t *testing.T) {
	weight := []run.Field{{Key: "weight-lb", Type: run.Number}}

	t.Run("a letter is rejected as it is typed, with visible feedback", func(t *testing.T) {
		dest := &fakeDest{}
		m := run.New(sources("/src/a.mp4"), weight, run.Deps{Store: &fakeStore{}, Dest: dest})
		m = drive(t, m, typed("1"), typed("a"))
		if !strings.Contains(m.View(), "digit") {
			t.Fatalf("View() = %q, want a visible rejection of the letter", m.View())
		}
		m = drive(t, m, typed("8"), enter)
		if len(dest.copies) != 1 || dest.copies[0].name != "18.mp4" {
			t.Fatalf("copies = %v, want 18.mp4 — the rejected letter must not enter the value", dest.copies)
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
