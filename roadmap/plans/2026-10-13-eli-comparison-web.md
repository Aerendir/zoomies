# ZF-243: web capture and retained inspection

13 October 2026. Draft [#991](https://github.com/eyupio/zoomies/pull/991).
The owner prioritised the web UI before CLI capture.
Starts from merged MCP/Eli replay #990 on main.

## Behaviour

Lazy Comparisons page, linked from Jobs. Explicit operator capture selects exact
baseline/candidate GitHub run attempts, hypothesis/change and an observation
window. Workflow comparisons request a retained revision-delta reference. Validate
identity precision, duplicates and bounds before calling the existing API.
Viewer replay uses explicit scope; public visitors do not read history. Shareable
links carry scope/ID only. No invented history-list API or automatic capture.

Reports expose separate measurements, all outcome/sample/missing counts, unknown
versus zero, eligibility reasons and per-arm identities/attempts. Operator text is
escaped. Inconclusive evidence is never presented as a verified improvement.
Missing history remains unavailable; 429 blocks manual actions through Retry-After.
Whole tables scroll internally on mobile. In-flight GETs cannot overwrite newer
navigation; successful capture is displayed without a duplicate replay request.

## Validation

Input fixtures, Svelte/type checks, changed-file lint/format, production asset
budgets, desktop/mobile Playwright against the real controller shell with shaped
comparison endpoints, and strict docs. Backend authorisation/capture is covered
by existing Go API fixtures; no live GitHub experiment or model evaluation.

## Next

CLI capture follows the UI. Independently retained workload/configuration/cache
identity and complete historical selection are still required for meaningful
controlled comparisons.

Validation completed: zero Svelte errors/warnings, changed-file ESLint/Prettier,
input/design-token/block-spacing fixtures, production build and Go binary rebuild,
eight desktop/mobile browser checks, and strict docs pass. Shell 141.0 KiB/200 KiB;
status 18.6 KiB/30 KiB. Desktop/mobile screenshots inspected. Browser endpoints use
shaped comparison evidence; no live GitHub or model test is claimed.
