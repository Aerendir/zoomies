# ZF-243: capture known runner create results

13 October 2026. Stacked on observation foundation #996.

## Behaviour

Freeze whitelisted issued fields at queue dispatch before either agent transport
receives the task. Return a distinct top-level Spec to isolate embedded-agent
scalar preparation. Keep credentials/env out of the evidence snapshot; raw image
reference is in-memory only and is fingerprinted before retention.

ReportResult attempts retention after host/runner ownership validation and only
with exact known in-flight task/runner/pool/spec identity and successful create.
Unknown-after-restart, foreign, failed, NotStarted, missing-spec and mismatched
results remain unobserved. Missing Kind uses only known task identity. Scope is
installation ownership at receipt, not proof of original issuance ownership.

Observation retention is best effort, before lifecycle writes/lease acknowledgement.
Projection/scope/storage failure, expiry or conflict logs a bounded reason and
leaves a gap. It does not add a lifecycle error or block cleanup. Existing required
lifecycle failures still retain their retry/lease behaviour. No automatic evidence
retry. Immutable first records win, and duplicates after acknowledgement cannot
renew age. Requested configuration is never labelled applied or verified.

## Validation

Real controller queue/poll/result fixtures: frozen issued fields despite delivered
spec/current pool mutation, normal runner progression and lease acknowledgement,
duplicates, first-record conflicts, foreign/unknown/failed/not-started/mismatched
results, absent spec, legacy digest/times and observer scope failure. Existing
agent queue/requeue/host ownership/cleanup fixtures, controller vet and strict docs.
No real agent/backend/GitHub/model experiment or public report/ranking change.

## Next

Scoped pinned-job joins and private comparison retention of observation provenance,
then reviewed public report/UI fields. Resource/workload/cache/cohort proof remains
unsupported; preserve existing collection and ranking limits.

Validation completed: focused result/projection/queue/ownership and cleanup/retry
race groups pass, including closed-store required-write lease preservation.
Controller vet, strict docs and whitespace pass. Docs used the repository-only
header fallback. No live backend/agent/GitHub/model operation occurred.
