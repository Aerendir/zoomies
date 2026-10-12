# Eli repair history Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recent repairs gets server-side paging and filters, row selection with a frozen select-all, and a bulk **Clear from history** that hides rows without deleting them.

**Architecture:** A nullable `cleared_at` on `eli_repairs` hides a row from the history while the budget, deduplication, own-commit and allowance readers keep seeing it. The store gains a paged and filtered list, a clearable-id selection and a per-id clear. The API serves them as `GET /assistant/repairs` (now filtered and paged), `GET /assistant/repairs/selection` and `POST /assistant/repairs/bulk`. The history card moves out of `EliRepairs.svelte` into its own component.

**Tech Stack:** Go and SQLite (`internal/store`, `internal/api`), OpenAPI with generated clients, Svelte 5 runes, Playwright against the built binary.

**Spec:** `roadmap/plans/2026-10-12-eli-repair-history.md` (approved, including the migration).

## Two departures from the spec

1. **No demo-seed rows.** The spec said `internal/controller/seed.go` would gain about sixty repairs. The repo keeps repair history out of `SeedDemo` on purpose: only the screenshot harness adds two sample rows, through `web/tests/support/repair-fixture`, while its controller is stopped. Sixty seeded rows would also turn the docs' "sample history: a passed repair and an attempt needing attention" screenshots false. The browser tests therefore mock the three endpoints with `page.route`, as the Eli chat specs already do, and the real filtering, paging, scoping and clear rules are proven by Go tests against a real store and the real handlers.
2. **`ListEliRepairs` has 16 test call sites** in three packages. Replacing it, as the spec says, means a mechanical edit of each (Task 1).

## Global Constraints

* Migration `internal/store/migrations/0100_eli_repair_cleared.sql`: nullable `cleared_at INTEGER` (epoch milliseconds), no index. It is not added to `eliRepairCols`.
* **Clearing hides a row and nothing else.** `EnqueueEliRepair`'s budget and deduplication, `IsEliRepairCommit`, `EliRepairBudget`, `NextEliRepair` and `CheckingEliRepairs` never read `cleared_at`.
* Only `internal/store` writes SQL; `internal/api` is transport.
* State groups, defined once in the store: **active** `queued`, `working`, `checking`; **passed** `succeeded`; **attention** every other state. Trigger `automatic` means `trigger = 'automatic'`; `requested` means anything else.
* Paging uses the house `Limit` and `Offset` parameters: default 50, maximum 500. Selection returns at most 500 ids. A bulk request carries 1 to 500 ids and the single action `clear`.
* A bad `group`, `trigger`, `action` or `ids`, or a `repo` over 200 characters, is a 422 with a field error.
* Per-id clear errors are exactly `not found` and `still running`.
* Scope: administrators (`ActionAssistantRead`) reach every row; everyone else only rows whose `user_id` is theirs and not empty. *Amended by the final review:* that is the rule for the list and the selection, which are reads. The bulk clear is a write, so its wide reach uses `ActionAssistantWrite`: an administrator's token narrowed to `assistant:own` passes `ActionAssistantRead` and must not be able to clear everyone's repairs. `eliRepairScope` in `internal/api/handlers_eli_repairs.go` is the one place the rule is written.
* Audit: `assistant.repair_clear`, target kind `eli_repair`, detail holds the count and the ids, never repair text.
* Voice: British spelling, no em dashes, no `--`, comments say why. Test names are sentences.
* Commit messages are imperative sentences in sentence case with no prefix, ending with the two trailer lines the session was given (`Co-Authored-By` and `Claude-Session`).
* UI: tokens only, existing `Pagination`, `Checkbox`, `ConfirmDialog`, `Field`, `Select`, `Input` and `Button`, no new library, every control labelled.
* Before each push (steward skill): `make build-nogui`, `make fmt` then `make lint`, tests for what changed, generated files regenerated and not edited. Before the last push, the whole suite.

## Review Focus

