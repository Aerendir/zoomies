# ZF-243: retain private runner observation joins

13 October 2026. Draft [#1000](https://github.com/eyupio/zoomies/pull/1000), based on main after #994, #996 and #998 merged.

## Behaviour

Read historical scoped job runner/pool/available host links and immutable runner
observations without consulting current runner or configuration rows. Missing or
expired observations stay unknown; contradictory links, corrupt evidence and read
failures refuse capture. Copy only after the existing GitHub job/pinned head match.

Retain the full bounded non-secret observation, its original scoped digest, receipt
and historical links in the private comparison envelope. Assembly and replay check
version/digest/scope/job bindings and duplicate observations across arms. Deep-copy
all optional pointers. Replay uses no live source observation or freshness clock.
The inner digest covers installation/Data; the outer comparison digest covers
receipt and job bindings. Neither digest attests backend limits or image equivalence.

Public reports and eligibility are unchanged. Existing 20-second budget, shared
admission, ten selected remote attempts, 2,000 local jobs, 4 KiB observation payload
and 2 MiB comparison envelope bounds remain. At most one indexed observation read
per linked local job is added. Oversized final envelopes are refused without trimming.
Old private envelopes replay; strict older binaries reject the new optional fields.

## Validation

Scoped collection, pruning, source expiry with independent comparison retention,
private capture/replay with one quota admission, pinned revision, pointer isolation,
request/claim non-promotion, missing historical links, contradictory pool/host,
malformed digest/scope/version and duplicate bindings. Digest JSON round-trip tests
include time offsets and sub-millisecond timestamps. Focused race tests, store and
controller vet, strict docs and whitespace checks. No real remote/backend/model run.

Validation completed: focused controller/store race groups, including shared
admission and altered inner observation replay with a fresh outer digest, pass.
Controller/store vet, strict docs and whitespace pass. Docs used the repository-only
header fallback for unavailable optional GitHub metadata.

## Next

Reviewed descriptive public report and web evidence inspection, without changing
comparison eligibility. Regenerate API/web clients when the public schema changes;
review the bounded MCP/Eli projection before exposing new fields there.
