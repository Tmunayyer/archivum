# Skip, Undo, and Resume are out of scope for v1

Three features were considered and deliberately excluded: **Skip** (defer a hard-to-identify file and return to it later), **Undo** (revert a misclassification within a run), and **Resume** (continue a half-finished run). Each requires per-run state that v1 otherwise has no reason to model — a pending queue, an action log, a checkpoint — and the guiding philosophy is to accept more manual work rather than add machinery. Since Archivum only ever copies ([ADR-0001](./0001-copy-never-move.md)), the fallback for all three is the same and non-destructive: quit, delete the bad copy from the destination, and re-run.

## What this does not defer

**Going back a field is in scope.** During a file's prompt sequence, a key returns to the previous field with its value pre-filled for editing. This is not Undo: nothing has been copied yet, so there is nothing to revert — the state involved is a slice of values and an index, entirely in memory.

The distinction matters because without it the prompt loop has no exits at all. Realising on the third field that the first one was wrong would leave only two options: finish the file knowing it is wrong and repair it by hand afterwards, or Ctrl-C and lose your place in the batch. Neither is acceptable in a tool built to label a hundred files in one sitting, and neither is what deferring Undo was meant to buy.

## Consequences

An interrupted run over a large folder restarts from the beginning, and already-copied files will collide and gain `_<n>` suffixes unless cleaned up first. If that friction bites in practice, Resume is the first of the three to reconsider.
