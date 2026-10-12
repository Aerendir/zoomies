# Eli PR delay: contextual UI and scripted scenarios

Seventh ZF-242 slice, following merged #947. Baseline refreshed to `66ff3857`.
Use the existing read-only API, client contracts and Ask Eli mechanism.

## Behaviour

Offer signed-in viewers an explicit PR-number form in the job drawer. Opening
or receiving live fleet updates must not spend installation reads. Key the card
by job ID and head, cancel reads on close/change, and reject a mismatched response.
Show pinned head, observation time and window, protection/observation coverage,
opaque check identities and nullable phase timings. Do not label observations a
critical path or full merge readiness. Pre-start includes queue and startup.
Show twenty check rows initially and allow local expansion without a fresh read.

The server's Retry-After holds the manual button; no automatic retry or polling
is scheduled. Ask Eli shares only labelled displayed facts and requests an
explanation of that snapshot without another installation diagnostic.

## Evidence

Add browser tests on desktop and mobile: no read on drawer open, input validation,
one explicit real API request, partial/missing evidence and Retry-After refusal.
Success/refusal rendering uses a shaped diagnostic response on the real app;
controller/GitHub behaviour is covered by the API and scripted Go tests.

Extend the existing Eli harness with three scenarios: checks passed with unknown
queue/startup, a missing required check and a superseded head. Plant instructions
in the target branch and assert the actual evidence and plant reached the model.
A scripted model proves transport and fences, not an unscripted model's reasoning.
No live provider evaluation, source read, write or experiment is authorised here.

Focused checks completed:

- `go test -race -count=1 -run TestEliScenariosPlayedByTheScriptedModelScoreClean ./internal/api`: pass for all existing and three new scenarios.
- Focused Node unit checks for PR delay, Eli prompts and contexts: pass.
- UI lint and focused final ESLint: pass. `npm run check`: zero errors/warnings.
- Production UI build and asset budgets: pass. Go binary build: pass.
- `mkdocs build --strict` and diff checks: pass.

Desktop/mobile browser verification passes: all six targeted Playwright tests,
including close/reopen snapshot reset. Two further desktop/mobile checks prove
the bottom action is reachable within the scrolling drawer. Screenshots were inspected. Playwright's browser download was denied
by the network policy; system Chromium 151 is available. The local temporary
Playwright config selects desktop/mobile and the monitoring server from the
repository config, with launchOptions.executablePath=/usr/bin/chromium and
args=['--no-sandbox']. It is a verification helper, not a shipped configuration.
An earlier scenario run overlapped a UI rebuild and failed on replaced embedded
assets; the sequential rerun above passes.
The full repository suite remains unverified after earlier interruptions; a fresh
`go test -race -count=1 -p 2 ./...` is running with workspace build/cache paths.
Record its result before replacing that status.
