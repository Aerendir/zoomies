# Plan 002: Send bulk runner errors through the same funnel as single ones

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**, in the repo root:
>
> ```
> git diff --stat a0f21dba..HEAD -- internal/api/handlers_runners.go internal/api/errors.go internal/api/api_test.go
> ```
>
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security (data minimisation in error responses)
- **Planned at**: commit `a0f21dba`, 2026-10-19

## Why this matters

`POST /api/v1/runners/bulk` puts the raw `err.Error()` of each failed drain or
delete straight into its JSON response, while the same error on the
single-runner routes goes through `fail`/`internal`, which maps the known
sentinels to human sentences and sends everything else to the log with only a
request ID in the response. The store's own convention (see
`internal/store/AGENTS.md`, "Sentinel errors") is that unknown errors carry
driver internals; on a transient database fault the bulk route is the one
place that text reaches a client. The fix reuses the mapping `fail` already
states, so both paths answer alike.

## Current state

- `internal/api/handlers_runners.go` holds the bulk handler. The response
  types (lines 266-273) and the loop (lines 316-339):

```go
	// internal/api/handlers_runners.go:266-273
	type bulkRunnerResult struct {
		// ... ID, OK fields elided
		Error string `json:"error,omitempty"`
	}
	type bulkRunnerResponse struct {
		Results []bulkRunnerResult `json:"results"`
	}
```

```go
	// internal/api/handlers_runners.go:316-339 (excerpt)
	results := make([]bulkRunnerResult, 0, len(req.IDs))
	changed := 0
	for _, runnerID := range req.IDs {
		var err error
		var run *store.Runner
		if req.Action == "drain" {
			run, err = s.ctrl.DrainRunner(r.Context(), runnerID, drainReason(r), req.Confirm)
		} else {
			run, err = s.ctrl.RemoveRunner(r.Context(), runnerID, drainReason(r), req.Force, req.Confirm)
		}
		if err != nil {
			results = append(results, bulkRunnerResult{ID: runnerID, Error: err.Error()})   // line 327: raw error
			continue
		}
		// ... success row, audit row, Nudge
	}
```

- `internal/api/errors.go` states the standard the bulk route bypasses, at
  lines 272-277: "The cause is what an operator needs and an attacker would
  like, so it goes to the log; the request ID goes to the client". The
  sentinel mapping to copy is `fail` at lines 328-347:

```go
	// internal/api/errors.go:328-347 (excerpt)
	func (s *Server) fail(w http.ResponseWriter, r *http.Request, doing string, err error) {
		switch {
		case errors.Is(err, store.ErrNotFound):
			notFound(w, err.Error())
		case errors.Is(err, store.ErrConflict):
			conflict(w, err.Error())
		case errors.Is(err, store.ErrInvalidTransition):
			conflict(w, err.Error())
		// ... ErrStreamUnknown -> notFound, ErrConfirmationRequired -> conflict with
		//     ". Send it again with confirm=true to accept that, or wait for the job
		//      to finish and drain it then." appended
		default:
			s.internal(w, r, doing, err)
		}
	}
```

  The sentinel texts are human sentences because the store writes them that
  way (`internal/store/store.go:37` `ErrNotFound = errors.New("not found")`,
  wrapped as `fmt.Errorf("runner %s: %w", ...)` by the queries, so a missing
  runner reads "runner run_xxx: not found").
- The controller methods behind the loop: `DrainRunner` and `RemoveRunner` in
  `internal/controller/reconcile.go` at lines 802-839. They return either
  wrapped sentinels (human sentences) or raw store errors (possibly driver
  internals). `s.internal` (errors.go:279-305) is the pattern for logging the
  cause and answering with the request ID; `supportHint()` (errors.go:318-320)
  is the sentence every "something went wrong" ends with.
- The OpenAPI spec for this route (`api/openapi.yaml`, `/runners/bulk`, around
  line 3160) documents the per-row `error` field as `{ type: string }` with no
  promise about its content, so no spec change is needed.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Prepare the build (once, fresh checkout) | `make build-nogui` | exit 0; writes the UI placeholder `internal/api` embeds, without which plain `go test` fails |
| One test | `go test -run TestBulk ./internal/api/` | pass |
| Package tests | `go test ./internal/api/` | pass |
| Full suite | `make test` | exit 0 (`go test -race -count=1 ./...`) |
| Format check | `gofmt -l internal/api/` | no output |
| Vet | `go vet ./internal/api/` | no findings |

