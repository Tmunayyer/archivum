# Field values are scoped globally per field key, not per scheme

A field key owns one value history shared by every scheme that uses it — two schemes containing `movement` read from and write to the same pool of movements. The obvious alternative is scoping values under the scheme that captured them, which keeps unrelated schemes from polluting each other's suggestions. We chose global scoping because the point of recents is labeling consistency: the same physical vocabulary should come back regardless of which scheme produced the file, and a value typed once should never have to be retyped under a different scheme.

## Consequences

Field keys are a global namespace, so a key name is a commitment — reusing a generic key like `type` across unrelated schemes will blend their vocabularies. This shape is baked into the store, so reversing it means a data migration, not a code change.
