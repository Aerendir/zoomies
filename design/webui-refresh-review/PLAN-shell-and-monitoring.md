# Web UI refresh, pack 2 (shell and monitoring) implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce the concept pack for the app shell and the five monitoring pages: an interactive review page, captures of today's signed-in UI, three concepts per screen, the same checker proving them, and a README that recommends one concept per screen.

**Architecture:** Pack 1's method unchanged: plain-HTML mockups on the real `tokens.css`, switched by `data-theme` and `data-ui-style`, served by the dependency-free loopback server and judged by the Playwright checker. What changes is the material. These pages are dense, live and data-bearing, so every screen draws from one shared fixture fleet, so that a count on the Overview agrees with the rows on Runners, and the fixture is held to the REST API's own schema so a mockup cannot show a column the API cannot supply.

**Tech Stack:** HTML, CSS, vanilla ES modules, Node 22 (`node:http`, `node:test`), `@playwright/test` from `web/node_modules`. No new dependency.

**Spec:** `design/webui-refresh-review/SPEC.md`, section "Rolling on". Read it first, then `public/README.md` for the direction pack 1 recommended, which is this pack's baseline: one shared frame, one shared callout, the answer before the evidence, and the phone's primary action kept in reach. This plan only records what the spec leaves to the implementer.

**Earlier reviews this pack builds on, and does not repeat:** `design/overview-review` (Calm operations, implemented on 19 October 2026; see its `implemented/README.md`). The Overview's work in this pack is to say what has changed since and what the captures show now, not to propose it again.

## Global Constraints

* No change under `web/src`, `internal/`, `api/` or `cmd/`. No new dependency.
* No raw colour in A or B. A new token may appear only in `shell-and-monitoring/concept-c-tokens.css`, and the README lists each against the `docs/ui-guidelines.md` section it would change.
* Mockups need no API: they send no request, and use the shared fixture. Every field a mockup shows is one the REST API returns, and a test holds that to `api/openapi.yaml`, as pack 1's Fleet status does.
* Identity stays: Zoomies logo and backdrop choices, Eli, paw artwork. Use files from `web/public/brand`; never redraw them.
* Status colours keep their fixed mapping, and a state is never carried by colour alone: the word or a shape is always there.
* Contrast: 4.5:1 for text, 3:1 for control edges. Widths: 1440px and 375px, plus 320px for reflow. Classic's known control edge stays in `KNOWN`.
* Tables follow `docs/ui-guidelines.md`, "Tables use the space they need": a grid scrolls inside its own region and the page never scrolls sideways.
* Prose: British spelling, no em dash, no spaced double hyphen. Commit messages are imperative sentences in plain prose and end with these two lines:
  `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01GkdhFBr16YDz8GRKZd9MHf`.
* Work on branch `claude/jolly-maxwell-d96vdo`; push after every task; the pull request is eyupio/zoomies#1207 until it is split.

## Review Focus

1. A 300-character repository, workflow and job name, and a 120-character unbroken runner label, in a grid row must truncate or wrap with the full text still reachable, and must not widen the page at 320px (Tasks 5 to 8).
2. A grid on a phone must scroll inside its own region, or become cards, with every row action still reachable by keyboard and by touch at 44px or more (Tasks 5 to 8).
3. Every grid has four states that are drawn, not assumed: loading, empty, an error with a retry, and "showing stale rows" after a failed refresh (Tasks 5 to 8).
4. The twelve navigation entries at 375px: the bottom bar keeps the four primary ones, the rest live in the menu, and the menu opens, traps focus, and closes on Escape and returns focus to its opener (Task 3).
5. A state badge keeps its meaning in forced-colours mode and in greyscale: the word is present, and the shapes differ (Tasks 5 to 8).

## File structure

```text
design/webui-refresh-review/
  tools/serve.mjs, check.mjs, capture.mjs, capture-current.mjs   gain a --pack argument, default public
  public/review.js, review-state.js, ui.js, components.css, icons.js   shared by both packs, mounted at /shared/
  shell-and-monitoring/
    README.md
    mockups.html             the review page, with the screens registry named by a meta tag
    fixture.js               one deterministic fleet: pools, hosts, runners, jobs, runs, events
    fixture.test.mjs         held to api/openapi.yaml, and to the invariants a fleet must keep
    screens/                 shell.js, overview.js, runners.js, queue.js, jobs.js, workflows.js, index.js
    concept-{a,b,c}.css
    concept-c-tokens.css, concept-c-pairs.json
    current-*.png, current-measurements.json, findings.md
    concept-*.png, viewport-review.json
```

## Tasks

### Task 1: Let the tools serve a second pack

