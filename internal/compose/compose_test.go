package compose

import "testing"

func TestName(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		ext    string
		want   string
	}{
		{"joins values with underscores preserving extension", []string{"bench-press", "185", "2026-06-28"}, ".mp4", "bench-press_185_2026-06-28.mp4"},
		{"single value", []string{"squat"}, ".MOV", "squat.MOV"},
		{"dashes stay inside values", []string{"farmers-walk", "2026-06-28"}, ".heic", "farmers-walk_2026-06-28.heic"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Name(c.values, c.ext); got != c.want {
				t.Errorf("Name(%v, %q) = %q, want %q", c.values, c.ext, got, c.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	taken := func(names ...string) func(string) bool {
		set := map[string]bool{}
		for _, n := range names {
			set[n] = true
		}
		return func(n string) bool { return set[n] }
	}

	cases := []struct {
		name   string
		exists func(string) bool
		want   string
	}{
		{"no collision keeps the name", taken(), "a_2026-06-28.mp4"},
		{"collision appends _1 before the extension", taken("a_2026-06-28.mp4"), "a_2026-06-28_1.mp4"},
		{"increments past taken suffixes", taken("a_2026-06-28.mp4", "a_2026-06-28_1.mp4", "a_2026-06-28_2.mp4"), "a_2026-06-28_3.mp4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Resolve("a_2026-06-28", ".mp4", c.exists); got != c.want {
				t.Errorf("Resolve = %q, want %q", got, c.want)
			}
		})
	}
}
