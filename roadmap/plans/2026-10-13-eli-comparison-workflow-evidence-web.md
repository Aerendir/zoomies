# ZF-243: inspect descriptive workflow evidence in the web UI

13 October 2026. Ready for review [#1014](https://github.com/eyupio/zoomies/pull/1014), based on main after #1012 merged.

## Behaviour

Add a separate optional REST workflow-evidence-v1 projection over validated
retained inputs. Whitelist exact decimal run IDs, nullable trigger/path fingerprints
and root commit/blob/content digests/bytes. Bound rows at 100 per arm with full
selected/root-source counts and omission flags. Exclude raw paths/source text.
Capture/replay make no new reads, and original reports/digests/eligibility stay.

Show native per-attempt details and unknown/unsupported views in the UI with
mobile digest wrapping. Explain root-only coverage, path versus content and
absence of execution attestation, dependency/input/test-scope or workload proof.
Regenerate both OpenAPI copies. No migration, dependency or new authority.

## Validation

Controller/API race fixtures for exact identities, nulls, privacy, copied values,
full counters, replay and request counts. Desktop/mobile browser checks for
root-only claims, old/future views, unknowns, readability and overflow. Type/lint,
UI unit/build budget, generated contract checks, vet, strict docs and whitespace.
Validation completed: controller/API race fixtures, 20 desktop/mobile comparison
checks, all 109 UI unit tests, type/lint checks, both UI builds within budgets,
changed-package vet, generated contract/source matching, strict docs and whitespace
pass. Exact-ID fixtures fail when string encoding is removed. Docs used the
repository-only header fallback. No live model, source or fleet run.

## Next

Bounded MCP/Eli and CLI human presenters, keeping the same descriptive semantics.
Independent historical workload/configuration/cache/dependency and selection proof
remains outstanding.
