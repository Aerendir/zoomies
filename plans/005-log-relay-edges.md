# Plan 005: Make the log relay's edges honest: races, drops, empty downloads, late tails

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
> git diff --stat a0f21dba..HEAD -- internal/controller/logs.go internal/api/sse.go internal/agent/logbuffer.go
> ```
>
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED (concurrent relay code; the race detector and the existing
  relay tests are the guard, but the merge in Step 4 is the one genuinely new
  moving part)
- **Depends on**: none (but land before Plan 009 if that is executed; they
  share `internal/controller`)
- **Category**: bug
- **Planned at**: commit `a0f21dba`, 2026-10-19

## Why this matters

Four edges of the log relay mislead an operator, each silently:

1. A viewer joining an existing follow stream gets no tail at all: its `Tail`
   request is ignored, so a second tab (or a reconnect) shows an empty pane
   until the runner prints more.
2. A log download that exceeds the relay's byte budget is truncated with no
   mark: the agent's own buffer says what it dropped
   (`internal/agent/logbuffer.go:93-101`), the controller's relay drops bytes
   silently (`internal/controller/logs.go:269-280`), so a 20 MB CI log can
   arrive as a seamlessly gapped file with a 200 status.
3. A download that gets no first chunk within 10 s returns 200 with an empty
   attachment, because the headers are written before the wait, and the wait
   can be shorter than task delivery (an agent in poll backoff waits up to
   30 s, `internal/agent/daemon.go`'s `maxPollBackoff`).
4. A viewer attaching in the instant between "the last viewer left" and "the
   stream is torn down" joins a stream that is closed under it moments later.

## Current state

Read all three files in full before starting: `internal/controller/logs.go`
(392 lines, the relay), `internal/api/sse.go` lines 445-609 (the follow and
download handlers), `internal/agent/logbuffer.go` (the agent's own buffer,
whose drop marker is the pattern to copy). Excerpts:

```go
	// internal/controller/logs.go:131-153 (excerpt): subscribe ignores a late
	// joiner's Tail
	func (lr *logRelay) subscribe(r *store.Runner, opts backend.LogOptions) (<-chan []byte, func(), error) {
		lr.mu.Lock()
		var s *logStream
		var streamID string
		fresh := true
		if opts.Follow {
			if curID, ok := lr.byRunner[r.ID]; ok {
				s = lr.streams[curID]
				if s != nil && !s.isClosed() {
					streamID = curID
					fresh = false     // the late joiner: opts (incl. Tail) is never used
				}
			}
		}
		if fresh {
			streamID = "log_" + store.NewSecret(8)
			s = &logStream{id: streamID, runnerID: r.ID, hostID: r.HostID, subs: map[int]chan []byte{}}
			lr.streams[streamID] = s
			if opts.Follow {
				lr.byRunner[r.ID] = streamID
			}
		}
		lr.mu.Unlock()
		ch, subID, err := s.add()
		// ... if fresh: lr.c.enqueue(r.HostID, agent.Task{Kind: agent.TaskStreamLogs, RunnerID: r.ID,
		//        StreamID: streamID, LogOptions: &opts})
	}
```

```go
	// internal/controller/logs.go:175-199 (excerpt): the teardown race
	func (lr *logRelay) unsubscribe(s *logStream, subID int) {
		last := s.remove(subID)      // decides "last viewer" under s.mu ...
		if !last {
			return
		}
		lr.mu.Lock()                 // ... but leaves the maps alone until here
		if lr.streams[s.id] == s {
			delete(lr.streams, s.id)
		}
		if lr.byRunner[s.runnerID] == s.id {
			delete(lr.byRunner, s.runnerID)
		}
		lr.mu.Unlock()
		// ... enqueue TaskCancelLogs, then s.close()
	}
```

The window: between `s.remove` returning true and the `lr.mu.Lock`, a
concurrent `subscribe` finds the stream still in `byRunner` and not closed,
joins it, and is closed under it by the teardown's `s.close()`.

```go
	// internal/controller/logs.go:266-280 (excerpt): silent drop
	buf := make([]byte, logReadChunk)
	for {
		n, err := r.Read(buf)
		if n > 0 && !s.spend(n, c.Now()) {
			// Over budget: the bytes are read and dropped, never waited on.
			c.metrics.logRelayDropped.Add(float64(n))
			n = 0
			if s.isClosed() {
				return nil
			}
		}
```

The agent's own marker, for contrast (`internal/agent/logbuffer.go:93-101`):

```go
	// reportDropped queues the notice for whatever was refused since the last one.
	func (b *relayBuffer) reportDropped() {
		if b.dropped == 0 {
			return
		}
		b.put([]byte(fmt.Sprintf("\n[log stream fell behind, %d bytes dropped]\n", b.dropped)))
		b.dropped = 0
	}
```

```go
	// internal/api/sse.go:560-572 (excerpt): headers before the first chunk
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+logFilename(run.Name)+`"`)
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	const firstChunkWait = 10 * time.Second
	const quietFor = 2 * time.Second
