# Plan 010: Resolve the stream's narrowing identity per heartbeat, not per frame

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
> git diff --stat a0f21dba..HEAD -- internal/api/sse.go internal/api/handlers_events.go internal/api/auth.go
> ```
>
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: MED (a security-relevant resolution point moves; the plan keeps
  the fail-closed semantics and the staleness equal to what the stream
  already accepts for `reader`, and the tests pin the narrowing itself)
- **Depends on**: none, but land after Plan 005 if that one is executed
  (both touch `internal/api/sse.go`, different regions)
- **Category**: perf
- **Planned at**: commit `a0f21dba`, 2026-10-19

## Why this matters

Every frame the event stream sends calls `s.resolveIdentity(r)` for each
narrowed frame kind, and each resolution is a SQLite session read, per
frame, per connected tab. The stream already knows the cheaper shape and
uses it for `reader` (the kind-checking identity): it resolves at open and
once per heartbeat, with a comment saying exactly why ("a busy fleet sends
many frames a second, and each resolution of a session is a database read",
`internal/api/sse.go:237-241`). This plan moves the *narrowing* identity to
the same tick. The cost: a role demotion reaches the narrowed fields up to
one heartbeat (20 s) late, which is the same staleness the stream already
accepts for `reader` and for `endIfRevoked` (the revoked check runs on the
same tick); the security posture (fail closed when resolution fails, drop
frames the caller may not see) is unchanged.

## Current state

- `internal/api/sse.go:243-279`, the live stream's loop (excerpt):

```go
	// Who the frames are chosen for. Resolved when the stream opens and again
	// on every heartbeat, which is when the stream re-checks its credential
	// anyway, rather than once per frame: a busy fleet sends many frames a
	// second, and each resolution of a session is a database read.
	reader := Identity(ctx)

	for {
		select {
		// ...
		case ev, ok := <-sub.C:
			// ...
			if !frameReadable(reader, ev.Kind) {
				continue
			}
			data, ok := s.frameFor(r, ev)      // line 258: per-frame resolution happens inside
			// ...
		case <-ticker.C:
			if s.endIfRevoked(stream, r, auth.ActionEventsRead) {
				return
			}
			if fresh, err := s.resolveIdentity(r); err == nil {
				reader = fresh                    // lines 269-271: the heartbeat refresh
			}
			// ...
		}
	}
