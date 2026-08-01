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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Tmunayyer/archivum/internal/run"
)

// --- fakes -----------------------------------------------------------------

type fakeStore struct {
	recorded [][2]string // key, value pairs in RecordValue order
	saves    int
}

func (s *fakeStore) RecordValue(key, value string) {
	s.recorded = append(s.recorded, [2]string{key, value})
}

func (s *fakeStore) Save() error {
	s.saves++
	return nil
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

// --- keystroke driver --------------------------------------------------------

func typed(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	enter = tea.Msg(tea.KeyMsg{Type: tea.KeyEnter})
	ctrlC = tea.Msg(tea.KeyMsg{Type: tea.KeyCtrlC})
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
			[]string{"/src/IMG_0001.MOV", "/src/IMG_0002.jpg"},
			[]string{"movement", "weight", "date"},
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
		m := run.New([]string{"/src/a.mp4"}, []string{"movement"}, run.Deps{Store: &fakeStore{}, Dest: dest})
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
		m := run.New([]string{"/src/a.jpg"}, []string{"movement", "place"}, run.Deps{Store: st, Dest: dest})
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
		m := run.New([]string{"/src/a.jpg"}, []string{"movement"}, run.Deps{Store: &fakeStore{}, Dest: dest})

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

		m := run.New(srcs, []string{"movement"}, run.Deps{Store: &fakeStore{}, Dest: run.DirDest{Dir: destDir}})
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
			[]string{"/src/a.jpg", "/src/b.jpg"},
			[]string{"movement", "weight"},
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

func TestCopyFailure(t *testing.T) {
	t.Run("a copy failure stops the run naming the file it was on", func(t *testing.T) {
		st := &fakeStore{}
		dest := &fakeDest{err: errors.New("read-only file system")}
		m := run.New([]string{"/src/IMG_0042.jpg"}, []string{"movement"}, run.Deps{Store: st, Dest: dest})
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
