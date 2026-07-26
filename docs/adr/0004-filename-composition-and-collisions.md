# Filename composition and collision handling

Composed names join a file's field values with underscores in the scheme's fixed order, preserving the original extension (`bench-press_185_2026-06-28.mp4`). Values already carry their internal dashes, because normalization puts them there on entry ([ADR-0009](./0009-values-stored-normalized.md)) — so the "dashes within, underscores between" rule is not one rule but two, one per stage. On collision, an incrementing `_<n>` is appended before the extension (`…_1.mp4`, `…_2.mp4`) rather than overwriting or prompting, so a run never blocks and never destroys a previous result.

**Composed names are write-only.** Nothing parses them back into fields, and nothing is designed on the assumption that anything ever will. The two-delimiter split is justified by readability and by sorting sensibly in a file browser — not by round-tripping.

## Consequences

The convention is written into filenames on disk, so changing it later does not migrate files already archived. Because names are write-only, a run leaves no record of which source file produced which copy — provenance is unrecoverable after the fact. A collision suffix is also indistinguishable from a trailing numeric field value, which costs nothing today and would need solving before any future feature reads names back.
