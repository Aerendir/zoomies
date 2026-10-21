# Plan 008: Record the tailscale.com pin's channel policy, where the pin is defended

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**, in the repo root:

```
git diff --stat a0f21dba..HEAD -- go.mod docs/dependencies.md internal/docs/dependencies_test.go
```

> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW (one doc row, one comment, one test)
- **Depends on**: none
- **Category**: dependencies (policy, not a version change)
- **Planned at**: commit `a0f21dba`, 2026-10-19

## Why this matters

`tailscale.com v1.105.0-pre` is a *direct* requirement on an unstable
pre-release channel, and it drags roughly 130 indirect modules into the
graph (go.mod lines 28-159), including heavyweight ones (`gvisor.dev/gvisor`,
fourteen `aws-sdk-go-v2` modules) and even staticcheck's own module. The CVE
pipelines (govulncheck, osv-scanner, CodeQL) catch vulnerabilities, but
nothing catches a channel drift, a silently grown graph, or a tag Tailscale
never promotes; and the repo's own rule (root `AGENTS.md`, "Dependencies":
review "what does it pull in transitively") is impractical to apply to a
130-module delta that arrives unannounced in a bump. The dependency earns its
place (non-test code imports only `tailscale.com/tailcfg`, and the tests need
the DERP relay), but the *policy* for riding the pre channel is recorded
nowhere. This plan records it where the pin is defended, and adds a test so
the policy cannot silently vanish. It deliberately does not change any
version: moving to a stable tag is an owner's decision with real
compatibility risk, recorded here as a follow-up option, not taken.

## Current state

- `go.mod:25` (inside the direct require block, lines 9-26):

```go
	require (
		github.com/bradleyfalzon/ghinstallation/v2 v2.19.0
		// ... elided ...
		github.com/tailscale/tailcat v0.7.0
		golang.org/x/crypto v0.58.0
		golang.org/x/oauth2 v0.37.0
		golang.org/x/sys v0.49.0
		golang.org/x/term v0.47.0
		gopkg.in/yaml.v3 v3.0.1
		modernc.org/sqlite v1.60.1
		tailscale.com v1.105.0-pre
	)
```

  The go.mod preamble (lines 5-7) already carries one comment in this style
  (the `ignore ./web/node_modules` rationale), so a comment here matches the
  file's own convention.
- `docs/dependencies.md` (166 lines), the Go table's two relevant rows
  (lines 20-21):

```markdown
	| `github.com/tailscale/tailcat` | Built-in encrypted private transport for agents and, through `zoomies gateway`, for providers on a home network; NAT traversal and relay fallback without installing or supervising a separate tunnel process. Pinned in `go.mod`, which is the one place its version is written; requires Go 1.27.1. |
	| `tailscale.com` | Tailcat’s network and key types, plus a local DERP test relay to verify real tunnel behaviour without hosted infrastructure. |
```

- Non-test usage (verified at `a0f21dba`): `internal/api/tailcat.go:20` and
  `internal/gateway/gateway.go:37` import only `tailscale.com/tailcfg`. Test
  usage: `internal/api/tailcat_relay_test.go`, `internal/api/tailcat_test.go`,
  `internal/gateway/gateway_test.go`,
  `internal/controller/machines_tailcat_recovery_test.go` (the DERP relay and
  tailcfg test helpers).
- `internal/docs/dependencies_test.go` (lines 1-118) already holds go.mod to
  the doc (every direct module needs a row); the new test extends that file's
  job to the one row whose policy is otherwise unwritten.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Prepare the build (once, fresh checkout) | `make build-nogui` | exit 0; writes the UI placeholder `internal/api` embeds, without which plain `go test` fails |
| One test | `go test -run TestTheTailscale ./internal/docs/` | pass |
| Package tests | `go test ./internal/docs/` | pass |
| Compile check after the go.mod comment | `go build ./internal/gateway/ ./internal/api/` | exit 0 (a comment cannot break it; this is the belt to the braces) |
| Full suite | `make test` | exit 0 |
| Format check | `gofmt -l internal/docs/` | no output |
| Vet | `go vet ./internal/docs/` | no findings |

## Conventions that apply here

- British spelling in prose. No em dash characters anywhere, and never a
  spaced double hyphen used as a dash.
- `docs/dependencies.md` rows carry *why*, not *what*, one line each; the
  tailcat row (line 20) is the exemplar of the register.
- `internal/docs` tests are prose-checks with a reason-comment; match
  `dependencies_test.go`'s shape exactly.
- Commit messages are imperative sentences in plain prose, sentence case, no
  prefix; for example "Say which channel the tailscale pin rides, and hold
  the doc to it".

## Scope

**In scope** (the only files you should modify):

- `docs/dependencies.md` (rewrite the one `tailscale.com` row)
- `go.mod` (add the comment above the require block; no version change)
- `internal/docs/dependencies_test.go` (add one test; or a new
  `internal/docs/tailscale_pin_test.go` if extending the existing file
  fights its helpers)

**Out of scope** (do NOT touch):

