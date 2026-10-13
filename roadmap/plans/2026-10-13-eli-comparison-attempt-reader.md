# E2 bounded retained-attempt reader

ZF-243 slice 3 follows #974. Existing ListJobsForRun reads every attempt
without a row bound. Comparisons need explicit run/attempt identity, installation
and repository scope, all outcomes and a visible truncation marker.

- [x] Add a store read restricted by repository, installation, run and attempt.
  Validate positive identities and a 1..1000 job limit; fetch at most limit+1.
- [x] Preserve failed, cancelled, skipped and pending jobs. Never filter by
  success or collapse retries. Stable order is queued_at then id.
- [x] Report truncation and bound the returned rows. Empty means no retained
  evidence, not a complete empty workflow. No retention/full-GitHub coverage claim.
- [x] Add a controller collector restricted to an installation covering the
  repository. Bound selection to 100 attempts and 2000 total returned jobs,
  refuse duplicates before reading and use a 20-second context deadline.
- [x] Keep selection coverage false. The fleet store cannot prove full workflow
  coverage, inputs, cache or historical configuration. No measurement ranking.
- [x] Test exact scope, retry preservation, all outcomes, truncation, invalid
  arguments, controller joins and duplicate selection. Update the live handoff.

The transport must authorise the caller's read access before invoking collection.
No credentials, GitHub calls, dispatch, publication, migration or new dependency.
Next: join independently retained workload/configuration evidence and establish
coverage before converting these rows into an ExperimentSnapshot. Then transports.

Validation, Go 1.27.2: the new store/controller behavioural tests failed to
compile before implementation. Focused store race tests pass (8.358s), focused
controller collection race tests pass (23.298s), full analysis race tests pass
(1.179s), store/controller vet and diff whitespace checks pass. Initial controller
runs were blocked by the sandbox's localhost socket policy; the successful run
used explicit per-command network permission for the existing fake GitHub harness.
No live GitHub request or full repository test rerun was made.
