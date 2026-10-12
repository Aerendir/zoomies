# Eli PR delay: admission and read-only API

Fifth ZF-242 slice, following the on-demand collector in #938. Expose
`GET /api/v1/jobs/{id}/pr-delay?pull_number=N`, protected by the existing
Jobs read action and `jobs:read` token scope. The caller supplies only a job ID
and one positive PR number; installation and repository come from that job.

## Admission

One installation shares 20 diagnostic admissions per rolling hour and one
in-flight request across all viewers. These counters are in-memory and reset
on restart. An admission is charged even if collection later fails; a request
refused because another is in flight does not spend a slot. The release runs
on every exit, including cancellation and quota refusal.

Before evidence reads, query the installation's core quota using GitHub's
uncharged rate-limit endpoint. Refuse unknown/inconsistent quota and preserve
half the reported allowance after the diagnostic's worst-case 13 requests:
two PR reads, classic protection, five rules pages and five check pages.
This is a preflight against an observation, not exclusive ownership of the
quota shared with runners or other clients. Existing installation holds remain
in force. Admission and quota refusals carry HTTP 429 and Retry-After.

## Contract and privacy

The report's JSON has stable snake-case fields, explicit coverage and
`window_basis: earliest_observed_job`. Unknown timing remains null. Required
names remain opaque, and check reporting time stays separate from execution.
Source, annotations, model calls and writes remain outside this endpoint.
Responses are no-store. Stale/closed/retargeted PRs are 409, malformed input 400,
and missing job/installation records 404. Caller authentication/permission gates
run before any collector or quota read.

OpenAPI defines the route and report/result/check/timing schemas. Regenerate
the compressed server contract and web TypeScript types with the repository
generators, and keep the hand-written route authorisation table in sync.

## Verification

Focused race tests pass for the calculation, collector, admission and API.
HTTP tests cover unauthorised requests spending neither quota nor evidence
reads, viewer token scope, malformed/duplicate PR numbers, opaque output,
nullable phases, report shapes, exhausted allowance, quota reserve and changed
heads. Route/spec coverage and embedded/source contract equality pass.
Formatting and diff checks pass. `make lint` passes, including normal/e2e/drill/load
vet and web lint; staticcheck was unavailable and skipped. `mkdocs build --strict`
passes with the repository-only header fallback for unavailable optional GitHub
metadata. The earlier interrupted full test suite is not a pass.

Next slices expose this one route through MCP/CLI and a contextual UI card,
then extend Eli's evaluation harness. No source tools or model mutation rights
are added merely because this diagnostic exists.
