# Web UI refresh, pack 1 (public pages) implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce the concept pack for the five public screens: an interactive review page, captures of today's UI, three concepts per screen, a checker that proves them, and a README that recommends one concept per screen.

**Architecture:** Plain-HTML mockups link the repository's own `tokens.css`, so colours cannot drift, and are switched by `data-theme` and `data-ui-style` on `<html>`, exactly as the app does. A small dependency-free Node server mounts the real assets. Node scripts in `tools/` drive Chromium through the `@playwright/test` already installed in `web/`, to capture the real controller and to check every mockup combination.

**Tech Stack:** HTML, CSS, vanilla ES modules, Node 22 (`node:http`, `node:test`), `@playwright/test` from `web/node_modules`. No new dependency.

**Spec:** `design/webui-refresh-review/SPEC.md`. Read it first; this plan only records what the spec leaves to the implementer.

## Global Constraints

* No change under `web/src`, `internal/`, `api/` or `cmd/`. No new dependency.
* No raw colour in A or B. A new token may appear only in `public/concept-c-tokens.css`.
* Mockups need no API: they send no request, and use illustrative data.
* Identity stays: Zoomies logo and backdrop choices, Eli, paw artwork. Use files from `web/public/brand`; never redraw them.
* Status colours keep their fixed mapping. The fleet status page may use only fields of `GET /api/v1/status`.
* Never echo the address in the not-found page: a path taken from the URL is text anybody can put in a link.
* Contrast: 4.5:1 for text, 3:1 for control edges. Widths: 1440px and 375px, plus 320px for reflow.
* Prose: British spelling, no em dash, no spaced double hyphen. Commit messages are imperative sentences in plain prose and end with these two lines:
  `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01GkdhFBr16YDz8GRKZd9MHf`.
* Work on branch `claude/jolly-maxwell-d96vdo`; push after every task; pull request is eyupio/zoomies#1207.

## Review Focus

1. A 300-character client name and a 120-character unbroken host in the consent and sign-in screens must wrap, not overflow (Tasks 5 and 7).
2. 320px wide (WCAG reflow) must not scroll sideways on any screen (Tasks 3 and 10).
3. Opening `mockups.html` from `file://`, or with `tokens.css` unreachable, must show a visible explanation, not a blank or unstyled page (Task 5).
4. A hash naming an unknown screen, concept, style or mode must fall back to defaults without an exception (Task 5).
5. On the status page, state must survive forced-colours mode: the shape and the word carry it, not colour alone (Task 8).

## File structure

```text
design/webui-refresh-review/
  tools/lib/browser.mjs        launch Chromium from web's Playwright
  tools/serve.mjs              the loopback review server (+ serve.test.mjs)
  tools/check.mjs              overflow, console, name, heading, tab-order and contrast checks (+ check.test.mjs)
  tools/capture-current.mjs    the real controller, photographed
  tools/capture.mjs            the mockups, photographed
  public/mockups.html          shell: review bar, stage, noscript and failure notes
  public/review.css           review chrome only (never product styling)
  public/review-state.js      parseHash / formatHash, pure (+ review-state.test.mjs)
  public/review.js            wires the bar, the hash and the stage
  public/screens/{signin,first-account,connect-app,fleet-status,not-found}.js
  public/concept-a.css  concept-b.css  concept-c.css  concept-c-tokens.css  concept-c-pairs.json
  public/README.md  current-*.png  concept-*.png  viewport-review.json
```

The tree in `SPEC.md` omits `tools/` and the per-concept files; Task 2 updates it.

## The concept brief (decisions the implementer must not re-open)

