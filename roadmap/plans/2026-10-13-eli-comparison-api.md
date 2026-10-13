# E2 authorised capture and retained-report API

ZF-243 slice 8 follows #986. Capture changes local retained history; it is not
an Eli read tool or dispatch/publication. Get/replay reads only the retained row.

- [x] Controller capture prevalidates the operator record and both selections,
  combines them into one admission/whole-request budget of at most ten attempts,
  splits the joined result into arms and atomically retains report plus audit.
- [x] POST /comparisons requires operator, comparisons:write and jobs:read, an identified caller and visitor
  refusal. Strict 32 KiB request, no arbitrary actor/evidence/raw-envelope fields.
- [x] GET /comparisons/{id} requires jobs:read/viewer and exact installation_id/repo
  query scope, refuses duplicate/unknown arguments, and replays without GitHub.
- [x] No-store responses; clear 400/401/403/404/409/429 limits and Retry-After.
  Actor is authenticated identity, not caller input. Same-origin POST guard stays.
- [x] Document complete typed schemas, regenerate embedded spec and TypeScript.
- [x] Test auth/scope/public refusal, capture/replay, one quota/whole batch, strict
  input, actor ownership, unknown data, retention misses and response contracts.
- [x] Update handoff/progress and API reference with validation and limits.

No uncontrolled comparison can claim improvement. Retained history is shared by
repository readers and bounded to 30 days/32 per installation/256 globally. Capture
is explicitly initiated and must not be marked read-only in future MCP adapters.

Validation, Go 1.27.2: focused API capture/replay/refusal/quota and scope/action
coverage race tests pass (13.798s). Complete auth race suite passes (153.319s).
Additional strict-query/role cases, viewer replay with one audit, cookie CSRF and
metadata-movement conflict cases pass. Complete GitHub race suite passes after
the new movement sentinel (18.660s). The route/spec check first found query text
being treated as path segments; normalisation now removes the query and recognises
cmp_ IDs, and route coverage/operation-role tests pass (1.218s). Router coverage
passed in the earlier group. Changed-package vet and whitespace checks pass.
Embedded and TypeScript contracts regenerated; strict docs pass (5.38s) using the
repository-only header fallback when optional GitHub facts were unavailable.
No full API/repository run, live model/GitHub experiment or browser UI check.