1. A person who guesses another person's repair id in a clear gets `not found`, the same as for an id that does not exist, and the row is untouched. (Task 1, Task 2)
2. A repository search of `%`, `_` or `\` is literal text, and one of only spaces means no filter. (Task 1)
3. A person who asks again for a pull request whose repair they cleared sees that row back in their history. (Task 1)
4. A clear that empties the page being viewed, or leaves the offset past the new total, steps back instead of showing an empty page. (Task 3 unit, Task 4 browser)
5. A selection survives the ten-second refresh and page changes, is dropped by a filter change, and never holds an in-flight row. (Task 4)

## Threat and failure-mode review

`CONTRIBUTING.md` asks for this on authorisation and deletion paths.

* **Reaching another person's rows by id:** every id is checked against the caller's scope inside the same transaction as the write, and a foreign id and a missing id answer alike. Task 1 and Task 2 test it.
* **A viewer clearing history they do not own, or clearing everything:** non-administrators are scoped to their own rows. An administrator clearing everything is the point of the feature, behind a confirmation that says what it does.
* **Using a clear to get round the spend cap or to re-run a webhook:** impossible by construction, since every guard keeps reading cleared rows. Task 1 pins each one.
* **Clearing a row that is still running:** refused as `still running`. Finished states are terminal, so a row that passes the check cannot become active again before the write; both happen in one writer transaction.
* **A mass clear by mistake:** the rows are kept, so it is recoverable by an operator in the database, but the UI has no restore. The dialog says so, and restoring is out of scope.
* **Evidence:** one audit row per request with the ids cleared.

## File structure

* `internal/store/queries_eli_repair_history.go` (create): the filter, the paged list, the selection and the clear. One responsibility, so `queries_eli_repairs.go` does not grow.
* `web/src/lib/repairs/history.ts` (create): pure helpers, so a test can hold them without a browser.
* `web/src/lib/repairs/RepairHistory.svelte` (create): the card. `EliRepairs.svelte` (about 650 lines) loses the markup, state and styles it moves out.

---

### Task 1: Hide a repair from the history without forgetting it (migration and store)

**Files:**
* Create: `internal/store/migrations/0100_eli_repair_cleared.sql`, `internal/store/queries_eli_repair_history.go`, `internal/store/eli_repair_history_test.go`
* Modify: `internal/store/queries_eli_repairs.go` (delete `ListEliRepairs`, `ListEliRepairsForOwner`, `listEliRepairs`; change the non-retry branch of `EnqueueEliRepair`)
* Modify, tests only: every caller of the two deleted functions. Find them with `grep -rn "ListEliRepairs" internal cmd` (16 sites: `internal/store/eli_repairs_test.go`, `internal/controller/eli_repairs_test.go`, `internal/controller/eli_experience_test.go`, `internal/api/eli_repairs_setup_test.go`) and the handler, which Task 2 replaces.

**Interfaces:**
* Produces, in package `store`:
  ```go
  const (
      DefaultEliRepairPage  = 50
      MaxEliRepairPage      = 500
      MaxEliRepairSelection = 500
  )
  var EliRepairGroups   = []string{"attention", "active", "passed"}
  var EliRepairTriggers = []string{"automatic", "requested"}
  type EliRepairFilter struct {
      OwnerID string // with Scoped: only rows whose user_id is OwnerID and not empty
      Scoped  bool
      Group   string // "" or one of EliRepairGroups; anything else is treated as ""
      Repo    string // case-insensitive substring after TrimSpace; "" means any
      Trigger string // "" or one of EliRepairTriggers
      Limit   int    // 0 means DefaultEliRepairPage; clamped to MaxEliRepairPage
      Offset  int
  }
  func (s *Store) ListEliRepairsPage(ctx context.Context, f EliRepairFilter) ([]*EliRepair, int, error) // page, total
  func (s *Store) EliRepairSelection(ctx context.Context, f EliRepairFilter) ([]string, int, error)   // clearable ids (at most MaxEliRepairSelection), total clearable
  type EliRepairClear struct {
      ID    string `json:"id"`
      OK    bool   `json:"ok"`
      Error string `json:"error,omitempty"`
  }
  func (s *Store) ClearEliRepairs(ctx context.Context, ids []string, ownerID string, scoped bool) ([]EliRepairClear, error)
  ```
  Lists and selections exclude cleared rows and order by `created_at DESC, id DESC`. The API, not the store, rejects unknown groups and triggers.
* The caller rewrite rule: `rows, err := x.ListEliRepairs(ctx)` becomes `rows, _, err := x.ListEliRepairsPage(ctx, store.EliRepairFilter{Limit: store.MaxEliRepairPage})` (no `store.` prefix inside package `store`). `TestPersonalRepairHistoryIsFilteredBeforeThePageLimit` becomes a scoped filter with `OwnerID: "owner"`.

- [ ] **Step 1: Write the failing tests** in `internal/store/eli_repair_history_test.go`, with a helper `insertRepair(t *testing.T, s *Store, r EliRepair) string` that INSERTs directly with `eliRepairCols` (defaults: `ID` from `NewID(PrefixEliRepair)`, `DedupKey` the id, `Trigger` `mention`, `State` `succeeded`, `CreatedAt` `s.Now()`), so a test controls state, time and owner without the daily budget `EnqueueEliRepair` enforces. Tests:
  * `TestTheRepairHistoryIsPagedFilteredAndCounted`: insert eight rows, oldest first, as (repo, state, trigger): `acme/site failed mention`, `acme/site failed mention`, `acme/site failed mention`, `acme/site succeeded mention`, `acme/widgets succeeded mention`, `acme/widgets working automatic`, `acme/widgets queued automatic`, `acme/gizmo checks_failed automatic`, each a minute after the last. Assert: `Limit: 3` gives 3 rows and total 8, the first being the last inserted; `Limit: 3, Offset: 6` gives 2 rows, total 8; `Group: "attention"` total 4, `"active"` total 2, `"passed"` total 2; `Repo: "WIDGETS"` total 3 (case-insensitive); `Trigger: "automatic"` total 3 and `"requested"` total 5; `Group: "attention", Repo: "site"` total 3.
  * `TestRepairPagingIsStableWhenRowsShareATimestamp`: five rows with one `CreatedAt`; pages of 2 at offsets 0, 2 and 4 return every id exactly once.
  * `TestARepairSearchTreatsPercentUnderscoreAndBackslashAsText`: repos `acme/a_b`, `acme/axb`, `acme/100%`, `acme/x`. `Repo: "a_b"` matches one row, `"%"` matches `acme/100%` only, `"\\"` matches none, `"   "` matches all four.
  * `TestAPersonSeesOnlyTheirOwnRepairsAndAnEmptyOwnerIsNobody`: rows for `owner`, `other` and `""`; `Scoped: true, OwnerID: "owner"` returns only theirs, and `Scoped: true, OwnerID: ""` returns none.
  * `TestClearingARepairHidesItFromTheHistoryAndNothingElse`: through `EnqueueEliRepair` (limit 1) enqueue a row with a `CommitSHA` saved as `succeeded`, clear it, then assert it is absent from `ListEliRepairsPage`, and that each guard still counts it: a second `EnqueueEliRepair` with a new dedup key in the same repo returns `ErrConflict`; `IsEliRepairCommit(repo, sha)` is true; `EliRepairBudget(repo, user)` counts it.
  * `TestClearReportsEachRowAndRepeatsAreHarmless`: ids `[ownRow, ownRow, activeRow, otherPersonsRow, "missing"]` with `scoped` for the owner give, in order, `{ownRow ok}`, `{activeRow, still running}`, `{otherPersonsRow, not found}`, `{missing, not found}` (the duplicate answers once); clearing `ownRow` again is `ok`; an administrator (`scoped` false) clears `otherPersonsRow`.
  * `TestARequestForAClearedRepairBringsItBackIntoView`: clear a `succeeded` row, enqueue the same `DedupKey` again, assert `added` is false and the row is back in the list.
  * `TestSelectionIsTheClearableMatchesCappedWithTheTotal`: insert `MaxEliRepairSelection+5` succeeded rows and three `working` rows. Selection with no filter returns `MaxEliRepairSelection` ids, total `MaxEliRepairSelection+5`, and none is a working row; `Group: "active"` returns no ids and total 0.
  * `TestTheClearedColumnMigrationKeepsExistingRepairsInTheHistory`: use `atSchemaBefore(t, "0100_eli_repair_cleared.sql")`, insert a row with raw SQL (`id, dedup_key, installation_id, repo, trigger, created_at, updated_at`), `Open` the path, and assert `ListEliRepairsPage` returns it.
- [ ] **Step 2: Run to see them fail.** `go test -count=1 ./internal/store/ -run 'RepairHistory|RepairPaging|RepairSearch|PersonSees|ClearingARepair|ClearReports|ClearedRepair|SelectionIsTheClearable|ClearedColumn'`. Expected: compile failure for the missing functions.
- [ ] **Step 3: Write the migration and implement the four functions** per the Interfaces block. Filter SQL: `cleared_at IS NULL`; scope `user_id=? AND user_id<>''`; `repo LIKE ? ESCAPE '\'` with the existing `likePattern`; groups as `state IN ('queued','working','checking')`, `state='succeeded'`, and the negation of both; trigger `trigger='automatic'` or `trigger<>'automatic'`. `ClearEliRepairs` runs in one `s.tx`, deduplicates ids keeping order, and sets only `cleared_at` (not `updated_at`). Change the non-retry branch of `EnqueueEliRepair` to run `UPDATE eli_repairs SET cleared_at=NULL WHERE id=? AND cleared_at IS NOT NULL` before returning the saved row.
- [ ] **Step 4: Rewrite the 16 callers** by the rule above and delete the old functions.
- [ ] **Step 5: Keep `internal/api` compiling.** `handleEliRepairs` still calls the deleted functions. Switch it to one `ListEliRepairsPage` call, scoped for non-administrators and with `Limit: store.MaxEliRepairPage`, returning the same `{items}` body; Task 2 replaces it with the real handler.
- [ ] **Step 6: Run.** `go test -count=1 ./internal/store/ ./internal/controller/ ./internal/api/` all pass.
- [ ] **Step 7: Commit.** `git add internal/store internal/controller internal/api` then a message such as "Let a repair be hidden from the history without losing what it counts towards".

