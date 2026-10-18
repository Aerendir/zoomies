# Eli repair workspace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every Eli repair a page that says what it is fixing: a repair page for one failure, an attempt page for one repair, a hub, and a setup page, backed by additive API changes.

**Architecture:** Readiness returns the pull request and a per-check list; a repair keeps a small `case` snapshot in its existing `details` JSON and can be read by id; error output is fetched on demand and never stored. The UI splits the 600-line composite into four lazily loaded routes built from small components over pure, unit-tested helpers.

**Tech Stack:** Go 1.27 (store, controller, API), SQLite through `internal/store` only, Svelte 5 runes, TypeScript, Vite, Playwright, OpenAPI 3.1 with generated Go and TypeScript copies.

**Spec:** [2026-10-18-eli-repair-workspace.md](2026-10-18-eli-repair-workspace.md)

## Global Constraints

* Go 1.27.1 or later; Node 22 or later. No new dependency, Go or npm (`docs/dependencies.md`).
* SQL is written in `internal/store` only. No migration: the snapshot lives in the existing `eli_repairs.details` JSON.
* After editing `api/openapi.yaml` run `make openapi` and commit both generated files (`internal/api/openapi_spec.go`, `web/src/lib/api/schema.d.ts`). A new or changed route also updates the route and role table in `internal/api/api_test.go` and `docs/api-surface.md` in the same task.
* Prose, comments, docs, error messages and UI strings: British spelling; no em dashes; no spaced `--`.
* UI: tokens only, never a raw colour or a value that already has a token; the six status hues keep their fixed meanings and every status comes from `web/src/lib/status.ts`; no state-management library, charting library or router change beyond adding routes; the app shell stays under 200 KB gzipped, so every new page is a lazily loaded route.
* Nothing is reachable from the UI that is not on a documented REST route.
* A new component state is added to the gallery (`web/src/gallery/`) before a route reaches it.
* Readiness never calls a model. Starting a repair is only ever the press of **Fix with Eli**.
* Everything a workflow controls (job, step, workflow and branch names, error lines) and every pull request title is untrusted: rendered as text, and a link only when its scheme is `http` or `https`.
* Tests: standard-library `testing`, table-driven, `:memory:` stores, injected clocks, the fake GitHub; test names are sentences about behaviour. UI unit fixtures are built to the generated types, never cast.
* Commit messages: an imperative sentence in plain prose, sentence case, no prefix. Each commit ends with the two attribution lines the session was given.

## Review Focus

* A repair written before this change (no `details.case`) opens on the attempt page and lists in history without error. Owned by Task 1 (store) and Task 9 (page).
* A job name, step name, pull request title or error line holding markup or a `javascript:` address renders as text and never becomes a link. Owned by Task 6 (links, case), Task 8 (page spec) and Task 12 (hostile-input spec).
* A run whose Zoomies job records were pruned or never seen still gives a repair page that explains itself. Owned by Task 6 (case from live data) and Task 8 (page).
* A pull request that closes or is superseded while the page is open replaces the card with the explanation and offers no start. Owned by Task 8.
* A phone-width page never scrolls sideways with a very long branch name, test name or error line. Owned by Task 8 (repair page), Task 9 (attempt page) and Task 12 (every new route).
* An Enterprise Server job address yields repository, run and commit links on the same host. Owned by Task 6.
* The legacy `/repairs?repo=&run=` address still lands on the repair page. Owned by Task 8.

---

## Stage 1: Data

### Task 0: Prepare the tree

**Files:** none changed.

- [ ] **Step 1:** `make build-nogui` (writes the web placeholder that every plain `go` command needs).
- [ ] **Step 2:** `cd web && npm ci`.
- [ ] **Step 3: Baseline.** `go test -count=1 ./internal/store ./internal/github ./internal/controller ./internal/api` and `cd web && npm run check && npm run test:unit`. Expected: all PASS. If anything fails here it is not ours; note it and carry on.
- [ ] **Step 4:** `export PLAYWRIGHT_CHROMIUM=/opt/pw-browsers/chromium-1194/chrome-linux/chrome` for every Playwright run in this plan.

### Task 1: The store keeps a case and reads one repair by id

**Files:**
- Modify: `internal/store/queries_eli_repairs.go`
- Test: `internal/store/eli_repairs_test.go`

**Interfaces:**
- Produces: `type EliRepairCaseJob struct{ Name, Step, URL string; DurationMS int64 }` and `type EliRepairCase struct{ PullTitle, PullURL, Author, Branch, RunURL, Workflow string; RunNumber int64; Jobs []EliRepairCaseJob }` with the JSON names in the spec (`pull_title`, `pull_url`, `author`, `branch`, `run_url`, `workflow`, `run_number`, `jobs`; each job `name`, `step`, `url`, `duration_ms`; all `omitempty`); `EliRepairDetails.Case *EliRepairCase` (`json:"case,omitempty"`); `const MaxEliRepairCaseJobs = 10`; `func (s *Store) GetEliRepair(ctx context.Context, id, ownerID string, scoped bool) (*EliRepair, error)`.

- [ ] **Step 1: Write the failing tests** in `eli_repairs_test.go`:
  * `TestARepairKeepsItsCaseThroughEveryWrite`: enqueue a row with `Details.Case` set (title, one job), change `State` and `Details.Stage`, `SaveEliRepair`, `GetEliRepair(id, "", false)`; assert the case is equal.
  * `TestARepairWrittenBeforeCasesExistStillReads`: enqueue with a nil case, read it back; assert `Details.Case == nil` and no error.
  * `TestGetEliRepairScopesLikeTheHistory`: owner reads own row; another user's `GetEliRepair` is `ErrNotFound`; `scoped=false` reads any; `scoped=true` with `ownerID=""` reads nothing even for an automatic row (empty owner); a missing id is `ErrNotFound`; a row cleared with `ClearEliRepairs` is still returned.