```

- `internal/api/sse.go:291-405`, `frameFor`, calling `s.resolveIdentity(r)`
  once per narrowed kind, at lines 298 (problems), 318 (transfer), 328
  (updates), 345 (host), 366 (jobs naming an acknowledger), 388 (audit), 399
  (pools). Two representative excerpts:

```go
	// internal/api/sse.go:291-312 (excerpt)
	func (s *Server) frameFor(r *http.Request, ev events.Event) ([]byte, bool) {
		data := []byte(ev.Data)
		if ev.Kind == events.KindProblems {
			// The same two lists the REST route serves, chosen per
			// subscriber. Resolved per frame rather than once at
			// connect, for the reason the pool frames are: a viewer may
			// keep the stream open across a demotion.
			id, err := s.resolveIdentity(r)
			platform := err == nil && id != nil && id.Role.AtLeast(store.RolePlatform)
			// ...
		}
```

```go
	// internal/api/sse.go:379-394 (excerpt, the audit kind, the most
	// expensive: an unmarshal and a remarshall per subscriber per frame)
		if ev.Kind == events.KindAudit {
			var row store.AuditEvent
			if err := json.Unmarshal(data, &row); err != nil {
				return nil, false
			}
			id, _ := s.resolveIdentity(r)
			filtered, err := json.Marshal(auditRowFor(row, id))
			// ...
		}
```

- `internal/api/handlers_events.go:327-352`, the replay path,
  `recentFrame`, with its own comment (lines 328-330): "Resolved once per
  request, which lasts at most a wait, as the stream resolves it once per
  heartbeat" (aspirational for `frameFor` today; true after this plan):

```go
	// internal/api/handlers_events.go:336
	data, ok := s.frameFor(r, ev)
```

- `s.resolveIdentity` is declared at `internal/api/auth.go:184`; it re-reads
  the request's credential. Its per-call cost is the finding: routed
  through to `auth`'s `GetSessionByTokenHash`, a SQLite read.
- Fail-closed today: inside `frameFor`, a resolution *error* is treated as
  "not entitled" (the problems kind drops the frame for a non-platform
  caller; the transfer kind drops it outright; the audit kind narrows as a
  visitor via `auditRowFor(row, nil)`). A nil identity behaves the same.
  This must survive unchanged.
- Tests: `internal/api/handlers_events_test.go` covers the narrowing
  extensively (`TestRecentEventsNarrowFramesTheWayTheStreamDoes` at line
  385, `TestRecentEventsSendAFrameOnlyToSomebodyWhoCouldReadIt` at 584, and
  their neighbours); no test asserts per-frame (as opposed to
  per-heartbeat) demotion immediacy, which is what makes this change safe.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Prepare the build (once, fresh checkout) | `make build-nogui` | exit 0; writes the UI placeholder `internal/api` embeds, without which plain `go test` fails |
| One test | `go test -run TestRecentEvents ./internal/api/` | pass |
| Package tests | `go test -race ./internal/api/` | pass |
| Full suite | `make test` | exit 0 |
| Format check | `gofmt -l internal/api/` | no output |
| Vet | `go vet ./internal/api/` | no findings |
| The resolution-point check | `grep -n 'resolveIdentity' internal/api/sse.go` | exactly one call site, the heartbeat refresh (see Done criteria) |

## Conventions that apply here

- British spelling in prose. No em dash characters anywhere, and never a
  spaced double hyphen used as a dash.
- Comments explain why: the comments that say "Resolved per frame" must be
  rewritten to say what is now true (resolved once per heartbeat on the live
  stream, once per request on the replay), carrying the same reason (a
  viewer may keep the stream open across a demotion) plus the new sentence
  (the demotion reaches the narrowed fields on the same tick `reader` and
  the revocation check already accept).
- Fail closed: a resolution that fails treats the caller as nobody. Do not
  add a fallback that retries or caches a stale identity past the heartbeat.
- Test names are sentences; table-driven, standard library `testing` only.
- Commit messages are imperative sentences in plain prose, sentence case, no
  prefix; for example "Resolve the stream's narrowing identity on the
  heartbeat tick, like the reader's".

## Scope

**In scope** (the only files you should modify):

- `internal/api/sse.go` (`frameFor`'s signature and the live loop's call
  site, plus the comments)
- `internal/api/handlers_events.go` (`recentFrame`'s call site and comment)
- `internal/api/handlers_events_test.go` (only if a comment-bearing test
  name or assertion references per-frame resolution; read first)

**Out of scope** (do NOT touch, even though they look related):

- `internal/api/auth.go`: `resolveIdentity` itself is correct; it is called
  less often, not changed.
- `frameFor`'s narrowing logic (the filters, the drop rules, the
  `withoutAcknowledger`/`withoutEnvValues`/`auditRowFor` helpers): the
  entitlement decisions are untouched; only where the identity comes from
  changes.
- `endIfRevoked`, the heartbeat cadence (`s.streamHeartbeat`), the bus.
- `internal/auth`: no changes.

## Git workflow

- Branch: `perf/stream-identity-tick`.
- One commit, message: `Resolve the stream's narrowing identity on the
  heartbeat tick, like the reader's`.
- Do NOT push or open a pull request unless the operator instructed it.

## Steps

### Step 1: Thread the identity through `frameFor`

Change the signature:

```go
	// frameFor is one event as this caller may be shown it: the payload narrowed to
	// their audience, or false when the frame is not theirs to see at all.
	//
	// It is shared by the live stream and GET /events/recent, so the two cannot
	// disagree about who is entitled to what. ...
	// id is who the frames are narrowed for: nil is nobody, and a nil id
	// fails closed exactly as a resolution error did when frameFor resolved
	// per frame.
	func (s *Server) frameFor(r *http.Request, ev events.Event, id *auth.Identity) ([]byte, bool) {
```

Inside, replace every `id, err := s.resolveIdentity(r)` (lines 298, 318, 328,
345, 366, 388, 399) with use of the parameter:

- `platform := id != nil && id.Role.AtLeast(store.RolePlatform)` (the
  `err == nil` term is gone; a nil id is the failure).
- The transfer kind: `if id == nil || !id.Role.AtLeast(store.RolePlatform) { return nil, false }`.
- The audit kind: `filtered, err := json.Marshal(auditRowFor(row, id))`
  (a nil id is the "resolved nobody" it already handles).
- The pools kinds: `if id == nil || !auth.Allowed(id, auth.ActionPoolsWrite)`
  (check what the live code's condition is and keep its logic with the nil
  term).
- The job kinds: the condition `id, err := s.resolveIdentity(r); if err != nil || id == nil || auth.IsVisitor(id)`
  becomes `if id == nil || auth.IsVisitor(id)`.

Then update each block's comment: "Resolved per frame rather than once at
connect, for the reason the pool frames are: a viewer may keep the stream
open across a demotion" becomes the true statement: resolved by the caller
once per heartbeat on the live stream and once per request on the replay,
for the same reason; a demotion reaches the narrowed fields on the same
tick the reader and the revocation check already accept, and a resolution
that fails is a nil id, which is nobody.

**Verify**: `go build ./internal/api/` (the two call sites are fixed in
Steps 2 and 3; do not run the tests yet if it does not compile, fix the call
sites first, then `go vet ./internal/api/`).

### Step 2: The live stream passes the heartbeat identity

In the loop (`sse.go:243-279`): `reader` is already resolved at open (as
`Identity(ctx)`, the middleware's identity, line 241) and refreshed from
`s.resolveIdentity(r)` on the heartbeat (lines 269-271). Pass it through:

```go
		data, ok := s.frameFor(r, ev, reader)
```

The heartbeat's own refresh stays as it is. Note in the loop's comment (the
one at 237-241) that `reader` is now also who the narrowed frames are chosen
for, which is why its refresh and the revocation check share the tick.

**Verify**: `go test -race ./internal/api/` passes (the whole narrowing
suite; it exercises the routes, and the entitlement decisions are
unchanged).

### Step 3: The replay resolves once per request

In `recentFrame` (`handlers_events.go:327-352`), resolve once at the top
and pass it in:

```go
	// The stream's rule: a frame goes only to a caller who could have fetched
	// it from its GET route. Resolved once per request, which lasts at most a
	// wait, as the stream resolves it once per heartbeat; a resolution that
	// fails is nobody, and nobody's frames are narrowed as a visitor's.
	id, _ := s.resolveIdentity(r)
	// ... frameReadable stays as it is
	data, ok := s.frameFor(r, ev, id)
```

(Match the actual live shape: if `recentFrame` is a method without the
request handy, it has `r`; read it first. If the replay handler loops over
many events, the one resolution is the point: it replaces one per frame.)

**Verify**: `go test -run TestRecentEvents ./internal/api/` passes.

### Step 4: Confirm the narrowing survives, at both resolutions

No new behaviour is introduced, so no new test is strictly owed; but the
change is security-adjacent, so add the one pin that says what the new
resolution point is:

- In `internal/api/handlers_events_test.go`, extend (or add, following the
  file's harness) one test asserting the replay narrows for the *request's*
  identity across many frames in one page: a viewer's replay page over a mix
  of problems/pool/audit frames contains no platform-only fields (this is
  close to what `TestRecentEventsNarrowFramesTheWayTheStreamDoes` already
  asserts; if it already covers exactly this, do not duplicate it; instead
  add one comment line to it noting the resolution is once per request, and
  record in your report that no new test was owed).
- Grep first: `grep -n 'resolveIdentity' internal/api/sse.go` must now show
  exactly one call (the heartbeat refresh); none inside `frameFor`.

**Verify**: `make test` exits 0; `gofmt -l internal/api/` clean;
`go vet ./internal/api/` clean.

## Test plan

- The existing narrowing suite (`handlers_events_test.go`, especially
  `TestRecentEventsNarrowFramesTheWayTheStreamDoes` and
  `TestRecentEventsSendAFrameOnlyToSomebodyWhoCouldReadIt`) is the guard;
  it must pass unchanged.
- One pin if the existing suite does not already cover it (Step 4 decides):
  a viewer's replay page over mixed kinds carries no platform-only fields.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -n 'resolveIdentity' internal/api/sse.go` shows exactly one
      call site: the heartbeat refresh.
- [ ] `grep -n 'resolveIdentity' internal/api/handlers_events.go` shows at
      most one call site: the replay's once-per-request resolution.
- [ ] `frameFor`'s signature takes the identity as a parameter; no
      resolution inside.
- [ ] `go test -race ./internal/api/` passes; `make test` exits 0.
- [ ] `gofmt -l internal/api/` and `go vet ./internal/api/` clean.
- [ ] No files outside the in-scope list are modified (`git status`).
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back (do not improvise) if:

- Any *other* caller of `frameFor` exists besides the two this plan knows
  (`grep -rn 'frameFor(' internal/`): a third caller needs its own
  resolution-point decision, not a guess.
- A test fails in a way that shows a real entitlement difference (not a
  stale assertion): the fail-closed semantics may have been load-bearing in
  a way the plan missed; report the test and the difference.
- The live loop's `reader` turns out not to be refreshed from
  `resolveIdentity` on the heartbeat (the excerpt's lines 269-271 have
  changed): then the staleness claim is wrong and the plan's premise fails.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- The demotion-during-a-stream defence now has a single, documented cadence:
  the heartbeat tick (reader, revocation check, and now the narrowing). Any
  future narrowed frame kind should take the identity parameter and never
  re-resolve; a reviewer seeing a `resolveIdentity` inside a per-frame path
  should treat it as a regression of this plan.
- If the heartbeat cadence (`s.streamHeartbeat`) is ever shortened or
  lengthened, the demotion latency for narrowed fields moves with it; that
  is the accepted trade, and the comments now say so at the point a reviewer
  would look.
- If a future frame kind needs *fresh* per-frame entitlement (nothing today
  does), it should say why in a comment at its block, and resolve on its
  own, visibly, rather than quietly re-growing the per-frame pattern.
