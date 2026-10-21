# Plan 009: Extract the Kennel Club loop from the controller package, as the first seam

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. This plan is the riskiest in the set: it moves a
> working subsystem between packages. Its STOP conditions are load-bearing;
> use them. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**, in the repo root:

```
git diff --stat a0f21dba..HEAD -- internal/controller/kennel.go internal/controller/kennel_budget.go internal/controller/kennel_views.go internal/kennel/ docs/architecture.md AGENTS.md
```

> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: L
- **Risk**: MED (a mechanical move guarded by the race suite and the
  package-boundary tests; the risk is in what the cluster touches, which
  Step 1 inventories before anything moves)
- **Depends on**: land after Plans 004 and 005 if those are executed (they
  touch the same package; conflicts, not correctness)
- **Category**: tech-debt
- **Planned at**: commit `a0f21dba`, 2026-10-19

## Why this matters

`internal/controller` is a quarter of the Go tree (142 non-test files, about
52,600 lines against 212,690 under `internal/`) and no longer what its
documentation says it is ("Wiring: the reconcile loop, the machine loop,
webhook ingest, the agent task queue, the log relay"): whole subsystems live
in it, Kennel Club's loop being one of them (11 `kennel*.go` files, about
3,300 lines including tests). Every feature pull request therefore lands in
the same package, merge conflicts concentrate there, and the purity and
boundary tests that protect every other package do not exist *inside*
controller, so the subsystems are free to entangle. The repo has the exact
pattern to copy: `internal/updates` (pure) plus `internal/updates/channel`
(impure half), and `internal/kennel` (pure) already exists. This plan moves
one cluster, the Kennel Club loop, into a subpackage beside its pure core,
as the template; the eli, assistant and AI-context clusters follow the same
recipe later (Maintenance notes), one pull request each.

## Current state

- The cluster (all in `internal/controller`, all `package controller`):

  `kennel.go` (1078 lines, the loop and its runtime),
  `kennel_budget.go`, `kennel_guidance.go`, `kennel_guidance_setup.go`,
  `kennel_operations.go`, `kennel_problems.go`, `kennel_protection.go`,
  `kennel_settings.go`, `kennel_setup.go`, `kennel_snapshot.go`,
  `kennel_views.go`, `kennel_workflows.go`, and their
  `*_test.go` files (list them with `ls internal/controller/kennel*`).

  `kennel.go`'s own doc comment (lines 20-36) already describes the loop as
  a self-contained thing with its own goroutine, its own runtime state
  (`kennelRuntime`, line 59) and its own budget rule; it imports only
  `config`, `events`, `github`, `kennel`, `store` (lines 13-17) plus stdlib.

- The pure core: `internal/kennel` ("Kennel Club's decisions ... Pure like
  the scheduler ... a test that fails if that changes",
  `docs/architecture.md:315`) and its subpackage `internal/kennel/workflow`
  (architecture.md:316). The exemplar of an impure half in a subpackage:
  `internal/updates/channel` (architecture.md:314).
- The controller surface the cluster uses (from a grep at `a0f21dba`; Step 1
  re-verifies): `c.st` (the store), `c.cfg()`, `c.log`,
  `c.publish`/`c.publishDerived`/`c.publishKennelDeleted` (the event-bus
  publishing helpers; remember the invariant: every `*.updated` event is the
  resource's GET shape, rendered by the controller's publish helpers, never
  a store row on the bus), `c.Now`, `c.holdGitHub`/`c.githubHeld` (the
  GitHub stand-down bookkeeping), plus the kennel cluster's own methods
  calling each other. `taskKey` and the agent task queue are NOT used by this
  cluster (it writes no tasks).
- Tests that must stay green: the whole `internal/controller` suite,
  `internal/kennel`'s purity test (check its scope in Step 1), the
  `internal/docs` tests (architecture.md's kennel paragraph,
  `docs/architecture.md:333-342`, and the Components row at line 315, both of
  which this plan updates), and `make test` overall.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Prepare the build (once, fresh checkout) | `make build-nogui` | exit 0; writes the UI placeholder `internal/api` embeds, without which plain `go test` fails |
