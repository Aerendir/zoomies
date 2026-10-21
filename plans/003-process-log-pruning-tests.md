# Plan 003: Test the process backend's log bounding, which nothing covers today

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**, in the repo root:

```
git diff --stat a0f21dba..HEAD -- internal/backend/process_logs.go internal/agent/native_logs.go internal/backend/process_files_test.go internal/agent/fakes_test.go
```

> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW (tests only; no production code changes)
- **Depends on**: none
- **Category**: tests
- **Planned at**: commit `a0f21dba`, 2026-10-19

## Why this matters

A process-backend fleet's runner logs are bounded on the host disk by exactly
two pieces of code: `boundLog` in `internal/backend/process_logs.go` (a
copy-truncate that archives the tail and empties the live file) and
`nativeLogLoop` in `internal/agent/native_logs.go` (the per-minute loop that
calls every backend implementing `logMaintainer`). Neither is referenced by
any test in the tree, so a regression in either, corrupting or deleting
diagnostic logs, or silently stopping the bounding, stays green in CI. The
repo's own standard (root `AGENTS.md`, "Testing": roughly one test file for
every source file) exists to catch exactly this. This plan adds the missing
tests and touches no production code.

## Current state

- `internal/backend/process_logs.go` (80 lines, read it in full first):

```go
	// internal/backend/process_logs.go:15-39 (excerpt)
	func (b *ProcessBackend) PruneLogs(ctx context.Context, limit int64) error {
		entries, err := os.ReadDir(filepath.Join(b.root, runnersDirName))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, e := range entries {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(b.root, runnersDirName, e.Name())
			if _, err := readMeta(dir); err != nil {
				continue
			}
			if err := boundLog(filepath.Join(dir, runnerLogFile), limit); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	}
```

```go
	// internal/backend/process_logs.go:40-79 (excerpt)
	func boundLog(path string, limit int64) error {
		if limit <= 0 {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() <= limit {
			return nil
		}
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		defer f.Close()
		opened, err := f.Stat()
		if err != nil {
			return err
		}
		if !os.SameFile(info, opened) {
			return nil
		}
		archive, err := os.CreateTemp(filepath.Dir(path), "log-archive-*")
		// ... io.CopyN(archive, io.NewSectionReader(f, info.Size()-limit, limit), limit),
		//     archive.Close(), os.Rename(archive.Name(), path+".previous"), f.Truncate(0)
	}
```

So after a prune of an oversized log: the live file is 0 bytes, `path +
".previous"` holds exactly the last `limit` bytes, and the inherited
descriptor stays valid (that is the point of copy-truncate; the file's own
doc comment at lines 11-14 says so and is the invariants list to pin).

The behaviour to pin: the `.previous` file's content equals the last `limit`
bytes of the original.

- `internal/agent/native_logs.go` (36 lines, read in full): the loop's
  contract is: runs one pass immediately on start, then once a minute; for
  each backend kind in `a.opts.Backends.Kinds()`, calls `PruneLogs(ctx with
  10s timeout, 10<<20)` on backends implementing `logMaintainer`, skips the
  rest, logs a warning per error, and returns nil when the context is
  cancelled.
- The constants `runnersDirName`, `runnerLogFile` and the `readMeta` helper
  are used by other process-backend code; find them with
  `grep -rn 'runnersDirName\|runnerLogFile\|func readMeta' internal/backend/`
  and read the file that defines them before writing tests (a runner
  directory only counts once `readMeta` accepts it; the existing tests in
  `internal/backend/process_files_test.go` show how a backend, its root and
  its runner directories are laid out in a `t.TempDir()`).
- `internal/agent/fakes_test.go` holds the fake backends and transports;
  `internal/agent/daemon_test.go:41` holds
  `newAgent(t *testing.T, capacity int, tweaks ...func(*Options)) (*Agent, *fakeTransport, *fakeBackend, *testClock)`.
  Read both to see how a test swaps in its own backends registry
  (`a.opts.Backends`, the `Kinds()`/`Get(kind)` interface) before writing the
  loop test.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Prepare the build (once, fresh checkout) | `make build-nogui` | exit 0; writes the UI placeholder `internal/api` embeds, without which plain `go test` fails |
