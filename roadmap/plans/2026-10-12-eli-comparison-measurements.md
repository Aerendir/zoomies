# E2 selected-run measurements

ZF-243 slice 2 follows eligibility PR #973. This remains pure and has no
runtime surface. Retain all attempt measurements in the eligibility snapshot.

- [x] Successful latency has a separate distribution (min, median, p95, max).
- [x] Execution and allocated idle consumption include every attempt outcome,
  including failed, cancelled and superseded attempts. Missing samples remain
  explicit; a total is null unless every selected attempt was measured.
- [x] Failed/completed reliability is separate from successful latency.
- [x] A fixed descriptive 1.10 ratio latency/consumption and 5 percentage point failure
  regression rule describes changes only for eligible sufficiently measured
  cohorts. No confidence interval, causal claim or cost attribution is invented.
- [x] Fixtures prove faster latency with higher consumption, failure regression,
  missing measurements, real zero, small cohorts and invalid/overflow values.
- [x] Update handoff and package records with validation and the next boundary.

Next: bounded controller assembly verifies access and complete selected evidence.
Runner consumption is execution time, not wall-clock latency. Allocated idle is
its own reported measure; repository-level costs and model costs remain absent.

Validation: new measurement tests failed before implementation. Complete analysis
package race tests and vet pass with Go 1.27.2; diff whitespace check passes.
Consumption ranks per-attempt means; totals remain separate for unequal cohorts.
Nearest-rank percentiles are documented in the live handoff. Exact five-point
failure thresholds use integer arithmetic. No live experiment or model evaluation.
