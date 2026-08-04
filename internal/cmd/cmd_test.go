// Preflight is the one piece of cmd with behaviour worth pinning: every
// required tool is named at startup, and tmux is refused loudly because
// its failure mode is silent (ADR-0006).
package cmd

import (
	"fmt"
	"slices"
	"strings"
	"testing"
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
