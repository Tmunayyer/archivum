# Archivum

Archivum is a generic, scheme-driven media labeler: it walks a folder of videos and images, prompts for a configurable set of fields per file, and copies each file to a destination folder under a composed name. The workout/movement use case is one instantiation, not the domain.

## Language

### Naming model

**Scheme**:
A named, ordered list of field keys that determines which fields a file is prompted for and in what order. Every field in a scheme yields a value for every file — a field that does not apply is expressed by choosing a scheme without it, not by leaving it blank.
_Avoid_: template, format, profile, config

**Field key**:
A globally-unique name paired with a field type — the reusable building block a scheme composes. The key owns its value history, its recents, and its input behaviour.
_Avoid_: column, attribute, tag, property

**Field value**:
A single entry recorded against a field key, always stored in normalized form (`bench-press`, `185`). Values belong to the field key, not to the scheme that captured them. There is no absent value — what a value means is the user's business, not Archivum's.
_Avoid_: label, entry, answer

**Normalization**:
The transformation applied to a value on entry, before it is stored — the stored value is the normalized one, and no other form is kept. This is what makes duplicate values impossible rather than merely discouraged.
_Avoid_: slugify, sanitize, clean

**Field type**:
A property of the field key, fixed when the key is created, that determines how its values are suggested and validated. One of **label**, **date**, or **number**. Every type stores one canonical form; how a value reaches it differs by type (see ADR-0009).

**Recents**:
The three most-recently-used values for a field key, offered as one-keystroke choices before free-form entry.
_Avoid_: suggestions, history, autocomplete

**Offer list**:
The one-keystroke choices shown above free-form entry for the current field — recents for label and number fields, capture date then today for date fields.
_Avoid_: menu, options, picker

**Composed name**:
The filename built by joining a file's field values with underscores, preserving the original extension (`bench-press_185_2026-06-28.mp4`). Values already carry their internal dashes from normalization.
_Avoid_: new name, output name, formatted name

**Collision suffix**:
The incrementing `_<n>` appended before the extension when a composed name already exists in the destination.

### Run model

**Store**:
The single global JSON file holding every scheme, field value, and usage record. There is one store across all invocations — no per-scheme or per-run config.
_Avoid_: database, config, state file

**Run**:
One invocation of Archivum over a source folder with a chosen scheme and a declared destination.
_Avoid_: session, batch, job

**Source folder**:
The folder Archivum reads files from. Never written to.

**Destination folder**:
The flat folder that composed copies are written to, declared at the start of each run.
_Avoid_: output dir, archive, target

**Preview**:
The inline terminal rendering shown before prompting — the image itself, or three extracted frames for a video.
_Avoid_: thumbnail, thumbnails

**Capture date**:
The date a file's content was recorded, read from the media's own metadata and expressed in local time. Distinct from file modification time, which records when the file arrived on this machine.
_Avoid_: EXIF date, creation date, timestamp
