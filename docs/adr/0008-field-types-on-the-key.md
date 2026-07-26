# Field types are a property of the field key

A field key is a name paired with a **field type** — one of `label`, `date`, or `number` — fixed when the key is created and global thereafter. `movement` is a label in every scheme that uses it; `date` is a date in every scheme that uses it.

The alternatives were inferring the type from the key's name (the field literally called `date` gets date handling) and declaring it per scheme slot. Name-inference is invisible magic that silently withholds date intelligence from `session-date` or `recorded-on`. Per-scheme declaration puts type in two places and is incoherent with values already being global per key ([ADR-0003](./0003-field-values-scoped-globally.md)): one shared value pool cannot sensibly have two validation rules. Type on the key is the only option where the key owns all of its behaviour — value history, recents, suggestions, and validation.

## The types

- **label** — recents plus free-form entry.
- **date** — capture date, then today, then free-form enforcing `YYYY-MM-DD` ([ADR-0010](./0010-capture-time-ordering.md)).
- **number** — digits with at most one decimal point. Nothing else: no unit suffixes, no signs, no words. A unit belongs in the key name (`weight-lb`, `weight-kg`), never in the value, so that `185` and `185lb` cannot split one recents pool in two.

## No absent values

Every field in a scheme yields a value for every file. There is no empty, no placeholder, and no omitted segment — a composed name always has exactly as many segments as its scheme has fields. A field that does not apply to some files is expressed by **choosing a scheme without that field**, not by blanking it out: mobility work uses `movement, date` while lifting uses `movement, weight, date`, and `movement` keeps one shared history across both. What a given value *means* is the user's business, not Archivum's.

## Consequences

A key's type is a permanent commitment stored per key, so changing it is a data migration rather than a code change — and the strict numeric rule means a `number` key can never record a non-numeric reading. Schemes are cheap and field keys are reusable precisely so that composing a new scheme is the answer when a field does not fit.