- The `tailscale.com` or `tailcat` versions themselves: moving to a stable
  tag is an owner decision (see Maintenance notes); a plan that changed the
  pin would need the DERP relay and tailcfg compatibility verified against
  the real tag, which is not this plan.
- `go.sum`: no version change means no sum change; if any command you run
  rewrites `go.sum`, that is a STOP condition.
- `internal/api/tailcat.go`, `internal/gateway/gateway.go`: read-only
  evidence.

## Git workflow

- Branch: `docs/tailscale-channel-policy`.
- One commit, message: `Say which channel the tailscale pin rides, and hold
  the doc to it`.
- Do NOT push or open a pull request unless the operator instructed it.

## Steps

### Step 1: Rewrite the row

Replace the `tailscale.com` row at `docs/dependencies.md:21` with (correct
the two file references first if the imports have moved; verify with
`grep -rn '"tailscale.com/tailcfg"' --include='*.go' internal/ | grep -v _test.go`):

```markdown
| `tailscale.com` | Tailcat’s network and key types, plus a local DERP test relay that verifies real tunnel behaviour without hosted infrastructure. Non-test code imports only `tailscale.com/tailcfg` (`internal/api/tailcat.go`, `internal/gateway/gateway.go`), yet the pin carries about a hundred and thirty indirect modules with it, so it is the one dependency whose bumps are reviewed as a graph, not a module: a bump that changes what it pulls in says so in its commit. The pin rides the pre-release channel `tailcat` tracks, at or above the version `tailcat` itself requires, so the types cannot drift from the transport that uses them; govulncheck and osv-scanner catch vulnerabilities, not a channel that quietly moved. |
```

### Step 2: Point at the policy from the pin itself

In `go.mod`, above the `require (` block that holds `tailscale.com`
(immediately before line 9), add:

```go
	// tailscale.com rides the pre-release channel tailcat tracks, at or above
	// the version tailcat itself requires; the rule, and why the pin carries a
	// graph with it, is in docs/dependencies.md.
```

go.mod tolerates comments anywhere a line can go; `gofmt` does not reflow
go.mod, and `go mod tidy` is not run (out of scope; see STOP conditions).

**Verify**: `go build ./internal/gateway/ ./internal/api/` exits 0, and
`git diff go.sum` is empty.

### Step 3: Hold the row to the policy

Add one test to `internal/docs/dependencies_test.go` (reusing the file's
`section` helper if that is its shape; read the file first), named
`TestTheTailscalePinNamesItsChannel`:

- Read `go.mod`; find the `tailscale.com` requirement; extract its version
  with the file's existing `requirement` regexp (lines 24-29).
- If the version contains `-pre` (the pin rides the unstable channel), the
  `tailscale.com` row in `docs/dependencies.md` must contain both the word
  "pre-release" and the sentence fragment "changes what it pulls in says so"
  (the policy, not the exact wording: assert on those two markers, and say
  in the test's comment that the row may be reworded as long as it keeps
  naming the channel and the graph rule).
- If the version is a stable tag (a future owner decision), the test passes
  unconditionally: the policy it pins exists only while the unstable
  channel is the one being ridden.

Carry the file's reason-comment voice: why the policy must stay written (a
bump's graph delta is invisible to the CVE pipelines; the row is the one
place an owner defends it).

**Verify**: `go test -run TestTheTailscale ./internal/docs/` passes.

### Step 4: Full gates

**Verify**: `make test` exits 0; `gofmt -l internal/docs/` clean;
`go vet ./internal/docs/` clean.

## Test plan

The one test of Step 3, modelled on `internal/docs/dependencies_test.go`'s
existing shape (its regexps, its read-and-fail style, its reason-comments).
No other new tests; this change is prose, a comment, and their pin.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -c 'pre-release' docs/dependencies.md` is at least 1 (the row
      names the channel).
- [ ] `go test -run TestTheTailscale ./internal/docs/` passes.
- [ ] `git diff go.sum` is empty (no version moved).
- [ ] `make test` exits 0.
- [ ] No files outside the in-scope list are modified (`git status`).
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back (do not improvise) if:

- The `tailscale.com` require line has moved out of a block (a single-line
  require; then place the comment above that line instead, and say so in
  your report).
- The non-test imports of `tailscale.com/...` have grown beyond the two
  `tailcfg` sites (the row's claim would then be wrong; report the sites).
- Any command rewrites `go.sum` or `go.mod`'s require lines (a tool
  "helpfully" tidying; revert it and report which command did it).
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- The stable-tag option is deliberately not taken: it is an owner decision
  that needs the DERP test relay and tailcfg compatibility verified against a
  real stable tag. If taken, Step 3's test self-disables (the policy only
  applies while the pre channel is ridden), which is the point.
- Every future bump of this pin should show its graph delta in the commit
  message (`git diff go.sum` before and after is the quick check); the row
  now says so, and the reviewer's expectation follows the row.
- If `tailcat` ever moves to requiring stable `tailscale.com` tags, the row
  and the go.mod comment both need rewriting in that change; the test will
  not force it (it passes on stable), so the maintenance note here is the
  reminder.
