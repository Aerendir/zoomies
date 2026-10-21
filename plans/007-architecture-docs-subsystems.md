# Plan 007: Put the newer subsystems into architecture.md, and hold them there

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report; do not improvise. When done, update the status row for this plan in
> `plans/README.md` unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**, in the repo root:

```
git diff --stat a0f21dba..HEAD -- docs/architecture.md internal/docs/ mkdocs.yml
```

> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S-M
- **Risk**: LOW (prose plus one pinning test)
- **Depends on**: none
- **Category**: docs
- **Planned at**: commit `a0f21dba`, 2026-10-19

## Why this matters

The root `AGENTS.md` routes every structural change through
`docs/architecture.md` ("Read docs/architecture.md before changing anything
structural; it explains the shape and the reasons for it"), but the doc stops
at the fleet core: its Components table (lines 304-331) and its closing
sections end at "Verified AI Context snapshots" (line 673), and the packages
that arrived since (`internal/optimisation`, `internal/toolscan`,
`internal/agentguidance`, `internal/prrepair`, the assistant adapters,
`internal/analysis`, `internal/hosttune`, `internal/catalog`,
`internal/registry`) have no architectural rationale anywhere but a one-line
entry in `AGENTS.md`'s layout. So the doc an agent or contributor is told to
read first is quietly wrong about its own coverage, exactly where a wrong
structural change is most likely. This plan adds the missing rows and one
prose section, and pins the coverage with a test so it cannot silently
drift again (no test in `internal/docs` binds architecture.md to the code
today; the dependencies page is the exemplar of the pinning pattern).

## Current state

- `docs/architecture.md` (705 lines). The Components table runs lines 306-331
  and ends with these rows:

```markdown
	| `internal/controller` | Wiring: the reconcile loop, the machine loop, the automatic pool reconciler, webhook ingest, the agent task queue, the log relay. |
	| `internal/agent` | The runner-executing half and its transport to the controller. |
	| `internal/gateway` | `zoomies gateway`: the provider-side end of a private connection. Runs beside a hypervisor the controller cannot reach and forwards the tunnel to that one API. |
	| `internal/installer` | `zoomies init`, `zoomies uninstall`, the GitHub App manifest flow, service installation. |
	| `internal/events` | In-process pub/sub that the SSE endpoint fans out. |
	| `internal/mcp` | The fleet over the Model Context Protocol: the tools, and both transports, stdio for `zoomies mcp`, Streamable HTTP for the controller's `/mcp`. Every tool is a documented REST route called with the caller's own token, so it adds no authority. |
	| `internal/migrate` | Rewriting a workflow's `runs-on` line, and nothing else in the file. |
```

  The headings after it: "The runner state machine", "The machine state
  machine", "Why the agent connects outbound", "Storage", "Security posture",
  "What this is not", "Private agent connections", "Verified AI Context
  snapshots" (the last, lines 673-705).
- `internal/docs/dependencies_test.go` (lines 1-67) is the pinning-pattern
  exemplar: it reads `docs/dependencies.md` and go.mod and fails when a
  direct module has no row. Note its voice: the comment explains why the doc
  must stay honest ("The doc is the place an owner decides whether a module
  still earns its keep").
- Every row in the Components table is one sentence, why-first, in the doc's
  established voice; read rows like `internal/backup` (line 311) for the
  register before writing.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Prepare the build (once, fresh checkout) | `make build-nogui` | exit 0 (writes the UI placeholder; needed before plain `go test`) |
| One test | `go test -run TestArchitecture ./internal/docs/` | pass |
| Package tests | `go test ./internal/docs/` | pass |
| Full suite | `make test` | exit 0 |
| Format check | `gofmt -l internal/docs/` | no output |
| Vet | `go vet ./internal/docs/` | no findings |
| Docs build (optional, needs python3) | `mkdocs build --strict` from the repo root, or the Makefile's docs target if one exists (`grep -n 'docs' Makefile`) | exit 0 |

## Conventions that apply here

- British spelling in prose (behaviour, authorisation). No em dash characters
  anywhere, and never a spaced double hyphen used as a dash.
- The doc's voice: why-first, one sentence per package, no narrating of
  method names. Read the existing rows before writing; match them.
- A new documentation page goes under `docs/` and into `mkdocs.yml`'s nav;
  this plan edits an existing page, so no nav change.
- `internal/docs` tests are prose-checks with a reason-comment; match
  `dependencies_test.go`'s shape.
- Commit messages are imperative sentences in plain prose, sentence case, no
  prefix; for example "Give architecture.md the subsystems it forgot, and a
  test that holds them there".

## Scope

**In scope** (the only files you should modify or create):

- `docs/architecture.md` (the Components table rows and one new prose
  section)
- `internal/docs/architecture_test.go` (create)

**Out of scope** (do NOT touch, even though they look related):

- `mkdocs.yml`: the page is already in nav; no change.
- `AGENTS.md`'s layout table: it is a different document with a different
  job (entry point); it stays.
- `docs/dependencies.md`, `roadmap/`: read for grounding, do not edit.
- The packages themselves: if a draft row proves factually wrong, fix the
  row from the package's code and scoped `AGENTS.md`; never "fix" the code.

## Git workflow

- Branch: `docs/architecture-subsystems`.
- One commit, message: `Give architecture.md the subsystems it forgot, and a
  test that holds them there`.
- Do NOT push or open a pull request unless the operator instructed it.

## Steps

### Step 1: Verify the drafts against the packages

The rows below were drafted against commit `a0f21dba`. Before adding them,
read each package's top doc comment or scoped `AGENTS.md`
(`internal/hosttune/AGENTS.md` exists; the rest are covered by the root
layout) and confirm the one sentence is still true. Where a draft is wrong,
correct it from the code, and note the correction in your report.

### Step 2: Add the rows to the Components table

Append to the table after the `internal/migrate` row (line 331), in this
order, adjusted per Step 1:

```markdown
| `internal/registry` | What a container registry says a tag points at, so the digest-pinning generators can pin what they build; a hand-rolled client, for the reason the Docker one is. |
| `internal/aicontext` | Commit-pinned repository context, independent of the fleet's own jobs: verified snapshots of a repository's files, admitted only after the repository's own checks agree. See [Verified AI Context snapshots](#verified-ai-context-snapshots). |
| `internal/assistant` | The model-provider contract behind the assistant chat and its adapters; the assistant reaches the fleet only through the MCP tools, with the chatting person's identity, so it adds no authority of its own. |
| `internal/prrepair` | The bounds on what a model may edit when it proposes a pull request: an allow-list, blob caps, and a refusal of anything credential-shaped, enforced on the server before a file is written. |
| `internal/toolscan` | Reads workflow files for the toolchains their jobs install, so a pool can be warned about a toolchain it cannot provide. |
| `internal/agentguidance` | Inspects a repository's agent instruction files (AGENTS.md and its neighbours) without executing them. |
| `internal/optimisation` | Reviewable proposals computed from evidence handed in; reads no source and calls no model, so a proposal is reproducible from its inputs. |
| `internal/analysis` | Evidence-backed findings computed from explicit snapshots: why jobs waited, what got worse, whether a host's failures came when it was busy. |
| `internal/hosttune` | Host health and tuning: detection that never writes, and changes only an operator approved. See [Host health](host-health.md). |
| `internal/catalog` | The one machine-readable list of problem codes and Kennel Club checks, shared by the controller, the API and the MCP tools. |
```

Check the two cross-links before adding them: `#verified-ai-context-snapshots`
(the section exists, line 673) and `host-health.md` (grep
`grep -n 'host-health' mkdocs.yml docs/architecture.md` to confirm the page
exists and the doc already references it; if the doc already links it
elsewhere, match that link's form). If a link target does not exist, drop
the link sentence, do not invent one.

**Verify**: `mkdocs build --strict` (or the Makefile's docs target) exits 0,
or, if python3 is unavailable in your environment, say so in your report and
rely on the Step 4 test plus review.

### Step 3: Add one prose section for the model-facing half

After "Verified AI Context snapshots" (the file's last section, ending at
line 705), add one section, `## The model-facing subsystems and their
bounds`, of two paragraphs, this content corrected per your Step 1 reading:

- Paragraph one: the assistant, PR repair, AI context and optimisation
  subsystems all follow the rule the kennel loop set (its paragraph at
  architecture.md lines 333-342 is the pattern to point at): each runs
  outside the reconcile lock, in a bounded loop of its own, because model
  work and GitHub reads are slow and nothing about scaling should wait with
  them. Each is bounded at its edges before anything is written: redaction
  strips credentials from text bound for a model (`internal/redact`, say
  it), and `internal/prrepair` enforces its allow-list server-side.
- Paragraph two: none of them adds authority. The assistant reaches the
  fleet through the MCP tools with the chatting person's identity; MCP itself
  re-enters the router with the caller's own token (`internal/mcp`'s row
  already says this), so the model-facing half inherits, and cannot exceed,
  what the person driving it could do with their own token.

**Verify**: re-read the section aloud against the doc's existing prose; then
`mkdocs build --strict` as in Step 2.

### Step 4: Pin the coverage with a test

Create `internal/docs/architecture_test.go`, package `docs`, modelled on
`dependencies_test.go`:

```go
// docs/architecture.md is the document AGENTS.md sends every structural
// change to first, and until this test nothing held it to the packages the
// tree actually carries: it stopped at the fleet core, and the assistant,
// repair and analysis subsystems had no architectural rationale anywhere an
// agent was sent to look. The fixed list below is deliberate: every package
// in it is one a structural change is likely to touch wrongly without the
// doc's picture, and a package joining this list is a reviewer's call, made
// in the change that adds the package, not silently by a glob.
func TestArchitectureNamesThePackagesAStructuralChangeTouches(t *testing.T) {
	body, err := os.ReadFile("../../docs/architecture.md")
	// ... fail on err
	for _, pkg := range []string{
		"internal/registry", "internal/aicontext", "internal/assistant",
		"internal/prrepair", "internal/toolscan", "internal/agentguidance",
		"internal/optimisation", "internal/analysis", "internal/hosttune",
		"internal/catalog",
		// the fleet core stays pinned too, so a rewrite cannot drop it:
		"internal/store", "internal/scheduler", "internal/controller",
		"internal/agent", "internal/api", "internal/events", "internal/backend",
	} {
		if !bytes.Contains(body, []byte("`"+pkg+"`")) {
			t.Errorf("docs/architecture.md no longer names %s; the document AGENTS.md sends structural changes to first has to cover it", pkg)
		}
	}
}
```

**Verify**: `go test -run TestArchitecture ./internal/docs/` passes.

### Step 5: Full gates

**Verify**: `make test` exits 0; `gofmt -l internal/docs/` clean;
`go vet ./internal/docs/` clean.

## Test plan

The one test of Step 4, modelled structurally on
`internal/docs/dependencies_test.go` (its reason-comment and its
read-and-fail shape). No other new tests; this is a docs change.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go test -run TestArchitecture ./internal/docs/` passes.
- [ ] `grep -c 'internal/optimisation\|internal/prrepair\|internal/toolscan\|internal/agentguidance' docs/architecture.md`
      returns 4 (each named at least once).
- [ ] `make test` exits 0.
- [ ] `mkdocs build --strict` exits 0 (or python3 was unavailable and the
      report says so).
- [ ] No files outside the in-scope list are modified (`git status`).
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back (do not improvise) if:

- A draft row is factually wrong in a way a one-line correction cannot fix
  (for example a package has been renamed, merged or split since this plan
  was written).
- The doc's structure has changed (the Components table moved, the closing
  section is no longer "Verified AI Context snapshots").
- `mkdocs build --strict` fails on something this change introduced that is
  not a typo (a bad anchor, a broken cross-link you cannot resolve by
  dropping the link sentence).
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- The fixed package list in the test is deliberate (see its comment): extend
  it in the same change that adds a structurally significant package, never
  by a glob over `internal/` (half the directories there are one-file
  helpers a structural change is unlikely to touch wrongly).
- When Plan 009 (if executed) moves a subsystem out of
  `internal/controller`, the `internal/controller` row and the kennel
  paragraph (architecture.md lines 333-342) are the two places to update in
  the same change; this test will not catch prose drift, only absence.
- A reviewer should check the two paragraphs of Step 3 hardest: they state
  security posture, and a wrong sentence there is worse than a missing one.
