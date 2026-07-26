# Filename composition and collision handling

Composed names use dashes within a single field value (`bench-press`) and underscores between values (`bench-press_185_2026-06-28`), with the original extension preserved and field order fixed by the scheme. The two-delimiter split is what makes a name mechanically parseable back into its fields — a single delimiter would make `bench-press` and a field boundary indistinguishable. On collision, an incrementing `_<n>` is appended before the extension (`…_1.mp4`, `…_2.mp4`) rather than overwriting or prompting, so a run never blocks and never destroys a previous result.

## Consequences

The convention is written into filenames on disk, so changing it later does not migrate files already archived. A collision suffix is also indistinguishable from a trailing numeric field value under naive parsing.
