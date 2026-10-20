# Plan 004: Read one runner's detail without re-reading the whole fleet

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**, in the repo root:

```
git diff --stat a0f21dba..HEAD -- internal/api/handlers_runners.go internal/controller/views.go internal/store/queries_fleet.go
```

> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW (the response shape is guarded by existing tests on both the
  REST route and the event stream)
- **Depends on**: none
- **Category**: perf
- **Planned at**: commit `a0f21dba`, 2026-10-19

## Why this matters

`GET /api/v1/runners/{id}` is among the most-opened views in the UI, and
today it answers by re-reading the fleet to render one row: `RunnerRenderer`
lists every pool and every host (the host listing runs a GROUP BY over every
live runner to fill counts), then the handler reads the host again (re-running
that GROUP BY for one host's counts), then the pool and a whole `PoolRenderer`
before it. The repo already has the cheaper pattern: `jobViewIn`
(`internal/controller/views.go:742-753`) renders a job from rows the caller
already holds. This plan gives the runner detail the same shape, without
changing the response the API promises.

## Current state

- `internal/api/handlers_runners.go:74-106` (read the whole handler first):

```go
	// internal/api/handlers_runners.go:74-106 (excerpt)
	func (s *Server) handleGetRunner(w http.ResponseWriter, r *http.Request) {
		run, err := s.ctrl.Store().GetRunner(r.Context(), chiURLParam(r, "id"))
		if err != nil {
			s.fail(w, r, "reading the runner", err)
			return
		}
		view, err := s.ctrl.RunnerRenderer(r.Context(), []*store.Runner{run})   // lists every pool and host
		if err != nil {
			s.internal(w, r, "reading the runner", err)
			return
		}
		detail := runnerDetailResponse{
			RunnerView: view.View(run).For(isVisitor(r.Context())),
			Timeline:   runnerTimeline(run, s.ctrl.Now()),
		}

		if host, herr := s.ctrl.Store().GetHost(r.Context(), run.HostID); herr == nil {   // second host read + counts GROUP BY
			h := s.ctrl.HostView(host).For(isPlatform(r))
			detail.Host = &h
			detail.LogsAvailable = host.Healthy(s.ctrl.Now()) && run.State != store.RunnerRemoved
		}
		if pool, perr := s.ctrl.Store().GetPool(r.Context(), run.PoolID); perr == nil {   // second pool read
			if pv, verr := s.ctrl.PoolRenderer(r.Context()); verr == nil {                 // its own aggregates
				p := poolFor(r, pv.View(pool))
				detail.Pool = &p
			}
		}
		writeJSON(w, http.StatusOK, detail)
	}
```

- `internal/controller/views.go:939-994` is `RunnerRenderer`: it lists all
  pools and all hosts into maps, then fetches each busy runner's job:

```go
	// internal/controller/views.go:939-982 (excerpt)
	// RunnerRenderer names pools and hosts without a query per runner.
	type RunnerRenderer struct {
		pools map[string]*store.Pool
		hosts map[string]*store.Host
		jobs  map[string]*store.Job
	}

	func (c *Controller) RunnerRenderer(ctx context.Context, runners []*store.Runner) (*RunnerRenderer, error) {
		pools, err := c.st.ListPools(ctx)
		// ... hosts, err := c.st.ListHosts(ctx); for each busy runner: c.st.GetJob
		//     (a missing job row is tolerated: errors.Is(err, store.ErrNotFound) -> continue)
	}
```

  `View` (985 onward) reads the maps for the pool name, host name and the
  current job's `JobView`, then builds the `RunnerView` struct literal
  (995-1030ish).

- The precedent to copy, `jobViewIn` (`internal/controller/views.go:742-754`):

```go
	// internal/controller/views.go:742-753 (excerpt)
	// jobViewIn is jobView for a caller that has just read the pools to claim the
	// job. It names the job's pool from them, instead of reading the pool again,
	// whole, to learn what the pools in hand already say; ...
	func (c *Controller) jobViewIn(ctx context.Context, j *store.Job, pools []*store.Pool) JobView {
		if p := poolIn(pools, j.PoolID); p != nil {
			return NewJobView(j, p.Name)
		}
		return c.jobView(ctx, j)
	}
```

- `PoolRenderer` (`views.go:1254-1292`) runs its own three aggregate queries
  (count by pool, installations, queued jobs) plus in-memory problem lists; it
  does not list pools. The handler needs it for the drawer's pool summary, so
  it stays. What it does not need again is the pool *row*, which
  `Store().GetPool` reads.
- `internal/store/queries_fleet.go:764-803`: `GetHost` fills a single host's
  live runner counts through `fillHostCounts` -> `countRunnersByHost`, the
  GROUP BY that `ListHosts` (837-858) also runs for every host. One host read
  is needed for the drawer (its `HostView` carries the counts); the waste is
  the *second* one, inside `RunnerRenderer`, plus the pool and host listings
  that render one row's names.
- Existing tests that guard the shape: `internal/api/api_test.go` walks every
  route in the OpenAPI spec with five callers (search it for `runners/{id}`),
  and the handlers tests cover the detail response. The `For(isVisitor(...))`
  / `isPlatform(r)` narrowing must survive unchanged.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Prepare the build (once, fresh checkout) | `make build-nogui` | exit 0; writes the UI placeholder `internal/api` embeds, without which plain `go test` fails |
| One test | `go test -run TestHandleGetRunner ./internal/api/` (or the name you find, see Step 4) | pass |
| Package tests | `go test ./internal/api/ ./internal/controller/` | pass |
| Full suite | `make test` | exit 0 |
| Benchmarks (before and after, per the repo's rule) | `make bench` (see Step 5; `BENCH`, `BENCH_PKGS` narrow it) | runs, results tee'd to files |
| Format check | `gofmt -l internal/api/ internal/controller/` | no output |
| Vet | `go vet ./internal/api/ ./internal/controller/` | no findings |

## Conventions that apply here

- British spelling in prose. No em dash characters anywhere, and never a
  spaced double hyphen used as a dash.
- Comments explain why: the new controller method's comment should carry the
  same reasoning `jobViewIn`'s does (rows the caller already holds; the read
  it saves), adapted to the runner case.
- The repo's rule for speed changes (root `AGENTS.md`, "Commands"): run
  `make bench` before and after on an otherwise idle machine, once per
  package, tee'd to files, and put the `benchstat` comparison in the commit
  message. Step 5 covers what to do if no benchmark exercises this route.
- Test names are sentences about behaviour; table-driven, standard library
  `testing` only.
- Commit messages are imperative sentences in plain prose, sentence case, no
  prefix; for example "Render a runner's detail from the rows its host and
  pool already gave".

## Scope

**In scope** (the only files you should modify):

- `internal/controller/views.go` (extract the render helper, add
  `RunnerViewIn`)
- `internal/api/handlers_runners.go` (rewrite `handleGetRunner`'s reads)
- `internal/api/handlers_runners_test.go` or wherever the runner detail tests
  live today (find with `grep -rln 'handleGetRunner\|runners/{id}\|RunnerDetail' internal/api/*_test.go`);
  create `internal/api/handlers_runners_detail_test.go` (package `api`) if
  none covers it.

**Out of scope** (do NOT touch, even though they look related):

- `internal/store/queries_fleet.go`: `GetHost`, `ListHosts`,
  `fillHostCounts` and `countRunnersByHost` stay exactly as they are. The plan
  removes redundant *calls*, not queries.
- `RunnerRenderer`, `PoolRenderer` and `HostView`: their constructors and
  behaviour stay as they are; only `RunnerRenderer.View`'s body is extracted
  (Step 1) with no behaviour change.
- The list route `GET /api/v1/runners` and the event stream's runner views:
  they keep using `RunnerRenderer`; do not "fix" them too.
- `api/openapi.yaml`: the response shape does not change; no regeneration.
- `web/`: the UI reads the same JSON.

## Git workflow

- Branch: `perf/runner-detail-reads`.
- One commit, message: `Render a runner's detail from the rows its host and
  pool already gave` (plus the benchstat block per Step 5, if it applies).
- Do NOT push or open a pull request unless the operator instructed it.

## Steps

### Step 1: Extract the runner view rendering from the renderer

In `internal/controller/views.go`, make `RunnerRenderer.View`'s body delegate
to a new pure function, so both callers render identically:

```go
// runnerView renders one runner from the rows that name it: its pool, its host
// and, when it is busy, the job it is executing. It is RunnerRenderer.View
// for a caller that holds one runner's rows already, which is what a detail
// route is; the renderer's maps are the same three rows for a whole page.
func runnerView(r *store.Runner, pool *store.Pool, host *store.Host, job *store.Job) RunnerView {
	// the body of today's View: poolName/hostName from the rows (nil-safe,
	// exactly as the map lookups are today), the current job's JobView built
	// the way View builds it from v.jobs (read View's body to the end first;
	// everything after the struct literal, For(...) excluded, moves here
	// unchanged)
}
```

`View` becomes: look up `v.pools[r.PoolID]`, `v.hosts[r.HostID]`,
`v.jobs[r.CurrentJobID]` and call `runnerView`. No behaviour change; the nil
row (pool or host or job missing) must render exactly as it does today, where
the map lookup yields nil. Note before you start: read `View` to its last
line; if it does more than name-building and job-view-building (for example
narrowing, which stays in the handler's `.For(...)` call), treat the extra as
part of the helper only if it is pure; otherwise STOP.

**Verify**: `go test ./internal/controller/ ./internal/api/` passes (the
renderer's own tests and the route tests keep the shape honest), `gofmt -l
internal/controller/` clean.

### Step 2: Add `Controller.RunnerViewIn`, the `jobViewIn` shape for runners

In `internal/controller/views.go`, next to the renderer:

```go
// RunnerViewIn renders one runner for a caller that already holds its host
// and pool rows, the way jobViewIn names a job from pools in hand: a detail
// route reads its host and pool anyway, and listing every pool and host to
// name one runner is the fleet read a single drawer never needed. The busy
// runner's job is fetched here, and a job row that has gone since the runner
// was read is tolerated the way RunnerRenderer tolerates it: the runner is
// rendered without it.
func (c *Controller) RunnerViewIn(ctx context.Context, run *store.Runner, host *store.Host, pool *store.Pool) (RunnerView, error) {
	var job *store.Job
	if run.CurrentJobID != "" {
		j, err := c.st.GetJob(ctx, run.CurrentJobID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return RunnerView{}, fmt.Errorf("reading the job a runner is executing: %w", err)
		}
		job = j
	}
	return runnerView(run, pool, host, job), nil
}
```

Mirror the wording of `RunnerRenderer`'s own job fetch (views.go:963-980) so
the two stay in step, comment included.

**Verify**: `go vet ./internal/controller/` clean; `go test
./internal/controller/` passes.

### Step 3: Rewrite the handler around the rows it already needs

In `internal/api/handlers_runners.go`'s `handleGetRunner`, in order:

1. `GetRunner` as today.
2. `host, herr := s.ctrl.Store().GetHost(...)` once; keep today's
   `herr == nil` branch (HostView, `LogsAvailable`) exactly.
3. `pool, perr := s.ctrl.Store().GetPool(...)` once.
4. Replace the `RunnerRenderer` call with
   `s.ctrl.RunnerViewIn(r.Context(), run, host, pool)`, mapping its error to
   the same `s.internal(w, r, "reading the runner", err)` the renderer path
   used.
5. Keep `.For(isVisitor(r.Context()))` on the view, keep `Timeline`, keep
   the `PoolRenderer` block (it needs the pool *view*, with its aggregates;
   feed it the `pool` row you already read instead of reading it again
   inside; if the live code reads the pool inside that block, hoist the single
   read so the row is read once and both uses take it).
6. `writeJSON` as today.

The JSON byte-for-byte shape must not change: same fields, same narrowing,
same `omitempty` behaviour.

**Verify**:

```
go test ./internal/api/
```

passes (the route-walking tests in `api_test.go` cover the shape for five
callers), and `gofmt -l internal/api/ internal/controller/` is clean.

### Step 4: Add the one behaviour test the change introduces

The new behaviour worth pinning is `RunnerViewIn`'s tolerance: a busy runner
whose job row is gone (deleted or replaced between reads) renders without the
job instead of 500-ing, exactly as the list route does. Write one test
(following an existing handlers test for its harness and seeding pattern):

- Seed a busy runner whose `CurrentJobID` points at no row (the store tests
  show how rows are seeded; or seed a job and delete it through the store).
- `GET /api/v1/runners/{id}` answers 200 with the runner and no current-job
  fields, and `host` and `pool` present.

Name it as a sentence, for example
`TestARunnerWhoseJobRowWentAwayStillRendersItsDetail`. Add the why-comment.

**Verify**: `go test -run TestARunnerWhoseJobRowWentAway ./internal/api/`
passes.

### Step 5: Benchmarks, per the repo's rule

Check whether any benchmark exercises the runner detail route or the
renderers:

```
grep -rn 'func Benchmark' internal/api/ internal/controller/ | grep -i 'runner\|render'
```

- If one exists: run `make bench` before the change (you are after it now, so
  stash or use `git stash` to measure before, then unstash; alternatively
  measure on the parent commit in a scratch worktree), and after, tee'd to
  files, once per package on an idle machine, and put the `benchstat`
  comparison in the commit message.
- If none exists, do not write one as part of this plan (a benchmark belongs
  with the route it measures, and inventing one under time pressure makes a
  flaky one); instead say in the commit message: no existing benchmark covers
  this route; the win is the removed reads, counted by the code.

## Test plan

- New test: the job-row-gone tolerance test (Step 4). Pattern: an existing
  runner handlers test (grep first) for harness and seeding.
- Existing tests carry the shape: `internal/api/api_test.go` walks every
  route with five callers; the controller's view tests pin the renderers.
- Verification: `make test` exits 0.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -n 'RunnerRenderer' internal/api/handlers_runners.go` returns no
      matches (the detail route no longer builds a fleet-wide renderer).
- [ ] `grep -c 'GetHost\|GetPool' internal/api/handlers_runners.go` shows
      each exactly once inside `handleGetRunner`.
- [ ] `go test ./internal/api/ ./internal/controller/` passes; `make test`
      exits 0.
- [ ] The response of `GET /api/v1/runners/{id}` for a live runner is
      byte-identical in shape to before the change (the route-walking tests
      assert this; their passing is the criterion).
- [ ] `gofmt -l` clean on both packages; `go vet` clean.
- [ ] No files outside the in-scope list are modified (`git status`).
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back (do not improvise) if:

- `RunnerRenderer.View`'s body turns out to do more than build names, the
  current job's view, and the struct literal (Step 1's extraction is then not
  the mechanical move this plan claims).
- The handler or the response type has drifted from the excerpts (a newer
  field, a different narrowing chain).
- Making the two `herr`/`perr` branches read once requires touching the store
  or the views' public types.
- The route-walking tests fail after Step 3 in a way that points at a real
  shape difference rather than a test to update: STOP and report the diff
  rather than bending the test.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- `runnerView` (the pure helper) and `RunnerRenderer.View` must stay in step;
  a reviewer should check that `View` still delegates rather than re-growing
  its own body.
- The same single-reads pattern is deliberately not applied to the list route
  or the event stream; those are fleet-wide by nature and the renderer is the
  right tool there.
- If a future change adds a field to the detail response that needs another
  fleet aggregate, prefer extending `PoolRenderer`'s aggregates over a new
  per-detail read; that is the seam this plan leaves behind.