---

### Task 2: Serve the history, a selection and a bulk clear (API and contract)

**Files:**
* Modify: `api/openapi.yaml`, `internal/api/handlers_eli_repairs.go`, `internal/api/router.go` (near line 395), `internal/api/api_test.go` (the route table), `docs/api-surface.md` (the `/assistant/repairs` row near line 377 and two new rows)
* Create: `internal/api/eli_repair_history_test.go`
* Regenerate, never edit: `internal/api/openapi_spec.go` (`go run internal/api/gen_openapi.go`) and `web/src/lib/api/schema.d.ts` (`make openapi`)

**Interfaces:**
* Consumes: Task 1's `EliRepairFilter`, `ListEliRepairsPage`, `EliRepairSelection`, `ClearEliRepairs`, `EliRepairGroups`, `EliRepairTriggers`.
* Produces (operation ids in brackets):
  * `GET /assistant/repairs` [`listEliRepairs`]: query `group`, `repo` (max 200), `trigger`, `limit`, `offset` (the `Limit` and `Offset` parameters). Returns `{items: EliRepair[], total: integer}`; `EliRepairList` gains required `total`.
  * `GET /assistant/repairs/selection` [`selectEliRepairs`]: the same filters without paging. Returns `{ids: string[], total: integer, truncated: boolean}`, `truncated` being `total > len(ids)`.
  * `POST /assistant/repairs/bulk` [`bulkEliRepairAction`]: `{action: "clear", ids: string[1..500]}`. Returns `{results: [{id, ok, error?}]}`. All three have `x-zoomies-role: viewer`, tag `assistant`, and a 422 response.
  * In Go: `func eliRepairFilter(w http.ResponseWriter, r *http.Request, paged bool) (store.EliRepairFilter, bool)` parses and validates the filters and builds the scope from `Identity(r.Context()).Can(auth.ActionAssistantRead)`; it writes the 422 itself and returns false.