| Screen | A, evolution | B, evolution | C, bold |
| --- | --- | --- | --- |
| Sign in | Keep the split layout. One shared brand header (mark, no black stage on a phone) so the form is above the fold at 375x667. Host becomes "Signing in to {host}". Shared callout for failures. | Drop the brand panel for the same centred card the first-account and consent screens use, so all pre-auth screens share one frame. The pitch becomes one quiet footer line. | Full-bleed scene using the existing orbital backdrop, the tagline large on the left, the form in a raised panel on the right. New tokens for the scrim and panel. |
| First account | Same card. One-sentence lede. The token's location is shown at once as per-install tabs (Compose, `zoomies init`, systemd, other) with a copy button. | Two columns from 960px: "Find your setup token" with the install selector, beside the form. On a phone the token help folds into a disclosure. | A left rail listing the four steps after sign-up (GitHub App, host, pool, workflow), echoing the Overview checklist, beside the form. |
| Connect an app | Summary block (who, where it returns, what it may do) first. Warning keeps its note pattern, reworded. Primary button reads "Allow {client}". | Role choice as large cards with a plain consequence each, and a "never" list. Buttons stick to the bottom on a phone. | A two-end diagram, Zoomies and the client, with the redirect host as the verified destination. Inline SVG, no image. |
| Fleet status | Brand header, larger state banner, facts as four tiles, reasons list. | One plain sentence first ("Jobs are starting normally. Typical wait: under a minute."); detail below in a disclosure. | Two-column board on wide screens: state hero, queue tiles, reasons. New tokens for the hero. |
| Not found | Keep the empty state; add "Go back" beside "Go to the overview". | Offer the three most useful destinations from `shell/sections.ts`. | The existing mark large, with a calmer single action. |

## State matrix (how each state is produced)

| Screen | State | Produced by |
| --- | --- | --- |
| Sign in | password only; wrong credentials | real first-run controller (port 8098), real submit |
| Sign in | SSO plus password; SSO only; rate limited; unreachable; two-step verify; enrol; recovery codes; auth off | `page.route` stubs of `/api/v1/meta` and the auth routes, as `web/tests/two-step.spec.ts` does |
| First account | empty; field errors; weak password; caps lock | real first-run controller |
| First account | closed (409) | stub `POST /api/v1/auth/bootstrap` to 409 |
| Connect an app | loading, normal, loopback, refused, nothing to approve, expired | stubs, following `web/tests/mcp-oauth.spec.ts` |
| Fleet status | blocked, slowed | real status controller (port 8095, stuck fleet) |
| Fleet status | healthy, stale, sign-in required, not published | stub `GET /api/v1/status` with a `FleetStatus` fixture or 401/404 |
| Not found | the one state | real demo controller (`tests/support/serve.mjs`), path `/no-such-page` |

---

### Task 1: Toolchain and a real build

**Files:** none changed (`web/node_modules` and `internal/api/webdist` are ignored).

**Interfaces:** Produces a `zoomies` binary with the UI embedded, and a Chromium path in `PLAYWRIGHT_CHROMIUM`.

- [ ] **Step 1:** `cd /home/user/zoomies/web && npm ci`. Expected: exits 0. If the proxy blocks it, stop, record the error, and switch Task 4 to the fallback in the spec (existing `docs/screenshots`), saying so in the README.
- [ ] **Step 2:** `cd /home/user/zoomies && make build`. Expected: exits 0 and `./zoomies version` prints.
- [ ] **Step 3:** Confirm the browser exists: `ls /opt/pw-browsers/chromium-*/chrome-linux/chrome`. The installed Playwright pins a newer build than the one preinstalled, so `launch()` (Task 2) passes this path as `executablePath` and nothing downloads a browser.
- [ ] **Step 4:** Smoke test: in `web/`, run `node tests/support/serve-firstrun.mjs 8098 <a file in the scratchpad directory>` in the background, then `curl -s http://127.0.0.1:8098/healthz`. Expected: a healthy reply. Stop it. No commit.

### Task 2: The review server

**Files:** Create `tools/lib/browser.mjs`, `tools/serve.mjs`, `tools/serve.test.mjs`. Modify `SPEC.md` (tree).

**Interfaces:**
- Produces `launch(): Promise<Browser>` in `tools/lib/browser.mjs`, loading `@playwright/test` with `createRequire` from `web/package.json`. It uses `PLAYWRIGHT_CHROMIUM` when set and otherwise the newest `/opt/pw-browsers/chromium-*/chrome-linux/chrome`, because a shell variable does not survive between tool calls.
- Produces `serve(port?: number): Promise<{ url: string; close(): Promise<void> }>`, default port 8101, bound to `127.0.0.1`. Mounts: `/` to `web/public`, `/web/src/` to `web/src`, `/fonts/` to the `@fontsource` files `fonts.css` names, `/review/` to `public/`. Anything else is 404.

