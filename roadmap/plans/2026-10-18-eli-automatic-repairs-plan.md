# Eli automatic repairs: visibility and controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an administrator see which repositories repair themselves, what that has cost and done, and stop it, on the hub, the attempt page and the repair page.

**Architecture:** Two additive administrator routes over existing tables (an overview of Automatic policies with their allowance, health and latest repair, and a stop that only relaxes a policy), and readiness naming the repository's mode. The UI adds a hub panel and a stop dialog, a card on an automatic repair's page and a note on the repair page, over a pure, unit-tested helper module.

**Tech Stack:** Go 1.27 (store, controller, API), SQLite through `internal/store` only, Svelte 5 runes, TypeScript, Vite, Playwright, OpenAPI 3.1 with generated Go and TypeScript copies.

**Spec:** [2026-10-18-eli-automatic-repairs.md](2026-10-18-eli-automatic-repairs.md)

## Global Constraints

* Go 1.27.1 or later; Node 22 or later. No new dependency, Go or npm (`docs/dependencies.md`).
* SQL is written in `internal/store` only. No migration.
* After editing `api/openapi.yaml` run `make openapi` and commit both generated files (`internal/api/openapi_spec.go`, `web/src/lib/api/schema.d.ts`). A new route also updates the route and role table in `internal/api/api_test.go` and `docs/api-surface.md` in the same task.
* Prose, comments, docs, error messages and UI strings: British spelling; no em dashes; no spaced `--`; an absent value is `NO_VALUE` from `web/src/lib/format.ts`.
* UI: tokens only, never a raw colour or a value that already has a token; the six status hues keep their fixed meanings and every status comes from `web/src/lib/status.ts`; no state-management library, charting library or router change; the app shell stays under 200 KB gzipped, so every new component lives in an existing lazily loaded route.
* Nothing is reachable from the UI that is not on a documented REST route.
* A new component state is added to the gallery (`web/src/gallery/`) before a route reaches it.
* A repository that is not automatic adds nothing to any page. Stopping never turns anything on.
* Everything a workflow, a person or an operator names (repository, provider and model names) is untrusted: rendered as text, and a link only when its scheme is `http` or `https`.
* Tests: standard-library `testing`, table-driven, `:memory:` stores, injected clocks, the fake GitHub; test names are sentences about behaviour. UI unit fixtures are built to the generated types, never cast.
* Commit messages: an imperative sentence in plain prose, sentence case, no prefix. Each commit ends with the two attribution lines the session was given.

## Review Focus

* A policy whose provider was deleted still stops, because that is the day the control is needed. Owned by Task 3.
* An ordinary user is refused the overview and sees one sentence on the repair page, never what Eli has done. Owned by Task 4 and Task 8.
* A lowered daily limit leaves a repository with more attempts used than allowed; the row reads "all used", never a negative or a bar past full. Owned by Task 5.
* A repository, provider or model name holding markup renders as text and never overflows a phone. Owned by Task 9.
* Stopping a repository somebody else already stopped is a neutral "already stopped", not an error. Owned by Task 3 and Task 6.
* The last repository stopping removes the panel without leaving keyboard focus on nothing. Owned by Task 6.
* A repair that is queued when its repository is stopped does not run. Owned by Task 3.

---

## Stage 1: Data

### Task 0: Prepare the tree

**Files:** none changed.

- [ ] **Step 1:** `make build-nogui` (writes the web placeholder that every plain `go` command needs).
- [ ] **Step 2:** `cd web && npm ci --no-audit --no-fund`.
- [ ] **Step 3: Baseline.** `go test -count=1 ./internal/store ./internal/controller ./internal/api` and `cd web && npm run check && npm run test:unit`. Expected: all PASS. If anything fails here it is not ours; note it and carry on.
- [ ] **Step 4:** `export PLAYWRIGHT_CHROMIUM=/opt/pw-browsers/chromium` for every Playwright run in this plan.

### Task 1: The store reads allowance use and automatic activity

**Files:**
- Create: `internal/store/queries_eli_automatic.go`
- Test: `internal/store/eli_automatic_test.go`