- [ ] **Step 1: Write the failing tests** in `internal/api/eli_repair_history_test.go` (create rows with `h.st.EnqueueEliRepair(h.ctx, r, 50)` then `SaveEliRepair` for state; keep each person to at most eight rows, the per-person daily cap being ten):
  * `TestEliRepairHistoryIsPagedFilteredAndCountedForTheCaller`: an administrator and a viewer who owns two of six rows. The administrator's `GET ...?limit=2&offset=2` returns 2 items and `total` 6; `group=attention` and `trigger=automatic` narrow `total` as in Task 1; the viewer's `total` is 2. `?group=nonsense` and `?trigger=nonsense` are 422 with field `group` and `trigger`; a 201-character `repo` is 422.
  * `TestEliRepairSelectionIsTheCallersClearableMatches`: with one `working` row among the viewer's rows, the selection excludes it, `total` counts only clearable rows, `truncated` is false.
  * `TestEliRepairsAreClearedInBulkAndReportedOneByOne`: a viewer sends `[own, own, activeOwn, someoneElses, "missing"]`. Status 200; `results` in that order with the duplicate answered once; errors exactly `still running`, `not found`, `not found`; the cleared row is gone from the viewer's list and the other person's row is still in the administrator's. One audit row with action `assistant.repair_clear` exists (read with `h.st.ListAudit(h.ctx, store.AuditFilter{Actions: []string{"assistant.repair_clear"}}, store.Page{Limit: 5})`). `{"action":"delete"}`, an empty `ids` and 501 ids are each 422.
  * Add to the route table in `api_test.go`: `GET /api/v1/assistant/repairs/selection` and `POST /api/v1/assistant/repairs/bulk`, both `role: store.RoleViewer, action: auth.ActionAssistantOwn`, the second with `body: map[string]any{"action": "clear", "ids": []string{"missing"}}`.