## Conventions that apply here

- British spelling in prose (behaviour, authorisation). No em dash characters
  anywhere, and never a spaced double hyphen used as a dash.
- Error and warning messages are written for a person to act on; errors carry
  a human message. Match the sentences `fail` and `internal` already use.
- Comments explain why, not what: the comment on the new helper should name
  the failure mode (the bulk route was the one route that answered with the
  cause) rather than narrate the mapping.
- Test names are sentences about behaviour; table-driven, standard library
  `testing` only.
- Commit messages are imperative sentences in plain prose, sentence case, no
  prefix; for example "Answer a bulk runner action's failures the way the
  single ones do".

## Scope

**In scope** (the only files you should modify):

- `internal/api/handlers_runners.go` (the mapping helper and the loop)
- `internal/api/handlers_runners_test.go` or the file holding the bulk
  handler's tests today; find it first with
  `grep -rln 'runners/bulk' internal/api/*_test.go`. If none covers it,
  create `internal/api/handlers_runners_bulk_test.go`, package `api`.

**Out of scope** (do NOT touch):

- `internal/api/errors.go`: read it, copy its mapping, do not modify it. The
  single-route funnel is correct as it is.
- `internal/controller/reconcile.go`: `DrainRunner`/`RemoveRunner` are correct
  to return wrapped sentinels; do not pre-format errors in the controller.
- `api/openapi.yaml` and the generated copies: the spec promises nothing
  about the row's error text, so no regeneration is needed. If you believe a
  description change is warranted, that is a STOP condition, not a judgement
  call to make.
- The bulk endpoints for machines and provisioning repairs
  (`/machines/bulk`, `/provisioning/bulk`, `/assistant/repairs/bulk`): they
  may deserve the same treatment, but one route per change keeps the diff
  reviewable; note them in the commit message instead of touching them.

## Git workflow

- Branch: `fix/bulk-runner-errors`.
- One commit, message: `Answer a bulk runner action's failures the way the
  single ones do`.
- Do NOT push or open a pull request unless the operator instructed it.

## Steps

### Step 1: Add a pure mapping for one row's error

