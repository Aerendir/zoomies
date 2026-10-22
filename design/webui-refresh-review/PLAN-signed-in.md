# Web UI refresh, signed-in pages implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a mock, in the portal's look, of every signed-in page: an interactive review page with the page today beside the mock, one shared fleet of data, the checker over every combination, and a README.

**Architecture:** Pack 1's method with one concept instead of three. Plain-HTML mocks on the product's own `tokens.css`, served by the dependency-free server, switched by `data-theme` and `data-ui-style`, and judged by the Playwright checker. A small kit of portal-style parts (shell, page header, card, icon tile, pill, stat, stage, table card, steps) is built once and every screen is made of it, so the pages share a look because they share parts. Every screen draws from one fixture fleet held to the REST API's schema, so a count on the Overview agrees with the rows on Runners and no mock shows a column the API cannot supply.

**Tech Stack:** HTML, CSS, vanilla ES modules, Node 22 (`node:http`, `node:test`), `@playwright/test` from `web/node_modules`. No new dependency.

**Spec:** `design/webui-refresh-review/SPEC.md`, "Direction chosen after pack 1". Read it, then `web/src/lib/public/` and `docs/brand.md` ("Using the system in the product") for the look as it is implemented.

## Global Constraints

* No change under `web/src`, `internal/`, `api/` or `cmd/`. No new dependency.
* Tokens only in the mocks: no raw colour, and no raw value that has a token. A new token may appear only in `signed-in/kit-tokens.css`, and the README lists each against the `docs/ui-guidelines.md` section it would change.
* Mocks need no API: they send no request and use `fixture.js`, whose every field is one the REST API returns, held to `api/openapi.yaml` by a test.
* Identity stays: logo and backdrop choices, Eli, paw artwork, from `web/public/brand`; never redraw them.
* Status colours keep their fixed mapping; a state is never carried by colour alone (a word or a shape is always there); status colours are not reused for decoration.
* Contrast 4.5:1 for text, 3:1 for control edges; widths 1440, 375, and 320 for reflow; Classic's known control edge stays in `KNOWN`.
* A table stays a table and scrolls inside its own card, never the page (`docs/ui-guidelines.md`, "Tables use the space they need").
* Prose: British spelling, no em dash, no spaced double hyphen. Commit messages are imperative sentences in plain prose and end with these two lines:
  `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01GkdhFBr16YDz8GRKZd9MHf`.
* Work on branch `claude/jolly-maxwell-d96vdo`; push after every task; the pull request is eyupio/zoomies#1207. The owner's portal screenshots are reference only and are never committed.

## Review Focus

1. A 300-character repository, workflow or job name and a 120-character unbroken runner label in a table card truncate or wrap with the full text reachable, and never widen the page at 320px.
2. Every table on a phone scrolls inside its card or becomes cards, with every row action reachable by keyboard and at 44px or more.
3. Every list page draws four states, not one: loading, empty, an error with a retry, and rows shown stale after a failed refresh.
4. The sidebar's entries at 375px: a bottom bar with the four primary ones and a menu with the rest, which opens, holds focus, closes on Escape and returns focus to its opener.
5. A status pill keeps its meaning in forced-colours mode and in greyscale: the word is there and the shapes differ.
6. A stage (the dark panel) never carries a status colour that is only legible on the theme it was drawn in.

## File structure

```text
design/webui-refresh-review/
  tools/serve.mjs                         gains the /signed-in/ mount
  public/review.js, review-state.js       learn a registry named by a meta tag, and a concept list
  public/icons.js                         the generator's list grows to the glyphs the shell and pages draw
  signed-in/
    README.md  findings.md
    mockups.html                          the review page, naming its registry
    kit.css, kit.js, kit-tokens.css       the portal-style parts and their tokens
    fixture.js, fixture.test.mjs          one fleet, held to the API schema
    screens/                              index.js and one module per page
    current-*.png, current-measurements.json
    concept-*.png, viewport-review.json
```

## Tasks

### Task 1: Let the review page serve a second pack

**Files:** Modify `tools/serve.mjs`, `tools/serve.test.mjs`, `public/review.js`, `public/review-state.js`, `public/review-state.test.mjs`, `public/mockups.test.mjs`, `tools/make-icons.mjs`; create `signed-in/mockups.html` as a stub.

**Interfaces:**
- `serve` also mounts `/signed-in/` to `design/webui-refresh-review/signed-in`; pack 1's paths are unchanged.
- `parseHash(hash, screens, { mode, concepts })`: `concepts` defaults to pack 1's four; a concept the pack does not offer falls back to the first offered after `current`.
- `review.js` reads `<meta name="review-screens" content="...">` and `import()`s that registry, defaulting to `./screens/index.js`; a registry may export `CONCEPTS` and `LABELS` to say what its concept control offers.

- [ ] **Step 1:** Tests first: the new mount serves a file and refuses traversal; `parseHash` with `concepts: ['current', 'a']` turns `concept=b` into `a`; the stub page lists its registry's screens. Expected: FAIL.
- [ ] **Step 2:** Implement; run every pack 1 test and `check.mjs` (1,044 combinations, 0 findings, 6 known). Expected: PASS and the same totals.
- [ ] **Step 3:** Add the glyphs the shell and pages need to `make-icons.mjs` and regenerate `public/icons.js`. Commit "Let the review page serve a second pack".

