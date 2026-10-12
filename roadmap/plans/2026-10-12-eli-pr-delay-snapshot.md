# Eli PR delay: verified snapshot assembly

Third ZF-242 slice, stacked on the bounded GitHub evidence reader. The
controller's `AnalysePRDelayEvidence` accepts explicit repository/installation
identity, an observation window, reader coverage and selected fleet job records.
It performs no network read, SQL, model call or mutation.

## Joining evidence

Only checks posted by GitHub Actions can map to the protection reader's
Actions-pinned requirements. A check's canonical run/job-link hint must match
a retained job's run ID, job ID, repository, installation, head and exact check
name. Unknown attempts, duplicate job records or unverified links remain
incomplete. A result never borrows a passing job from another installation or
revision. Names used for matching are replaced by opaque per-snapshot IDs.

Unpinned/external requirements remain counted as unmapped checks. Missing
protection, partial job collections and malformed or inconsistent outcomes
cannot become an all-clear. Closed PRs and changed target branches are refused;
changed heads and unavailable final currentness retain the core's explicit
states. No record is read without the future caller's authorisation checks.

## Timing

Queue/provisioning/execution intervals come from job evidence, with unknown
phases left absent. An optional `CheckCompletedAt` in the core separates the
required check's reporting completion from the job's execution completion.
Reporting time contributes to observed completion delay, not execution time.
Reversed or future reporting timestamps are invalid rather than normalised.

No commit/push time is invented. The caller must define the observation window
honestly; starting at an observed queued job is not a measurement of the full
time since the developer pushed. No dependency critical path or complete merge
readiness is inferred.

## Verification and next work

Focused controller/analysis race tests pass for identity isolation, opaque
output, external requirements, missing coverage, racing outcomes, known startup,
reporting delay, retargeted/closed/stale PRs and bounds. Package vet, full
repository tests and formatting are run before final publication; record any
failure as such. No real-provider evaluation or performance benchmark is claimed.

The next slice needs an authorised controller collector and a read-only API
contract. Freeze repository selection, source freshness, request budgets and
coverage before adding MCP/CLI or UI. Do not let a transport accept arbitrary
job/check records from a client and label them verified.