| One test | `go test -run 'TestPruneLogs\|TestBoundLog\|TestTheNativeLogLoop' ./internal/backend/ ./internal/agent/` | pass |
| Package tests | `go test ./internal/backend/ ./internal/agent/` | pass |
| Full suite | `make test` | exit 0 (`go test -race -count=1 ./...`; the race detector matters here, these are concurrent loops) |
| Format check | `gofmt -l internal/backend/ internal/agent/` | no output |
| Vet | `go vet ./internal/backend/ ./internal/agent/` | no findings |

## Conventions that apply here

- British spelling in prose. No em dash characters anywhere, and never a
  spaced double hyphen used as a dash.
- Test names are sentences about behaviour (`TestPruneLogsBoundsAnOversizedLogToItsTail`),
  table-driven, standard library `testing` only, helpers take `t` and call
  `t.Helper()` / `t.Cleanup`. Tests use `t.TempDir()`, never real paths.
- A test carries a comment explaining why the behaviour matters, not what the
  code does; for these tests the why is: this loop is the only thing standing
  between a chatty runner and a full host disk, and nothing else would notice
  it breaking.
- Comments may quote the design invariants from
  `internal/backend/process_logs.go:11-14` (copy-truncate preserves the
  inherited descriptor; diagnostic retention, never the authoritative log).
- Commit messages are imperative sentences in plain prose, sentence case, no
  prefix; for example "Test the log bounding the process backend and its
  agent loop".

## Scope

**In scope** (the only files you should create or modify):

- `internal/backend/process_logs_test.go` (create)
- `internal/agent/native_logs_test.go` (create)

**Out of scope** (do NOT touch, even though they look related):

- `internal/backend/process_logs.go` and `internal/agent/native_logs.go`:
  read them, do not change them. If a test cannot be written without a
  production change, that is a STOP condition.
- The Docker/Podman backends: they never implement `logMaintainer`; no tests
  for them belong in this change.
- `internal/backend/process_files_test.go` and other existing test files:
  read them as patterns; do not edit them.

## Git workflow

- Branch: `test/process-log-bounding`.
- One commit, message: `Test the log bounding the process backend and its
  agent loop`.
- Do NOT push or open a pull request unless the operator instructed it.

## Steps

### Step 1: Read the pieces and pin the layout

Read `internal/backend/process_logs.go`, the file defining `runnersDirName`,
`runnerLogFile` and `readMeta`, and `internal/backend/process_files_test.go`.
Identify how a `ProcessBackend` is constructed over a root directory in tests
(the existing tests construct one; copy that). Confirm with a scratch test
run if needed.

**Verify**: you can name (in your report) the constructor used and the exact
steps `process_files_test.go` takes to give a backend a runner directory that
`readMeta` accepts.

### Step 2: Write `internal/backend/process_logs_test.go`

Cover, as table-driven as the cases allow:

- `boundLog` (called via `PruneLogs`, which is the exported surface; call
  `PruneLogs(ctx, limit)` on the backend, not the unexported `boundLog`,
  unless a case genuinely needs the path form, in which case calling
  `boundLog` directly is fine, the tests are in-package):
  1. An oversized regular log: after `PruneLogs(ctx, limit)`, the log file is
     0 bytes, `logFile + ".previous"` exists and its bytes equal the last
     `limit` bytes of the original (write a deterministic pattern, e.g. bytes
     `i % 251`, so a wrong slice is caught, not just a wrong length).
  2. A log at or under the limit: unchanged, no `.previous` created.
  3. A missing log file: no error returned (the `os.ErrNotExist` is
     tolerated by `PruneLogs`).
  4. A non-regular file (a symlink to an oversized regular file elsewhere):
     skipped, no error, neither file touched.
  5. `limit <= 0`: no-op, oversized log untouched.
  6. A runner directory without a meta file (`readMeta` fails): skipped,
     no error.
  7. A cancelled context: returns `context.Canceled` (or the ctx error)
     before touching anything.
- One case for the whole-runners-directory-missing path: `PruneLogs` on a
  backend whose root has no runners directory returns nil.

