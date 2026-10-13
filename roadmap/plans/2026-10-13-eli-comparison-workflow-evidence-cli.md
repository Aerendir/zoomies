# ZF-243: show workflow facts in CLI comparisons

13 October 2026. Prepared on main after #1015 merged.

## Behaviour

Capture/replay share human presentation of exact run/attempt identities, trigger,
path fingerprint, nullable root-file digests/bytes and full counters/omission flags.
Show all returned rows within existing server bounds. Preserve unknowns and label
root-only claims without promoting execution/workload equivalence or eligibility.

Validate independent REST whitelists and original arm/commit bindings before any
human output. Refuse bad/future human views with JSON inspection guidance. Older
absent views stay unknown. Raw structured output preserves future views and exact
numbers. No extra requests, help/flags, schema, dependency or authority changes.

## Validation

Focused CLI race fixtures cover capture/replay with one request, exact int64 IDs,
nulls/legacy views, full counts and source omissions, private/future text exclusion,
malformed counts/claims/bindings refused before output and raw future JSON. The
presentation fixture must fail when output is removed. Vet, strict docs and
whitespace pass. The presentation fixture fails when workflow output is removed.
Docs used the repository-only header fallback. No live model/provider/fleet run.

## Next

Bounded dependency inventory from existing root reads, with static versus unknown
coverage. Independent workload/configuration/cache and fair selections remain open.