**Interfaces:**
- Produces: `type EliRepairUsage struct{ Used int; Oldest time.Time }` and `func (s *Store) EliRepairUsage(ctx context.Context) (map[string]EliRepairUsage, error)`, keyed by repository, counting rows created in the last 24 hours (cleared ones included, any trigger) with `Oldest` the earliest `created_at` among them; `type EliAutomaticLast struct{ ID string; State string; PullNumber int; UpdatedAt time.Time }` with JSON `id,state,pull_number,updated_at`; `type EliAutomaticActivity struct{ Running int; Last *EliAutomaticLast }` and `func (s *Store) EliAutomaticActivity(ctx context.Context) (map[string]EliAutomaticActivity, error)` counting `trigger='automatic'` rows in `queued`, `working` or `checking` as `Running`, and `Last` the newest automatic row not cleared from the history (by `created_at`).

- [ ] **Step 1: Write the failing tests** in `eli_automatic_test.go`, using `newTestStore` and `insertRepair`:
  * `TestRepairUsageCountsTheLast24HoursPerRepositoryClearedOnesIncluded`: for `acme/site` a row 25 hours old, one 23 hours old, one cleared (`ClearEliRepairs`) 1 hour old; for `acme/other` one row 2 hours old. Assert `acme/site` is `Used` 2 with `Oldest` the 23-hour row's time, `acme/other` is 1, and a repository with no recent row is absent from the map.
  * `TestAutomaticActivityCountsRunningAndFindsTheNewestVisibleRepair`: `acme/site` has automatic rows `queued`, `working`, `checking`, `succeeded` (oldest), a `failed` one newer, a `mention` row in `working` (not counted) and an automatic one newest but cleared. Assert `Running` 3 and `Last` the `failed` row with its `State`, `PullNumber` and `UpdatedAt`; a repository with only requested repairs is absent.
- [ ] **Step 2:** `go test -count=1 ./internal/store -run 'RepairUsage|AutomaticActivity'`. Expected: FAIL (undefined).
- [ ] **Step 3: Implement** both reads: `EliRepairUsage` is one `SELECT repo, count(*), min(created_at) FROM eli_repairs WHERE created_at>=? GROUP BY repo`; `EliAutomaticActivity` is one grouped count for the running states and one ordered scan of `trigger='automatic' AND cleared_at IS NULL` taking the first row per repository.
- [ ] **Step 4:** the same command. Expected: PASS.
- [ ] **Step 5: Commit** "Read how much of its allowance each repository has used, and what automatic repairs did".

### Task 2: The controller builds the overview, and readiness names the mode

**Files:**
- Create: `internal/controller/eli_automatic.go`
- Modify: `internal/controller/eli_experience.go`
- Test: `internal/controller/eli_automatic_test.go`, `internal/controller/eli_experience_test.go`

**Interfaces:**
- Consumes: Task 1's two reads.
- Produces: `type EliAutomaticRepository struct{ Repo, InstallationID, ProviderID, ProviderName, Model, Problem string; AllowWorkflows bool; DailyLimit, Used, Running int; NextFreeAt *time.Time; Last *store.EliAutomaticLast }` with JSON `repo,installation_id,provider_id,provider_name,model,problem(omitempty),allow_workflows,daily_limit,used,next_free_at(omitempty),running,last(omitempty)`; `func (c *Controller) EliAutomaticRepairs(ctx context.Context) ([]EliAutomaticRepository, error)` returning one entry per policy that is `Enabled && Automatic`, the ones with a problem first and then by repository name, `NextFreeAt` set to `Oldest+24h` when `Used > 0`; `EliReadiness.PolicyMode string` (`json:"policy_mode,omitempty"`) set to `automatic`, `ask_first` or `manual` when readiness reaches the policy check.
- `Problem` is empty when the provider is an enabled installation provider, and otherwise one sentence: the provider was deleted (the policy keeps no provider id, because the column is set null when its provider goes); the provider is disabled (naming it); the provider belongs to a person. Each tells an administrator what to do in setup. A removed installation cannot appear: the policy is deleted with it.

