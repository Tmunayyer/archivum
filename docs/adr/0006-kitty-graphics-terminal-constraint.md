# Inline previews require a Kitty-graphics terminal, and no tmux

Previews are rendered with `chafa --format kitty`, which requires a terminal supporting the Kitty graphics protocol. The target is cmux (libghostty-based). This is a hard runtime constraint that is invisible in the code: on a terminal without the protocol, Archivum runs but shows nothing useful.

⚠️ Do **not** run Archivum nested inside `tmux` within cmux — the Kitty escape sequences do not pass through, and images silently fail to render. Use a plain cmux pane.

## Consequences

The TUI library must coexist with raw inline image output rather than owning the whole screen — this constrains the TUI choice and is worth prototyping before committing to one.