| Cluster inventory | the greps in Step 1 | the symbol list below |
| Package tests | `go test -race ./internal/controller/... ./internal/kennel/... ./internal/api/` | pass |
| Full suite | `make test` | exit 0 |
| Boundary tests | `go test ./internal/docs/ ./internal/kennel/` | pass |
| Format check | `gofmt -l internal/` | no output |
| Vet | `go vet ./internal/...` | no findings |

## Conventions that apply here

- British spelling in prose. No em dash characters anywhere, and never a
  spaced double hyphen used as a dash.
- Comments explain why; every moved file keeps its comments exactly as they
  are (this plan moves code, it does not rewrite it; the one new comment is
  the subpackage's doc comment).
- Every `*.updated` event is the resource's `GET` shape: the publishing stays
  behind the controller's helpers, injected as methods, so no store row can
  reach the bus.
- Test names are sentences; the moved tests keep their names and bodies
  (only the package line and any receiver/type references change).
- Commit messages are imperative sentences in plain prose, sentence case, no
  prefix; for example "Give Kennel Club's loop its own package beside its
  decisions".

## Scope

**In scope** (the only files you should move, create or modify):

- `internal/controller/kennel*.go` and their tests (move to
  `internal/kennel/club/`)
- `internal/controller/controller.go` (the field, the construction, the
  delegating methods; read it first, its layout decides where these go)
- `internal/kennel/club/` (create)
- `docs/architecture.md` (the kennel row at line 315 and the paragraph at
  333-342)
- `AGENTS.md` (the layout row for `internal/kennel`)
- `plans/README.md` (status)

**Out of scope** (do NOT touch, even though they look related):

- The eli, assistant and AI-context clusters: they follow this recipe in
  their own changes (Maintenance notes). Touching them here doubles the diff
  and halves the reviewability.
- `internal/kennel`'s own files (`kennel.go` etc. in that directory): the
  pure core is untouched.
- `internal/controller/views.go`, `derived.go`, `problems.go`, the reconcile
  loop, the agent task queue: read-only.
- `internal/api`: the handlers must keep compiling unchanged, which is what
  the thin delegating methods on the controller are for.
- The MCP tools' calls: they go through the REST routes; no change.

## Git workflow

- Branch: `refactor/kennel-club-package`.
- One commit per step where a step is independently green (the move, then
  the wiring, then the docs), or one commit for the whole; imperative
  sentences.
- Do NOT push or open a pull request unless the operator instructed it.

## Steps

### Step 1: Inventory and the go/no-go checks (nothing moves yet)

Run, and record the answers in your report:

1. The purity test's scope: `grep -rn 'imports\|Imports' internal/kennel/*_test.go | head`,
   then read the test that holds `internal/kennel` to the standard library.
   If it scopes to the `internal/kennel` directory's own package only (the
   pattern of `internal/updates`'s purity test, which tolerates
   `internal/updates/channel`), a `club` subpackage is fine. If it walks
   subdirectories or would fail, STOP: the fallback name is a top-level
   `internal/kennelclub`, and the operator should choose.
2. The controller surface: `grep -oh 'c\.[a-z][A-Za-z0-9]*(' internal/controller/kennel*.go | sort -u`,
   minus the cluster's own `c.kennel*` methods (they move with the cluster
   and become the new type's methods). What remains must be a subset of:
   `c.st`, `c.cfg(`, `c.log` (a field, not a call), `c.publish*`,
   `c.publishDerived`, `c.publishKennelDeleted`, `c.Now`, `c.holdGitHub`,
   `c.githubHeld`, and the construction/delegation the controller keeps. If
   anything outside that list appears (a reach into `c.reconcileMu`,
   `c.events`, `c.gh`, anything else), record it; more than two or three
   such symbols is a STOP (the cluster is more entangled than this plan
   claims, and the extraction needs a design conversation, not a recipe).