- [ ] **Step 1: Write `serve.test.mjs`** (`node:test`). Assertions: `/web/src/lib/styles/tokens.css` is 200 with `text/css`; `/brand/logo-black-transparent.svg` is 200 with `image/svg+xml`; `/review/mockups.html` is 404 until Task 5, so assert 200 for `/review/` plus a known file you create now (`public/.keep`); `/review/..%2F..%2Fgo.mod` and `/%2e%2e/go.mod` are 403 or 404; a POST is 405.
- [ ] **Step 2:** `node --test design/webui-refresh-review/tools/`. Expected: FAIL (module missing).
- [ ] **Step 3: Implement** `serve` and `launch` as above. Use `node:http`, an extension-to-type table for `.html .css .js .mjs .svg .png .webp .woff2 .json`, and reject any resolved path that leaves its mount. Add a CLI guard (`import.meta.url === pathToFileURL(process.argv[1]).href`) that listens and prints the URL.
- [ ] **Step 4:** Rerun the tests. Expected: all pass.
- [ ] **Step 5:** Edit the tree in `SPEC.md` to match "File structure" above. Run `go test -count=1 ./internal/docs`. Expected: ok.
- [ ] **Step 6: Commit** "Serve the review pack from the real tokens and brand assets".

### Task 3: The checker

**Files:** Create `tools/check.mjs`, `tools/check.test.mjs`.

**Interfaces:**
- Produces `contrast(fg: string, bg: string): number`, WCAG relative-luminance ratio for CSS colours the browser resolves to `rgb()` or `rgba()`.
- Produces `checkPage(page: Page): Promise<Finding[]>` with `Finding = { kind: 'overflow' | 'console' | 'name' | 'heading' | 'landmark' | 'tabindex' | 'focus' | 'contrast'; detail: string }`. Rules: `document.documentElement.scrollWidth <= innerWidth`; no `console.error` or `pageerror` since navigation; every `button`, `a[href]`, `input`, `select`, `textarea` has an accessible name; exactly one `h1`; one `main`; no `tabindex` above 0; pressing Tab up to 60 times, every stop in the stage shows a non-`none` outline or box-shadow (kind `focus`).
- Produces `checkPairs(page: Page, pairs: Pair[]): Promise<Finding[]>` (kind `contrast`) where `Pair = { fg: string; bg: string; min: 3 | 4.5 }` and `fg` and `bg` are custom property names read from `getComputedStyle(document.documentElement)`. The CLI passes, for A and B, `--z-text` and `--z-text-muted` on `--z-surface` (4.5), `--z-accent` on `--z-surface` (4.5), `--z-danger` on `--z-danger-subtle` (4.5) and `--z-border-strong` on `--z-surface` (3). For C it adds the pairs listed in `public/concept-c-pairs.json`, an array of `Pair`, written in Task 5 beside `concept-c-tokens.css`.
- CLI: `node tools/check.mjs [--screen id] [--out file]`. Runs every screen, concept, style (`classic`, `mission-control`) and mode at widths 320, 375 and 1440, writes JSON, exits 1 on any finding.

- [ ] **Step 1: Write `check.test.mjs`.** `contrast('rgb(0,0,0)','rgb(255,255,255)')` equals 21 to two places; `contrast('rgb(119,119,119)','rgb(255,255,255)')` is below 4.5. Using `launch()` and `page.setContent`, a page with a 2000px-wide block yields an `overflow` finding; an unlabelled button yields `name`; a page with two `h1` yields `heading`; a clean page yields `[]`. These prove the checker can fail.
- [ ] **Step 2:** Run the tests. Expected: FAIL.
- [ ] **Step 3: Implement** to the interfaces. Accessible name via `locator.ariaSnapshot()` or `page.accessibility`-equivalent role queries; do not add a library.
- [ ] **Step 4:** Rerun. Expected: pass. Commit "Check overflow, names, headings and contrast on every mockup combination".

### Task 4: Today's UI, photographed, and the findings confirmed

**Files:** Create `tools/capture-current.mjs`, `public/current-<screen>-<width>.png`, `public/findings.md` (working notes, folded into the README in Task 11).