Name the file's tests as sentences; one possible shape is a single
`TestPruneLogsBoundsTheNativeRunnerLogs` with cases, plus
`TestPruneLogsLeavesAFleetWithoutRunnersAlone`. Add the why-comment.

**Verify**:

```
go test -run 'TestPruneLogs' ./internal/backend/
```

passes, including under `-race` (add `-race` to the command once, before the
full suite).

### Step 3: Write `internal/agent/native_logs_test.go`

Using the scaffolding from `internal/agent/fakes_test.go` and
`daemon_test.go` (read both first; the tests are in-package, so the loop's
unexported surface is callable):

- A fake backends registry (or the existing fake's registry, if
  `fakes_test.go` provides one that `Kinds()`/`Get()` can be pointed at)
  holding: one backend implementing `PruneLogs` (record the limit it was
  given and a counter), one backend that does not (a plain fake backend).
- `TestTheNativeLogLoopBoundsEveryBackendThatKeepsLogs`: start
  `a.nativeLogLoop(ctx)` on its own goroutine; wait (a channel in the
  recording fake, or `require.Eventually`-style polling written by hand with
  the test clock, never a bare `time.Sleep` longer than needed) until the
  maintainer has been called once; assert the limit it was given is
  `10<<20`; cancel the context; assert the loop returns nil (via the
  goroutine's returned error on a channel).
- `TestTheNativeLogLoopSurvivesABackendThatFailsToPrune`: the maintainer
  returns an error; the loop must still return nil on cancel and must have
  called the non-maintainer backend's `Get` (i.e. it walked all kinds) and
  not panicked.

If the existing fakes make waiting for the first tick awkward (the loop's
first pass is immediate, before the first `select`, so a channel signalled
from the fake works without any clock control), use that: the first pass
runs before any ticking.

**Verify**:

```
go test -race -run 'TestTheNativeLogLoop' ./internal/agent/
```

passes.

### Step 4: Run the full gates

**Verify**:

```
go test -race ./internal/backend/ ./internal/agent/
```

passes, then `make test` exits 0, `gofmt -l internal/backend/ internal/agent/`
returns no output and `go vet` is clean for both packages.

## Test plan

The whole plan is the test plan: the two files above, covering the copy
truncate's bounds (Step 2's cases), the loop's coverage of every backend kind,
its limit and its error tolerance (Step 3). Structural patterns:
`internal/backend/process_files_test.go` for the backend layout in a temp
dir, `internal/agent/daemon_test.go` for the agent scaffolding.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -rn 'PruneLogs' internal/backend/*_test.go internal/agent/*_test.go`
      returns matches (the grep that today returns none).
- [ ] `go test -race ./internal/backend/ ./internal/agent/` exits 0.
- [ ] `make test` exits 0.
- [ ] `git status` shows no modified production files; only the two new test
      files (plus `plans/README.md`).
- [ ] `gofmt -l` clean on both packages.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back (do not improvise) if:

- Writing a case requires changing `process_logs.go` or `native_logs.go`
  (for example a hook or an exported seam); the finding is then "untestable
  as written", which the operator should decide on, not you.
- The `ProcessBackend` constructor or the runner-directory layout in tests
  turns out to be materially different from what `process_files_test.go`
  shows (report what you found and stop).
- The agent's `Options.Backends` registry cannot be populated with a custom
  fake from a test (then the loop needs the scaffolding changed, again an
  operator decision).
- The `os.SameFile` re-check (the file swapped between `Lstat` and open)
  cannot be covered without injection: that specific branch is expected to
  stay uncovered; note it in your report and continue, it is not a STOP on
  its own. It becomes a STOP only if you find yourself adding hooks for it.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- These tests pin the copy-truncate invariants; if `boundLog` ever changes
  strategy (for example to a truncating tail in place), the `.previous` case
  is the one to rewrite, and the comment at `process_logs.go:11-14` is the
  contract to re-read first.
- The loop test's `10<<20` and `10*time.Second` are pinned from
  `native_logs.go`; if the loop's budget becomes configurable, the test
  should follow the setting rather than the constant.
- The deliberately uncovered `os.SameFile` branch (no seam without hooks) is
  recorded here so the next audit does not re-report it.