- [ ] **Step 2:** `go test -count=1 ./internal/store -run 'ARepair|GetEliRepair'`. Expected: FAIL (undefined `Case`, `GetEliRepair`).
- [ ] **Step 3: Implement** the types, the `Case` field and `GetEliRepair` (one `SELECT eliRepairCols FROM eli_repairs WHERE id=?`; `sql.ErrNoRows` and a scope miss, `scoped && (UserID == "" || UserID != ownerID)`, both become `ErrNotFound`, so the two cannot be told apart).
- [ ] **Step 4:** the same command. Expected: PASS.
- [ ] **Step 5: Commit** "Let a repair keep a case and be read by its id".

### Task 2: The GitHub client reads the base branch

**Files:**
- Modify: `internal/github/eli_repairs.go`
- Test: `internal/github/eli_repairs_test.go`

**Interfaces:** Produces `RepairPull.Base string` (GitHub's `base.ref`).

- [ ] **Step 1: Failing test** `TestRepairPullReadsTitleAuthorBranchAndBase` using `repairTestClient`: serve `/repos/acme/repo/pulls/1` with `title`, `html_url`, `user.login`, `head.ref`, `base.ref:"main"`; assert `Title`, `URL`, `Author`, `Branch`, `Base == "main"`.
- [ ] **Step 2:** `go test -count=1 ./internal/github -run TestRepairPullReads`. Expected: FAIL (`Base` undefined).
- [ ] **Step 3:** Add `Base` to `RepairPull` and decode `base.ref` in `RepairPull()`.
- [ ] **Step 4:** Same command. Expected: PASS.
- [ ] **Step 5: Commit** "Read the base branch of a pull request Eli repairs".

### Task 3: Readiness says what it found

**Files:**
- Modify: `internal/controller/eli_experience.go`, `api/openapi.yaml`, `docs/api-surface.md`, `web/tests/repair-setup.spec.ts` (its readiness fixture)
- Regenerated: `internal/api/openapi_spec.go`, `web/src/lib/api/schema.d.ts`
- Test: `internal/controller/eli_experience_test.go`, `internal/api/eli_repairs_setup_test.go`

**Interfaces:**
- Produces in `controller`: `type EliReadinessPull struct{ Number int; Title, Author, Branch, Base, URL string; Open bool }` (JSON `number,title,author,branch,base,url,open`); `type EliCheck struct{ Code, Status, Message string }` (JSON `code,status,message`); consts `EliCheckOK = "ok"`, `EliCheckBlocked = "blocked"`, `EliCheckUnchecked = "unchecked"`; `EliReadiness` gains `Pull *EliReadinessPull` (`json:"pull,omitempty"`) and `Checks []EliCheck` (`json:"checks"`), always nine entries in this order of `Code`: `identity, provider, installation, access, policy, budget, pull, checks, permissions`.
- Consumes: Task 2's `RepairPull.Base`.

Mapping from the existing blockers to checks (a check not reached stays `unchecked` with the message "Not checked yet."; a blocker marks its check `blocked` with the blocker's message): `identity`→identity; `provider`→provider; `installation`→installation; `github` raised for an unavailable or unsupported client→installation; `access`→access; `policy`→policy; `budget`, `budget_unavailable`→budget; `pull`, `closed`, `superseded` and a `github` raised while resolving the pull request→pull; `fixed`, `checks_pending` and a `github` raised while reading checks→checks; `permissions`→permissions. An `ok` check carries a one-sentence finding, for example for `access`: "`login` can write to `repo`."

- [ ] **Step 1: Write the failing tests** (use `repairFake` and `h.c.clients.entries` as `TestReadinessExplainsAClosedPRAndDistinguishesGitHubFailure` does; extend `repairFake.RepairPull` with settable `title`, `author`, `url`, `base` fields):
  * `TestReadinessReturnsThePullRequestAndEveryCheckWhenReady`: all nine checks `ok`, `Pull` set with title, branch, base, url, `Open`, `Ready`.
  * `TestReadinessStopsAtWriteAccessAndLeavesTheRestUnchecked`: `repairFake.noWrite`; checks 0 to 2 `ok`, `access` `blocked` with the blocker's message, checks 4 to 8 `unchecked`, and `Pull == nil` (the privacy rule).
  * `TestReadinessWithNoLinkedAccountBlocksIdentityAndStillChecksProvider`: `identity` blocked, `provider` evaluated, `access` onward `unchecked`, `Pull == nil`.
  * `TestReadinessKeepsThePullRequestWhenItIsClosed`: a closed pull request gives `Pull` with its title, `pull` check `blocked`, blocker code `closed`.
  * `TestReadinessBlockersAreUnchangedByChecks`: the existing blocker codes and messages for the cases above equal what the current tests expect.
  * In `internal/api/eli_repairs_setup_test.go`: `TestEliReadinessAlwaysReturnsNineChecks`, decoding `checks` from the HTTP answer.
- [ ] **Step 2:** `go test -count=1 ./internal/controller ./internal/api -run 'Readiness'`. Expected: FAIL.
- [ ] **Step 3: Implement** by building the check list alongside the existing flow in `EliReadiness`: a small local `mark(code, status, message)` that records a check, with `block(code, message)` calling it for the matching check through the mapping above. Set `out.Pull` once the pull request resolves, including the closed and superseded errors where `pr.Number > 0`. Keep every existing early return; after returning, fill any unset check as `unchecked` in one place (`finishChecks`).
- [ ] **Step 4: Contract.** In `api/openapi.yaml` add schemas `EliReadinessPull` and `EliReadinessCheck` (`status` enum `ok, blocked, unchecked`), add `pull` (optional) and `checks` (required array) to `EliReadiness`, describe that `pull` is present only after write access is verified. Run `make openapi`. Update the readiness row in `docs/api-surface.md`. Fix any UI fixture that now fails `cd web && npm run check` by adding `checks: []` (`web/tests/repair-setup.spec.ts`).
- [ ] **Step 5:** `go test -count=1 ./internal/controller ./internal/api` and `cd web && npm run check`. Expected: PASS.
- [ ] **Step 6: Commit** "Make readiness say what it found: the pull request and every check".

### Task 4: A repair remembers its run and its case

**Files:**
- Modify: `internal/controller/eli_repairs.go`, `internal/controller/eli_experience.go`, `internal/api/handlers_eli_repairs.go`, `internal/store/queries_events.go` (`JobFilter.HeadSHA`), `api/openapi.yaml`, `docs/api-surface.md`
- Regenerated: `internal/api/openapi_spec.go`, `web/src/lib/api/schema.d.ts`
- Test: `internal/controller/eli_repairs_test.go`, `internal/store` job filter test, `internal/api/eli_repair_history_test.go`

**Interfaces:**
- Produces: `func (c *Controller) repairCase(ctx context.Context, repo string, pr github.RepairPull, runID int64) *store.EliRepairCase`; `func runURLFromJobURL(jobURL string) string`; `store.JobFilter.HeadSHA string` (equality on `head_sha`); `RequestPinnedEliRepair(ctx, userID, repo string, pull int, instruction, expectedHead string, runID int64, requesterAdmin bool) (*store.EliRepair, error)` (a new `runID` parameter, added to the unexported `requestPinnedEliRepair` as well; `RequestEliRepair` passes `0`); the request body of `POST /assistant/repairs` takes optional `run_id`.

`repairCase` rules: the failed jobs (conclusion `failure` or `timed_out`) of the run when `runID > 0`, otherwise of the repository at `pr.HeadSHA`; at most `store.MaxEliRepairCaseJobs`; each job `Name` from `JobName`, `Step` from `Job.FailedStep().Name`, `URL` from `HTMLURL`, `DurationMS`; `Workflow` and `RunNumber` from the first job; `RunURL` from `runURLFromJobURL` (trim a trailing `/job/<digits>`); `PullTitle`, `PullURL`, `Author`, `Branch` from `pr`. Strings are clipped (names and titles to 200 runes, URLs to 500 characters) and a URL whose scheme is not `http` or `https` is dropped.

- [ ] **Step 1: Failing tests:**
  * `store`: `TestJobFilterNarrowsToAHeadCommit`.
  * `controller`: `TestARepairCaseNamesTheFailedJobsAndTheirSteps` (seed two failed jobs and one passing job for a run with steps; assert two jobs, the failing step names, the run URL without `/job/`, the workflow and run number); `TestARepairCaseFindsJobsByHeadWhenThereIsNoRun`; `TestARepairCaseSurvivesAPrunedRun` (no job rows: pull request facts only, no error); `TestARepairCaseClipsAndRefusesUnsafeAddresses` (a `javascript:` job URL and a 5 000 character title); `TestAUIRepairRecordsItsRunAndCase` (call `RequestPinnedEliRepair` with a run id; the stored row has `RunID` and `Details.Case`); `TestAcceptingAnOfferRecordsTheOffersRun`; `TestTheWorkerFillsAMissingCase` (a mention-triggered row with no case gains one when the worker resolves the pull request).
  * `api`: `TestRequestingARepairAcceptsARunID`.
- [ ] **Step 2:** `go test -count=1 ./internal/store ./internal/controller ./internal/api -run 'Case|JobFilter|RunID|Offer'`. Expected: FAIL.
- [ ] **Step 3: Implement** `repairCase` and `runURLFromJobURL`; set `Details.Case` and `RunID` in `requestPinnedEliRepair`; pass `o.RunID` from `ActEliOpportunity`; in `runEliRepair`, after `r.HeadSHA = pull.HeadSHA`, `if r.Details.Case == nil { r.Details.Case = c.repairCase(ctx, r.Repo, pull, r.RunID) }` before the existing persist that records stage `evidence`. Add `HeadSHA` to `JobFilter` and its where builder. The handler reads `run_id` into the request struct and passes it on.
- [ ] **Step 4: Contract.** Add `EliRepairCase` and `EliRepairCaseJob` schemas, `case` to `EliRepairDetails`, `run_id` to `EliRepairRequest`; `make openapi`; update the POST row in `docs/api-surface.md`.
- [ ] **Step 5:** `go test -count=1 ./internal/store ./internal/controller ./internal/api` and `cd web && npm run check`. Expected: PASS.
- [ ] **Step 6: Commit** "Record the run and the case a repair was started for".

### Task 5: One repair by id, and error output on demand

**Files:**
- Modify: `internal/controller/eli_experience.go` (or a new `internal/controller/eli_error_output.go`), `internal/api/handlers_eli_repairs.go`, `internal/api/router.go`, `internal/api/api_test.go`, `api/openapi.yaml`, `docs/api-surface.md`, `web/src/lib/api/client.ts`
- Regenerated: `internal/api/openapi_spec.go`, `web/src/lib/api/schema.d.ts`
- Test: `internal/controller/eli_error_output_test.go`, `internal/api/eli_repair_history_test.go`

**Interfaces:**
- Produces: `func (s *Server) handleGetEliRepair`, route `GET /assistant/repairs/{id}` (`getEliRepair`, viewer, `auth.ActionAssistantOwn`); `type EliErrorOutput struct{ Lines []string; Note string }` (JSON `lines`, `note,omitempty`); `var ErrEliErrorOutputDenied error`; `func (c *Controller) EliErrorOutput(ctx context.Context, user, repo string, jobID int64) (EliErrorOutput, error)`; route `GET /assistant/repairs/error-output?repo=&job_id=` (`getEliErrorOutput`, viewer, `auth.ActionAssistantOwn`, personal accounts only like readiness); in `client.ts`: `getEliRepair(id: string, signal?: AbortSignal)` and `getEliErrorOutput(query: Query<'getEliErrorOutput'>, signal?: AbortSignal)`.

`EliErrorOutput` rules: needs a confirmed linked identity and `RepairActor` write access to `repo`, else `ErrEliErrorOutputDenied`; reads through `client.RepairFailures(ctx, repo, github.RepairPull{}, jobID)`; passes every line through `prrepair.Redact`; returns a non-nil `Lines`; an empty result sets `Note` ("GitHub no longer has this log, or the job printed no error lines."); caches by `repo` and `jobID` for one minute on the controller's clock, evicting expired entries and holding at most 256; writes nothing to the store and logs no line text.

- [ ] **Step 1: Failing tests:**
  * `controller`: `TestErrorOutputRefusesAnUnlinkedOrReadOnlyCaller`; `TestErrorOutputRedactsWhatItReturns` (a line with a GitHub token shape comes back redacted); `TestErrorOutputReadsGitHubOncePerMinute` (two calls inside a minute, one `RepairFailures` call; a third after the clock moves 61 seconds, a second); `TestErrorOutputSaysSoWhenTheLogIsGone`.
  * `api`: `TestGetEliRepairAnswersOwnersAdministratorsAndNobodyElse` (owner 200, another viewer 404, administrator 200, the 404 body identical for a missing id); `TestErrorOutputNeedsAJobIDAndARepo` (422 naming the field); `TestErrorOutputIsForbiddenWithoutWriteAccess`.
  * `api_test.go` route table: `GET /api/v1/assistant/repairs/missing` and `GET /api/v1/assistant/repairs/error-output?repo=a/b&job_id=1` (`accountOnly: true`).
- [ ] **Step 2:** `go test -count=1 ./internal/controller ./internal/api -run 'ErrorOutput|GetEliRepair|TestRouteAuthorisation|TestRouteTableCoversTheSpec|TestTheSpecCoversTheRouter|TestEveryOperationRecordsItsRole'`. Expected: FAIL.
- [ ] **Step 3: Implement** the controller function, the two handlers (a 404 with one message for missing and not yours, 403 for denied, 422 for a bad query), the routes, the OpenAPI paths and `EliErrorOutput` schema, the `client.ts` functions.
- [ ] **Step 4:** `make openapi`; `go test -count=1 ./internal/controller ./internal/api ./internal/docs`; `cd web && npm run check`. Expected: PASS.
- [ ] **Step 5: Commit** "Read one repair by id and fetch a job's error output when asked".

---

## Stage 2: The repair page

### Task 6: Pure helpers for links, checks, guidance and status

**Files:**
- Create: `web/src/lib/repairs/links.ts`, `checks.ts`, `guidance.ts`, `case.ts`, `workspace.ts`
- Modify: `web/src/lib/status.ts`
- Test: `web/unit/repair-links.test.ts`, `repair-checks.test.ts`, `repair-guidance.test.ts`, `repair-case.test.ts`, `repair-workspace.test.ts`, `repair-status.test.ts`

**Interfaces:**
- Produces:
  * `links.ts`: `safeHttpUrl(value: string | null | undefined): string | undefined`; `linksFromJobUrl(jobUrl: string | null | undefined): { job?: string; run?: string; repo?: string }`; `commitUrl(repoUrl: string | undefined, sha: string): string | undefined`; `pullUrl(repoUrl: string | undefined, number: number): string | undefined`.
  * `checks.ts`: `type CheckStatus = 'ok' | 'blocked' | 'unchecked'`; `type CheckAction = { kind: 'recheck' } | { kind: 'setup' } | { kind: 'choose' } | { kind: 'link'; href: string; label: string; external?: boolean }`; `interface CheckRow { code: string; label: string; status: CheckStatus; message: string; action?: CheckAction }`; `checkRows(readiness: Schemas['EliReadiness'], repo: string): CheckRow[]`; `blockingRow(rows: readonly CheckRow[]): CheckRow | undefined`.
  * `guidance.ts`: `interface Guidance { headline: string; steps: string[] }`; `guidanceFor(repair: Pick<Schemas['EliRepair'], 'state' | 'details'>): Guidance` with the copy table in the spec.
  * `case.ts`: `interface CaseJob { key: string; name: string; step?: string; stepNumber?: number; url?: string; durationMs?: number; domain: 'workflow' | 'fleet' | ''; faultLabel?: string; fix?: string; opened: boolean; githubJobId?: number }`; `interface CaseView { repo: string; repoUrl?: string; title: string; hasPull: boolean; pullNumber?: number; pullUrl?: string; author?: string; branch?: string; base?: string; sha?: string; commitUrl?: string; runLabel?: string; runUrl?: string; failedAt?: string; ran?: { pool?: string; runner?: string; poolId?: string; runnerId?: string }; jobs: CaseJob[] }`; `caseFromLive(input: { repo: string; jobs: readonly Job[]; readiness: Schemas['EliReadiness'] | null; openedJobId?: string }): CaseView`; `caseFromSnapshot(repair: Schemas['EliRepair']): CaseView`.
  * `workspace.ts`: `type WorkspaceStage = 'checking' | 'blocked' | 'ready' | 'changed' | 'error'`; `interface RepairTarget { repo: string; pull: number; run: number; job: string; offer: string }`; `parseTarget(query: URLSearchParams): RepairTarget`; `targetHref(target: Partial<RepairTarget>): string` (builds `/repairs/new?...`, omitting empty and zero values); `changedBlocker(r: Schemas['EliReadiness']): { code: 'fixed' | 'closed' | 'superseded'; message: string } | undefined`; `stageOf(r: Schemas['EliReadiness'] | null, failed: boolean): WorkspaceStage`.
  * `status.ts`: `repairStatus(state: string): StatusMeta` mapping per the spec table, with a neutral fallback for an unknown state.

- [ ] **Step 1: Write the failing unit tests**, each building fixtures to the generated types:
  * links: a github.com job URL yields job, run and repo; a `https://ghe.example.com/acme/site/actions/runs/9/job/3` URL yields the same host; `javascript:alert(1)`, `data:text/html,x` and `` give `undefined`; a URL that does not match the job pattern gives only `job`; `commitUrl(repo, 'a41c9e2...')` and `pullUrl(repo, 3)` build on the repo address and give `undefined` without one.
  * checks: a readiness with nine `ok` checks gives nine ok rows and no `blockingRow`; the blocked-at-access fixture gives `blockingRow().code === 'access'` with a `link` action to `https://.../settings/access` and a `recheck`; rows for unchecked checks have `status: 'unchecked'`; a readiness from an older server with no `checks` falls back to rows derived from `blockers`; an unknown code keeps its code as its label.
  * guidance: one assertion per row of the spec's table (`unsupported_fix` first step starts "Add a note", `rate_limited` mentions the quota, a `checks_failed` state with no code mentions the commit), and an unknown code falls back to the `unsupported_fix` copy.
  * case: `caseFromLive` with jobs, a fleet-fault job (`fault_domain: 'fleet'`, `fault_kind: 'out_of_memory'`) marked `domain: 'fleet'` with its `fault_fix`, the opened job flagged, steps numbered from `failed_step.number`; with no jobs and no readiness it still returns a titled case; a `<script>` title and a `javascript:` URL come through as text and `undefined`; `caseFromSnapshot` on a row with no `details.case` returns a case titled from `repo` and `pull_number`.
  * workspace: `parseTarget` and `targetHref` round trip; `stageOf` returns `changed` for a `fixed` blocker, `blocked` for any other blocker, `ready` when `ready`, `error` when the read failed.
  * status: each state of the spec table gives its tone and shape; `repairStatus('nope')` is neutral.
- [ ] **Step 2:** `cd web && node --import ./unit/register.mjs --test unit/repair-*.test.ts`. Expected: FAIL (modules missing).
- [ ] **Step 3: Implement** the modules to those signatures. `case.ts` reads `Job` fields `repo`, `workflow`, `job_name`, `failed_step`, `html_url`, `head_branch`, `head_sha`, `run_number`, `run_attempt`, `completed_at`, `pool_id`, `pool_name`, `runner_id`, `runner_name`, `fault_domain`, `fault_kind`, `fault_fix`, `duration_ms`, `github_job_id`, and `faultLabel` from `faultLabel` in `web/src/lib/faults.ts`.
- [ ] **Step 4:** the same command, then `npm run check`. Expected: PASS.
- [ ] **Step 5: Commit** "Add the helpers the repair pages are built from".

### Task 7: The repair page's components, and their gallery states

**Files:**
- Create: `web/src/lib/repairs/CaseCard.svelte`, `FailedJobs.svelte`, `ErrorOutput.svelte`, `ReadinessChecklist.svelte`, `DecisionCard.svelte`, `RepairStatusBadge.svelte`
- Modify: `web/src/gallery/Gallery.svelte` (a "Eli repairs" section)
- Test: `web/tests/gallery.spec.ts` (run through the existing combinations)

**Interfaces:**
- Consumes: Task 6.
- Produces (props, all `$props()` with a typed `Props`): `CaseCard { view: CaseView; failedLabel?: string }`; `FailedJobs { jobs: CaseJob[]; repo: string; canReadLogs: boolean; logsBlockedReason?: string }` (fetches error output itself through `getEliErrorOutput` when a job's disclosure opens and `canReadLogs`); `ErrorOutput { repo: string; jobId: number; enabled: boolean; reason?: string }`; `ReadinessChecklist { rows: CheckRow[]; busy: boolean; onrecheck: () => void; onsetup: () => void; onchoose: () => void }`; `DecisionCard { stage: WorkspaceStage; readiness: Schemas['EliReadiness'] | null; rows: CheckRow[]; busy: boolean; instruction: string; allowInstruction: boolean; onstart: () => void; onrecheck: () => void; onsetup: () => void }` (bindable `instruction`); `RepairStatusBadge { state: string }` (a `Badge` from `repairStatus`).
- Layout follows the approved mockup: a red-railed case card, one decision card, a disclosure per failed job (steps collapsed once error output is available), a checklist that collapses to one line when everything passes.

- [ ] **Step 1: Failing check.** Add the gallery entries for: case card (with and without a pull request), failed jobs (workflow fault, fleet fault, locked error output, loaded error output, an error line of 600 characters), checklist (blocked at access with unchecked rows, all ok), decision card (blocked, ready, changed), status badge for every state. Run `cd web && npx playwright test tests/gallery.spec.ts --project=gallery`. Expected: FAIL (imports missing).
- [ ] **Step 2: Implement** the six components with tokens only, native `<details>` for disclosures, `role="status"` regions for results, text nodes for every untrusted string, `overflow-wrap: anywhere` on names and error lines so nothing scrolls sideways, and links through `safeHttpUrl`.
- [ ] **Step 3:** `cd web && npm run check && npm run lint && npx playwright test tests/gallery.spec.ts --project=gallery`. Expected: PASS (the spec runs each style and mode, including its accessibility pass).
- [ ] **Step 4: Commit** "Add the components for the Eli repair page".

### Task 8: The repair page, `/repairs/new`

**Files:**
- Create: `web/src/routes/EliRepairNew.svelte`, `web/src/lib/repairs/RepairTargetDialog.svelte` (`{ open: boolean (bindable) }`: parses a pasted pull request URL with `parseRepairTarget` and navigates to `targetHref({ repo, pull })`), `web/tests/repair-page.spec.ts`, `web/tests/support/repairs.ts` (a `mockRepairApi(page, options)` helper that stubs `/api/v1/**` the way `repair-setup.spec.ts` does, with the session, meta, jobs, readiness, error-output and repair routes configurable)
- Modify: `web/src/lib/router.ts` (routes `eli-repair-new` `/repairs/new`, title "New repair", listed before `eli-repair`), `web/src/routes/EliRepairs.svelte` (replace `/repairs?repo=&run=|pull=` with `/repairs/new` plus the same query, with `replace: true`), `web/src/lib/overview/RecentFailures.svelte` and `web/src/lib/jobs/JobWhy.svelte` (link to `targetHref({ repo, run, job: job.id })`)
- Test: `web/tests/repair-page.spec.ts`

**Interfaces:**
- Consumes: Tasks 6 and 7, `getEliReadiness`, `listJobs` (`{ repo, run_id: [run], include_steps: true }`), `requestEliRepair` (body gains `run_id`), `actEliOpportunity`, `listEliOpportunities`, `router`.
- Produces: the page described in the spec's "The repair page": reads the target with `parseTarget`; loads jobs and readiness in parallel; derives `caseFromLive`; shows the stage from `stageOf`; on **Fix with Eli** calls `requestEliRepair({ repo, pull_number, head_sha, instruction, run_id })` (or `actEliOpportunity(offer, { action: 'accept' })` for an offer), then `router.navigate('/repairs/' + row.id)`; a 409 re-reads readiness and shows the changed explanation; a target with only `pull` or only a pasted URL works; **Repair a different PR** opens a dialog that parses a URL with the existing `parseRepairTarget`.

- [ ] **Step 1: Write the failing Playwright spec** `repair-page.spec.ts` (desktop `chromium` and phone `mobile` projects both run it), each test using `mockRepairApi`:
  * `blocked on access shows the failure, names the jobs and keeps the rest unchecked`: with the run's jobs (two failed, one with a failed step) and a readiness blocked at `access`, assert the case card shows the repository, branch, short commit and run links, both job names with their steps, "Pull request not looked up yet", the checklist rows `Write access` blocked and `Repository policy` reading "Not checked yet", the decision card heading "Fix write access first", and that no `POST /assistant/repairs` happened.
  * `ready shows the pull request, loads error output when the job opens and starts one repair`: readiness ready with `pull`; opening the first job requests `/assistant/repairs/error-output?repo=...&job_id=...` once and shows the lines; pressing **Fix with Eli** posts `run_id` and `pull_number` and ends on `/repairs/rep-1`.
  * `a fleet fault says a repair is unlikely to help`: a job with `fault_domain: 'fleet'` shows the fleet badge and its `fault_fix`.
  * `a closed pull request replaces the card and offers no start`: a `closed` blocker shows "This PR is closed", the run link and no **Fix with Eli** button.
  * `a run with no recorded jobs still explains itself`: the jobs call returns `items: []`; the page shows the repository and run and says Zoomies has no record of the jobs.
  * `the old address still works`: visiting `/repairs?repo=acme/site&run=9` ends on `/repairs/new?repo=acme%2Fsite&run=9`.
  * `hostile text renders as text`: a pull request title of `<img src=x onerror=alert(1)>` and a job URL of `javascript:alert(1)` render literally and the job has no GitHub link.
  * `no horizontal scroll`: a 120 character branch name and a 300 character error line at the phone viewport leave `document.documentElement.scrollWidth <= innerWidth`.
- [ ] **Step 2:** `cd web && npx playwright test tests/repair-page.spec.ts --project=chromium --project=mobile`. Expected: FAIL (route missing).
- [ ] **Step 3: Implement** the route, the router entry, the redirect, the two entry-point links and the `mockRepairApi` helper. The page sets `router.setTitle` from the case, moves focus to the heading on arrival as every page does, and announces a result through one polite live region.
- [ ] **Step 4:** the same command plus `npm run check && npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** "Add the repair page that shows the failure Eli is asked to fix".

---

## Stage 3: The attempt page and the hub

### Task 9: The attempt page, `/repairs/:id`

**Files:**
- Create: `web/src/routes/EliRepairAttempt.svelte`, `web/src/lib/repairs/ProgressTrack.svelte`, `OutcomeBanner.svelte`, `TechnicalDetails.svelte`, `web/tests/repair-attempt.spec.ts`
- Modify: `web/src/lib/router.ts` (route `eli-repair`, `/repairs/:id`, title "Repair", after `/repairs/new` and `/repairs/setup`), `web/src/gallery/Gallery.svelte`
- Test: `web/tests/repair-attempt.spec.ts`, `web/unit/repair-track.test.ts`

**Interfaces:**
- Consumes: `getEliRepair`, Task 6 (`caseFromSnapshot`, `guidanceFor`, `repairStatus`), `trackIndex`, `REPAIR_TRACK` and `isActive` from `web/src/lib/repairs/journey.ts` and `history.ts` (keep those exports; `journey.ts` loses only the stage machine in Task 11).
- Produces: `ProgressTrack { repair: Schemas['EliRepair']; now: number }`, `OutcomeBanner { repair: Schemas['EliRepair'] }`, `TechnicalDetails { repair: Schemas['EliRepair'] }` (a table of model calls; a step without reported usage reads "unknown", never 0).

- [ ] **Step 1: Failing tests.** Unit `repair-track.test.ts`: `trackIndex` for queued, working at `source`, `prepare_fix`, `publish`, `watch_ci` and a published commit (existing behaviour pinned, with the stage ids). Playwright `repair-attempt.spec.ts`: `a running repair shows live progress and stops polling when it finishes` (stub `GET /assistant/repairs/rep-1` first as `working`/`prepare_fix`, then `checks_failed`; assert the track marks the third step current, then the outcome banner appears and further polls stop); `a repair that could not repair says why and what to try` (an `unsupported_fix` row: banner with its message, the first step "Add a note", a Try another attempt link to `/repairs/new?repo=...&pull=...`); `a repair written before cases existed still opens` (no `details.case`: title from repo and number, no error); `someone else's repair says it is not in their history` (404); `no horizontal scroll on a phone`.
- [ ] **Step 2:** `cd web && node --import ./unit/register.mjs --test unit/repair-track.test.ts && npx playwright test tests/repair-attempt.spec.ts --project=chromium --project=mobile`. Expected: FAIL.
- [ ] **Step 3: Implement** the page (polls `getEliRepair` every 4 000 ms while `isActive(state)` and the tab is visible, one final read at a terminal state, aborts on leaving), the three components with the gallery states, and the route.
- [ ] **Step 4:** the same commands plus `npm run check && npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** "Add the attempt page that follows one repair to its outcome".

### Task 10: The hub, richer history rows and Select mode

**Files:**
- Modify: `web/src/routes/EliRepairs.svelte` (rewritten as the hub), `web/src/lib/repairs/RepairRow.svelte` (rewritten), `RepairHistory.svelte` (a **Select** mode), `RepairOffers.svelte` (keep the Overview banner)
- Create: `web/src/lib/repairs/OfferList.svelte`, `SetupBar.svelte`, `ActiveRepairs.svelte`
- Modify: `web/tests/repair-history.spec.ts`
- Test: `web/tests/repair-hub.spec.ts`, `web/tests/repair-history.spec.ts`

**Interfaces:**
- Consumes: `listEliOpportunities`, `actEliOpportunity`, `listEliRepairs({ group: 'active' })`, `getEliRepairSettings`, Task 6 (`caseFromSnapshot`).
- Produces: `OfferList { offers: Schemas['EliOpportunity'][]; busy: boolean; ondismiss: (id: string) => void }` linking **Review repair** to `targetHref({ offer: id, repo, pull: pull_number, run: run_id })`; `SetupBar { settings: Schemas['EliRepairSettings'] | null; admin: boolean }`; `ActiveRepairs { items: Schemas['EliRepair'][] }`; the hub polls every ten seconds as the history does and reads offers at most once a minute.

- [ ] **Step 1: Failing tests.** `repair-hub.spec.ts`: `the hub lists offers, running repairs and history with the failing job on every row` (an offer shows its PR title, repository, branch and failing job; a running repair shows its stage; a history row shows the PR title, the failing job and step from `details.case`, and links to `/repairs/<id>`); `Review repair links to the repair page for that offer`; `Dismiss hides the offer`; `the setup bar says whether the account is linked and allowed`. In `repair-history.spec.ts` update the selection tests to press **Select** first and add `rows are not selectable until Select is pressed`.
- [ ] **Step 2:** `cd web && npx playwright test tests/repair-hub.spec.ts tests/repair-history.spec.ts --project=chromium --project=mobile`. Expected: FAIL.
- [ ] **Step 3: Implement** the hub and the components, keep the history's filters, paging, selection set, confirmation and clear report exactly as they are, and hide the checkbox column until **Select** is on. The hub keeps the legacy redirect from Task 8 (its test stays green) and its **Repair a PR** action opens `RepairTargetDialog`.
- [ ] **Step 4:** the same command plus `npm run check && npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** "Rebuild the Eli repairs hub around offers, running repairs and a readable history".

---

## Stage 4: Setup, clean-up and docs

### Task 11: Setup moves to its own page

**Files:**
- Create: `web/src/lib/repairs/RepairSetupPanel.svelte` (the account card, repository policies and link-an-account forms extracted from `web/src/lib/settings/EliRepairs.svelte`, behaviour unchanged), `web/src/routes/EliRepairSetup.svelte`, `web/src/lib/settings/RepairsCard.svelte`
- Modify: `web/src/lib/router.ts` (route `eli-repair-setup`, `/repairs/setup`, title "Repair setup", before `/repairs/:id`), `web/src/lib/settings/AssistantPanel.svelte` (render `RepairsCard` instead of the composite), `web/tests/repairs.spec.ts`, `web/tests/repair-setup.spec.ts`
- Delete: `web/src/lib/settings/EliRepairs.svelte`, `web/src/lib/repairs/RepairJourney.svelte`, and the stage machine in `web/src/lib/repairs/journey.ts` (keep `REPAIR_TRACK`, `trackIndex`, `isTerminal`, `outcomeTone`, `stateOf`); delete `web/src/lib/repairs/RepairOffers.svelte` only if nothing imports it (the Overview does, so keep it)
- Test: `web/tests/repairs.spec.ts`, `web/tests/repair-setup.spec.ts`, `web/unit/journey.test.ts` if one exists

**Interfaces:** `RepairSetupPanel { providerRevision?: number; initialRepo?: string }` with `reload(): Promise<void>` exported as the old component did; the blocked repair page still opens `RepairSetup.svelte` for the failure in hand.

- [ ] **Step 1: Failing tests.** `repairs.spec.ts`: the manual-policy save test now starts at `/repairs/setup` (no `#repair-setup > summary` click); add `Settings, Eli AI Assistant links to the hub and to setup and no longer lists repairs`. `repair-setup.spec.ts`: the wizard from a blocked repair page still saves and re-checks readiness for the same run.
- [ ] **Step 2:** `cd web && npx playwright test tests/repairs.spec.ts tests/repair-setup.spec.ts tests/settings.spec.ts --project=chromium --project=mobile`. Expected: FAIL.
- [ ] **Step 3: Implement** the extraction and the page; delete the old composite and journey once `grep -rn "settings/EliRepairs\|RepairJourney" web/src web/tests` is empty.
- [ ] **Step 4:** the same command plus `npm run check && npm run lint && npm run test:unit`. Expected: PASS.
- [ ] **Step 5: Commit** "Move repair setup to its own page and retire the composite".

### Task 12: Accessibility, phones, hostile input and screenshots

**Files:**
- Modify: `web/tests/a11y.spec.ts`, `web/tests/mobile.spec.ts`, `web/tests/navigation.spec.ts`, `web/tests/hostile-input.spec.ts`, `web/tests/support/screenshots.mjs`, `web/tests/ui-style.spec.ts` (only if it names the old page structure)
- Test: those specs

- [ ] **Step 1: Failing additions.** Add `/repairs`, `/repairs/new?repo=acme/site&run=9` and `/repairs/setup` (with `mockRepairApi`) to the accessibility and mobile route lists; add the hostile-input case for the repair and attempt pages (script-shaped title and step names, a `javascript:` URL, a 10 000 character error line); add a navigation test that Eli repairs stays lit on `/repairs/new`, `/repairs/setup` and `/repairs/rep-1`.
- [ ] **Step 2:** `cd web && npx playwright test tests/a11y.spec.ts tests/mobile.spec.ts tests/navigation.spec.ts tests/hostile-input.spec.ts --project=chromium --project=mobile`. Expected: FAIL on whatever the new pages get wrong; fix it in the components, not the tests.
- [ ] **Step 3:** Update the screenshot script: replace the `eli-repair-readiness` entry's path with the repair page, add the attempt page and the hub, keep the setup entry on `/repairs/setup`; make the stubbed data match `mockRepairApi`'s. (`make screenshots` needs Pillow; run it only if the environment has it, otherwise leave the images and say so in the pull request.)
- [ ] **Step 4:** the Step 2 command. Expected: PASS.
- [ ] **Step 5: Commit** "Hold the new Eli repair pages to the accessibility, phone and hostile-input rules".

### Task 13: Docs, progress record and the full run

**Files:**
- Modify: `docs/eli.md` (the repair tour, the flow, the history paragraph, what a repair remembers, error output), `docs/ui.md` (the Eli repairs tour), `docs/api-surface.md` (already touched; check every new and changed row), `docs/ui-guidelines.md` (the navigation line), `roadmap/progress.md` (a row for this work, in the current Eli section), this plan's own header note if anything diverged
- Test: `internal/docs`

- [ ] **Step 1:** Write the docs from the spec in the repository voice. Where a screenshot reference changes, point at the new image names.
- [ ] **Step 2: Full verification.** `make lint`; `make test`; `cd web && npm run check && npm run lint && npm run test:unit`; `make build VERSION=dev && cd web && npx playwright test tests/repair-page.spec.ts tests/repair-attempt.spec.ts tests/repair-hub.spec.ts tests/repair-history.spec.ts tests/repairs.spec.ts tests/repair-setup.spec.ts tests/gallery.spec.ts tests/a11y.spec.ts tests/mobile.spec.ts tests/navigation.spec.ts tests/hostile-input.spec.ts`. Expected: all PASS. Then `git status` shows only intended files and `git diff --stat origin/main` is read once for anything surprising.
- [ ] **Step 3:** Run the docs voice tests: `go test -count=1 ./internal/docs`. Expected: PASS (no em dash, no spaced `--` in new lines).
- [ ] **Step 4: Commit** "Document the Eli repair pages and record the work".
