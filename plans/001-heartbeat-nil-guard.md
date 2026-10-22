# Plan 001: Guard a heartbeat against a transport that answers nothing

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**, in the repo root:

```
git diff --stat a0f21dba..HEAD -- internal/agent/daemon.go internal/agent/fakes_test.go internal/agent/daemon_test.go
```

> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `a0f21dba`, 2026-10-19

## Why this matters

The agent's heartbeat method dereferences the transport's response three
times *before* the `resp == nil` guard that was written to protect those very
dereferences, so the guard is dead code. Both transports in the tree today
always return a non-nil response, but nothing documents or enforces that
contract, and the heartbeat loop's goroutine is not recovered anywhere: one
future transport change that returns `(nil, nil)` panics the whole agent
process, which ends host reporting for every running job on that host. Moving
the guard costs one line and a test pins the contract.

## Current state

- `internal/agent/daemon.go` (2610 lines) is the agent's whole lifecycle.
  `heartbeat` is at lines 1015-1073. It calls the transport at line 1026:

```go
	// internal/agent/daemon.go:1026-1072 (excerpt)
	resp, err := a.tr.Heartbeat(hctx, HeartbeatRequest{
		// ... fields elided; the request is large and does not matter here
	})
	if err != nil {
		a.expireBoosts(ctx)
		return err
	}
	if update != nil && !resp.UpdateHeld {          // line 1050: first use
		a.markUpdateDelivered(delivery)
	}
	a.mu.Lock()
	a.missedBeats = 0
	a.mu.Unlock()
	// The beat carried every runner the agent tracks, so the controller now
	// knows how each finished one ended, and its workload may go.
	a.mutationsPaused.Store(resp.MutationsPaused)   // line 1058: second use
	a.markReported(runners)

	if !a.ready.Swap(true) {
		// ... notify.send READY=1, log line
	}
	a.notify.send(fmt.Sprintf("STATUS=%d runner(s), ...", ...))  // line 1068

	if resp == nil {                                // line 1070: the guard, after every use
		return nil
	}
	a.warnSkew(resp.ControllerVersion)
```

The three uses (1050, 1058) sit before the guard (1070). Note that
`a.notify.send` at 1068 does not use `resp`; only the lines noted do.

- `heartbeatLoop` (lines 991-1013) calls `heartbeat` in a loop on the agent's
  goroutine; nothing in the loop or its caller recovers a panic.
- `internal/agent/httptransport.go` is the HTTP transport; its `Heartbeat`
  always returns a non-nil response today. The fake transport used by tests is
  in `internal/agent/fakes_test.go`.
- Test scaffolding: `internal/agent/daemon_test.go:41` has

```go
func newAgent(t *testing.T, capacity int, tweaks ...func(*Options)) (*Agent, *fakeTransport, *fakeBackend, *testClock)
```

so tests get an `*Agent` wired to a `*fakeTransport` and a `*testClock`. Read
`internal/agent/fakes_test.go` first to see how `fakeTransport.Heartbeat` is
implemented (it records the request and returns a canned response).

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Prepare the build (once, fresh checkout) | `make build-nogui` | exit 0; writes the UI placeholder `internal/api` embeds, without which plain `go test` fails |
| One test | `go test -run TestAHeartbeat ./internal/agent/` | pass |
| Package tests | `go test ./internal/agent/` | pass |
| Full suite | `make test` | exit 0 (`go test -race -count=1 ./...`) |
| Format check | `gofmt -l internal/agent/` | no output |
| Vet | `go vet ./internal/agent/` | no findings |

## Conventions that apply here

This repo enforces some conventions by test; matching them is not optional:

- British spelling in prose and comments (behaviour, authorisation).
- No em dash characters anywhere, and never a spaced double hyphen used as a
  dash. Commands inside fenced code blocks are exempt.
- Comments explain *why*, not what. The comment you add in Step 1 should name
  the failure mode being designed out (a transport that answers nothing
  panics an unrecovered goroutine), not narrate the code.
- Test names are sentences about behaviour, not method names; see
  `TestOpenCreatesTheDatabaseDirectory` referenced in the root `AGENTS.md`.
  Helper constructors take `t` and call `t.Helper()` / `t.Cleanup`.
- Table-driven, standard library `testing` only; no assertion framework.
- Commit messages are imperative sentences in plain prose, sentence case, no
  prefix; for example "Guard a heartbeat against a transport that answers
  nothing".

## Scope

**In scope** (the only files you should modify):

- `internal/agent/daemon.go` (move the guard, add a comment)
- `internal/agent/daemon_test.go` (add one test; or a new
  `internal/agent/heartbeat_test.go` if you prefer, same package)

**Out of scope** (do NOT touch):

- `internal/agent/httptransport.go` and `internal/agent/transport.go` (or
  wherever the `Transport` interface is declared): the contract comment in
  Step 2 goes on the interface declaration only if it lives in one of these
  files, and then it is a comment, nothing else. No behaviour change there.
- `heartbeatLoop`, the shutdown path, any other method in `daemon.go`.
- The UI, the API, anything under `internal/controller`.