```

- Task queue keying: `internal/controller/agents.go:176-195`, `taskKey`:
  stream tasks key on `Kind|stream:<StreamID>`, so two concurrent streams for
  one runner each hold their own key; the late-joiner tail (Step 4) does not
  collide with the live stream's task.
- Tests: `internal/controller/logs_test.go` exists and covers the relay; the
  SSE handlers are covered by tests in `internal/api` (grep
  `grep -rln 'logs/download\|OpenLogStream' internal/api/*_test.go`).

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Prepare the build (once, fresh checkout) | `make build-nogui` | exit 0; writes the UI placeholder `internal/api` embeds, without which plain `go test` fails |
| Package tests | `go test -race ./internal/controller/ ./internal/api/` | pass (the race detector is the point; never run these without `-race`) |
| Full suite | `make test` | exit 0 |
| Format check | `gofmt -l internal/controller/ internal/api/` | no output |
| Vet | `go vet ./internal/controller/ ./internal/api/` | no findings |

## Conventions that apply here

- British spelling in prose. No em dash characters anywhere, and never a
  spaced double hyphen used as a dash.
- Comments explain why, not what; each of the four fixes lands with a comment
  naming the failure mode it designs out, in the voice of the neighbouring
  comments in `logs.go` (read them first; they are the style guide).
- The controller never dials an agent; log streaming stays inverted (the
  agent POSTs chunks). Nothing in this plan changes that.
- `*.updated` events are the resource's `GET` shape; the log stream is not
  an event-bus stream, so that rule does not apply here, but do not add any
  new event kinds.
- Test names are sentences about behaviour; table-driven, standard library
  `testing` only; helpers take `t` and call `t.Helper()` / `t.Cleanup`.
- Commit messages are imperative sentences in plain prose, sentence case, no
  prefix; for example "Say what a log relay dropped, and mean the tail a
  second viewer asked for".

## Scope

**In scope** (the only files you should modify):

- `internal/controller/logs.go` (Steps 1, 2 and 4)
- `internal/api/sse.go` (Step 3, the download handler only, lines ~540-597;
  and Step 4 if the late-joiner notice needs the follow handler)
- `internal/controller/logs_test.go` and the `internal/api` tests covering
  the log routes (extend; find them with
  `grep -rln 'OpenLogStream\|logs/download' internal/api/*_test.go`)
- `api/openapi.yaml` **only if** Step 4's STOP condition fires (documenting
  the late-joiner limitation instead of fixing it); then `make openapi` and
  commit both generated copies.

**Out of scope** (do NOT touch, even though they look related):

- `internal/agent/logbuffer.go`: its marker is the pattern to copy, not the
  code to change.
- `internal/controller/agents.go` (`taskKey`, the task queue): read it, do
  not modify it.
- The budgets themselves (`logStreamBytesPerSecond`, `logStreamBurst` at
  `logs.go:29-30`): raising them is a different decision; this plan makes the
  drops legible, it does not move the limits.
- `internal/events`, any event-bus code.

## Git workflow

- Branch: `fix/log-relay-edges`.
- One commit per step-pair is welcome (Steps 1+2 together, 3, 4), each an
  imperative sentence; or one commit for the whole plan.
- Do NOT push or open a pull request unless the operator instructed it (if
  they did, one pull request for the whole plan is the right shape).

## Steps

### Step 1: Close the teardown race by deciding "last viewer" under one lock

Reorder `unsubscribe` so the last-viewer decision and the map removals happen
under `lr.mu` together, before `s.close()` can race a join:

```go
func (lr *logRelay) unsubscribe(s *logStream, subID int) {
	// Decide "last viewer" and empty the maps under one hold of lr.mu, or a
	// viewer arriving between the two sees a stream in byRunner that is
	// being torn down, joins it, and has its channel closed moments later.
	// Lock order is lr.mu then s.mu everywhere now (subscribe already takes
	// them in that order), so this cannot deadlock with add/close.
	lr.mu.Lock()
	last := s.remove(subID)
	if last {
		if lr.streams[s.id] == s {
			delete(lr.streams, s.id)
		}
		if lr.byRunner[s.runnerID] == s.id {
			delete(lr.byRunner, s.runnerID)
		}
	}
	lr.mu.Unlock()
	if !last {
		return
	}
	// ... the TaskCancelLogs enqueue and s.close() stay as they are
}
```

`remove` (which takes `s.mu`) is now called while `lr.mu` is held; confirm
by reading `add`, `close`, `send` and `isClosed` (logs.go:328-392) that none
of them takes `lr.mu` (they do not; they are per-stream), and that
`subscribe` takes `lr.mu` then `s.mu` via `s.add()` (it does, logs.go:132-155).
`AcceptLogStream`'s own cleanup (logs.go:255-264) takes `lr.mu` then calls
`s.close()`; the same order, still safe.

**Verify**: `go test -race ./internal/controller/` passes (the existing relay
tests; add no test yet).

### Step 2: Mark what the relay drops, in band, the way the agent does

In the relay read loop (`logs.go:266-289`), stop dropping silently. Keep a
`dropped int` in the loop; when `spend` fails, add `n` to it and set `n = 0`
as today; before the next successful `send` (and on the stream's natural
end), flush one marker chunk:

```go
	// The agent's own buffer says what it dropped in band; the relay owes the
	// same sentence, or an operator reading a downloaded file cannot tell a
	// quiet build from a truncated one.
	marker := func() []byte {
		return []byte(fmt.Sprintf("\n[the controller's log relay fell behind and dropped %d bytes; read the log live, or download it again with a tail]\n", dropped))
	}
```

Send the marker through the ordinary `s.send` path (so every subscriber, the
live tail and the download alike, gets it), reset `dropped` to 0 after
sending, and keep the `logRelayDropped` metric as it is (it counts bytes; the
marker is bytes too, spend them through `spend` so the marker cannot recurse
the budget: if the marker's own spend fails, drop it and keep the counter
honest). Wording is yours to polish, but the bracketed sentence style, and
naming who dropped what, follow `logbuffer.go:99`.

**Verify**: `go test -race ./internal/controller/ ./internal/api/` passes.

### Step 3: Answer a silent download with 503, not an empty 200 with a filename

In `handleDownloadRunnerLogs` (`sse.go:540-597`): do not write the headers
until the first chunk arrives. Restructure:

- Move the `Content-Type`/`Content-Disposition`/`WriteHeader(200)` block
  below the first received chunk (keep `logFilename(run.Name)`, which needs
  `run`, read before the stream opens; that read stays where it is).
- The `firstChunkWait` timer now guards a *pre-header* wait: if it fires
  with no chunk ever received, answer with the funnel's 503 shape, a sentence
  a person can act on, for example via the existing `s.internal`-style
  helper if one fits a "no bytes arrived" cause, or a plain
  `writeError(w, http.StatusServiceUnavailable, ...)` with a message like
  "the runner's host did not begin sending its log within 10 seconds; it may
  be busy or restarting; try the download again, or read the log live".
  Match the error-envelope shape the package already writes (read
  `internal/api/errors.go`'s `writeError` users first).
- After the first chunk: write the headers, write that chunk, then continue
  the loop exactly as today (`quietFor` closing the response on quiet).
- The client-gone case before headers: return without writing anything, as
  today.

**Verify**: `go test ./internal/api/` passes; then check whether an existing
download test pins the "no output yet" case (grep `logs/download` in
`internal/api/*_test.go`) and extend it per Step 5.

### Step 4: Give a late follow viewer the tail it asked for

This is the one genuinely new mechanism; read `subscribe` (131-173), the
`taskKey` comment (`agents.go:176-195`) and `AcceptLogStream` (the read loop,
~230-304) once more before writing it.

Design: when a follow viewer joins an existing stream (`fresh == false`) and
`opts.Tail > 0`, open a *second*, short-lived stream for the same runner:
its own `log_` id, `Follow: false`, `Tail: opts.Tail`, **not** registered in
`byRunner` (a non-follow stream never is). Enqueue its own
`TaskStreamLogs` (its own `StreamID`, so its own task key; the agent POSTs a
second, tail-only relay). Fan both channels into one merged channel for the
new viewer:

```go
// merged := make(chan []byte, logSubscriberQueue)
// goroutine: forward from the live stream's channel and the tail channel
// into merged, closing merged when the live channel closes (the tail
// channel closes itself: a non-follow relay ends at EOF and its cleanup
// removes the second stream; read AcceptLogStream's cleanup path to
// confirm, and note it in your report).
```

The `cancel` func the handler defers must unsubscribe the viewer from the
live stream *and* let the tail stream finish (unsubscribing the tail's only
viewer closes it via the ordinary last-viewer path; the merge goroutine must
also stop on the ctx/channel-closed, with no leak: every goroutine this step
starts must end when the merged channel's readers go away).

If, when you get into `AcceptLogStream` and the task queue, the second
stream's lifecycle fights the queue's re-delivery or the relay's cleanup
(anything that needs `agents.go` or the queue changed to work), STOP: the
fallback, which the operator pre-approves here, is to document the behaviour
instead, in `api/openapi.yaml` on the follow route's `tail` parameter (one
sentence: the tail is served to the first viewer of a stream; a viewer
joining an existing stream begins at the present output), plus `make openapi`
and both generated copies committed, and a note in `plans/README.md` that
Step 4 was landed as documentation.

**Verify**: `go test -race ./internal/controller/ ./internal/api/` passes,
with the Step 5 tests added.

### Step 5: Pin all four edges in tests

Extend `internal/controller/logs_test.go` (pattern: its existing relay tests)
and the `internal/api` log-route tests:

1. `TestUnsubscribeAndSubscribeCannotInterleave` (or the existing file's
   naming style): hammer it: N goroutines subscribe while one unsubscribes
   the last viewer; every subscriber either gets a working channel or an
   `ErrStreamUnknown` refusal; none gets a channel closed under it without
   the relay's own end. Run under `-race`.
2. `TestTheRelaySaysWhatItDropped`: a stream over budget with a subscriber;
   assert the subscriber receives a chunk containing "dropped".
3. `TestADownloadWithNoFirstChunkIsRefusedNotEmpty` (in `internal/api`):
   a runner whose stream never delivers (the tests' fake agent path; read how
   the existing log tests simulate a host that does not answer) answers 503
   with a message, not 200 with an empty attachment.
4. `TestASecondViewerGetsTheTailItAskedFor` (or the documentation fallback's
   absence, if Step 4 STOPped): two sequential `subscribe` calls with
   `Follow: true, Tail: N`; the second receives the tail bytes before the
   live ones.

Each test carries its why-comment.

**Verify**: `make test` exits 0.

## Test plan

The four tests of Step 5, modelled on the existing relay tests in
`internal/controller/logs_test.go` and the log-route tests found by
`grep -rln 'OpenLogStream\|logs/download' internal/api/*_test.go`. Run
everything under `-race`; the merge goroutine of Step 4 is exactly the kind
of code the detector exists for.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -n 'logRelayDropped' internal/controller/logs.go` still finds the
      metric, and a marker sentence (`dropped`) is sent in band.
- [ ] In `internal/api/sse.go`'s download handler, `WriteHeader` is reached
      only after the first chunk, or not at all on the 503 path.
- [ ] The four Step 5 tests exist and pass under `-race`.
- [ ] `make test` exits 0; `gofmt -l` and `go vet` clean on both packages.
- [ ] No files outside the in-scope list are modified (`git status`).
- [ ] `plans/README.md` status row updated (and Step 4's fallback noted if
      taken).

## STOP conditions

Stop and report back (do not improvise) if:

- Step 4's second stream requires changes to `internal/controller/agents.go`
  or the task queue to work (take the documented fallback, which is
  pre-approved, and say so).
- The lock ordering in Step 1 turns out to be reversed anywhere (a caller
  holding `s.mu` that takes `lr.mu`): report the site; do not proceed with a
  half-reordered lock hierarchy.
- The download handler's pre-header restructure would leave the
  `Content-Disposition` unable to use `run` (it needs the runner row read
  before the stream; if that read has moved, stop and report).
- Any existing relay test's expectations are contradicted by the new marker
  bytes (a test asserting exact stream contents needs updating; that is
  fine, but report which, and why the new bytes are the corrected
  expectation).
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- The marker sentence of Step 2 is now part of a downloaded log's content;
  if a tool ever parses downloaded logs, the bracketed marker is the
  delimiter to skip. Say so in the commit message.
- Step 4's merge goroutine is the piece a reviewer should read hardest: its
  exit paths (viewer gone, live stream closed, tail done) must all end it.
  The race detector plus test 1 are the standing guard.
- The budgets (`logStreamBytesPerSecond`, `logStreamBurst`) were
  deliberately left alone; if an operator raises them, the marker becomes
  rarer, never wrong.
- The 503 wording of Step 3 should be revisited if the download route ever
  grows a retry-after; it does not have one today.
