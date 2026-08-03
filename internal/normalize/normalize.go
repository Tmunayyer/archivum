// Package normalize implements the value normalization rule of ADR-0009:
// lowercase, apostrophes deleted, runs of non-letter/non-digit characters
// collapsed to a single dash, leading and trailing dashes trimmed. Letters
// are unicode-aware, so accented characters survive.
package normalize

import (
	"strings"
	"unicode"
)

// Value returns the normalized form of s. A result of "" means the input
// carried nothing usable and must be rejected by the caller — there is no
// absent value (ADR-0008).
func Value(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r == '\'' || r == '’':
			// Deleted outright: farmer's walk → farmers-walk, not farmer-s-walk.
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	return b.String()
}
