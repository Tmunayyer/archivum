# Archivum

Archivum is a generic, scheme-driven media labeler: it walks a folder of videos and images, prompts for a configurable set of fields per file, and copies each file to a destination folder under a composed name. The workout/movement use case is one instantiation, not the domain.

## Language

### Naming model

**Scheme**:
A named, ordered list of field keys that determines which fields a file is prompted for and in what order.
_Avoid_: template, format, profile, config

**Field key**:
A reusable, globally-unique name for one labelable attribute (`movement`, `weight`, `date`). Field keys are building blocks; a scheme composes them.
_Avoid_: column, attribute, tag, property

**Field value**:
A single entry recorded against a field key (`bench-press`, `185`). Values belong to the field key, not to the scheme that captured them.
_Avoid_: label, entry, answer

**Field type**:
The input behaviour a field key gets — label, date, or number. Type changes how suggestions are produced, not how values are stored.

**Recents**:
The three most-recently-used values for a field key, offered as one-keystroke choices before free-form entry.
_Avoid_: suggestions, history, autocomplete

**Composed name**:
The filename built from a file's field values — dashes within a value, underscores between values, original extension preserved (`bench-press_185_2026-06-28.mp4`).
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
