# ZF-243: expose bounded runner evidence to MCP and Eli

13 October 2026. Ready for review [#1004](https://github.com/eyupio/zoomies/pull/1004), based on main after #1003 merged.

## Behaviour

Extend get_retained_comparison through its existing single scoped REST GET. Use
independent reviewed field whitelists and a closed nested output schema. Validate
v1 versions, known sources/kinds, positive nullable dimensions, supplied clocks,
digest syntax, canonical decimal job IDs, arm run/attempt membership and coverage
counts. Refuse unsupported or contradictory evidence; older absent views stay null.
Exclude future/private fields and all execution secrets. No direct store/controller
imports or new privileges, actions, retries, collection or provider calls.

Trim whole runner-job and attempt rows across arms inside existing 64 KiB MCP and
12 KiB Eli text budgets, including the untrusted-data boundary. Keep full enumeration
and cohort counters, locally computed shown counts, source/model omission flags and
identical text/structured rows. Keep one row per available kind/arm when possible.
Refuse when even the summaries cannot fit. Do not clip hashes/identities or promote
requested fields and agent claims to attested limits, image equivalence or eligibility.

## Validation

Focused/full MCP race tests; both output budgets, null/legacy claims, exact int64 IDs,
whitelist exclusion at every level, source/model row omission, malformed versions,
counts, bindings, resources, digests and clocks, and impossible budgets. Scripted
Eli fixture exercises real REST/replay with differing grant/request CPUs and an
injected hypothesis. This tests transport, not live model judgement. Regenerate the
MCP reference; check command/tool references, vet and strict docs.

Validation completed: full MCP race suite, focused runner/replay race fixtures,
scripted Eli route, generated command/MCP reference checks, changed-package vet,
strict docs and whitespace pass. Catalogue metadata was refreshed by make generate.
Docs used the repository-only header fallback. No live provider/model/backend run.

## Next

CLI human-readable runner facts with the same descriptive/unknown/coverage semantics.
Independent workload/configuration/cache and complete selection proof remain unresolved.
