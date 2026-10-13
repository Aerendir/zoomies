# ZF-243: retained comparison CLI replay

13 October 2026. E2 slice 9, draft [#988](https://github.com/eyupio/zoomies/pull/988),
stacked on capture/replay API #987.

## Contract

`zoomies comparisons get cmp_ID --installation ins_ID --repo OWNER/REPO`
performs one authorised scoped GET. It never collects GitHub evidence or creates
replacement history. Existing viewer/jobs:read authorisation applies. Expired,
evicted or absent reports remain unavailable.

JSON forwards the raw response. Human output preserves unknown versus zero,
sample/missing counts, independent directions and all eligibility reasons. Exact
integer milliseconds avoid rounding or overflow on retained totals. Supported
versions and evidence/repository identity are checked before presentation.

## Validation

Focused CLI race fixtures cover one scoped GET, unknown/zero presentation, raw
response preservation including precision-sensitive future fields, malformed
scope refusal and unavailable history without mutation or retry. Regenerate the
command reference and run its consistency tests, changed-package vet and strict
docs build. No real GitHub collection, experiment or model evaluation is claimed.

## Next boundary

A read-only MCP/Eli adapter must quote operator text as untrusted, bound visible
and structured evidence consistently and make no capture mutation. CLI capture
and UI initiation require explicit operator write intent.

Validation completed: focused CLI/reference race tests, changed-package vet,
`make generate` and strict MkDocs pass. The docs used the repository-only header
fallback because optional GitHub metadata was unavailable.