**Files:** Modify `tools/serve.mjs`, `tools/serve.test.mjs`, `tools/check.mjs`, `tools/capture.mjs`, `public/review.js`, `public/mockups.test.mjs`; create `shell-and-monitoring/mockups.html` as a stub.

**Interfaces:**
- Produces `serve(port, { pack = 'public' })`: `/review/` serves `<pack>/`, and `/shared/` serves `public/` whichever pack is chosen. Pack 1's `/review/` paths are unchanged.
- Produces `--pack <name>` on `check.mjs`, `capture.mjs` and `capture-current.mjs`, and `run({ url, ... })` unchanged.
- `review.js` reads its screens registry from `<meta name="review-screens" content="./screens/index.js">` with a dynamic `import()`, and keeps pack 1 working by defaulting to that path.

- [ ] **Step 1:** Tests first: `serve` with `{ pack: 'shell-and-monitoring' }` serves the pack's own file at `/review/` and pack 1's `review.js` at `/shared/review.js`, and still refuses traversal out of both. Pack 1's test suite and `check.mjs` run unchanged and pass. Expected: new tests FAIL, old ones PASS.
- [ ] **Step 2:** Implement, run every pack 1 test and `check.mjs` (1,044 combinations, 0 findings, 6 known). Expected: PASS and the same totals.
- [ ] **Step 3:** Commit "Let the review tools serve a second pack".

### Task 2: Today's signed-in UI, photographed, and the findings confirmed

**Files:** Modify `tools/capture-current.mjs`; create `shell-and-monitoring/current-*.png`, `current-measurements.json`, `findings.md`.

The material is the monitoring fixture (`web/tests/support/serve.mjs`, a seeded demo fleet with authentication off on loopback) and the stuck fleet (`serve-stuck.mjs`). Photograph, at 1440x900 and 375x812 in Mission Control dark, and in Classic light at 1440: the shell on Overview, Overview, Runners, Queue, Jobs, Workflows, and the phone's More menu open. Record, for each, the grid's column count, the row height, whether the page or the grid scrolls sideways at 375, where the primary action sits, and the position of Eli.

Hypotheses from reading the source, each to be confirmed or dropped in a browser before it reaches the README:

1. Jobs, Queue and Workflows each hand-write their own filter and status-view controls; Runners alone uses `FilterBar.svelte`.
2. The same state badge is drawn three ways across Runners, Jobs and Queue, and `StateCell.svelte` is used by only some.
3. At 375px the grids are the part of the product with the most sideways scrolling, and the row action is the first thing out of reach.
4. The sidebar's twelve entries read as one list when collapsed, because only icons remain.
5. Each page opens with a different amount of explanation before its first row.
6. A question, not a finding: whether Queue and Jobs are two pages for one question ("what is waiting, and why") and should read as one in the navigation.

- [ ] **Step 1:** Extend `capture-current.mjs` with a signed-in mode that starts `serve.mjs` and `serve-stuck.mjs`, reusing `look.mjs`. Run it. Expected: eleven or more PNGs and a JSON file of measurements.
- [ ] **Step 2:** Open every PNG with Read. Settle each hypothesis as confirmed, dropped or open, with the number, in `findings.md`.
- [ ] **Step 3:** Commit "Photograph the signed-in monitoring pages as they are".

### Task 3: The fixture, the review page and the App shell concepts

**Files:** Create `shell-and-monitoring/fixture.js`, `fixture.test.mjs`, `mockups.html`, `screens/index.js`, `screens/shell.js`, `concept-a.css`, `concept-b.css`, `concept-c.css`, `concept-c-tokens.css`, `concept-c-pairs.json`.

**Interfaces:**
- Produces `FLEET`, a frozen object with `pools`, `hosts`, `runners`, `jobs`, `runs` and `events`, and `fleetCounts(FLEET)` returning the numbers every screen shows, so no screen types a number of its own.
- Produces the pack-1 screen contract: `{ id, title, states, labels, currentAlt, render(concept, state) }`.

- [ ] **Step 1: Write `fixture.test.mjs`.** Every field the fixture uses is in the matching schema in `api/openapi.yaml`; the counts agree (runners per pool sum to the pool's live count, jobs per state sum to the queue's total); one runner has a 120-character label with no break, and one job a 300-character repository, workflow and job name.
- [ ] **Step 2:** Run. Expected: FAIL. Implement `fixture.js`. Expected: PASS.
- [ ] **Step 3:** Build the review page and `shell.js`, states: expanded rail, collapsed rail, phone with the bottom bar, phone with More open, instance notice and visitor notice showing, and the shortcut sheet. Three concepts: A tidies grouping and the hints, B moves the phone's primary actions to where a thumb rests and gives the top bar one job, C turns the rail into a command-first shell around the palette.
- [ ] **Step 4:** `node tools/check.mjs --pack shell-and-monitoring --screen shell`. Expected: no findings at 320, 375 and 1440. A test drives the More menu with the keyboard: it opens, focus is inside it, Escape closes it, focus returns to its opener.
- [ ] **Step 5:** Look at A, B and C with screenshots and Read; fix anything crowded or clipped. Commit "Add the monitoring review page, a shared fleet and three app shell concepts".

