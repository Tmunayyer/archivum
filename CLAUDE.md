# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Agent skills

### Issue tracker

Issues and PRDs live in GitHub Issues (`Tmunayyer/archivum`), managed via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles, using the default label strings. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: root `CONTEXT.md` + `docs/adr/`. See `docs/agents/domain.md`.

## Status

The design is settled and the **tracer bullet is in** (issue #5): `archivum run --source <dir> --dest <dir> --scheme <name>` labels a folder end-to-end with a hand-seeded scheme of `label` fields — filename order, free-form prompts, no preview yet. Packages live under `internal/` (`normalize`, `compose`, `source`, `store`, `run`, `cmd`); `run` is seam 1, driven in tests by `tea.KeyMsg` sequences against fakes. Previews, capture-date ordering, recents, field types beyond `label`, and inline scheme creation are later tickets under #4.

## Source of Truth

The design lives in this repo:

- **`CONTEXT.md`** (root) — the domain glossary. Use its vocabulary in issue titles, test names, and identifiers.
- **`docs/adr/`** — the settled decisions and why they were made. Read the ones touching your area before implementing; flag it explicitly if your work contradicts one.

Work items live in GitHub Issues (see `docs/agents/issue-tracker.md`). New design questions become issues, not notes.

## What Archivum Is

A Go CLI tool for **batch-labeling and archiving media**. Point it at a folder of videos/images; for each file it previews the content, prompts for a configurable set of label fields, composes a new filename, and **copies** (never moves) the file to a destination folder.

At its core it's a **generic, scheme-driven media labeler** — you define a naming scheme of N fields and each file is prompted for those fields in order. The workout/movement example (`movement, weight, date`) is just one instantiation. Recently-used values are surfaced per field to keep a batch fast and consistent.

## Key Architecture

- **Scheme = ordered list of field keys.** Fields and schemes are decoupled: a field key is a **reusable global building block**; a scheme is an ordered composition of field keys.
- **Global data store**, not per-scheme config. One store holds three things: **schemes** (named ordered field-key lists), **field values** (namespaced *globally per field key* — two schemes using `movement` share one value history), and **usage/recency** (drives "top 3 most recent" suggestions per field).
- **Field type is a property of the field key**, fixed at creation and global: label (recents + free-form), date (capture date → today → free-form enforcing `YYYY-MM-DD`), number (digits with at most one decimal point — unit goes in the key name, never the value). Every field in a scheme always yields a value; there is no empty.
- **Per-file flow:** preview → prompt each field → compose name → copy to destination → advance.
- **Preview:** images rendered directly; videos previewed via **3 frames at 25% / 50% / 75%** of duration.
- **Filename composition:** dashes *within* a value (`bench-press`), underscores *between* values (`bench-press_185_2026-06-28`), original extension preserved, field order fixed by scheme. **Collisions** append `_<n>` before the extension, incrementing.

## Settled Design Decisions

Each of these is recorded in `docs/adr/` with its reasoning — read the ADR before changing the behaviour.

- [ADR-0001](docs/adr/0001-copy-never-move.md) — **copy, never move.** Originals are always preserved; no destructive path exists. Prefer more manual work over any risk to originals.
- [ADR-0002](docs/adr/0002-json-file-store.md) — global JSON file as the store for v1.
- [ADR-0003](docs/adr/0003-field-values-scoped-globally.md) — field values scoped globally per field key, not per scheme.
- [ADR-0004](docs/adr/0004-filename-composition-and-collisions.md) — filename composition and `_<n>` collision handling.
- [ADR-0005](docs/adr/0005-video-preview-three-frames.md) — videos previewed as three frames, not played.
- [ADR-0006](docs/adr/0006-kitty-graphics-terminal-constraint.md) — Kitty-graphics terminal required, no tmux.
- [ADR-0007](docs/adr/0007-skip-undo-resume-deferred.md) — Skip, Undo, and Resume deferred from v1 — but going back a field *is* in scope.
- [ADR-0008](docs/adr/0008-field-types-on-the-key.md) — field type is a property of the field key; the three types; no absent values.
- [ADR-0009](docs/adr/0009-values-stored-normalized.md) — values are normalized on entry and stored that way.
- [ADR-0010](docs/adr/0010-capture-time-ordering.md) — capture-time processing order, and the timezone trap in video dates.
- [ADR-0011](docs/adr/0011-previews-print-above-a-bubble-tea-viewport.md) — previews print above the Bubble Tea viewport via `tea.Println`, inline (no alt-screen), and are never deleted. **Prototyped, not assumed** — see issue #2.

Not yet worth an ADR: **destination** is declared per run and flat for now (subfolders possible later); the **store** lives at `~/.config/archivum/store.json`; the source walk is **flat**, filtered by an extension allowlist, skipping dotfiles and sidecars.

## Tech Stack

- **Language:** Go — single static binary. Module: `github.com/Tmunayyer/archivum`.
- **CLI:** [Cobra](https://github.com/spf13/cobra).
- **TUI:** [Bubble Tea](https://github.com/charmbracelet/bubbletea), confirmed by prototype to coexist with inline Kitty image output. Run **inline, without alt-screen**; emit previews with `tea.Println`; never send a Kitty delete escape. ADR-0011 explains why each of those three is load-bearing.
- **Frame extraction:** `ffmpeg` (frames at 25/50/75% timestamps).
- **Inline rendering:** `chafa --format kitty`. Read the row count back off the payload's `r=` — chafa preserves aspect ratio and returns fewer rows than requested.
- **EXIF / media date:** an EXIF library or `exiftool` shell-out.

### Terminal constraint

Runs in **cmux** (libghostty-based, Kitty graphics protocol) so frames render inline. ⚠️ Do **not** run Archivum nested inside `tmux` in cmux — Kitty escape sequences don't pass through tmux there and images won't render. Use a plain cmux pane.

## Getting started

```sh
go build ./...                       # build all packages
go test ./...                        # run all tests
go test -run TestName ./internal/run # run a single test
go vet ./...                         # static checks
gofmt -l -w .                        # format
```
