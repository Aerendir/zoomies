# Plan 006: Give the MCP and assistant tool paths a per-caller budget

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**, in the repo root:

```
git diff --stat a0f21dba..HEAD -- internal/api/mcp.go internal/api/router.go internal/api/assistant_toolbox.go internal/api/mcp_test.go
```

> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED (bounding must not break the one legitimately large answer
  the design exists to serve: a log read)
- **Depends on**: none
- **Category**: security (resource exhaustion on the controller's process)
- **Planned at**: commit `a0f21dba`, 2026-10-19

## Why this matters

Every agent route on this server is budgeted (`agentBudget` in
`internal/api/handlers_agents.go:28-39` answers 429 before reading a body),
but the two model-facing entry points are not: `/mcp`
(`internal/api/router.go:82`, mounted with `noStore` and nothing else) and the
assistant chat path, both of which reach routes through `inProcessAPI`
(`internal/api/mcp.go:159-252`). Each tool call buffers a whole route answer
in memory, up to `mcpResponseLimit = 32 << 20` (`mcp.go:44`), including the
log download tool (`Stream`, `mcp.go:182-191`), with no per-caller
concurrency or byte ceiling. A viewer-role token (or any signed-in user
through assistant chat) can hold N concurrent answers pinned, on the single
process that owns the SQLite writer. The fix gives these two entry points
the same shape the agent routes already have: a per-caller budget, refused
early with a sentence.

## Current state

- `internal/api/mcp.go` (313 lines, read it in full first). The pieces:

```go
	// internal/api/mcp.go:41-44
	// mcpResponseLimit bounds what one tool call reads back from a route. It is
	// the stdio server's log limit: a runner's log is the only answer that is ever
	// large, and the tool keeps only its tail.
	const mcpResponseLimit = 32 << 20
```

```go
	// internal/api/mcp.go:174-191 (excerpt): every tool call funnels through do
	func (a inProcessAPI) Call(ctx context.Context, method, path string, q url.Values) ([]byte, error) {
		return a.do(ctx, method, path, q, "application/json", nil)
	}
	func (a inProcessAPI) CallBody(ctx context.Context, method, path string, q url.Values, body []byte) ([]byte, error) {
		return a.do(ctx, method, path, q, "application/json", body)
	}
	func (a inProcessAPI) Stream(ctx context.Context, path, accept string) (io.ReadCloser, error) {
		// A log download ends when the relay goes quiet, so reading it whole is
		// what the stdio server's client does too; there is no connection here to
		// hold open for it.
		body, err := a.do(ctx, http.MethodGet, path, nil, accept, nil)
		// ...
	}
```

```go
	// internal/api/mcp.go:239-251 (excerpt): the buffering itself
	rec := &bufferedResponse{header: http.Header{}, limit: mcpResponseLimit}
	a.s.handler.ServeHTTP(rec, req)
	// ... refusalFrom for rec.status >= 400
	return rec.body.Bytes(), nil
```

- `inProcessAPI` carries the caller: `as *auth.Identity` (the /mcp caller, or
  the person chatting via `direct: true`); `handleMCP` authenticates before
  building it (`mcp.go:68-98`), so `a.as` is non-nil on every real call path.
  `auth.Identity` has a `String()` (used at `mcp.go:95`).
- The agent-route precedent to copy the *shape* of (not the code):
  `internal/api/handlers_agents.go:22-39`, `agentBudget`, which spends from
  `s.ctrl.AllowAgentCall(host.ID)` and answers `rateLimited(w, msg, retry)`
  before the body is read.
- The assistant path: `internal/api/assistant_toolbox.go:24-28` builds the
  same `inProcessAPI` with `direct: true`; the budget added in `do` covers it
  without further changes.
- The stdio transport (`zoomies mcp`, not this file) needs nothing: it is a
  local process reading its own stdout.
- Tests: `internal/api/mcp_test.go` has the harness (`h.mcpCall`,
  `h.mcpTool`, `h.mcpToolNames`, `decodeMCP`, at lines 12-80); the tests are
  in-package, so they can reach the server's unexported fields.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Prepare the build (once, fresh checkout) | `make build-nogui` | exit 0; writes the UI placeholder `internal/api` embeds, without which plain `go test` fails |
| One test | `go test -run 'TestInProcess\|TestMCP' ./internal/api/` | pass |
| Package tests | `go test -race ./internal/api/` | pass |
| Full suite | `make test` | exit 0 |
| Format check | `gofmt -l internal/api/` | no output |
| Vet | `go vet ./internal/api/` | no findings |

## Conventions that apply here

- British spelling in prose. No em dash characters anywhere, and never a
  spaced double hyphen used as a dash.
- Comments explain why: the constants carry the failure mode they bound (a
  caller pinning answers on the process that owns the SQLite writer).
- MCP adds no authority; this plan adds no authority either: the budget
  refuses *how often* a caller may hold answers, never *what* they may read.
- Error messages are sentences a person (here: an agent) can act on; a
  refusal names what to do (wait for the calls in flight).
- Test names are sentences about behaviour; table-driven, standard library
  `testing` only.
- Commit messages are imperative sentences in plain prose, sentence case, no
  prefix; for example "Budget what one caller's tool calls may hold at once".

## Scope

**In scope** (the only files you should create or modify):

- `internal/api/mcp_budget.go` (create: the limiter)
- `internal/api/mcp.go` (wire the guard into `do`)
- `internal/api/mcp_budget_test.go` (create) and `internal/api/mcp_test.go`
  (the harness-driven refusal test)

**Out of scope** (do NOT touch, even though they look related):

- `internal/api/router.go`: `/mcp` keeps its mount; the guard belongs in
  `inProcessAPI.do`, which is the one place both transports (HTTP `/mcp` and
  assistant chat) funnel through. Mounting a middleware would miss the
  assistant path, which is half the finding.
- `mcpResponseLimit` and `bufferedResponse`: the 32 MiB answer cap stays.
  The plan bounds how many answers one caller holds *at once*, not how big
  one may be.
- `internal/mcp`, `internal/assistant`: read-only context.
- `auth.RateLimiter` (`internal/auth/ratelimit.go`): it is a sliding-window
  *rate* limiter, not a concurrency semaphore; do not bend it into one. The
  budget here is a keyed counting semaphore, which is why it is new code.

## Git workflow

- Branch: `fix/mcp-call-budget`.
- One commit, message: `Budget what one caller's tool calls may hold at
  once`.
- Do NOT push or open a pull request unless the operator instructed it.

## Steps

### Step 1: Write the keyed concurrency budget

Create `internal/api/mcp_budget.go`, package `api`:

```go
// inProcessBudget bounds how many route answers one caller may be holding
// at once. Every tool call buffers a whole answer (mcpResponseLimit is 32
// MiB), and the agent routes answer a caller that tries this with 429
// before reading a byte; the MCP and assistant paths funneled through
// inProcessAPI were the two entry points with no such ceiling, and they run
// on the process that owns the SQLite writer. A keyed semaphore rather than
// auth.RateLimiter, which bounds how often a key may call, not how many
// calls it may hold.
type inProcessBudget struct {
	mu        sync.Mutex
	held      map[string]int
	totalHeld int
}

const (
	// inProcessPerCaller is the ceiling per identity. Two concurrent large
	// log reads (one live, one download) are a plausible legitimate pair;
	// a third held answer from the same caller is a loop, not a reader.
	inProcessPerCaller = 2
	// inProcessTotal is the fleet-wide ceiling, so many callers together
	// still pin a bounded slice of this process's memory.
	inProcessTotal = 16
)

// take spends one slot for key and reports how long to wait when the
// ceiling refuses. key is the caller's identity string; the empty string is
// every call without an identity, which cannot happen on a real path but
// must not bypass the budget.
func (b *inProcessBudget) take(key string) (ok bool, retryAfter time.Duration)
// give returns one slot for key. It is safe to call without a matching take
// (a give for a key holding nothing is a no-op), because release runs on
// every exit path of do, including the ones a panic-free rewrite of do
// might add later.
func (b *inProcessBudget) give(key string)
```

Implement the two methods (mutex, map counts, total count; `retryAfter` is a
fixed short constant, for example 2 s, since the refusal is about calls in
flight, not a window; keep it simple and say why in the comment). Sweep the
map the way `auth.RateLimiter.sweep` does only if entries can leak: they
cannot, every `take` that returns true is paired with a `give` by `do`'s
defer, so no sweeping is needed; if you decide otherwise, document why.

**Verify**: `gofmt -l internal/api/` clean; `go vet ./internal/api/` clean;
the type compiles (`go build ./internal/api/`).

### Step 2: Spend it in `inProcessAPI.do`, and refuse like the agent routes

In `mcp.go`'s `do`, around the `ServeHTTP` into the `bufferedResponse`:

```go
	key := ""
	if a.as != nil {
		key = a.as.String()
	}
	if !a.s.inProcessBudget.take(key) {
		// The same shape the agent routes give: a refusal that names the
		// ceiling and what to do about it, as an mcpRefusal so both
		// transports read it the same way.
		return nil, &mcpRefusal{status: http.StatusTooManyRequests,
			message: "this caller already has tool answers being read; wait for one to finish and ask again"}
	}
	defer a.s.inProcessBudget.give(key)
```

Add the field `inProcessBudget inProcessBudget` to the `Server` struct and
zero-value it (the type's zero value must work: `held` nil map is fine if
`take` lazily initialises it; make sure it does). Find the struct and its
constructor with `grep -n 'type Server struct' internal/api/*.go` and
`grep -rn 'func NewServer' internal/api/*.go`; no constructor change is
needed if the zero value works.

The refusal surfaces to the caller as the tool call's error result (via
`refusalFrom`'s sibling path: `do` returning an `*mcpRefusal`), which both
the HTTP transport and the assistant render; confirm by reading how `do`'s
callers (`internal/mcp` tool dispatch) treat a returned error.

**Verify**: `go test ./internal/api/` passes unchanged (no existing call
path holds more than 2 at once; if one does, that test failure is a STOP,
not a ceiling to raise).

### Step 3: Test the ceiling, unit and route

1. `internal/api/mcp_budget_test.go`, `TestTheInProcessBudgetRefusesTheThirdCallOfOneCaller`:
   table over (per-caller, total): take/give accounting, the third take of
   one key refused, the take of a 17th distinct key refused when 16 are
   held, give restoring room, and the empty key still bounded.
2. `internal/api/mcp_test.go`, `TestAToolCallBeyondItsCallersBudgetIsRefusedWithTheSentence`:
   using the existing harness, take the caller's two slots directly (the
   tests are in-package: `h`'s server field is reachable; read how `h` is
   built in `api_test.go` first), then `h.mcpTool(token, "list_runners",
   nil)` and assert the result carries the refusal sentence (the
   `mcp.CallResult` error path; read how existing refusal tests assert it,
   for example the not-offered-tool tests in `mcp_test.go`).

