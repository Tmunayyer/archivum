# Files are processed in capture-time order

A run sorts the source folder by each file's capture date before prompting, rather than by filename or filesystem order. Recents only pays off when consecutive files share values — eight sets of the same movement shot back to back means seven files need one keystroke instead of a typed value. Capture order produces those runs; filename order only approximates it for single-camera folders and interleaves into nonsense as soon as a folder mixes sources.

The cost is a metadata pass over the whole folder before the first prompt appears. This is paid for twice over, since the same pass supplies each file's `date` suggestion, and exiftool reads a whole directory in one invocation.

## Resolving the capture date

Tag priority is `DateTimeOriginal → CreateDate → FileModifyDate`, read in a single exiftool pass with **`-api QuickTimeUTC=1`**.

That flag is load-bearing and must not be removed as noise. EXIF `DateTimeOriginal` (images) is local-naive wall-clock time, while QuickTime `CreateDate` (video) is specified as UTC. Read identically, an 8pm session recorded on 2026-06-28 in PDT reports `2026-06-29 03:00` and gets dated a day late — while stills from the same session, in the same folder, date correctly. One session splits across two dates, silently, and only for recordings made late in the day. The flag makes exiftool convert QuickTime's UTC to local time so both agree.

`FileModifyDate` sits last as a weak fallback: it is routinely rewritten by copies, AirDrop and cloud sync, so it often records when a file arrived on this machine rather than when its content was shot. When no tag yields anything, the date field falls through to today, then to free-form entry.

## Consequences

Correctness here depends on an exiftool flag rather than on Archivum's own code, so it needs a test fixture with a real evening video — the failure is invisible for any recording made before late afternoon.
