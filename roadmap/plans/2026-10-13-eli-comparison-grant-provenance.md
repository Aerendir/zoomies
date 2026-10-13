# ZF-243: retain recorded job grant provenance

13 October 2026. Draft [#994](https://github.com/eyupio/zoomies/pull/994).
Starts after merged CLI #993, without changing experiment rules.

## Evidence audit

Job grant stamps from migration 0071 are written once at assignment and survive
runner deletion. Values are controller-calculated guaranteed size; Docker-in-Docker
accounting can use pool settings at assignment. They are not independently attested
backend limits or a complete historical resource/configuration identity. Zero/empty
is unrecorded. Image digest, workload/cache inputs and stamp observation time are
unsupported. Current mutable runner/pool data is never substituted.

## Implementation

Optional nullable recorded_grant projection from the already bounded/scoped job
reader. Known allocation source plus retained_job_granted_size_v1 basis. Invalid
negative/non-finite numbers refuse evidence; unknown dimensions stay absent.
Deep-copy into matched pinned-head job evidence only; validate retained provenance
and enumeration binding before retention/replay. Persist in the private immutable
envelope, without public report/API/tool/UI changes or new SQL/migration.

Old rows without the field replay unchanged. Older strict readers refuse new
grant-bearing rows on rollback. Resource/configuration/image identity stays unknown,
selection stays incomplete and rank directions stay not_enough. Bounds, admission,
quota, fixed rule/measurement versions and all attempt outcomes remain intact.

## Validation

Focused controller race: first-stamp collection without any runner/pool fallback,
partial/legacy/unknown/corrupt figures, pinned-head binding and deep copy, durable
private envelope and offline replay, malformed grant refusal and unchanged
eligibility. Existing collection/join/snapshot/retention fixtures and store grant
stamp/upgrade checks. Changed-package vet, strict docs and whitespace checks.
No live agent/GitHub/model call or actual experiment.

## Next

Immutable bounded runner-creation observation from the exact issued task and
explicit authenticated agent report, with only non-secret whitelisted fields.
Distinguish issued configuration from observed/applied facts. Design lifecycle and
retention before exposing a public report. Workload/cache/cohort proof remains
separate; do not expand or batch capture budgets to manufacture a ranking.

Validation completed: focused controller/store/comparison API race checks,
controller/API vet, strict docs and whitespace pass. The API fixture confirms
private durable grants and unchanged public reports with no replay collection.
Docs used the documented repository-only header fallback. No live agent/GitHub/
model call or actual experiment occurred.