### Task 2: Today's signed-in pages, photographed

**Files:** Create `tools/capture-signed-in.mjs`, `signed-in/current-*.png`, `current-measurements.json`, `findings.md`.

Start the monitoring fixture (`web/tests/support/serve.mjs`, a seeded fleet with authentication off on loopback) and the stuck fleet, and photograph every route in `web/src/lib/router.ts` that needs no id, plus the first pool, host and runner's detail pages, at 1440x900 and 375x812 in Mission Control dark, whole page and no taller than 2400px. Record each page's height, grid column count, and whether the page scrolls sideways at 375.

- [ ] **Step 1:** Write the script; run it. Expected: a PNG per page and width, and the measurements.
- [ ] **Step 2:** Open each PNG with Read; note in `findings.md`, with the number, what the look would change on each page.
- [ ] **Step 3:** Commit "Photograph the signed-in pages as they are".

### Task 3: The kit, the fixture, and the shell and Overview

**Files:** Create `signed-in/kit.css`, `kit.js`, `kit-tokens.css`, `fixture.js`, `fixture.test.mjs`, `screens/index.js`, `screens/shell.js`, `screens/overview.js`; fill in `mockups.html`.

**Interfaces:**
- `FLEET` and `fleetCounts(FLEET)`, frozen, so no screen types a number of its own.
- `kit.js` exports `shell({ page, title, eyebrow, subtitle, actions, body })`, `card`, `tile`, `pill({ state })`, `stat`, `stage`, `tableCard`, `steps`, each returning markup, and a `pages` list the sidebar and bottom bar read.
- The screen contract is pack 1's: `{ id, title, states, labels, currentAlt, render(concept, state) }`.

- [ ] **Step 1:** `fixture.test.mjs` first: every field is in the matching schema in `api/openapi.yaml`; counts agree; one runner has a 120-character label with no break and one job a 300-character repository, workflow and job name. Expected: FAIL; then implement `fixture.js`. Expected: PASS.
- [ ] **Step 2:** Build the kit and the shell (expanded and collapsed sidebar, the phone's bar and its menu), then the Overview in its states (first run with the checklist, healthy, attention, loading, failed first fetch with retry).
- [ ] **Step 3:** Check it at 320, 375 and 1440, look at it, fix what is crowded, and show the owner the screenshots. Commit "Add the portal-style kit, the shell and the Overview".

### Task 4: Monitoring pages

**Files:** Create `screens/queue.js`, `jobs.js`, `runners.js`, `workflows.js`.

Each in the states of Review Focus 3, with a table card, filter pills, status pills, mono ids, and the row actions as buttons. Tests first for the long-name and phone cases, then the screens, then check and look. Commit "Mock the monitoring pages in the portal style".

### Task 5: Fleet management pages

**Files:** Create `screens/pools.js`, `hosts.js`, `providers.js`, `machines.js`, and the detail pages of a pool, a host and a runner.

Cards with icon tiles for what a pool, host or provider is, the numbered steps for adding one, and the same four states. Commit "Mock the fleet management pages in the portal style".

### Task 6: Insight and repairs pages

**Files:** Create `screens/kennel.js`, `repairs.js`, `ai-context.js`, `usage.js`, `comparisons.js`.

A stage for the one answer each page gives (is anything wrong, what did it cost, what changed), cards for the evidence under it, and what has changed since the Kennel Club and Eli repairs reviews. Commit "Mock the insight and repair pages in the portal style".

### Task 7: Settings and setup pages

**Files:** Create `screens/settings.js`, `audit.js`, `installations.js`, `migrate.js`, `wizard.js`.

Settings as sectioned cards with icon tiles and the forms inside them, the audit as a table card, and the setup flows as the portal's `01 02 03` steps. Commit "Mock the settings and setup pages in the portal style".

### Task 8: Check, capture, document

**Files:** Create `signed-in/concept-*.png`, `viewport-review.json`, `README.md`, `readme.test.mjs`; modify `tools/capture.mjs` and `tools/check.mjs` to take `--pack`.

- [ ] **Step 1:** `check.mjs --pack signed-in --out signed-in/viewport-review.json`. Expected: 0 findings, only the Classic control edge known.
- [ ] **Step 2:** Capture each page's mock in Mission Control dark at 1440 and 375, and Classic light at 1440, whole page; open every PNG and re-shoot any that is clipped or misleading.
- [ ] **Step 3:** Write the README in pack 1's shape (how to view, captures table, what changed on each page and why, where it departs from the portal and why, proposed tokens, the Classic edge, Not claimed) with a test tying it to the captures, the links and the check totals, as pack 1's is.
- [ ] **Step 4:** `go test -count=1 ./internal/docs` and every node test. Expected: all pass. Push, update the pull request, tell the owner.

## Out of this plan

Implementing any of these pages in the product. Each is a separate task, in the order the owner chooses, and starts from the mock for that page.
