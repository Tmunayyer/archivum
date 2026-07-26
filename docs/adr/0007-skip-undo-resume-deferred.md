# Skip, Undo, and Resume are out of scope for v1

Three features were considered and deliberately excluded: **Skip** (defer a hard-to-identify file and return to it later), **Undo** (revert a misclassification within a run), and **Resume** (continue a half-finished run). Each requires per-run state that v1 otherwise has no reason to model — a pending queue, an action log, a checkpoint — and the guiding philosophy is to accept more manual work rather than add machinery. Since Archivum only ever copies ([ADR-0001](./0001-copy-never-move.md)), the fallback for all three is the same and non-destructive: quit, delete the bad copy from the destination, and re-run.

## Consequences

An interrupted run over a large folder restarts from the beginning, and already-copied files will collide and gain `_<n>` suffixes unless cleaned up first. If that friction bites in practice, Resume is the first of the three to reconsider.
