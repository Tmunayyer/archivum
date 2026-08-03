package normalize

import "testing"

func TestValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases", "Bench Press", "bench-press"},
		{"apostrophe deleted not dashed", "Farmer's Walk", "farmers-walk"},
		{"punctuation run collapses to one dash", "clean & jerk", "clean-jerk"},
		{"space run collapses to one dash", "bench   press", "bench-press"},
		{"leading and trailing dashes trimmed", "  bench press  ", "bench-press"},
		{"already normalized unchanged", "bench-press", "bench-press"},
		{"uppercase with dashes", "BENCH-PRESS", "bench-press"},
		{"accents preserved", "Pardé Café", "pardé-café"},
		{"digits survive", "185.5", "185-5"},
		{"date shape survives", "2026-06-28", "2026-06-28"},
		{"only punctuation normalizes to empty", "  !!! ", ""},
		{"empty stays empty", "", ""},
		{"curly apostrophe deleted", "farmer’s walk", "farmers-walk"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Value(c.in); got != c.want {
				t.Errorf("Value(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