**Interfaces:** Consumes `launch()`. Produces captures at `1440x900` and `375x812`, Mission Control dark, named `current-signin-1440.png` and so on for the five screens.

- [ ] **Step 1:** Implement the script per the state matrix: start each controller as `playwright.config.ts` does (`serve-firstrun.mjs 8098`, `serve-status.mjs 8095`, `serve.mjs 8099`), set `localStorage` `zoomies.ui-style` and `zoomies.theme` before first paint with `addInitScript`, wait for the page heading, then screenshot. Stop the controllers in a `finally`.
- [ ] **Step 2:** Run it. Expected: ten PNGs; open each with Read and confirm it shows the intended screen, not an error or a blank.
- [ ] **Step 3:** Confirm or drop each of the six source observations in the spec. For each, write to `findings.md`: the claim, the evidence (a capture, or a measurement such as the pixel offset of the Create button at 1440x900), and "confirmed" or "dropped". Measure with `boundingBox()`; do not estimate.
- [ ] **Step 4: Commit** "Photograph the public pages as they are, and settle what the source suggested".

### Task 5: The review shell and Sign in

**Files:** Create `public/mockups.html`, `review.css`, `review-state.js`, `review-state.test.mjs`, `review.js`, `screens/signin.js`, `concept-a.css`, `concept-b.css`, `concept-c.css`, `concept-c-tokens.css`, `concept-c-pairs.json`.

**Interfaces:**
- Produces `parseHash(hash: string): ReviewState` and `formatHash(s: ReviewState): string`, `ReviewState = { screen: string; concept: 'current' | 'a' | 'b' | 'c'; style: 'mission-control' | 'classic'; mode: 'light' | 'dark'; state: string }`. Unknown or missing values fall back to `signin`, `a`, `mission-control`, the OS colour scheme, and the screen's first state.
- Produces the screen contract used by Tasks 6 to 9: `export default { id: string, title: string, states: string[], render(concept: 'a' | 'b' | 'c', state: string): string }`.
- Review bar: four labelled `select`s (screen, concept, style, mode) and a state `select`; the choice is written to `location.hash`. Style and mode set `data-ui-style` and `data-theme` on `<html>`; Classic means the attribute is absent.
- Concept `current` renders no live HTML: it shows the matching `current-<screen>-1440.png` (or `-375.png` below 800px) as an `<img>` with a text alternative, and says it is a capture of the real UI.

- [ ] **Step 1: Write `review-state.test.mjs`.** Defaults for `''`; round trip through `formatHash`; `#screen=nope&concept=z&style=x&mode=y` returns defaults without throwing; a state not in the screen's list falls back to the first.
- [ ] **Step 2:** Run. Expected: FAIL.
- [ ] **Step 3: Implement** `review-state.js` (pure) and `review.js` (DOM). `mockups.html` carries a `<noscript>` note, links `/web/src/lib/styles/tokens.css` and the review CSS, declares `@font-face` from `/fonts/` with a system fallback, and shows a visible banner when `getComputedStyle(document.documentElement).getPropertyValue('--z-space-4')` is empty (the `file://` and missing-assets case).
- [ ] **Step 4: Build `signin.js`** with every state in the matrix and all three concepts, to the brief. Real Lucide glyphs are inlined SVG. Brand comes from `/brand/`; the backdrop from the `--z-logo-*` tokens. Tests of long text: the `unreachable` and `rate-limited` states render a 120-character unbroken host, and the page sets `overflow-wrap: anywhere` where host text appears.
- [ ] **Step 5:** Serve and check: `node tools/serve.mjs &`, then `node tools/check.mjs --screen signin`. Expected: no findings at 320, 375 and 1440, in both styles and modes. Load the page with `file://` through `launch()` and assert the banner is visible.
- [ ] **Step 6:** Look at A, B and C in Mission Control dark at 1440 and 375 with screenshots and the Read tool. Fix anything crowded, clipped or off-brand. Commit "Add the review page and three Sign in concepts".

### Task 6: First account

**Files:** Create `screens/first-account.js`; modify the three concept CSS files.

