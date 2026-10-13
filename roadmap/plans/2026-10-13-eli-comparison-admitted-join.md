# E2 admitted workflow-attempt joins

ZF-243 slice 5 follows merged #978, baseline main `a132e6a2`.

- [x] Collect 1..10 unique attempts under a single 20-second deadline.
- [x] Check repository/installation coverage, installation holds and optional
  reader capability before remote reads. No credential fallback.
- [x] Share the existing per-installation diagnostic gate with PR-delay reads:
  one flight, 20 collections/hour, failures charged and busy refusals free.
- [x] Reserve half of core quota after the worst-case seven requests per selected
  attempt. Refuse malformed quota; release the gate on every exit; respect new holds.
- [x] Join exact remote job IDs and pinned source head to scoped retained rows,
  reporting missing/mismatched telemetry and extra retained jobs explicitly.
  Keep workflow enumeration separate from fleet telemetry and cohort completeness.
- [x] Make no input/cache/configuration, latency, cost or verified-improvement
  assertion. There is no transport in this slice; callers still require job access.
- [x] Test successful joins, cross-head non-matches, incomplete history, read bounds,
  quota reserve, busy/hourly refusals, installation holds and release after errors.
- [x] Update handoff, roadmap and progress with evidence and next contract.

The existing admission field names remain for compatibility. Sharing is deliberate:
separate gates would permit both diagnostics to spend quota concurrently. The
limit is in memory and resets on restart, as for PR delay. A ten-attempt batch can
spend at most 70 charged reads plus one uncharged rate-limit request. Larger
cohorts need explicitly retained batches before comparison; no automatic loop.

Validation: the first controller test attempt was blocked by sandbox networking
while fetching main's updated dependencies. With explicit per-command network
access, new behavioural tests failed to compile before implementation. Focused
race checks then passed for joins/admission and the shared PR gate (11.683s),
additional quota/reader/hold/batch cases (9.030s), and the unsupported-reader guard (7.037s).
These runs cover every new test; they are not a full controller or repository run.
Controller vet, Go formatting and diff whitespace checks pass. Local fake GitHub
fixtures only; no live provider/model call or real configuration experiment.
