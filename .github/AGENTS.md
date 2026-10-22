# AGENTS.md

Scoped guidance for CI and generated files, read in addition to the root [AGENTS.md](../AGENTS.md). It appends to that file and never overrides it.

## Things CI will fail you on

Beyond tests and lint, several files must stay in sync with their sources. Each
has a CI job that diffs them:

* `internal/api/openapi_spec.go` is generated from `api/openapi.yaml`, and
  `web/src/lib/api/schema.d.ts` from the same spec. After editing the spec, run
  `make openapi`, which writes both, and commit the result. (`go run
  internal/api/gen_openapi.go` alone writes only the first, and a stale second
  fails the `ui` job.)
* `internal/catalog/catalog.json` and the check list on `docs/kennel-club.md`
  are generated from `docs/problem-codes.md` and the Kennel registry by
  `make generate`; `TestTheCatalogIsCurrent` fails on a stale copy.
* The runner image catalogue in `internal/naming/images.go` is the source for
  both workflows' build matrices, the Makefile's `variant.*` rows, the table in
  `docs/naming.md` and the default base in `deploy/Dockerfile.runner`. Adding or
  swapping an operating system is a row there and then `make generate`; editing
  any of the five by hand fails a test in `internal/naming`. The base-image
  digests in those rows are moved weekly by `runner-bases.yml`, which opens a
  pull request (`make runner-bases` does the same by hand): Dependabot cannot
  see them, and five of seven had aged.
* `install.sh` at the repo root is copied verbatim to the site root; the script
  people `curl` is the script a contributor edits. Do not create a second copy.
  `install.ps1` is published the same way and is held to the same rule; the
  Windows job in `ci.yml` lints it, runs its Pester tests and installs it for
  real (`test/install/`).
* The app shell must stay under **200 KB gzipped** (`web/vite.config.ts`
  enforces it, the number is documented in `docs/ui-guidelines.md`). Route chunks
  are excluded, so move weight to a lazily loaded route rather than raising the
  budget.
* The full-release tag pattern in `release.yml` (`FULL_RELEASE_TAG`) must equal
  `updates.ValidTag`, which is the only tag shape `zoomies upgrade` offers. A
  test in `internal/docs` compares them, so changing one alone fails it; see
  [RELEASING.md](../RELEASING.md).
* `go mod tidy` must leave `go.mod`/`go.sum` unchanged, and `gofmt -l` must be
  empty.
* `govulncheck ./...` must find nothing reachable. It runs in its own workflow,
  on every change and weekly against `main`, so a module found vulnerable after
  it merged still gets reported.
* `.github/workflows/zoomies-ai-context.yml` and `.github/zoomies-ai-context/`
  belong to Zoomies AI Context, which verifies them byte for byte against
  `internal/aicontext`'s templates. Do not edit them by hand, and do not point
  Dependabot at them: a change is read as somebody's edit and closes the
  context's access gate until the bot reinstalls the file. A test in
  `internal/docs` keeps Dependabot away. Their pins move when a release changes
  the template, which needs the previous template kept as a recognised
  generation (`olderGenerations` in `workflow.go`, and `olderGeneratorLocks`
  there for the lockfile, which a dependency fix changes without a new
  template). Because Dependabot does not touch them, a CI step asks GitHub
  which release each action is at and fails when the template is behind: the
  pins it writes are the ones it knows, and a repository that took an update
  of its own would otherwise be moved back by the next reinstall. Taking a new
  version means keeping the one it replaces in `olderActionPins`, so the
  repositories that installed it are still recognised.
* The Go CI and releases use is written once, in `.github/actions/setup-go`, and
  every job calls that action; `go.mod` holds the lower floor a contributor
  needs and gets no `toolchain` line. The UI's Node major is `NODE_VERSION` in
  `ci.yml` and `release.yml`, `.nvmrc` and the `ui` stage of `deploy/Dockerfile`,
  changed together. `internal/docs` tests all of it, so one cannot move alone.
* Every `uses:` in `.github/workflows` is pinned to a commit with its release
  in a comment, and a workflow with more than one job grants no `write`
  permission at the top. Both are tested in `internal/docs`; Dependabot moves a
  pin and its comment together, and a new action gets the same treatment.
* A workflow that runs on `pull_request` lists `closed` among its types, groups
  its concurrency by pull-request number with `cancel-in-progress` true for
  pull requests only, and leads every job's condition with
  `github.event.action != 'closed'`, inside parentheses around any `||` the
  condition already had. Closing or merging a pull request starts a run whose
  only purpose is to take over the group and cancel the one in flight, so
  nothing may start for it. `internal/docs` tests all of it;
  `cancel-merged-checks.yml` is the one exemption, because it is the closed
  handler and a guard on its job would switch it off.
* `mkdocs build --strict`: a docs link that points nowhere fails the build. The
  site workflow also checks that `sitemap.xml` and `llms.txt` came out of it,
  both generated (by `overrides/sitemap.xml` and `hooks/seo.py`) rather than
  written, so a build that quietly stopped producing one would otherwise ship,
  and that the header names a release: `hooks/source.py` resolves the latest
  release, stars and forks from the GitHub API at build time, and an offline
  build is allowed to go without them where the published site is not.
* `ci-gate` in `ci.yml` is the one status check branch protection requires on
  `main`. It `needs` every other job in that file and fails if any failed or
  was cancelled; a skipped job counts as passed, because the `changes` filter
  skipped it on purpose. A new job goes into its `needs` list, and a test in
  `internal/docs` fails when one is left out: a job outside the gate can fail
  without blocking a merge.
* Every job in every workflow has a `timeout-minutes` at about two to three
  times the longest successful run, and a workflow file is named in lower-case
  words joined by hyphens. Both are tested in `internal/docs`: a hung job holds
  a fleet slot for GitHub's six-hour default, and a space in a name makes the
  file the one everything has to quote.
