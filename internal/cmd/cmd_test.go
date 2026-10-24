// Preflight and scheme resolution are the pieces of cmd with behaviour
// worth pinning: every required tool is named at startup, tmux is refused
// loudly because its failure mode is silent (ADR-0006), and a stored scheme
// the run model cannot drive fails before the first prompt.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tmunayyer/archivum/internal/run"
	"github.com/Tmunayyer/archivum/internal/store"
)

func onPath(bins ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		if slices.Contains(bins, name) {
			return "/usr/bin/" + name, nil
		}
		return "", fmt.Errorf("%s not found", name)
	}
}

func noEnv(string) string { return "" }

func TestPreflight(t *testing.T) {
	all := []string{"exiftool", "ffmpeg", "ffprobe", "chafa"}

	t.Run("passes with every required tool on PATH, outside tmux", func(t *testing.T) {
		if err := preflight(onPath(all...), noEnv); err != nil {
			t.Fatalf("preflight() = %v, want nil", err)
		}
	})

	t.Run("each missing tool fails naming itself", func(t *testing.T) {
		for i, missing := range all {
			rest := slices.Concat(all[:i], all[i+1:])
			err := preflight(onPath(rest...), noEnv)
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("preflight() without %s = %v, want an error naming it", missing, err)
			}
		}
	})

	t.Run("refuses to start inside tmux even with every tool present", func(t *testing.T) {
		inTmux := func(key string) string {
			if key == "TMUX" {
				return "/private/tmp/tmux-501/default,613,0"
			}
			return ""
		}
		err := preflight(onPath(all...), inTmux)
		if err == nil || !strings.Contains(err.Error(), "tmux") {
			t.Fatalf("preflight() inside tmux = %v, want a loud refusal (ADR-0006)", err)
		}
	})
}

// loadStore writes raw as a store file and loads it, so resolution is
// exercised against the real store rather than a fake.
func loadStore(t *testing.T, raw string) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "store.json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return st, path
}

func TestSchemeFields(t *testing.T) {
	t.Run("a stored scheme with no field keys fails fast naming the scheme", func(t *testing.T) {
		st, path := loadStore(t, `{"version":1,"fieldKeys":{},"values":{},"schemes":{"empty":[]}}`)
		_, _, err := schemeFields(st, "empty", path)
		if err == nil || !strings.Contains(err.Error(), `"empty"`) || !strings.Contains(err.Error(), path) {
			t.Fatalf("schemeFields() = %v, want an error naming the empty scheme and the store", err)
		}
	})

	t.Run("an unknown scheme is not an error: it is left to inline composition", func(t *testing.T) {
		st, path := loadStore(t, `{"version":1,"fieldKeys":{},"values":{},"schemes":{}}`)
		fields, known, err := schemeFields(st, "sets", path)
		if err != nil || known || len(fields) != 0 {
			t.Fatalf("schemeFields() = %v, %v, %v; want no fields, unknown, nil", fields, known, err)
		}
	})

	t.Run("declared keys take their type, undeclared keys are labels, in scheme order", func(t *testing.T) {
		st, path := loadStore(t, `{"version":1,"fieldKeys":{"weight-lb":{"type":"number"},"date":{"type":"date"}},"values":{},"schemes":{"sets":["movement","weight-lb","date"]}}`)
		fields, known, err := schemeFields(st, "sets", path)
		want := []run.Field{{Key: "movement", Type: run.Label}, {Key: "weight-lb", Type: run.Number}, {Key: "date", Type: run.Date}}
		if err != nil || !known || !slices.Equal(fields, want) {
			t.Fatalf("schemeFields() = %v, %v, %v; want %v, known, nil", fields, known, err, want)
		}
	})

	t.Run("a declared type that does not parse fails naming the key, never a silent label", func(t *testing.T) {
		st, path := loadStore(t, `{"version":1,"fieldKeys":{"movement":{"type":"wat"}},"values":{},"schemes":{"sets":["movement"]}}`)
		_, _, err := schemeFields(st, "sets", path)
		if err == nil || !strings.Contains(err.Error(), "movement") {
			t.Fatalf("schemeFields() = %v, want an error naming the corrupt key", err)
		}
	})
}
