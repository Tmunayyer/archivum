# Previews are printed above the Bubble Tea viewport, and never deleted

Previews are emitted with `tea.Println` — Bubble Tea's "print above the program" channel — while the program runs **inline, without alt-screen**. The prompt repaints in its own block underneath; the image sits in the scrollback above it, untouched. No Kitty deletion escape is ever sent.

This was prototyped before any production code (issue #2, branch `prototype/kitty-under-bubbletea`). The concern was that Bubble Tea repaints the whole frame on every keystroke while Kitty graphics live in the terminal's own image store, which the renderer knows nothing about — so the frames would be clobbered or scrolled away on the first repaint. **That does not happen.** Typing, arrowing through recents lists, and going back a field left the images completely undisturbed. A repainting TUI and inline Kitty previews coexist without any special handling, because Bubble Tea's inline renderer only ever repaints the lines it owns.

The one thing that *did* destroy previews was deliberate cleanup. Sending `\x1b_Ga=d,d=A\x1b\\` before each new preview — intended to stop images piling up — deletes placements **globally**, including the preview already scrolled up into the history. The terminal drops the picture, but the rows it occupied are ordinary grid rows and simply go blank, leaving a preview-shaped hole above the prompt. Not deleting is both simpler and correct: each file's preview scrolls up into the history as the batch proceeds, which reads as a log of work done.

Two details that look like noise and are not:

- **`chafa`'s returned `r=` is authoritative, not the row count requested.** Asking for `100x12` came back `c=100,r=10`; aspect ratio wins. Anything that reserves rows for an image must read the value back off the payload rather than trusting its own request, or everything below the image misaligns.
- **Do not send the payload through `View()`.** Re-emitting on every frame means pushing ~1 MB of base64 across ~1571 escape chunks down the pty per keystroke. The prototype supports it; there is no reason to use it.

## Consequences

Nothing frees the transmitted image data, so a long batch leaves every preview in the terminal's image store — roughly a megabyte per file. This is untested at batch scale and is the thing to look at first if a long run degrades. The fix, if one is needed, is deleting the *previous* preview by its specific Kitty image id, never `d=A`.

Alt-screen mode is ruled out for the main loop: it has no "above the viewport" to print into, and it would discard the scrolled-back history of previews that inline mode gives for free.

Confirms the TUI stack rather than the plain-stdin fallback, and with it the interaction design that depends on a stateful prompt — back-a-field with pre-filled editing (ADR-0007), recents lists, keystroke-driven selection. The fallback remains available and is not a disaster; it is simply not forced.