- [ ] **Step 2: Run to see them fail.** `go test -count=1 ./internal/api/ -run 'EliRepair'`. Expected: 404s and a route-table failure.
- [ ] **Step 3: Edit `api/openapi.yaml`** (the list operation near line 5660, the schema near line 12928, new operations beside them, results items shaped like `/machines/bulk` with `required: [id, ok]`), then regenerate both generated files with the commands above.
- [ ] **Step 4: Implement the handlers and routes** (`ActionAssistantOwn`, as the list route has). Drop the post-fetch ownership loop in `handleEliRepairs`: the store's scope is the one source. Record one audit `Act(ctx, Identity(ctx), "assistant.repair_clear", "eli_repair", "", map[string]any{"count": n, "ids": clearedIDs})` per request, after the store call, for the ids that were cleared.
- [ ] **Step 5: Update `docs/api-surface.md`**: the changed list row (filters, `total`, page of 50) and two new rows.
- [ ] **Step 6: Run.** `go test -count=1 ./internal/api/ ./internal/docs/` pass, and `go run internal/api/gen_openapi.go && make openapi` leaves `git status` showing only the intended changes.
- [ ] **Step 7: Commit.**

---

### Task 3: Page and filter Recent repairs (client, helpers and component)

**Files:**
* Create: `web/src/lib/repairs/history.ts`, `web/src/lib/repairs/RepairHistory.svelte`, `web/unit/repair-history.test.ts`, `web/tests/repair-history.spec.ts`
* Modify: `web/src/lib/api/client.ts` (line 1323), `web/src/lib/settings/EliRepairs.svelte`

