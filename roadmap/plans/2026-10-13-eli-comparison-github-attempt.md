# E2 bounded GitHub attempt evidence

ZF-243 slice 4 follows #977. Fleet-only rows cannot prove workflow coverage.
Add an optional read-only GitHub capability for the exact run/attempt endpoint.

- [x] Validate repository, positive run/attempt, pinned head and repository identity.
- [x] Read at most five pages of 100 jobs, retaining identities/outcomes/timestamps
  only. Preserve all conclusions; omit names, logs, steps and outputs.
- [x] Recheck the exact run attempt uncached. Changed metadata refuses the report;
  incomplete pagination, non-terminal runs or inconsistent counts stay partial.
- [x] Bound each body to 4 MiB using the existing isolated diagnostic transport,
  impose a 20-second deadline, and make at most seven requests. No shared cache
  mutation, new credential scope, automatic background read or dispatch.
- [x] Test exact attempt paths, identity refusal, head movement, coverage limits,
  incomplete counts and private text omission. Update handoff and package record.

This reader verifies job enumeration, not source/workflow inputs, cache, historical
runner configuration or an experiment cohort's selection fairness. Controller
installation holds, admission/quota reservation and local joins follow before any
runtime route. No full workflow latency is derived from incomplete fleet rows.

Validation: new behavioural tests failed to compile before implementation. Full
GitHub package race tests pass with Go 1.27.2 (4.565s), as do GitHub vet and diff
whitespace checks. Tests used local fixture servers with explicit network access;
no live GitHub or model calls and no full repository suite rerun. The five-page
fixture reaches 500 jobs and exactly seven requests, retaining partial coverage.
