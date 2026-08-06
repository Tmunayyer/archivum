# Issue #10 — Inline scheme creation

## Stage 1: Store primitives
**Goal**: `FieldKeys()` (union of declared/valued/scheme-referenced keys, sorted), `PutFieldKey`, `PutScheme` on `internal/store`.
**Success Criteria**: round-trip through Save/Load; hand-seeded keys (values only) appear in FieldKeys.
**Tests**: store unit tests.
**Status**: Complete

## Stage 2: Composition phase on the run model (seam 1)
**Goal**: `run.NewComposing(files, schemeName, deps)` — offer-creation prompt → key naming loop (existing keys offered, normalized, duplicates refused) → type asked once per new key → esc finishes → scheme saved via tea.Cmd → summary printed → file loop.
**Success Criteria**: acceptance criteria of #10 pass as keystroke tests.
**Tests**: creation end-to-end into the file loop; key reuse (no type asked, incl. hand-seeded and normalize-matched); type asked exactly once across two compositions; saved scheme survives to a fresh Load; empty/duplicate/no-key refusals.
**Status**: Not Started

## Stage 3: cmd wiring + docs
**Goal**: unknown scheme routes to `NewComposing` instead of erroring; CLAUDE.md status updated.
**Success Criteria**: build green, full suite green, vet/gofmt clean.
**Status**: Not Started
