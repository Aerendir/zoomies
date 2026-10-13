# E2 selected-run comparison eligibility

Owner authorised continued implementation and stacked PRs. Baseline: main
`1f4bcef8`. This is ZF-243's first slice, separate from ZF-241's period
correlation. No dispatch, provider call, persistence or transport is added.

## Contract and work

- [x] Define a pure bounded snapshot of selected attempts and two configuration
  arms. Require retained evidence identifiers, initiating user, repository,
  hypothesis, intended delta, observation window and the fixed rule version.
- [x] Classify eligibility without claiming causation or measured improvement.
  Runner-only comparisons need the same source and workflow revision. Workflow
  comparisons retain the exact revision delta. Both arms need identical inputs
  and test scope, observed matched cache classes and observed quiet hosts.
- [x] Keep failures in the selected cohort. Count cancelled/superseded attempts
  separately. Missing terminal outcomes, incomplete selection, duplicate attempts
  or attempts outside the retained window make evidence inconclusive.
- [x] Fix minimum completed samples at 30 per arm in version 1. Reasons are
  stable codes, ordered deterministically; reject malformed or oversized input.
- [x] Prove matched arms, different workloads, missing metadata, noisy hosts,
  unknown caches, small samples, failures, exclusions and duplicate identity.
- [x] Update roadmap, progress and live handoff in the implementation PR.

## Following slices

Measurements remain separate: successful wall-clock duration, all-attempt runner
consumption, allocation, reliability and administrator-rate cost estimates.
This eligibility result authorises no publication, dispatch or optimisation.
Controller assembly must verify repository authorisation and load all attempts
in the declared selection, including failures. A caller's completeness assertion
is evidence supplied to the pure core, not an independently verified fact.
REST/CLI/MCP/Eli/UI follow bounded controller assembly. E3 may not treat an
eligible historical cohort as proof of causation or verified improvement.

Validation: new tests failed to compile before implementation. Complete analysis
package race tests and vet pass with Go 1.27.2; `git diff --check` passes.
No live provider/model calls or full repository rerun were needed for this pure slice.