3. The exported entry points the API or CLI reach: `grep -rn 'ctrl\.\|c\.ctrl\.' internal/api/*_test.go internal/api/*.go | grep -i kennel | head -40`,
   and the same for `cmd/`. These become the controller's thin delegating
   methods (Step 3). If handlers reach kennel *state* directly (fields, not
   methods), STOP.
4. Confirm the cluster writes no agent tasks and takes no controller locks:
   `grep -n 'enqueue\|reconcileMu\|machineMu' internal/controller/kennel*.go`
   should return nothing.

**Verify**: the four answers recorded; every green light the plan expects.

### Step 2: Move the files, one package line at a time

1. `mkdir internal/kennel/club` and
   `git mv internal/controller/kennel*.go internal/kennel/club/` (move the
   test files with them; `git mv` preserves history).
2. In every moved file, change `package controller` to `package club`.
3. The moved code's `c *Controller` receivers become `c *Club` receivers, and
   the new `Club` type is declared in `kennel.go`'s moved home, holding
   exactly what Step 1 found the cluster uses:

```go
// Club is Kennel Club's loop: the impure half of internal/kennel, the way
// internal/updates/channel is of internal/updates. It holds the store, the
// settings, the logger, the clock and the publishing the loop needs, each
// injected by the controller that wires it, so nothing here can reach around
// them into the controller's own state.
type Club struct {
	st    *store.Store
	cfg   func() *config.Config
	log   *slog.Logger
	now   func() time.Time
	pubs  publisher   // the publish* helpers, an interface declared here
	gh    githubHolds // holdGitHub / githubHeld, an interface declared here
	// ... the moved kennelRuntime and whatever the cluster already held
}
```

   The two small interfaces (`publisher`, `githubHolds`) are declared in the
   club package with one reason-comment each: they exist so the moved code
   cannot name the controller, and the controller's publish helpers stay the
   only way anything reaches the bus (the GET-shape invariant).
4. Every moved identifier the rest of the controller or the API used stays
   reachable through the delegating methods of Step 3; inside the moved
   files, references to unmoved controller identifiers become references to
   the injected fields.
5. Fix the moved test files the same way (their `package controller` line,
   their references to controller identifiers; many will construct the Club
   directly, which is now *easier*, the tests are in-package).

Do not rewrite logic, reorder functions, or "improve" anything while moving;
the diff should read as a move plus a receiver change plus the injection. If
a moved test cannot compile without changing what it asserts, STOP.

**Verify**: `go build ./internal/kennel/...` exits 0 (the controller will
not link yet; that is Step 3).

### Step 3: Wire the controller to the club

In `internal/controller`:

1. Add the field (next to the loop fields it neighbours today; read
   `controller.go`'s struct first): `kennelClub *club.Club`.
2. In the controller's construction (find it with
   `grep -n 'func New' internal/controller/controller.go`), build the Club
   with the injected fields, implementing the two interfaces with thin
   methods on the controller (the `publish*` methods it already has, and the
   holdGitHub pair; each interface method is one line delegating to the
   existing helper).
3. Add the thin delegating methods for every exported kennel entry point
   Step 1.3 found, one line each:

```go
	// The API and the CLI keep calling the controller; each of these is one
	// line, and the loop's home is an implementation detail they never see.
	func (c *Controller) KennelOverview(ctx context.Context /*, the signature it already had */) (/* ... */) {
		return c.kennelClub.Overview(ctx /*, ... */)
	}
```

   Keep the exact signatures the handlers call today, so `internal/api`
   compiles unchanged. If a signature cannot be kept (it names a controller
   type the club now cannot see), STOP and report the type.
4. The loop's start and stop (however the controller starts the kennel
   goroutine today; `grep -n 'kennel' internal/controller/controller.go internal/controller/background.go`)
   becomes `c.kennelClub.Run(ctx)` or its existing entry, called from the
   same place.