- [ ] **Step 1: Write the failing tests:**
  * `TestTheAutomaticOverviewListsOnlyEnabledAutomaticPolicies`: policies for an automatic one, an ask-first one, a manual one and a disabled automatic one; assert one entry, with `ProviderName`, `Model`, `DailyLimit`, `AllowWorkflows` from the policy and provider.
  * `TestTheAutomaticOverviewSaysWhyAProviderCannotPay`: a table over a deleted provider (the policy is left with none), a disabled one, a personal one (`OwnerID` set) and a working one; assert `Problem` is non-empty for the first three, mentions the provider's name for the disabled one, and is empty for the last.
  * `TestTheAutomaticOverviewCountsTheAllowanceAndSaysWhenAPlaceFrees`: limit 2, one automatic and one requested attempt at known times with the injected clock; assert `Used` 2 and `NextFreeAt` equal to the older time plus 24 hours; a repository with nothing used has no `NextFreeAt`.
  * `TestTheAutomaticOverviewOrdersProblemsFirstThenByName`.
  * In `eli_experience_test.go`, `TestReadinessNamesTheRepositorysMode`: over an automatic, an ask-first and a manual policy, reusing the file's ready scaffold; assert `PolicyMode` is `automatic`, `ask_first`, `manual`, and empty when the person is not linked (readiness stops before the policy).
- [ ] **Step 2:** `go test -count=1 ./internal/controller -run 'AutomaticOverview|ReadinessNamesTheRepositorys'`. Expected: FAIL (undefined).
- [ ] **Step 3: Implement** `EliAutomaticRepairs` in `eli_automatic.go` from `EliRepairPolicies`, `EliRepairUsage`, `EliAutomaticActivity`, `GetAssistantProvider` and `GetInstallation`, and set `PolicyMode` beside the existing `policy` check using `repairModeWords`' three cases.
- [ ] **Step 4:** the same command. Expected: PASS.
- [ ] **Step 5: Commit** "Say which repositories repair themselves, what it has cost and whether it can run".

### Task 3: The controller stops automatic repairs

**Files:**
- Modify: `internal/controller/eli_automatic.go`
- Test: `internal/controller/eli_automatic_test.go`

**Interfaces:**
- Consumes: `store.SetEliRepairPolicy`, `store.EliRepairPolicies`.
- Produces: `const EliStopAskFirst = "ask_first"`, `const EliStopOff = "off"`; `type EliAutomaticStop struct{ Repo string; Changed bool; Error string }` (JSON `repo,changed,error(omitempty)`); `func (c *Controller) StopEliAutomatic(ctx context.Context, repos []string, then string) ([]EliAutomaticStop, error)`; an unknown `then` is an error, so the API validates first.

Behaviour: for each repository, once, in order: no policy is `Changed` false with `Error` "No repair policy for this repository."; a policy that is not both enabled and automatic is `Changed` false and no error; otherwise `ask_first` writes `Automatic` false, `AskFirst` true; `off` writes `Enabled` false and nothing else. Provider, limit and workflow setting are never touched and the provider is never looked up.

- [ ] **Step 1: Write the failing tests:**
  * `TestStoppingAutomaticRepairsToAskFirstKeepsTheProviderAndLimits` and `TestStoppingAutomaticRepairsToOffOnlyClearsTheSwitch`: assert the saved policy field by field.
  * `TestStoppingWorksWhenTheProviderIsGone`: delete the provider (the policy is left with no provider id), stop both ways, assert success and `Changed`.
  * `TestStoppingTwiceChangesNothingTheSecondTime` and `TestStoppingARepositoryWithNoPolicySaysSo`.
  * `TestAStoppedRepositoryBlocksItsQueuedAutomaticRepair`: enqueue an automatic row, stop, run `runEliRepair` with the file's fake client; assert the row is `blocked`, no commit was made and the model was not asked. (It pins behaviour the worker already has; ledger it as a characterisation test.)
- [ ] **Step 2:** `go test -count=1 ./internal/controller -run 'StoppingAutomatic|StoppingWorks|StoppingTwice|StoppingARepository|StoppedRepository'`. Expected: FAIL (undefined), except the pinning test.
- [ ] **Step 3: Implement** `StopEliAutomatic`.
- [ ] **Step 4:** the same command. Expected: PASS.
- [ ] **Step 5: Commit** "Let an administrator stop automatic repairs without touching the policy's provider".

### Task 4: The API routes

**Files:**
- Modify: `internal/api/handlers_eli_repairs.go`, `internal/api/router.go`, `internal/api/api_test.go`, `api/openapi.yaml`, `docs/api-surface.md`
- Regenerated: `internal/api/openapi_spec.go`, `web/src/lib/api/schema.d.ts`
- Test: `internal/api/eli_automatic_test.go`

