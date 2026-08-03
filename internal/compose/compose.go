// Package compose builds composed filenames per ADR-0004: values joined by
// underscores in scheme order, original extension preserved, collisions
// resolved by an incrementing _<n> before the extension. Names are
// write-only — nothing here (or anywhere) parses them back into fields.
package compose

import (
	"fmt"
	"strings"
)

// Resolve composes a name from already-normalized values and returns it, or
// the first _<n>-suffixed variant that exists reports false for, so a
// re-run never overwrites an earlier result. The predicate keeps this
// package free of filesystem knowledge.
func Resolve(values []string, ext string, exists func(name string) bool) string {
	base := strings.Join(values, "_")
	name := base + ext
	for n := 1; exists(name); n++ {
		name = fmt.Sprintf("%s_%d%s", base, n, ext)
	}
	return name
}