**Interfaces:**
* Consumes: Task 2's generated types and routes.
* Produces, in `history.ts`:
  ```ts
  export type RepairGroup = 'all' | 'attention' | 'active' | 'passed';
  export type RepairTrigger = 'any' | 'automatic' | 'requested';
  export interface RepairFilters { group: RepairGroup; repo: string; trigger: RepairTrigger }
  export const NO_REPAIR_FILTERS: RepairFilters; // { group: 'all', repo: '', trigger: 'any' }
  export const filtered = (f: RepairFilters) => boolean; // any filter set
  export const isActive = (state: string) => boolean; // queued, working, checking
  export const stateLabel = (state: string) => string; // moved verbatim from EliRepairs.svelte
  export function listQuery(f: RepairFilters, limit: number, offset: number): Query<'listEliRepairs'>; // leaves out 'all', 'any' and a blank repo; repo trimmed
  export function selectionQuery(f: RepairFilters): Query<'selectEliRepairs'>;
  export function offsetAfterClear(offset: number, limit: number, total: number, cleared: number): number; // the largest page start still inside total - cleared, never negative
  ```
  In `client.ts`: `listEliRepairs(query?, signal?)`, `selectEliRepairs(query?, signal?)`, `bulkEliRepairAction(body)`, shaped like `selectProvisioning` and `controlProvisioning`.
  `RepairHistory.svelte` props: `onretry: (target: { repo: string; pull: number }) => void`; it exports `reload(): Promise<void>`.

- [ ] **Step 1: Write failing unit tests** in `web/unit/repair-history.test.ts`: `listQuery` omits defaults and trims (`{group:'all', repo:' site ', trigger:'any'}` at limit 50, offset 0 gives `{ repo: 'site', limit: 50, offset: 0 }`); `isActive` is true for exactly `queued`, `working`, `checking`; `stateLabel('checks_failed')` is `Checks failed` and an unknown state comes back as given; `offsetAfterClear(100, 50, 102, 1)` is 100 (the new total of 101 still has a row on the page starting at 100), `(100, 50, 101, 1)` is 50 (the new total of 100 leaves the page at 100 empty), `(100, 50, 101, 2)` is 50, `(0, 50, 3, 3)` is 0.
- [ ] **Step 2: Run to fail.** `cd web && node --import ./unit/register.mjs --test unit/repair-history.test.ts`.
- [ ] **Step 3: Implement `history.ts` and the three client functions.**
- [ ] **Step 4: Write the failing browser spec** `web/tests/repair-history.spec.ts` (`test.use(browserOverride)`). Mock `**/api/v1/assistant/repairs*` with a small in-test model: 120 generated rows cycling through the states, three repositories and both triggers, answering `limit`, `offset` and `group` by slicing and filtering, and recording each request URL. Tests:
  * `the history is paged by the server and says how many there are`: on `/repairs`, the pagination reads `1–50 of 120 repairs`; Next page requests `offset=50&limit=50`; changing Rows to 25 requests `limit=25` and `offset=0`.
  * `a filter reaches repairs beyond the first page and resets the page`: from page two, choosing Needs attention requests `group=attention` with `offset=0`; typing `widgets` in Repository sends one request with `repo=widgets` after the pause; Triggered by Automatic sends `trigger=automatic`.
  * `a filter that matches nothing says so and can be cleared`: the mock returns `{items: [], total: 0}` for `repo=none`; the text `No repairs match these filters` shows with a **Clear filters** button that restores the list. With no filters and `total: 0` the text is `No repairs yet`, with no filter controls.
  * `the history keeps its page and filters through the refresh`: with the page clock installed, go to page two with a filter, advance ten seconds, and the next request carries the same filter and offset.
  * Both desktop and mobile run it; add the sideways-scroll check used in `repairs.spec.ts`.
- [ ] **Step 5: Build the component.** Move the `Recent repairs` card, `copyRepair`, `stateLabel` and the related styles out of `EliRepairs.svelte`. State: `filters`, `limit` (`prefs.pageSize('eli-repairs')`, saved with `prefs.setPageSize`), `offset`, the page and `total`. A 300 ms pause on the repository box; any filter change sets `offset` to 0; its own ten-second timer re-reads with the current query and a request guard so a stale answer is dropped. Labels: filter fields `Show` (the four labels used today), `Repository` and `Triggered by`; pagination noun `repairs`. Show the filter bar when a filter is set or `total > 0`. In `EliRepairs.svelte` mount `<RepairHistory bind:this={history} onretry={...} />` with the existing retry behaviour, stop fetching repairs in `refresh()`, and call `history?.reload()` from `reload()`, from `onqueued` and after a successful `act`.
- [ ] **Step 6: Run.** `cd web && npm run lint && npm run check && npm run test:unit`; then `make build VERSION=dev` and `PLAYWRIGHT_CHROMIUM=/opt/pw-browsers/chromium npx playwright test tests/repair-history.spec.ts tests/repairs.spec.ts --project=chromium --project=mobile`.
- [ ] **Step 7: Commit.**

