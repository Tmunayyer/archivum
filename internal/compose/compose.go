// Package compose builds composed filenames per ADR-0004: values joined by
// underscores in scheme order, original extension preserved, collisions
// resolved by an incrementing _<n> before the extension. Names are
// write-only — nothing here (or anywhere) parses them back into fields.
package compose

import (
	"fmt"
	"strings"
)

// Name joins already-normalized values with underscores and appends the
// original extension (which keeps whatever case it arrived with).
func Name(values []string, ext string) string {
	return strings.Join(values, "_") + ext
}

// Resolve returns base+ext, or the first base_<n>+ext that exists reports
// false for, so a re-run never overwrites an earlier result. The predicate
// keeps this package free of filesystem knowledge.
func Resolve(base, ext string, exists func(name string) bool) string {
	name := base + ext
	for n := 1; exists(name); n++ {
		name = fmt.Sprintf("%s_%d%s", base, n, ext)
	}
	return name
}
