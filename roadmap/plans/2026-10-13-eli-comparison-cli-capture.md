# ZF-243: explicit CLI capture

13 October 2026. Starts after merged web #991, following the owner's UI-first order.

## Behaviour

`zoomies comparisons capture --file FILE` reads one documented JSON request within
32 KiB (`-` for stdin), validates explicit scope, bounded metadata, increasing RFC
3339 window and 2 to 10 unique positive exact attempts across two non-empty arms.
Workflow kind additionally needs a retained exact revision-delta reference. Run
identities retain int64 precision. Unknown fields and trailing documents refuse.

One POST uses existing operator/dual-scope authorisation and bounded controller
collection/retention. No preliminary probe, retry, follow-up GET, job dispatch or
MCP/Eli mutation. Captured reports share replay's supported-version/scope checks
and human/raw JSON presentation. Errors warn against automatic retry after budget
consumption or ambiguous retention. Replay is a read in the agent skill; capture
is a change. No live controller is operated while developing this command.

## Validation

Focused CLI race fixtures cover exact file/stdin requests, full int64 IDs, strict
input/combined selection bounds, workflow reference, raw report preservation,
unknown/zero human output and no retry on refusal or unsupported success. Generate
command references and check their consistency, command/skill classifications,
changed-package vet and strict docs. No real GitHub capture or model evaluation.

## Next

Independently retained workload/configuration/cache identity and complete selection
remain necessary for meaningful controlled comparisons. E2 remains in progress.

Validation completed: focused CLI/replay, generated reference, command/skill and
skill flag race checks pass, as do generation, changed-package vet, strict docs
and whitespace checks. No live controller/GitHub/model run occurred.