**Interfaces:**
- Consumes: Task 2 and Task 3.
- Produces: `GET /assistant/repairs/automatic` (operationId `getEliAutomaticRepairs`, `assistant.read`, administrator) answering `{repositories: [EliAutomaticRepository]}`; `POST /assistant/repairs/automatic/stop` (operationId `stopEliAutomaticRepairs`, `assistant.write`, administrator) taking `{repos, then}` and answering `{results: [EliAutomaticStop]}`; schemas `EliAutomaticRepairs`, `EliAutomaticRepository`, `EliAutomaticLast`, `EliAutomaticStopRequest`, `EliAutomaticStopResult`; `policy_mode` on `EliReadiness` (optional string with enum). One audit row `assistant.repair_automatic_stop` per repository changed, target type `repository`, target the repository, detail `{then}`.
- Validation: 422 for no repositories, more than 50, a blank name or a `then` that is not `ask_first` or `off`; names are trimmed and a repeat is stopped once.

- [ ] **Step 1: Write the failing tests** in `eli_automatic_test.go` using `newHarness`:
  * `TestTheAutomaticOverviewIsAnAdministratorRouteAndTakesTheSpecsShape`: a viewer and an operator are refused (403), an administrator gets 200, and `assertShape` holds for `EliAutomaticRepairs` and each repository (build one with a provider, a policy and a repair).
  * `TestStoppingAutomaticRepairsAnswersEachRepositoryAndWritesAnAuditRowForEachChange`: one automatic repository, one already ask-first, one with no policy; assert the three results, the policy now ask-first, and exactly one `assistant.repair_automatic_stop` row (for the changed one).
  * `TestStoppingAutomaticRepairsRefusesWhatItCannotUnderstand`: empty list, 51 names, a blank name and `then` "pause" are each 422 and change nothing.
  * Add the two route rows to the table in `api_test.go`; add `policy_mode` to the readiness shape assertion in `eli_repairs_setup_test.go`.
- [ ] **Step 2:** `go test -count=1 ./internal/api -run 'AutomaticOverview|StoppingAutomatic|RouteAuthorisation|Readiness'`. Expected: FAIL (route not found).
- [ ] **Step 3: Implement** the two handlers and routes; edit `api/openapi.yaml`; run `make openapi`; add both routes and the audit action to `docs/api-surface.md` in the style of the neighbouring rows.
- [ ] **Step 4:** the same command, then `go test -count=1 ./internal/docs`. Expected: PASS.
- [ ] **Step 5: Commit** "Add the routes that show automatic repairs and stop them".

---

## Stage 2: The UI

### Task 5: Helpers, client and fixtures

**Files:**
- Create: `web/src/lib/repairs/automatic.ts`, `web/unit/repair-automatic.test.ts`
- Modify: `web/src/lib/api/client.ts`, `web/src/lib/repairs/hub.ts`, `web/unit/repair-fixtures.ts`, `web/unit/repair-hub.test.ts` (or the file that holds the `setupSummary` tests)

**Interfaces:**
- Produces in `client.ts`: `getEliAutomaticRepairs(signal?)` and `stopEliAutomaticRepairs(body)`, typed from the generated operations.
- Produces in `automatic.ts` (pure): `allowanceOf(repo)` returning `{ used, limit, full, ratio, words }` (ratio between 0 and 1 and never past full; `words` "3 of 5 attempts used in the last 24 hours"); `waitsUntil(repo)` giving "All 5 attempts used. Automatic repairs here wait until <time>." when full; `orderRepositories(list)` (problems first, then by name, case-insensitively); `lastWords(repo, now)` ("None yet", or the outcome label, pull request and age); `stopCopy(repos)` returning the dialog's title, intro and the two choices' words; `stoppedWords(results, then)` returning the toast's title and message, or the error text when some failed; `automaticNote(input)` returning which of `tried`, `none` or `neutral` the repair page shows and its sentences.
- `setupSummary` gains `automatic: number | null` (enabled policies set to Automatic, for an administrator, else null).
- `repair-fixtures.ts` gains `automaticRepository(over?)` built to the generated type.

- [ ] **Step 1: Write the failing tests** in `repair-automatic.test.ts`:
  * `a full allowance says when a place frees, and a lowered limit never reads past full` (used 7 of limit 5: `full` true, ratio 1, words "7 of 5" avoided in favour of "All 5 attempts used");
  * `the rows put a problem first and then sort by name without regard to case`;
  * `the last repair reads as none, running, or its outcome and age`;
  * `the stop copy names one repository or counts several, and says what happens to work in flight`;
  * `a stop that changed nothing is reported as already stopped, and a failure names the repository`;
  * `the note is neutral for anyone who cannot read automatic repairs, says nothing started for an administrator, and points to the attempt Eli already made` (matching on pull request and head, and a different head is not a match);
  * in the hub test: `the setup summary counts automatic repositories for an administrator only`.
