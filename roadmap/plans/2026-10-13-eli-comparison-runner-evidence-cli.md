# ZF-243: show runner evidence in CLI comparisons

13 October 2026. Ready for review [#1006](https://github.com/eyupio/zoomies/pull/1006), based on main after #1004 merged.

## Behaviour

Capture and replay share a human presenter over an independent REST whitelist.
Show assignment grants, requested resources, unverified agent digest/time claims,
unknown values and full coverage/omitted-row counts. Preserve exact decimal IDs,
fractional CPU values and separate controller/agent clocks. Show every returned
row within existing server bounds. No additional HTTP requests or authority.

Validate the version, required flags/counters, row bounds, original arm bindings,
unique canonical job IDs, sources, dimensions, clocks and digest syntax before
printing. Refuse unsupported/inconsistent human views. Missing older views remain
unavailable. Structured JSON bypasses this decoder and preserves future/raw fields.
Eligibility, measurements and sample requirements remain unchanged.

## Validation

Focused capture/replay race fixtures cover shared presentation and one-request
transport, unknown/zero values, omitted rows, exact IDs, future/raw JSON, private
field exclusion and malformed facts refused before output. Changed-package vet,
strict documentation and whitespace checks complete the slice. No live fleet or
model run; no schema, dependency or command-description change.

## Next

Independent historical workload/configuration/cache and complete selection proof
remain unresolved. Consult the handoff before extending retained provenance.
