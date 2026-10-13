# E2 observed comparison snapshots

ZF-243 slice 6 follows #982. No persistence, dispatch or transport is added.

- [x] Version eligibility as selected-runs-v2: absent arm metadata and unknown
  attempt observation time are inconclusive evidence, not structural errors.
  Oversized strings, invalid identities, invalid windows and known timestamps
  outside the selected window remain errors. Known mismatches remain explicit.
- [x] Assemble snapshots from scoped joined evidence and an operator record.
  Retain per-attempt head/workflow identity, authoritative run outcomes and the
  GitHub attempt start as the window basis. No guessed workflow revision/inputs,
  test scope, historical configuration, cache or host state.
- [x] Sum job elapsed time only when full enumeration and every job timestamp
  pair support it. Include failures/cancellations; missing data keeps total null.
  Successful workflow latency, allocated idle and costs remain unknown.
- [x] Force selection completeness false and retain every selected attempt.
  Reject mixed repositories/installations, duplicate identities, invalid joins,
  reversed/overflowing timing and oversized arms. No cherry-picked success subset.
- [x] Test incomplete metadata, unknown starts, mixed heads, failure/cancellation,
  unknown versus zero consumption, sample bounds and no descriptive ranking.
- [x] Update live handoff and records with version and evidence limitations.

The report is an observed historical diagnosis. An operator hypothesis/delta and
an evidence identifier do not prove configuration or fair cohort selection.
Persistence of the returned snapshot/evidence must precede reproducibility claims
in a product flow. Admission remains in the collector, not this pure assembly.

Validation: missing-metadata tests failed against v1 before the change; assembler
tests failed to compile before implementation. Complete analysis race tests pass
(1.208s), focused controller snapshot race tests pass (1.159s), and the additional
bound/duration-overflow cases pass. Analysis/controller vet, Go formatting and
whitespace checks pass with Go 1.27.2. No full controller/repository run, live
GitHub request or model/performance experiment. Existing descriptive threshold
and period-analysis fixtures pass in the complete analysis package.
