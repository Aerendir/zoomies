# Eli PR delay: first calculation slice

Status: implementation started at the owner's request, 12 October 2026.
Package ZF-242; enhancement handoff track E1. Local baseline `1e745d25`;
upstream refreshed to `d382fb7a` before publication. Neither inspected tree has
`internal/analysis` yet. Reconcile with any ZF-241
implementation before integrating; do not replace another analysis package.

## Scope and contracts

This slice implements the pure calculation and behavioural fixtures. It does
not expose a route, MCP tool or UI action, and makes no GitHub or model request.
No source, mutation or dispatch permission changes.

`analysis.MergeDelay` takes a snapshot with a pinned head, independently observed
current head, target branch, observation window, explicit protection/observation
coverage, opaque required-check IDs and observations mapped by a future reader.
The reader is responsible for producer identity and rules applying to that
target branch. It must not derive required checks from job names or silently
reuse default-branch rules. Separate matrix checks can have separate required IDs;
multiple matrix instances mapped to one identity remain ambiguous.

Observations identify their run, logical instance and attempt. The highest
attempt within an instance supersedes earlier observations; independent runs or
instances claiming the same required identity are ambiguous. Other heads cannot
contribute to the pinned head's timing. Absent current-head evidence is unknown;
a different current head supersedes the snapshot.

Result version `merge-delay-v1` records per-check states and known timing, plus
the last observed completed required check. It never claims a dependency critical
path, cause of slowness, safe optimisation or complete merge readiness. The result
has at most 500 required checks and the input at most 5,000 observations. Oversize
inputs are refused rather than silently truncated.

Pre-start wait covers queue-to-job-start. When runner provisioning start is
known, split that interval into queue-to-provisioning and provisioning-to-start.
Do not sum pre-start wait and startup again. Missing provisioning time leaves
queue/startup unknown while retaining known pre-start and execution intervals.
Malformed, reversed or future timestamps produce invalid timing, not zero wait.
All completion delays are relative to the explicit head observation window.

Complete observations and protection may produce checks-passed, checks-failed
or checks-pending. Missing, unmapped, ambiguous or invalid evidence yields
incomplete. No required checks yields its own state. These describe check
observations only; approvals, conflicts and merge queues are outside the contract.

## Evidence and remaining integration

Behavioural fixtures cover interval accounting, partial/refused reads, external
checks, pending/failing checks, old heads, unknown currentness, retries, matrix
members, competing producers, absent timestamps, stable ordering, input bounds
and no-required-check semantics. The purity test forbids IO imports and implicit
clock reads in production code.

Initial validation was blocked because `/usr/bin/go` is an unrelated program.
On continuing the work, explicit shell network access made the proxy available;
Go 1.27.2 was installed in `/tmp` from the official release with its published
SHA-256 verified. `gofmt`, `go test -race -count=1 ./internal/analysis` and
`go vet ./internal/analysis` now pass. No route or existing runtime path is
changed by this slice. Full repository checks remain separate integration
validation. No live model or GitHub experiment was run.

Next implementation needs bounded authorised protection/check readers, reader
coverage tests and controller assembly. Freeze that route contract before
building the UI; do not wire this calculation to incomplete default-branch
Kennel counts and call it a PR diagnosis.
