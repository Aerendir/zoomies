# E3 checkout preview: policy and web flow

Date: 19 October 2026. Package: ZF-244. Follows merged #1024.

## Outcome

An operator can read one workflow file for an exact successful push run, review
it, declare that full history is not needed, and keep an immutable preview of the
one-line change, from the web UI, with the same two calls on the REST API. Nothing
is published, approved or run.

## Decisions

* **Policy is two instance settings**, `optimisation.source_reads` and
  `optimisation.workflow_edits`, both off by default and live. `OptimisationPolicy`
  reads the running configuration on every call, so the existing before-read and
  before-save policy rechecks see a withdrawal at once. No request field carries
  policy.
* **Inspection is separate from capture.** A history review has to bind the digest
  and head the operator was shown, so the source must be read before the review
  can exist. Inspection runs the same admitted read as capture (shared flight,
  hourly allowance, half-core reserve, seven requests) and retains nothing.
  Capture reads again and refuses a digest that moved. The cost is two admissions
  from the 20 an hour; the alternative, retaining unreviewed source, was refused.
* **The scope of the declaration is fixed by the server.** A request supplies
  `history_reviewed: true` and the head and digest it saw.
* **Replay never returns the file.** Source is shown once, at inspection. A
  retained preview answers with the one changed line.
* **New actions** `optimisations.read` and `optimisations.write`, operator, with
  `jobs:read` also required, public visitors refused.
* **IDs.** GitHub run and job IDs are decimal strings on the wire and in the form.

## Not in this slice

Approval, draft publication, E2 verification of a published change, CLI, MCP and
Eli access. The retained envelope stays private; Eli is not offered these routes.

## Verification

Controller: policy follows live settings; inspection refuses while either switch is
off before any read, retains nothing. API: inspect, capture and replay shapes against
the spec; settings gate with the setting named in the refusal; roles, scopes,
anonymous and visitor refusals before any read; malformed and forged-field bodies;
unconfirmed review spends no read; stale digest is a 409 and is not retained. Web:
unit tests for exact identity parsing; browser tests on desktop and phone for escaped
source, the declaration gating the button, a selection edit dropping the review, a
refusal naming the setting, and a replay with no overflow. No live GitHub, model or
fleet.