**Verify**: `go build ./...` exits 0 from the repo root (after
`make build-nogui` once, per the commands table), and
`go test -race ./internal/controller/... ./internal/kennel/... ./internal/api/`
passes.

### Step 4: Update the two documents that name the loop's home

- `docs/architecture.md`: the `internal/kennel` row (line 315) gains the
  club, and the paragraph at 333-342 ("The loop in `internal/controller`
  builds snapshots") now names `internal/kennel/club` as the loop's home,
  in the same sentences, saying why (the pattern of `internal/updates`
  and `internal/updates/channel`, pure decisions beside impure wiring).
- `AGENTS.md`'s layout row for `internal/kennel` likewise; and the
  `internal/controller` row ("Wiring: the reconcile loop, ...") needs the
  kennel loop struck from it only if it names it (read it; at `a0f21dba` the
  architecture.md row does, in the paragraph, and the AGENTS.md table does
  not).

**Verify**: `go test ./internal/docs/` passes (the doc-binding tests),
`make test` exits 0.

### Step 5: The full gates, and the report

**Verify**: `make test` exits 0; `gofmt -l internal/` clean;
`go vet ./internal/...` clean; `git status` shows only in-scope files.

In your report: the four Step 1 answers, the count of delegating methods,
and the diff's shape (files moved, lines changed excluding the package and
receiver edits; `git diff --stat` and
`git diff --find-copies-harder --stat` both, so the reviewer sees the move).

## Test plan

No new tests: the moved tests are the coverage (they move with the code and
keep their names and assertions), and the package-boundary tests
(`internal/kennel`'s purity test, `internal/docs`) are the guards that this
plan must not violate. That is deliberate: a move that arrives with new
tests is a move mixed with a behaviour change, and unreviewable as either.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `ls internal/controller/kennel*.go` returns nothing; the cluster lives
      in `internal/kennel/club/`.
- [ ] `grep -rn 'package controller' internal/kennel/` returns nothing.
- [ ] `go build ./...` exits 0; `make test` exits 0; the race suite included.
- [ ] `internal/api` and `cmd/` are unmodified (`git status`).
- [ ] `docs/architecture.md` and `AGENTS.md` name the loop's new home.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back (do not improvise) if:

- Step 1's purity-test check shows `internal/kennel`'s test scopes beyond
  its own package (the subpackage needs the owner's naming decision).
- Step 1's surface inventory finds more than two or three controller symbols
  outside the expected set (the cluster is more entangled than the plan
  claims; the extraction needs a design conversation, not a recipe).
- Any handler or CLI call reaches kennel *state* rather than methods
  (Step 1.3).
- A moved file turns out to be shared by two clusters (a `kennel_*` file
  importing the eli or assistant halves, or vice versa; the shared file
  needs a decision, not a guess).
- A moved test cannot compile without changing what it asserts.
- Anything in the move requires touching `internal/api`, `cmd/`, or the
  reconcile loop.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- This plan is the template: the eli cluster (`eli_*.go`, 7 files plus
  tests), the assistant cluster (`assistant*.go`) and the AI-context cluster
  (`ai_context*.go`) each follow the same recipe in their own change, each
  with its own Step 1 inventory, and each landing after this one so the
  pattern is proven once before it is repeated three times.
- The two injected interfaces are the seam future clusters should reuse or
  parallel: `publisher` especially, since the GET-shape event invariant
  (root `AGENTS.md`, "Every `*.updated` event is the resource's `GET`
  shape") is enforced by routing every publish through the controller's
  helpers.
- A reviewer should read the diff with `--find-copies-harder` and check the
  receiver edit is the only logic change; anything else in the diff is a
  question to answer, not a style to accept.
- When the second cluster moves, the `internal/controller` row in AGENTS.md
  and architecture.md should be rewritten to describe what actually remains;
  this plan only keeps them true for the kennel loop.
