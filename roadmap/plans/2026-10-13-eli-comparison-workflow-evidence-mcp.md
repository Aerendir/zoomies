# ZF-243: present bounded workflow evidence to MCP and Eli

13 October 2026. Ready for review [#1015](https://github.com/eyupio/zoomies/pull/1015), based on main after #1014 merged.

## Behaviour

Extend the existing single scoped replay GET with an independent REST whitelist
and closed nullable schema. Validate known versions, required counters/flags,
row bounds, canonical unique IDs, full original-arm membership and exact root
commit/revision binding. Preserve nulls, path fingerprints and root-only claims.
Exclude private/future fields. No collection, provider call or new authority.

Balance whole runner/workflow/original attempt rows per arm inside existing
64 KiB MCP and 12 KiB Eli budgets including the quoted untrusted-data boundary.
Compute shown locally; retain full counts and source/model omissions. Keep one
row per available kind/arm when possible; refuse impossible budgets. Matching
root digests do not prove execution, dependencies, inputs, test scope or workload
equivalence. Older absent views stay null; present bad/future views refuse.

## Validation

Focused/full MCP race tests, combined budgets, source/model omissions, private
field exclusion, exact IDs, legacy nulls and invalid binding/count/hash/claim
refusals. The commit-binding fixture must fail with its guard removed. Scripted
Eli replay fixture verifies real transport and one read-only tool call, without
live model reasoning. Generate references and check catalogue/tool descriptions,
vet, strict docs and whitespace. All these checks pass. The commit-binding fixture
fails with its guard removed. Catalogue entries are unchanged; normal source-revision
metadata was refreshed. Docs used the repository-only header fallback. No live
fleet/provider/model operation.

## Next

CLI human workflow presentation; independent workload/dependency/configuration/
cache and complete fair selection proof remains outstanding.