In `internal/api/handlers_runners.go`, add a helper next to the bulk handler
(not a method on `Server`; it needs nothing from the server, and keeping it
pure is what makes Step 3's test trivial):

```go
// bulkRunnerMessage is one row's error as the single routes would answer it:
// the store's own sentences for the errors an operator can act on, and a
// short line that carries no cause for everything else. The bulk loop was
// the one route that answered with err.Error() itself, which on a database
// fault is the driver's own text: the sentence the single routes give is at
// internal/api/errors.go, and this is that funnel, per row.
func bulkRunnerMessage(err error) (message string, known bool) {
	switch {
	case errors.Is(err, store.ErrNotFound),
		errors.Is(err, store.ErrConflict),
		errors.Is(err, store.ErrInvalidTransition),
		errors.Is(err, controller.ErrStreamUnknown):
		return err.Error(), true
	case errors.Is(err, controller.ErrConfirmationRequired):
		// The same sentence fail appends, so both routes teach the same
		// way out of a 409 that is asking for consent.
		return err.Error() + ". Send it again with confirm=true to accept that, " +
			"or wait for the job to finish and drain it then.", true
	default:
		return "", false
	}
}
```

Adjust the sentinel list only if the live `fail` (errors.go:328-347) maps
different ones; it is the source of truth, this helper mirrors it.

**Verify**: `gofmt -l internal/api/` returns no output and
`go vet ./internal/api/` reports nothing.

### Step 2: Use it in the bulk loop, logging what it hides

Replace line 327's raw append with:

```go
		if err != nil {
			if msg, known := bulkRunnerMessage(err); known {
				results = append(results, bulkRunnerResult{ID: runnerID, Error: msg})
				continue
			}
			// The cause is what an operator needs and an attacker would
			// like, so it goes to the log with the request ID, exactly as
			// internal() sends it for the single routes.
			s.logger(r).Error("bulk runner action failed", "doing", req.Action,
				"runner", runnerID, "error", err, "request_id", requestID(r))
			results = append(results, bulkRunnerResult{ID: runnerID, Error: "this runner could not be " +
				req.Action + "d; " + supportHint()})
			continue
		}
```

Before writing this, check how `s.logger(r)` and the request ID are obtained
in this package: `internal()` (errors.go:279-299) reads `infoFrom(r.Context())`
and `info.id`. If a `requestID`-style helper already exists, use it; if not,
inline the same two lines `internal` uses. Keep the log message a sentence
("bulk runner action failed" plus the fields) and keep the per-row message
short; `supportHint()` already ends it with where the cause is.

Note: `req.Action` is validated earlier in the handler to be exactly "drain"
or "delete", so `"d"` suffix reads "drained"/"deleted"; verify that against
the validation at lines 289-295 before relying on it, and if you prefer,
write the two words out in a two-line switch instead.

**Verify**:

```
go build ./internal/api/ && go vet ./internal/api/
```

both clean.

### Step 3: Test the mapping and the route

Write two tests, in the file found in Scope (or the new
`handlers_runners_bulk_test.go`):

1. `TestBulkRunnerMessageHidesWhatTheSingleRoutesHide`, a table test calling
   `bulkRunnerMessage` directly:
   - `store.ErrNotFound` wrapped with a runner ID: known, message contains
     "not found".
   - `controller.ErrConfirmationRequired` wrapped: known, message contains
     "confirm=true".
   - `controller.ErrInvalidTransition` wrapped: known.
   - `errors.New("sql: disk I/O error: /var/lib/zoomies/db.sqlite")`:
     not known, and the message the handler builds for it (assert by calling
     the same string the handler writes, or by asserting `known == false`
     and testing the handler in test 2).
2. `TestBulkDrainAnswersOneRowWithTheStoreSentenceAndLogsTheRest`, using the
   package's existing request harness (grep `internal/api/api_test.go` and
   the handlers tests for the `h.do(request{...})` shape; the bulk route is
   already exercised at `api_test.go` around line 856 for its role gate):
   - Seed two runners through the harness's store helpers the way existing
     runner tests do; one drains, one does not exist.
   - POST `{"action":"drain","ids":[<live>, "run_missing"]}` with an operator
     token.
   - Assert 200; one row OK; the other row's error contains "not found" and
     contains no SQL text.

Add a short comment on each test saying why the behaviour matters, in the
repo's voice.

**Verify**:

```
go test -run 'TestBulk' ./internal/api/
```

passes, then `go test ./internal/api/` passes.

## Test plan

- New tests: the two named above, covering the sentinel mapping, the
  confirm-advice sentence, and the unknown-error hiding.
- Structural pattern: the existing bulk-route role test at
  `internal/api/api_test.go` (search for `runners/bulk`) for the harness
  shape; the store-seeding helpers used by any existing runner drain test
  in `internal/api` (grep `DrainRunner\|drain` in `internal/api/*_test.go`).
- Verification: `make test` exits 0.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -n 'Error: err.Error()' internal/api/handlers_runners.go` returns
      no matches.
- [ ] `go test -run 'TestBulk' ./internal/api/` passes.
- [ ] `make test` exits 0.
- [ ] `gofmt -l internal/api/` and `go vet ./internal/api/` are clean.
- [ ] No files outside the in-scope list are modified (`git status`).
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back (do not improvise) if:

- The live `fail` in `internal/api/errors.go` maps different sentinels than
  the excerpt above, in a way that makes the helper's list wrong (copy the
  live list, but say so in your report).
- The bulk handler at `handlers_runners.go:316-339` has changed shape such
  that the per-row error is no longer a plain `err.Error()` append.
- Making the unknown-error row testable requires touching `errors.go` or
  anything else out of scope.
- The OpenAPI spec turns out to document the row's error text in a way the
  change would contradict (then the spec, and its generated copies, need a
  decision from the operator, not from you).
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- The same raw-error pattern may exist in the other bulk endpoints
  (`/machines/bulk`, `/provisioning/bulk`, `/assistant/repairs/bulk`); this
  plan deliberately fixes one route. A reviewer extending this should move
  the mapping helper somewhere shared only when a second caller exists, not
  before.
- `bulkRunnerMessage` must be kept in step with `fail`: when `fail` learns a
  new sentinel, this helper learns it too. A reviewer should diff the two
  lists.
- The request-ID plumbing in the handler should reuse whatever `internal()`
  uses if those helpers change; watch that in review.
