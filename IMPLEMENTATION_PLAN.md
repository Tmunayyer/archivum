# Issue #9 — Number and date field types

## Stage 1: Field type plumbing
**Goal**: `run.New` takes `[]run.Field{Key, Type}`; store exposes a field key's type; cmd wires it (absent key → label for hand-seeded stores).
**Success Criteria**: All existing tests pass with the new signature; store accessor covered.
**Tests**: Existing seam-1 suite (mechanical call-site update via helper); store test for `FieldKeyType`.
**Status**: In Progress

## Stage 2: Number fields
**Goal**: Per-keystroke grammar (digits, at most one decimal point), visible rejection, recents like label, value composed as typed (never normalized — `185.5` must not become `185-5`).
**Success Criteria**: Issue #9 criteria 1–2.
**Tests**: Seam-1 keystroke tests — letter/symbol/space rejected with note, second dot rejected, `185.5` composes as typed, no-digit entry refused on confirm, recents offered and selectable.
**Status**: In Progress

## Stage 3: Date fields
**Goal**: Offers capture date first and today second (deduped; zero capture date → today only) via the list machinery; free-form must be `YYYY-MM-DD` on confirm; no store recents; confirmed values still recorded.
**Success Criteria**: Issue #9 criteria 3–5.
**Tests**: Seam-1 keystroke tests — offer order, one-keystroke select, format rejection on confirm, no history in the list, store recording, zero-date fallback.
**Status**: In Progress
