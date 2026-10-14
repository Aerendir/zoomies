# ZF-244: private preview retention and replay

14 October 2026. Preview foundation #1021 merged. Main baseline cb7fa3e1.
PR pending; E3 remains in progress.

## Scope and boundary

Retain the internal deterministic checkout preview, original complete regular
source, timing and explicit history review. No admission, collector or runtime
route. Source/timing/policy passed to the internal adapter must come from an
independently authorised server path before a future transport can use it.
Client-supplied assertions do not establish evidence or authority.

Private source-bearing records need their own authorisation boundary; existing
comparison viewer/jobs:read must not expose them. No public DTO, API schema, UI,
CLI or model tool changes. No model, publication, dispatch or causal savings.

## Storage and replay

Migration 0107, opv IDs, checkout-retained-v1 envelopes. At most 256 KiB per record,
twenty per installation, one hundred globally, fourteen-day lifetime. Atomic
installation/repository scope check, immutable insert and source-free audit.
Existing-ID identical retries retain time/audit; conflicts refuse. Scope misses,
expired/evicted history and installation deletion are unavailable, never enriched.
Private source is part of the database/backup access boundary.

Bind envelope to row ID, installation/repository, review actor, version and
no_model_call funding. Keep original mode explicitly, because repair File JSON
omits it. Replay checks digest, known-field decoding, metadata and exact recomputed
preview. Verification stays inconclusive and currentness is not_checked. Historic
recomputation never grants current policy, source/edit or publication authority.

## Acceptance and validation

Scoped immutable reads, audit rollback, caps/expiry, reopen/backup, corruption,
metadata/unknown fields and altered calculation are exercised with fixtures.
Focused race suites, migration name/idempotency checks, backup metadata checks,
replay-guard mutation, changed-package vet, strict docs and whitespace. No full
repository race suite or live performance/model result claimed.

## Next

Source/timing admission and scoped web inspection before CLI/MCP. Resolve exact
run/attempt/job/step correspondence conservatively, never infer YAML index from
API step number or matrix/generated name. Root push source is associated evidence,
not attested executed bytes. Define source/edit/history-read policy and budgets,
record actor/funding from server context, recheck head and refuse missing evidence.
Use a reviewed bounded source-bearing web DTO, not this private envelope. Explicit
approval, reconciled draft publication and E2 verification follow separately.

## Validation record

Focused store/controller/backup race fixtures pass, including scope, immutability,
audit rollback, caps/expiry, corruption, restart/backup, restored mode and exact
actor/funding/source/calculation bindings. Migration name/idempotency and backup
metadata checks pass. Disabling recomputed-preview binding makes its fixture fail;
it passes after restoration. Changed-package vet, strict docs, repository prose
and whitespace pass. Docs used the repository-only fallback for optional GitHub
header metadata. No whole-repository race run or live source/model/fleet/experiment.
