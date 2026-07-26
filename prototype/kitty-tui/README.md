# PROTOTYPE — Kitty images under a Bubble Tea redraw

**Throwaway code. Do not build on it.** It answers one question, then goes on a
branch off main and stays there. Issue
[#2](https://github.com/Tmunayyer/archivum/issues/2).

## The question

> Do inline Kitty-protocol images survive a Bubble Tea redraw?

Archivum's per-file loop is *show a 3-frame preview, then prompt for N fields*.
Bubble Tea repaints the frame on **every keystroke**; Kitty graphics are placed
relative to the cursor and live in the terminal's own image store, which the
renderer knows nothing about. The plausible failure is that the frames get
clobbered or scrolled away on the first repaint.

This is **not** a state-model prototype. The prompt is deliberately fake —
three hardcoded fields, hardcoded recents, no store, no file copying. The thing
under test is *what is still visible on screen* after the keystrokes land.

## Run it

```sh
./prototype/kitty-tui/run                       # generates 3 sample videos
./prototype/kitty-tui/run -dir ~/some/videos    # or use real ones
./prototype/kitty-tui/run -strategy 3           # start on a given strategy
```

Requires `ffmpeg` and `chafa` (`brew install ffmpeg chafa`).

⚠️ **Not inside tmux.** Kitty escapes don't pass through and images silently
fail to render — you'd read a false negative (ADR-0006). The program refuses to
start if `$TMUX` is set. Plain ghostty/cmux pane only.

## What you're looking for

Success criteria from the issue: after **20+ keystrokes and 3 file
transitions**, all three frames still visible above a responsive prompt — no
flicker, no disappearance, no duplicated or orphaned images drifting up the
scrollback. The header counts keystrokes and transitions for you and ticks
green when each threshold is met.

## The five strategies (`ctrl+s` cycles)

The issue named three axes worth varying. They collapse into five combinations,
switchable live so you can compare them in one session without restarting:

| # | Name | How the image gets on screen |
|---|------|------------------------------|
| 0 | `inline/println` | `tea.Println`, once per file — Bubble Tea's own "print above the viewport" channel |
| 1 | `inline/reemit`  | payload embedded in `View()`, re-sent every frame |
| 2 | `inline/raw`     | written straight to `os.Stdout`, once per file, bypassing the renderer |
| 3 | `alt/reemit`     | `WithAltScreen` + payload in `View()` every frame |
| 4 | `alt/raw`        | `WithAltScreen` + raw stdout write, once per file |

`ctrl+d` toggles the third axis: whether the Kitty delete-all escape
(`\x1b_Ga=d,d=A\x1b\\`) is sent before each emit, i.e. explicit image lifetime
management vs. letting the renderer and the terminal fight.

`ctrl+r` forces a clean repaint plus re-emit, for when a strategy has left the
screen in a state you want to un-wedge without losing the counters.

### Keys

`enter` next field (and, on the last field, next file) · `shift+tab` back a
field, pre-filled (ADR-0007) · `↑`/`↓` recents · `ctrl+s` strategy ·
`ctrl+d` delete-first · `ctrl+r` repaint · `ctrl+c` quit

## What's worth keeping

`preview/` — frame extraction at 25/50/75% (ADR-0005), hstacking the three into
one strip, and the chafa→Kitty payload. It shells out but touches no terminal
state and knows nothing about Bubble Tea, so it lifts into the real module
as-is. `main.go` is the throwaway shell.

Two things `preview` already knows that the real code will need:

- **chafa's `r=` is authoritative, not the row count you asked for.** A request
  of `100x12` came back `c=100,r=10` — aspect ratio wins. `preview.Rows()`
  reads the actual value off the payload; padding by the requested count
  misaligns everything below the image.
- **The payload is big.** ~1 MB of base64 in ~1571 escape chunks for one
  1000x200 strip. Strategy 1 and 3 push that down the pty on *every keystroke*,
  so if they render correctly but feel sluggish, that's why — and it's a
  finding, not a bug to fix here.

## Already checked (without eyes on the screen)

Builds clean, `go vet` clean. The ffmpeg → hstack → chafa pipeline produces a
valid single-placement Kitty payload. Driving `Update()` directly with the ten
keystrokes `abc ⏎ ↓ ⏎ ⇧⇥ ⏎ ↓ ⏎` yields `composed abc_185_2026-07-26.mp4` and one
file transition, so the fake prompt's own state handling is not what you're
debugging if something looks off.

Do **not** try to drive this from a script or piped stdin — `script`/pty
harnesses drop and reorder keys while the 1 MB image payloads are being
written, which looks exactly like a state bug and isn't one. Type at it.

## Verdict

_To be filled in on issue #2 once it's been driven by hand._ Nothing here has
been judged visually yet — the whole question is what the screen looks like,
and that needs a human in front of it.
