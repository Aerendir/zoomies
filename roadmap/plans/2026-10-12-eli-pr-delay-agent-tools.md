# Eli PR delay: CLI and agent tools

Sixth ZF-242 slice, following merged #943. Expose the existing bounded API
through `zoomies jobs pr-delay JOB_ID --pull-number N` and MCP `get_pr_delay`.
Both make one request and propagate admission failures without automatic retries.
MCP depends only on the REST transport. Add the read-only tool to Eli's explicit
allowlist; the existing fleet-access setting continues to apply.

The CLI preserves the server document in structured output. Human output shows
coverage, nullable timing and the last observed required-check completion,
with caveats that these facts do not prove merge readiness or a critical path.
Reject malformed identities and missing/non-positive PR numbers before reads.
No source reads, writes, model calls or comparison experiments are added.

Public visitors can read retained fleet data, but this route triggers fresh
installation reads. Refuse them before collector or quota access; signed-in
viewers and tokens scoped to jobs:read retain access. Keep the generic route
authorisation fixture, OpenAPI and API-surface documentation aligned.

## Verification and next work

Focused race checks cover one-request MCP/CLI behaviour, malformed input,
no retry on server failure, unknown versus zero durations, byte-preserving JSON,
public visitor refusal with zero installation reads, Eli's explicit allowlist,
generated command/MCP references and embedded contract equality. MCP, CLI and generated-reference checks pass against the resolved parent;
API visitor, allowlist and embedded-contract checks also pass. The strict documentation build passes.
Full lint passed before that resolution, with staticcheck unavailable.
The full repository test suite remains unverified after earlier interruptions.

Regenerate OpenAPI server/web contracts and CLI/MCP references after merging
upstream changes. Next add a contextual on-demand UI and scripted Eli scenarios.
Do not mark ZF-242 complete or claim a live model evaluation from this slice.