- [ ] **Step 1:** Run `node tools/check.mjs --screen first-account`. Expected: FAIL (unknown screen), proving the check bites.
- [ ] **Step 2:** Implement `first-account.js` to the brief and the state matrix. The token help text names the four ways to read the log, using the commands in `Bootstrap.svelte`. Copy buttons are real buttons with names.
- [ ] **Step 3:** Rerun the check. Expected: no findings. Screenshot A, B, C at 1440 and 375 and inspect. At 1440x900 the "Create the account" button must be fully visible in A; record the measured offset for B and C.
- [ ] **Step 4: Commit** "Add three First account concepts".

### Task 7: Connect an app

**Files:** Create `screens/connect-app.js`; modify the concept CSS.

- [ ] **Step 1:** Run the check; expected FAIL.
- [ ] **Step 2:** Implement all six states. The `normal` and `loopback` states use a 300-character client name and a 120-character unbroken redirect host, wrapped. The loopback note stays within the fixed status mapping: no new colour for it in A or B.
- [ ] **Step 3:** Run the check at 320, 375, 1440. Expected: no findings. Press Tab through B at 375 and confirm the sticky buttons never cover the focused field (assert with `boundingBox()`).
- [ ] **Step 4:** Inspect captures. Commit "Add three Connect an app concepts".

### Task 8: Fleet status

**Files:** Create `screens/fleet-status.js`; modify the concept CSS.

- [ ] **Step 1:** Run the check; expected FAIL.
- [ ] **Step 2:** Implement the six states from `FleetStatus` fields only (`state`, `since`, bands, wait minutes, `reasons`, `explanations`, `version`). Reuse the shape-plus-word pattern of `StatusPage.svelte` in every concept.
- [ ] **Step 3:** Add a check emulating `forcedColors: 'active'` through Playwright's `page.emulateMedia({ forcedColors: 'active' })` and assert each state's shape element and word are still visible and distinct.
- [ ] **Step 4:** Run the full check for this screen; inspect captures. Commit "Add three Fleet status concepts".

### Task 9: Not found

**Files:** Create `screens/not-found.js`; modify the concept CSS.

- [ ] **Step 1:** Run the check; expected FAIL.
- [ ] **Step 2:** Implement per the brief. Add a test in `check.test.mjs` asserting that the rendered not-found markup contains no `location` or `pathname` echo.
- [ ] **Step 3:** Run the check and inspect. Commit "Add three Not found concepts".

### Task 10: Captures and the record

**Files:** Create `tools/capture.mjs`, `public/concept-<screen>-<concept>-<width>.png`, `public/viewport-review.json`.

**Interfaces:** Consumes `serve`, `launch`. Produces, per screen and concept, Mission Control dark at 1440 and 375, plus one Classic light 1440 capture of the concept the README will recommend.

- [ ] **Step 1:** Implement `capture.mjs` by setting the hash and screenshotting the stage only, so the review bar is not in the picture.
- [ ] **Step 2:** Run it, then `node tools/check.mjs --out design/webui-refresh-review/public/viewport-review.json`. Expected: 35 PNGs, JSON with zero findings.
- [ ] **Step 3:** Open every PNG with Read. Re-shoot any that is clipped or misleading. Commit "Capture the public page concepts and record the checks".

### Task 11: README, recommendation, gates

**Files:** Create `public/README.md`; modify `PLAN-public.md` status line.

- [ ] **Step 1:** Write the README in the earlier reviews' shape: date, how to view (`node tools/serve.mjs`, then `http://127.0.0.1:8101/review/mockups.html`, needing `npm ci` for exact fonts), a captures table, the findings from Task 4 with only the confirmed ones, each concept in two sentences, one recommendation per screen with the reason, the Svelte files each recommendation would touch, the proposed tokens from `concept-c-tokens.css` with the guideline section each would change, and "Not claimed" copied from the spec.
- [ ] **Step 2:** `go test -count=1 ./internal/docs` and `node --test design/webui-refresh-review/tools/ design/webui-refresh-review/public/`. Expected: all pass.
- [ ] **Step 3:** `git status` shows no stray build output. Push; update the pull request description with the pack's contents and what was and was not run.
- [ ] **Step 4:** Tell the owner pack 1 is ready, then write the plan for the next pack (shell and monitoring) from the same method.

## Out of this plan

The four signed-in packs each get their own plan, written after pack 1 and using its recommended direction as the baseline.