---

### Task 4: Select repairs and clear them from the history

**Files:**
* Modify: `web/src/lib/repairs/RepairHistory.svelte`, `web/src/lib/repairs/history.ts`, `web/unit/repair-history.test.ts`, `web/tests/repair-history.spec.ts`, `docs/eli.md` (near line 466, "Eli repairs and Settings, Eli AI Assistant show repair history")

**Interfaces:**
* Consumes: Task 3's component and helpers; `selectEliRepairs`, `bulkEliRepairAction`.
* Produces, in `history.ts`:
  ```ts
  export interface ClearReport { cleared: number; skipped: { id: string; error: string }[] }
  export function clearReport(results: readonly { id: string; ok: boolean; error?: string }[]): ClearReport;
  export function clearConsequences(): readonly string[]; // the two sentences the dialog shows
  ```

- [ ] **Step 1: Write failing unit tests**: `clearReport` counts `ok` results and lists the rest with their errors, in order; `clearConsequences()` returns `['They still count towards each repository's daily limit.', 'Eli will not repair its own commits from them again.']`.
- [ ] **Step 2: Write the failing browser tests** (extend the model: a selection route returning clearable ids and `truncated`, and a bulk route that removes the ids it clears). Tests:
  * `rows are selected one at a time, by the page, and by everything that matches`: the header checkbox named `Select the 50 repairs shown` selects the clearable rows and the bar reads `N selected`; **Select all 120 matching** calls the selection route with the current filters and the bar then reads that count; a truncated reply reads `500 of 700 selected`.
  * `an in-flight repair cannot be selected`: a `working` row's checkbox is disabled and titled `Still running`.
  * `clearing says what it does, then clears and reports`: **Clear from history** opens a dialog whose text names both consequences; confirming posts `{action: 'clear', ids}`; the bar and the dialog close; a skipped row shows in a disclosure with its reason; the list and total update.
  * `a selection survives the refresh and a page change, and a filter change drops it`.
  * `clearing the last row on the last page steps back a page`.
- [ ] **Step 3: Run to fail.**
- [ ] **Step 4: Implement.** One `Set` of chosen ids, kept across polls and pages; a header `Checkbox` (indeterminate when some shown rows are chosen) labelled `Select the {n} repairs shown`; row checkboxes labelled `Select repair {repo} #{pull}`; **Select all {total} matching** appears once every shown clearable row is chosen and more match, and is disabled while it loads; an `aria-live` bar with **Clear from history** and **Clear selection**; `ConfirmDialog` with title `Clear from history`, `tone="default"`, `confirmLabel="Clear from history"`, the description `The repairs stop showing in this list.` and `clearConsequences()`. After a clear: empty the set, apply `offsetAfterClear`, reload, and show `clearReport` with skipped rows in a `<details>`. A filter change empties the set. Tokens only; wrap on a phone.
- [ ] **Step 5: Document.** `docs/eli.md`: one paragraph on filtering, paging and **Clear from history**, saying plainly that a cleared repair still counts towards the daily limit and the own-commit guard and cannot be restored from the page.
- [ ] **Step 6: Run** the Task 3 verification again plus `go test -count=1 ./internal/docs/`.
- [ ] **Step 7: Commit.**

---

## Final verification (before the last push)

`make build-nogui`, `make fmt` then `make lint`, `go test -race -count=1 ./...`, `cd web && npm run test:unit`, then `make build VERSION=dev` and the Playwright specs `repair-history`, `repairs`, `assistant` and `a11y` on both projects. `go run internal/api/gen_openapi.go && make openapi` and `go mod tidy` leave no unintended diff. No em dashes in the diff.

## Not in this plan

Screenshots of the repairs page (`make screenshots` needs Pillow) will show the new filter bar and pagination and should be refreshed as a separate step; restoring cleared rows; hard deletion; sorting; MCP and CLI exposure.
