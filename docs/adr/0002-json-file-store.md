# Global JSON file as the store for v1

The store is a single JSON file read and written with std-lib `encoding/json`. It holds schemes, field values, and usage data together — schemes are stored data, not a separate config file. The alternatives were `bbolt` (pure-Go embedded KV, transactional) and `modernc.org/sqlite` (pure-Go SQLite, SQL querying); both are viable upgrade paths and neither needs CGO, so the single-static-binary goal survives either way. For a single-user tool with one writer at a time and a store measured in kilobytes, neither buys anything yet.

## Consequences

Whole-file read and write on every mutation, and no concurrency safety — two Archivum processes running at once can lose writes. Acceptable while it is one person labeling one folder; it is the trigger to move to `bbolt`.
