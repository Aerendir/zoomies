# E2 immutable retained evidence and offline replay

ZF-243 slice 7 follows #985. Add bounded SQLite retention, not a new external
publication or model action. SQL remains inside internal/store.

- [x] Migration 0102: immutable ID, repository/installation/actor, envelope
  version, SHA-256, JSON payload and creation time. Installation deletion cascades.
- [x] Bound payload to 2 MiB, retain at most 32 per installation and 256 globally,
  expire after 30 days. Prune with writes; expired reads return not found.
- [x] Validate installation coverage and actor identity. Insert audit atomically;
  same-ID/same-content retries do not duplicate audit or extend retention. Conflicting
  content is refused. Scope every read to repository and installation.
- [x] Controller constructs the envelope from input evidence and assembled report,
  stamps authenticated actor/server ID, and retains it before returning success.
- [x] Replay validates version/hash/scope and recomputes from stored input without
  GitHub, current pool settings or a model. Refuse altered reports or stale versions.
- [x] Test persistence, immutable retry/conflict, scopes, bounds, expiry, audit and
  deterministic offline replay. Update live handoff and package records.

Retention is a bounded history: eviction/expiry means unavailable evidence, not
an empty report. Counts bound logical payload storage; SQLite reuses freed pages
without promising immediate file shrinkage. No route or tool is added yet. Future
transport must authorise reads/writes and public visitors must remain refused.

Validation, Go 1.27.2: store/controller tests failed before implementation.
Focused store retention race tests pass (12.915s), controller replay race tests
pass (10.595s), and added corruption/retry/database-reopen cases pass (12.944s).
Older-schema upgrade and full-database backup tests pass under race (35.173s).
Store/controller vet, formatting and whitespace checks pass. No full repository
run, live provider/model request or API/UI exposure. Installation export inclusion
is deliberately deferred; full database backup preserves the table.
