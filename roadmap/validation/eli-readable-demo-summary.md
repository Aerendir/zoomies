# Readable Eli demo summaries

The demo model previously repeated a truncated JSON tool result in its answer.
It now decodes the complete fleet-status response and writes Markdown with
headings for current activity, recent outcomes and the next step. Jobs are
scoped to this Zoomies fleet when that scope is present. Counts, host health
and queue waits use readable labels; missing values are omitted. The next
step follows reported memory faults, other runner faults, host health or the
queue, without claiming an unmeasured cause.

Only recognised numeric fields become prose. Invalid or unsupported responses
produce a readable explanation rather than raw data or invented statistics.
The real provider prompts and the Markdown renderer are unchanged.

Both Eli chat screenshots were recaptured from the built app on 12 October
2026. These supersede the JSON replies recorded in the original
[UI tour refresh](ui-tour-refresh.md). The demo uses a real fleet lookup and
a deterministic summary; no external model account or key is involved.

Validation:

- `go test -race -count=1 ./internal/assistant` passed, covering the provider
  contract, full response parsing, fleet scoping, missing values, invalid
  responses and exclusion of arbitrary tool text.
- `go vet ./internal/assistant/` and the documentation Go tests passed.
- The fleet-access browser test passed against the real binary on desktop
  and phone, including rendered lists, readable labels and lookup evidence.
- The production UI build, strict MkDocs build, Prettier, ESLint and the
  screenshot script's Node syntax check passed.
- Both refreshed screenshots show the full summary, its next step and the
  tool evidence, and are included in the UI tour and Eli guide.
