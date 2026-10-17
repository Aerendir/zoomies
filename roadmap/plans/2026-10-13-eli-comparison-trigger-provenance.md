# ZF-243: retain private trigger/workflow context

13 October 2026. Ready for review [#1007](https://github.com/eyupio/zoomies/pull/1007), based on main after #1006 merged.

## Behaviour

Retain GitHub's optional event and workflow path for each exact selected attempt.
Recheck both around job pagination with the existing two attempt GETs. Refuse
movement and invalid bounded metadata. No extra requests, source/log reads,
privileges or budget changes. Paths are opaque untrusted text, never resolved.

Validate the same fields for alternate readers and offline replay. Optional fields
stay private inside the immutable retained envelope and storage digest; old absent
fields stay unknown and omitted without changing their bytes. Public reports,
runner views, measurements and eligibility remain identical. No schema migration,
API contract, dependency or generated command-description change.

## Validation

Fixture-backed GitHub race tests cover present/absent metadata, changes and
appearance/disappearance, invalid/control/oversized fields and unchanged request
counts. Remove the new identity recheck and observe the movement fixture fail.
Controller race fixtures cover private immutable retention, legacy payloads,
alternate-reader validation and unchanged report/eligibility bytes. Vet, strict
documentation and whitespace checks complete the slice. All these checks pass;
docs used the repository-only header fallback. No live external run.

## Next

Resolve executed workflow revisions per event before collecting source content.
Do not treat the run path or head SHA as proof of workflow content, dispatch
inputs or test scope. Consult the handoff for projection and budget constraints.
