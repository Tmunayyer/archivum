// Only the pure seam is tested: Rows parses the payload chafa hands back.
// The ffmpeg/ffprobe/chafa shell-outs are deliberately untested (issue #4's
// Testing Decisions) — their behaviour was pinned by the prototype (ADR-0011).
package preview

import "testing"

func TestRows(t *testing.T) {
	t.Run("reads r= off the transmit escape, not the size requested", func(t *testing.T) {
		// The shape chafa emits: a transmit control with the geometry keys,
		// then base64 chunks. Asking for 12 rows can come back r=10.
		payload := "\x1b_Ga=T,f=32,s=430,v=240,c=43,r=10,m=1,q=2\x1b\\\x1b_Gm=1;AAAA\x1b\\\x1b_Gm=0\x1b\\\n"
		if got := Rows(payload); got != 10 {
			t.Fatalf("Rows() = %d, want 10", got)
		}
	})

	t.Run("a payload with no r= key reports zero", func(t *testing.T) {
		if got := Rows("plain text, no escape"); got != 0 {
			t.Fatalf("Rows() = %d, want 0", got)
		}
	})

	t.Run("a malformed r= value reports zero", func(t *testing.T) {
		if got := Rows("\x1b_Ga=T,r=x,m=1\x1b\\"); got != 0 {
			t.Fatalf("Rows() = %d, want 0", got)
		}
	})
}