- [ ] **Step 2:** `cd web && npm run test:unit -- --test-name-pattern='allowance|stop copy|automatic|last repair|note'`. Expected: FAIL (module not found).
- [ ] **Step 3: Implement** `automatic.ts`, the two client functions, the `setupSummary` field and the fixture.
- [ ] **Step 4:** `cd web && npm run check && npm run test:unit`. Expected: PASS.
- [ ] **Step 5: Commit** "Add the words and the reads the automatic repair pages need".

### Task 6: The hub panel, the stop dialog and the setup line

**Files:**
- Create: `web/src/lib/repairs/AutomaticRepairs.svelte`, `web/src/lib/repairs/StopAutomaticDialog.svelte`
- Modify: `web/src/lib/repairs/SetupBar.svelte`, `web/src/routes/EliRepairs.svelte`, `web/src/gallery/sections/EliRepairs.svelte`, `web/tests/support/repairs.ts`
- Test: `web/tests/repair-automatic.spec.ts` (new)

**Interfaces:**
- Consumes: Task 5.
- Produces: `AutomaticRepairs` with props `repositories`, `error` and `onstop(repos: string[])`; `StopAutomaticDialog` with `open` (bindable), `repos: string[]`, `busy` and `onconfirm(then)`; `SetupBar` gains `automatic: number | null`. `mockRepairWorld` serves `GET /assistant/repairs/automatic` and `POST /assistant/repairs/automatic/stop`, with the world's settings carrying the matching policies.

- [ ] **Step 1: Write the failing Playwright tests** in `repair-automatic.spec.ts` (desktop and phone):
  * `the hub shows nothing about automatic repairs where none is on`, for an administrator with only ask-first policies and for an ordinary user whose settings hold no policies;
  * `the hub lists each automatic repository with who pays, its allowance, its health and its latest repair`: two repositories, one with a problem (listed first, with its fix link to `/repairs/setup?repo=`), one with an allowance of 3 of 5 and a running repair;
  * `a spent allowance says when automatic repairs here resume`;
  * `the setup line counts automatic repositories`;
  * `stopping one repository asks what should happen instead, defaulting to ask me first, sends that, and the panel drops it`: assert the request body and the toast;
  * `stop all stops every automatic repository and the panel goes, with keyboard focus on the page heading`;
  * `a stop that fails keeps the panel and says which repository`;
  * `a repository somebody else already stopped reads as already stopped`.
