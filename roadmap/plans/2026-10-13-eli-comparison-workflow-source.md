# ZF-243: retain push root-workflow source digests

13 October 2026. Prepared on main after #1007 merged.

## Behaviour

Read one bounded root workflow file at the exact head commit for push attempts
with an unambiguous canonical local workflow path. Unsupported triggers/paths
remain unknown without source reads. Validate inline file path/type/encoding,
UTF-8 and 128 KiB size, plus Git blob integrity. Never follow download URLs or
fall back to a branch. Final attempt identity recheck includes the source read.

Retain only private versioned commit/path/blob/content digests and byte counts,
not source text. Validate retained bindings for alternate readers and replay.
Keep public reports, verified identities and eligibility unchanged. Source
permission/integrity failures refuse capture. App contents read permission is
required for supported captures. Reserve eight requests per selected attempt
through the existing whole-request quota/admission bounds. No API caller scope,
migration, dependency or public contract change.

## Validation

Focused GitHub race fixtures for pinned source, skipped contexts, invalid
responses, permissions and final movement; observe a bad-blob fixture fail when
the integrity guard is removed. Controller race fixtures cover private immutable
retention, unchanged eligibility/report bytes, binding validation and quota
boundary. Changed-package vet, strict docs and whitespace checks complete the
slice. These checks and capture/replay API race tests pass. Docs used the
repository-only header fallback. No live model, GitHub or fleet run.

## Next

Expose reviewed descriptive provenance if needed. Define transitive dependencies
and non-push revision resolution before promoting verified workflow identity.
