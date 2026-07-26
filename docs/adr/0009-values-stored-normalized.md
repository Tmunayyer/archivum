# Values are stored normalized

A value is normalized when it is entered, and the normalized form is the only form kept. Typing `Bench Press`, `bench press`, or `BENCH-PRESS` stores `bench-press` in all three cases. The alternative — store what was typed, normalize later when composing the filename — keeps a prettier recents list at the cost of letting casing and spacing variants accumulate as separate entries that all produce the identical filename. That degradation is silent and compounds over months of labeling, and it attacks the one mechanism the tool depends on for speed.

Storing the normalized form makes duplicates impossible by construction rather than merely discouraged, and it means the stored value is literally what lands in the filename. Nothing else consumes a value, so a second "display" representation would be a fiction maintained for no reader.

## The rule

1. Lowercase.
2. Delete apostrophes — `farmer's walk` must become `farmers-walk`, not `farmer-s-walk`.
3. Replace each run of non-letter, non-digit characters with a single dash.
4. Trim leading and trailing dashes.

Letters are unicode-aware (Go's `unicode.IsLetter`), so accented characters survive rather than being mangled; macOS handles them in filenames without trouble.

There is no symbol-expansion table. `clean & jerk` normalizes to `clean-jerk`, not `clean-and-jerk` — to get the latter, type the word. An expansion table has no natural boundary (`&`→and invites `+`→plus, `@`→at, `%`→percent), and every entry is an arbitrary rule that has to be remembered and matched by whoever types the next value.

## Consequences

Recents displays slug-form values rather than prettily-cased ones. Normalization is lossy and applied before storage, so the original keystrokes are unrecoverable — and changing the rule later does not retroactively re-normalize values already in the store, which would leave two conventions coexisting in one pool.