- [ ] **Step 2:** `cd web && PLAYWRIGHT_CHROMIUM=/opt/pw-browsers/chromium npx playwright test repair-automatic --project=chromium`. Expected: FAIL.
- [ ] **Step 3: Implement** the components, wire the hub to read `getEliAutomaticRepairs()` for administrators on its existing timer (a failure keeps the last answer and sets the panel's one-line error), put the panel between In progress and History, add the gallery states (one row, a problem, a spent allowance, running, the dialog for one and for several, an error line), and extend the world stub.
- [ ] **Step 4:** the same command, then `--project=mobile`, then `npm run check && npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** "Show the repositories that repair themselves on the hub, and let an administrator stop them".

### Task 7: The attempt page says it started on its own

**Files:**
- Create: `web/src/lib/repairs/AutomaticCard.svelte`
- Modify: `web/src/lib/repairs/AttemptFacts.svelte`, `web/src/routes/EliRepairAttempt.svelte`, `web/src/gallery/sections/EliRepairs.svelte`
- Test: `web/tests/repair-automatic.spec.ts`

**Interfaces:**
- Produces: `AutomaticCard` with props `repository: Schemas['EliAutomaticRepository'] | null`, `repo: string`, `error: boolean` and `onstop()`; `AttemptFacts` shows **Started by** for an automatic repair.

- [ ] **Step 1: Write the failing tests:** `an automatic repair says it was started by the repository's policy`; `an administrator sees where the policy stands and can stop it from the repair`; `a repository that has stopped being automatic says so and links to setup`; `an unreadable status leaves the rest of the page standing`; `a requested repair shows no automatic card and makes no overview read`.
- [ ] **Step 2:** the Task 6 command. Expected: FAIL.
- [ ] **Step 3: Implement** the card and the fact; read the overview once when the repair is automatic and the viewer is an administrator, and again after a stop.
- [ ] **Step 4:** the same command on both projects, then `npm run check`. Expected: PASS.
- [ ] **Step 5: Commit** "Say on an automatic repair that the repository's policy started it".

### Task 8: The repair page says the repository repairs itself

**Files:**
- Create: `web/src/lib/repairs/AutomaticNote.svelte`
- Modify: `web/src/routes/EliRepairNew.svelte`, `web/src/gallery/sections/EliRepairs.svelte`
- Test: `web/tests/repair-automatic.spec.ts`

**Interfaces:**
- Consumes: `readiness.policy_mode`, `listEliRepairs({ repo, trigger: 'automatic', limit })`, `automaticNote` from Task 5.
- Produces: `AutomaticNote` with the note's state and sentences; the page passes the matching attempt's id for the link.

- [ ] **Step 1: Write the failing tests:** `an administrator is told Eli already tried this failure on its own, and shown the attempt`; `an administrator is told nothing started and why, and the allowance is named when it is the blocked check`; `anyone else gets one neutral sentence and no read of automatic repairs`; `nothing is said for a repository that asks first or is manual`; `a repair already running still opens its own page`.
- [ ] **Step 2:** the Task 6 command. Expected: FAIL.
- [ ] **Step 3: Implement** the note above the decision card, reading the automatic history only for an administrator and only when the mode is automatic.
- [ ] **Step 4:** the same command on both projects, then `npm run check`. Expected: PASS.
- [ ] **Step 5: Commit** "Tell a person on the repair page when the repository repairs itself".

### Task 9: Accessibility, phone and hostile-input coverage

**Files:**
- Modify: `web/tests/a11y.spec.ts`, `web/tests/mobile.spec.ts`, `web/tests/hostile-input.spec.ts`, `web/tests/support/repairs.ts`

- [ ] **Step 1: Write the failing tests:** the axe audit of the hub with the panel and of the open dialog; the phone pass (no sideways scroll, targets large enough) of the hub with a very long repository, provider and model name, and of the attempt card; a hostile-input test with markup and a `javascript:` string in a repository, provider and model name that renders as text and makes no link.
- [ ] **Step 2:** `cd web && PLAYWRIGHT_CHROMIUM=/opt/pw-browsers/chromium npx playwright test a11y mobile hostile-input -g 'automatic'`. Expected: FAIL where a rule is broken, or PASS if Tasks 6 to 8 already hold the line; ledger which.
- [ ] **Step 3:** Fix what the audits find in the components.
- [ ] **Step 4:** the same command on both projects. Expected: PASS.
- [ ] **Step 5: Commit** "Hold the automatic repair pages to the accessibility and phone audits".

### Task 10: Documentation, screenshot and full verification

**Files:**
- Modify: `docs/eli.md`, `docs/ui.md`, `roadmap/progress.md`, `docs/screenshots/eli-repairs-{dark,light}.webp`, `web/tests/support/screenshots.mjs`, `web/tests/support/repair-fixture/*` as the harness needs, the spec's status line and this plan's header note

- [ ] **Step 1:** `docs/eli.md`: a section "Watching and stopping automatic repairs" (what the panel shows, the allowance and what "all used" means, how a problem reads, the stop dialog and what each choice does, that a queued repair is blocked and one that has not pushed stops, what the attempt page and the repair page add, the two routes and the audit action). `docs/ui.md`: the tour gains the panel, the card and the note, and the hub screenshot's alt text names the panel. `roadmap/progress.md`: a dated Log entry.
- [ ] **Step 2:** Seed one automatic policy (with an installation provider) and repairs in the screenshot fixture so the hub capture holds the panel, then retake `eli-repairs` in both themes and look at them against the alt text.
- [ ] **Step 3: Full verification.** `make lint`; `make test`; `cd web && npm run check && npm run test:unit`; Playwright for `repair-automatic repair-hub repair-page repair-attempt repair-history repairs repair-setup a11y mobile hostile-input overview navigation` on both projects and the gallery project; `make openapi` and `make generate` leave the tree unchanged (do not commit the catalogue's `generated_from` stamp); `make build VERSION=dev`.
- [ ] **Step 4: Commit** "Document watching and stopping automatic repairs".