Then run the package under the race detector (the budget is a mutex-protected
map hit on every tool call).

**Verify**: `go test -race ./internal/api/` passes.

## Test plan

The two tests of Step 3. Structural patterns: the in-package harness in
`internal/api/mcp_test.go` (lines 12-80, `h.mcpCall`/`h.mcpTool`) and any
existing refusal-shape test in that file for how an error result is asserted.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -n 'inProcessBudget' internal/api/mcp.go internal/api/mcp_budget.go`
      shows the guard inside `do`, covering both `Call`, `CallBody` and
      `Stream` (all three funnel through `do`; one guard, three paths).
- [ ] `go test -race ./internal/api/` passes, including the two new tests.
- [ ] `make test` exits 0.
- [ ] `gofmt -l internal/api/` and `go vet ./internal/api/` clean.
- [ ] No files outside the in-scope list are modified (`git status`).
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back (do not improvise) if:

- Any existing test fails because a real, legitimate flow holds more than
  two in-process answers at once for one identity (that would make the
  constant wrong, not the test; report the flow).
- The `Server` struct or its constructor's shape has changed such that a
  zero-value field is impossible.
- `inProcessAPI.do` turns out to be re-entrant (a route served through `do`
  that itself calls a tool through another `inProcessAPI`; then a single
  goroutine could self-deadlock on the budget; report the path).
- Wiring the guard requires touching `internal/api/router.go` or anything
  else out of scope.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- The two constants are the tunables; if operators report legitimate flows
  refused (a heavily parallel MCP client), the per-caller ceiling is the one
  to raise first, and the comment on it should say what the legitimate flow
  was.
- A future bytes-based budget (counting buffered bytes, not calls) would be
  strictly better; the call-count ceiling is the honest first bound, and the
  seam (`take`/`give` in `do`) is where a bytes budget would slot in.
- The stdio transport (`zoomies mcp`) deliberately has no budget: it holds
  answers in its own process, and the controller's memory is not its own.