### Task 4: Overview, since the Calm operations review

**Files:** Create `screens/overview.js`.

- [ ] **Step 1:** In `findings.md`, list what `design/overview-review/implemented/README.md` shipped and what the captures from Task 2 show now, and write only the differences. Where nothing has changed, say so and propose nothing.
- [ ] **Step 2:** Tests first for the states: first run, healthy, attention (three failures), loading, and a failed first fetch with retry; then build the three concepts as the smallest change that answers what the captures show. Build on the frame and callout pack 1 recommended.
- [ ] **Step 3:** Check, look, commit "Add three Overview concepts".

### Task 5: Runners

**Files:** Create `screens/runners.js`.

States: the grid with twelve runners, one busy filter applied, empty (no runners and no pool), loading, an error with retry, and stale rows after a failed refresh. Concepts: A the same grid with the state cell and the filter bar made the shared ones, B the grid as cards below 800px with the row action always visible, C a split view with the list and a detail pane for the selected runner.

- [ ] **Step 1:** Tests first: the long label wraps or truncates with its full text in the accessible name and a tooltip; at 320px the page does not scroll sideways; every row action is reachable by Tab and is at least 44px on a phone.
- [ ] **Step 2:** Build; check; look; commit "Add three Runners concepts".

### Task 6: Queue

**Files:** Create `screens/queue.js`.

States: jobs waiting, with provisioning controls; nothing waiting; the unmatched-labels case; loading; error. Concepts: A the grid with the wait broken into its causes at the top, B the answer first ("why has this not started") and the grid under it, C a lane view by pool.

- [ ] **Steps 1 and 2:** As Task 5, naming the 300-character job. Commit "Add three Queue concepts".

### Task 7: Jobs

**Files:** Create `screens/jobs.js`.

States: history with filters applied, sorted by queue wait; the unmatched filter; empty; loading; error. Concepts: A the shared filter bar and a sticky header row, B status views as the primary control and the filters behind one button, C jobs grouped by workflow run with the run as the heading.

- [ ] **Steps 1 and 2:** As Task 5. Commit "Add three Jobs concepts".

### Task 8: Workflows

**Files:** Create `screens/workflows.js`.

States: runs with one expanded in place to its jobs; a run still in progress; empty; loading; error. Concepts as Jobs, with the expansion kept in A and B, and C as a timeline of runs.

- [ ] **Steps 1 and 2:** As Task 5. Commit "Add three Workflows concepts".

### Task 9: Captures and the record

**Files:** Create `shell-and-monitoring/concept-*.png`, `viewport-review.json`; modify `tools/capture.mjs`.

- [ ] **Step 1:** `capture.mjs --pack shell-and-monitoring`: each concept at 1440 and 375 in Mission Control dark, and the recommended one in Classic light, each in its most telling state, with the window made as tall as the page.
- [ ] **Step 2:** `check.mjs --pack shell-and-monitoring --out shell-and-monitoring/viewport-review.json`. Expected: 0 findings, only the Classic control edge known.
- [ ] **Step 3:** Open every PNG, re-shoot any that is clipped or misleading, then commit "Capture the monitoring concepts and record the checks".

### Task 10: README, recommendation, gates

**Files:** Create `shell-and-monitoring/README.md` and `readme.test.mjs`.

- [ ] **Step 1:** Write the README in pack 1's shape: date, how to view, a captures table, confirmed findings only, each concept in two sentences, one recommendation per screen with its reason, the Svelte files each would touch, the tokens C proposes and the guideline section each would change, the Classic edge, and "Not claimed" copied from the spec. A test ties it to the captures, the recommendations and the check totals, as pack 1's does.
- [ ] **Step 2:** `go test -count=1 ./internal/docs` and `node --test "design/webui-refresh-review/tools/**/*.test.mjs" "design/webui-refresh-review/*/**/*.test.mjs"`. Expected: all pass.
- [ ] **Step 3:** `git status` shows no stray build output. Push; update the pull request description.
- [ ] **Step 4:** Write the plan for pack 3 (fleet management) from the same method.

## Out of this plan

Pools, Hosts, Providers and Machines (pack 3), Kennel Club, Eli repairs, AI context, Usage and Comparisons (pack 4), and Settings, Audit, Installations, Migrate and the wizards (pack 5). Each gets its own plan, written after the pack before it.
