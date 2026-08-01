package compose

import "testing"

func taken(names ...string) func(string) bool {
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	return func(n string) bool { return set[n] }
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		ext    string
		exists func(string) bool
		want   string
	}{
		{"joins values with underscores preserving extension", []string{"bench-press", "185", "2026-06-28"}, ".mp4", taken(), "bench-press_185_2026-06-28.mp4"},
		{"single value", []string{"squat"}, ".MOV", taken(), "squat.MOV"},
		{"dashes stay inside values", []string{"farmers-walk", "2026-06-28"}, ".heic", taken(), "farmers-walk_2026-06-28.heic"},
		{"collision appends _1 before the extension", []string{"a", "2026-06-28"}, ".mp4", taken("a_2026-06-28.mp4"), "a_2026-06-28_1.mp4"},
		{"increments past taken suffixes", []string{"a", "2026-06-28"}, ".mp4", taken("a_2026-06-28.mp4", "a_2026-06-28_1.mp4", "a_2026-06-28_2.mp4"), "a_2026-06-28_3.mp4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Resolve(c.values, c.ext, c.exists); got != c.want {
				t.Errorf("Resolve(%v, %q) = %q, want %q", c.values, c.ext, got, c.want)
			}
		})
	}
}