## Git workflow

- Branch: `fix/heartbeat-nil-guard` (the repo's convention is
  `fix/<short-description>`, per `CONTRIBUTING.md`).
- One commit at the end, message: `Guard a heartbeat against a transport that
  answers nothing`.
- Do NOT push or open a pull request unless the operator instructed it.

## Steps

### Step 1: Move the nil guard above the first use of the response

In `internal/agent/daemon.go`, delete the block at lines 1070-1072:

```go
	if resp == nil {
		return nil
	}
```

and insert the same check immediately after the error check (after line
1049's `}` closing `if err != nil`), with a comment explaining why, in the
repo's voice, roughly:

```go
	// A transport that could not answer says so with an error; one that
	// answers nothing at all is still refused here, because the loop above
	// is this process's whole report path and nothing recovers a panic in
	// it. Both transports return a response, so this is the contract held
	// rather than a state that is expected.
	if resp == nil {
		return nil
	}
```

Keep everything else in the method exactly as it is. After the move, every
use of `resp` (lines 1050, 1058, 1073 and onward) is below the guard.

**Verify**: `gofmt -l internal/agent/` returns no output, and

```
go vet ./internal/agent/
```

reports nothing.

### Step 2: Say the contract where it is declared

Find the `Transport` interface declaration (likely `internal/agent/transport.go`;
if it is elsewhere, that is fine, this step follows it). On the `Heartbeat`
method's doc comment, add one sentence stating the contract: a transport that
returns a nil error returns a non-nil response. Do not change any code in that
file.

**Verify**: `gofmt -l internal/agent/` returns no output.

### Step 3: Pin the contract with a test

In `internal/agent` (add to `daemon_test.go` or create `heartbeat_test.go`,
package `agent`), write one test, modelled on the `newAgent` scaffolding at
`daemon_test.go:41` and reading `fakes_test.go` first for how the fake
transport's `Heartbeat` behaves:

- Name it `TestAHeartbeatSurvivesATransportThatAnswersNothing`.
- Build an agent with `newAgent`, then replace or tweak the fake transport so
  its `Heartbeat` returns `(nil, nil)`. If the fake records calls and returns
  a canned response, the cleanest tweak is a small wrapper type embedding the
  `*fakeTransport` that overrides `Heartbeat` to `return nil, nil`, assigned
  to the agent's transport field the way the existing transport-level tests
  assign one (grep `internal/agent` tests for how a test swaps `a.tr`; if no
  test does, set the field directly, the tests are in-package).
- Call `a.heartbeat(context.Background())`.
- Assert it returns a nil error and does not panic: run the call under
  `defer func() { if r := recover(); r != nil { t.Fatalf(...) } }()` as
  well as checking the returned error. A panic here fails the test on its
  own, but saying so keeps the test's point readable.
- Add a one-paragraph comment explaining why the behaviour matters (the
  guard is the agent process's crash boundary), in the repo's voice.

**Verify**:

```
go test -run TestAHeartbeatSurvivesATransportThatAnswersNothing ./internal/agent/
```

passes.

## Test plan

- New test: `TestAHeartbeatSurvivesATransportThatAnswersNothing` in
  `internal/agent` (Step 3), covering the one regression this plan fixes: a
  transport answering `(nil, nil)` must end the heartbeat quietly, not panic
  the agent.
- Structural pattern: the `newAgent` scaffolding at
  `internal/agent/daemon_test.go:41` and the fakes in
  `internal/agent/fakes_test.go`.
- Verification: `go test ./internal/agent/` passes, then `make test` passes.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] In `internal/agent/daemon.go`, no use of `resp` appears before the
      `if resp == nil` check (read the method top to bottom to confirm).
- [ ] `go test -run TestAHeartbeatSurvivesATransportThatAnswersNothing ./internal/agent/`
      passes.
- [ ] `make test` exits 0.
- [ ] `gofmt -l internal/agent/` and `go vet ./internal/agent/` are clean.
- [ ] No files outside the in-scope list are modified (`git status`).
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/agent/daemon.go:1026-1072` does not match the excerpt
  above (the codebase has drifted since this plan was written).
- `heartbeat` uses `resp` in a way the moved guard would now skip but the
  old dead guard placement did cover (for example a use *after* line 1070
  that only makes sense when `resp` was nil; there is none today).
- The `Transport` interface cannot be found, or the fake transport's
  `Heartbeat` signature differs from `Heartbeat(ctx, HeartbeatRequest) (*HeartbeatResponse, error)`
  in a way that makes Step 3's wrapper impossible.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- Any new `Transport` implementation (a second protocol, a test double used
  in production paths) must keep the contract stated in Step 2: nil error
  implies non-nil response. The test in Step 3 is the pin; move it along if
  the transport interface moves.
- A reviewer should check exactly one thing: the guard moved, nothing else
  shifted in `heartbeat`.
- Deliberately deferred: making `heartbeat` defensive about individual nil
  *fields* of `resp`. Nothing today produces a half-nil response, and
  speculative guards would hide contract violations rather than fail loudly.
