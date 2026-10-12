# Eli PR delay validation, 12 October 2026

The E1 implementation merged through #959: calculation, bounded GitHub reads,
identity-checked assembly, collector, admission, read-only API, CLI/MCP and the
job-drawer card with displayed-snapshot Ask Eli context. Three scripted scenarios
exercise unknown phases, missing checks and a superseded head, with planted
branch text and assertions that evidence reached the model. These validate the
transport and fixtures, not an unscripted model's reasoning.

## Feature checks

Focused Go race tests, generated contract/reference checks, UI unit/lint/Svelte
checks, production UI build/asset budgets and strict documentation builds pass.
Six desktop/mobile Playwright tests pass against the built app using system
Chromium 151. Two further checks prove the bottom action is reachable and
closing/reopening clears the snapshot. Shaped diagnostic responses exercise
partial evidence and Retry-After; one test makes the real diagnostic API request.
No live GitHub installation, model evaluation or performance experiment ran.

## Broad checks and corrections

`go test -race -count=1 -p 2 ./...` completed: 44 packages passed and seven
failed. Corrected public-directory/previous-binary fixtures set their intended
0755 modes explicitly; the process backend restores source permissions after
fallback copying; the ledger removal fixture uses its temporary directory and
checks opening errors. These changes merged in #960, #961, #962 and #965.

| Corrective command | Result |
| --- | --- |
| `go test -race -count=1 ./internal/aicontext` | Pass, 18.349 s |
| `go test -race -count=1 ./internal/backend` | Pass, 32.369 s; focused source-mode test failed before the runtime fix |
| `go test -race -count=1 -timeout 30m ./internal/installer` | Pass, 140.344 s |
| `go test -race -count=1 ./test/e2e/proxmox` | Pass, 9.440 s; normal tests, no live e2e tag |
| `go test -race -count=1 -timeout 30m ./internal/api` | Pass, 1380.705 s |
| `go test -race -count=1 -timeout 30m ./internal/store` | Pass, 1122.901 s |
| Controller, all 1329 listed tests in disjoint 665/664 groups, each using `-race -count=1 -timeout 30m -run REGEX` | Both pass, 801.357 s and 844.029 s |

The ten-minute API/controller/store runs timed out while progressing through
tests. A single thirty-minute controller run also timed out without an assertion
failure. The complete split is described in the [validation plan](../plans/2026-10-12-eli-ledger-fixture-validation.md).
All originally failed packages have successful complete corrective coverage.
The original full command did not pass, and no clean single-command full run is
claimed. No verification process remains running.

## Environment and remaining evidence

Go 1.27.2; restrictive umask 0077. Build cache and temporary directories use
`/workspace/eli-tooling`, with Go in `/tmp/eli-toolchain/go` and GOPATH in
`/tmp/eli-go-path`. Preserve the root handoff's environment settings. Staticcheck
was unavailable and skipped. Optional GitHub docs-header lookups and the
Playwright browser download were denied; repository header fallback and system
Chromium allowed those checks to complete.

This records the E1 stack plus corrective validation changes. It does not
validate unrelated later changes on main, a live model's choices, real runner
performance, or later enhancement tracks. Reconcile ZF-241 before starting E2.
