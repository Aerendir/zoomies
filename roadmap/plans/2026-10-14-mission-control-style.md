# Mission Control UI implementation handoff

14 October 2026. Design reference: [PR #1027](https://github.com/eyupio/zoomies/pull/1027).

## Delivered

Classic remains the default. Mission Control is a separate browser-local choice,
selected in Appearance or the phone's More menu. Its light/dark palette and
larger type apply before first paint, including the first-account and sign-in
screens, and independently follow System, Light or Dark. Reset clears only the
new style key. Switching updates root tokens without remounting routes.

The original logo and backdrop, Eli, dog/paw artwork and animations, vocabulary,
reduced motion, navigation and saved grid choices remain intact. Busy work keeps
cyan; interaction stays in the Runner Blue family. Canvas log viewers follow
style changes without clearing output.

## Current status

- Design: [PR #1027](https://github.com/eyupio/zoomies/pull/1027), merged.
- Additive style (P1–P4): [PR #1030](https://github.com/eyupio/zoomies/pull/1030), merged.
- Operational layout (P5), verification and captures: [PR #1038](https://github.com/eyupio/zoomies/pull/1038), merged.
  Branch: `codex/mission-control-operational-layout`.

## Operational layout delivered

Mission Control's Overview now places current issues and four live metrics
before pool utilisation, active jobs and history. Startup/registration timings
remain available through a disclosure. First-run guidance stays first. Keyed
sections move without remounting panels when style changes.

On a Mission Control phone, Jobs puts search, status and records before the
collapsed Fleet context section. Advanced Filters disclose without clearing
selections or their URL. Job identity, repository, status and timings lead the
new default column order, with an explicit Inspect action. Saved column order
and widths still win. The same records and context components move through
keyed sections, retaining their state during style/viewport changes.

Fresh Mission Control grids start as Cards; Classic starts as Rows. A saved
per-grid view wins over an explicit global view; both win over the style
fallback. Legacy saved Rows values remain authoritative. Saving a logo or
vocabulary does not freeze an unchosen global table default. No preference
migration or new API is required.

The original Zoomies logo, Eli, quirky vocabulary, dog/paw animations and
reduced-motion behaviour remain unchanged. Classic retains its original
history-first Overview, full Jobs context and Rows fallback.

No workflow dispatch, scheduler or provider behaviour changes are included.
The public status page remains Classic.

## First-job activation (P8, manual GitHub handoff)

Implemented in [PR #1040](https://github.com/eyupio/zoomies/pull/1040), awaiting review,
on `codex/first-job-activation`, following the merged UI stages.
The Overview checklist now lets the operator choose a destination pool and
enter a repository covered by that pool's installation target. Single-repository
installations prefill their repository. Changing pool clears the previous
repository choice and regenerates the workflow with that pool's labels.

Read-only fleet checks explain a paused pool, a zero runner ceiling, an
unavailable/unhealthy installation and the absence of an available matching
host. Host matching reuses the pool editor's selector rules. These checks do
not promise scheduler capacity or live repository access; rented capacity can
still arrive. No new API contract, database migration or GitHub mutation is
introduced.

The handoff names the checks only GitHub can establish: App repository access,
Actions enabled, the operator's workflow/run permissions and runner-group
policy. It explains protected branches and merging into the default branch
before Run workflow appears. A validated repository gets an Actions link using
the installation's web origin, including GitHub Enterprise. Unknown/unsafe
origins retain the manual Actions-tab instructions.

The existing complete test workflow is copied, not committed or dispatched by
Zoomies. Its token permissions are empty and its timeout is five minutes.
Windows pools receive PowerShell commands; other pools keep the Unix probe.
Copying YAML or opening GitHub does not complete setup: the existing managed-job
counters and live events still announce running, successful and failed jobs
with queue/run timings. Queue and delivery-health links help when work stalls.

The original logo, Eli, quirks, appearance choices and existing prerequisite
steps remain in place. Repository discovery and in-app dispatch remain future
features requiring API and authorisation design.

### Activation validation

- Svelte check: no errors or warnings. UI lint and the web/Go builds passed.
- Focused unit checks cover repository targeting, URL safety, enterprise origins,
  pool labels and the bounded Windows/Unix test workflow.
- 41 browser checks passed across activation, first-account, GitHub connection
  and diagnostics fixtures. This includes the existing signed webhook/live-event
  regression announcing the first managed job with its timings.
- Activation is checked at 1440px and 375px in Classic and Mission Control:
  changing pools changes the YAML, invalid repositories have no Actions link,
  paused pools explain the prerequisite, and the page has no horizontal overflow.
- Budgets remain shell 142.4KB of 200KB and status 19.1KB of 30KB.
- The public-tree audit passes with the new files staged. No guard changes.
- No real repository writes or workflow dispatches were performed.

Review the real UI with deterministic activation fixtures:
[desktop](../../docs/screenshots/activation/setup-mission-control-1440.png) and
[375px phone](../../docs/screenshots/activation/setup-mission-control-375.png).
Classic captures are beside them. The fixture supplies pools/installations and
zero managed-job counters; it stops SSE updates so those prerequisites stay
stable. These are UI checks, not proof that a real repository is ready.

```sh
cd web
ZOOMIES_ACTIVATION_CAPTURES=1 npx playwright test tests/activation.spec.ts --project=chromium
```

## Additive-style validation (PR #1030)

- Svelte check: no errors or warnings.
- UI lint and the full unit suite passed. Focused bootstrap checks also verify
  unknown/blocked storage, independent system mode and HTML/CSS chrome parity.
- Existing Settings browser tests: 16 passed.
- Visual-style browser tests: 9 passed, including native keyboard selection,
  preference preservation, reload/reset, system changes, phone selection,
  unsaved pool draft, blocked storage and original first-account artwork.
- 14 monitoring routes in both Mission Control modes at 1440 and 375 pixels:
  56 route/viewport checks with no page-level horizontal overflow.
- Web build passed budgets: shell 142.3KB of 200KB; status 19.1KB of 30KB.
- The Go binary built successfully and the browser suite used the real binary
  with temporary databases and fixture services. No real repository, host,
  billing or paid model operations were performed.
- Eight authenticated real-app captures generated through the existing
  screenshot harness, covering Appearance (desktop and 375px phone), Overview
  and Runners in both modes. Appearance captures were visually inspected.

In the additive stage, two additional test selectors initially expected an exact pool-label name and
full-logo asset on a page that uses the circular mark. Source inspection
corrected those selectors; all affected tests then passed. The initial browser
harness command also used the system's unrelated `go` executable; selecting the
managed Go toolchain resolved fixture startup. These were test/setup issues,
not unaddressed application failures.

## Review the implemented UI

- [Desktop Appearance, dark](../../docs/screenshots/mission-control/appearance-dark.webp)
- [Desktop Appearance, light](../../docs/screenshots/mission-control/appearance-light.webp)
- [375px Appearance, dark](../../docs/screenshots/mission-control/appearance-mobile-dark.webp)
- [375px Appearance, light](../../docs/screenshots/mission-control/appearance-mobile-light.webp)
- [Overview, dark](../../docs/screenshots/mission-control/overview-dark.webp)
- [Overview, light](../../docs/screenshots/mission-control/overview-light.webp)
- [Runners, dark](../../docs/screenshots/mission-control/runners-dark.webp)
- [Runners, light](../../docs/screenshots/mission-control/runners-light.webp)

Reproduce with the built binary and browser installed:

```sh
cd web
ZOOMIES_SCREENSHOT_STYLE=mission-control node tests/support/screenshots.mjs ../docs/screenshots/mission-control overview,appearance,appearance-mobile,runners
```

The screenshot helper keeps optional-style captures in a separate directory
from Classic. The public status document retains Classic and its separate
budget. Switching back to Classic is the appearance rollback; no fleet
migration is required.

## Operational-stage validation

- Rebased onto current main, including the upstream public-audit corrections.
  The existing public-tree audit passes; no guard list was weakened.
- Full unit suite: 112 test files passed on the rebased checkout.
- Browser regressions: 23 existing Jobs tests passed; all 11 operational-layout
  tests and 9 visual-style tests passed. The final Overview ordering assertion
  was rerun after the last keyed-panel adjustment and passed.
- The style tests also cover 56 route/viewport/mode checks at 1440 and 375px,
  with no page-level horizontal overflow.
- Fresh phone job identity and Inspect are visible in the first 900px viewport.
  Real coarse-pointer Inspect targets are at least 44×44px.
- Global/per-grid views, column order/widths, first-run placement, active filter
  chips, saved queries and unrelated appearance choices are covered.
- Svelte check, UI lint, web build and Go binary build passed. Budgets remain
  shell 142.4KB of 200KB and status document 19.1KB of 30KB.
- Final real-app captures cover Overview, Jobs and Appearance in light/dark,
  with 375px captures for all three. The screenshot helper keeps the existing
  wider phone Overview capture distinct from `overview-phone-375`.

### Current operational captures

- [Overview, desktop](../../docs/screenshots/mission-control/overview-dark.webp)
- [Overview, 375px](../../docs/screenshots/mission-control/overview-phone-375-dark.webp)
- [Jobs, desktop](../../docs/screenshots/mission-control/jobs-dark.webp)
- [Jobs, 375px](../../docs/screenshots/mission-control/jobs-mobile-dark.webp)

```sh
cd web
ZOOMIES_SCREENSHOT_STYLE=mission-control node tests/support/screenshots.mjs ../docs/screenshots/mission-control overview,overview-phone-375,jobs,jobs-mobile,appearance,appearance-mobile
```

## Independent verification and reflow fix (14 October 2026)

In [PR #1045](https://github.com/eyupio/zoomies/pull/1045), branch
`theme/implementation-verification`. Re-ran the style and layout suites against a locally built binary on that
branch's base, main at `891737a0`: 25 browser checks passed, covering the existing style and
operational-layout specs plus four added here.

The added coverage closes the plan's width and zoom checks:

- No page-level overflow at 1440, 1180, 1179, 768, 375 and 360 pixels, in Classic
  and Mission Control, on Overview, Pools, Jobs and Appearance.
- A 200% zoom, tested by halving those widths to 720, 590 and 384 pixels, the
  same arithmetic the existing `a11y.spec.ts` reflow check uses.
- Enlarged text, a 200% root font size, at 1440, 1180 and 768 pixels.
- An open assistant conversation and its unsent draft survive a style switch
  reached through the app's own links, so the panel is not remounted.

The enlarged-text pass found one real defect. The Settings rail hides its group
labels with absolute positioning, but before 1180 pixels the rail was `static`,
so it was not the labels' containing block. With enlarged text the labels
escaped the rail's own horizontal scroll area and widened the document to
2098 pixels at a 1180-pixel viewport. `.rail` is now `position: relative` in
that breakpoint, which keeps the labels inside it. Classic was affected too.

**Known limit.** At 360 or 375 pixels with 200% text, both styles scroll
sideways (Classic as well as Mission Control). That combination lays the page
out in under 190 CSS pixels, below WCAG's 320-pixel reflow floor at default
text, and reflowing the dense phone layouts at that size is a general
follow-up rather than specific to the new style.

The plan's release-evidence list is complete: the component gallery is
built and described below. The phone-keyboard evidence is not a capture: the
assistant sheet's visual-viewport sizing, focus containment and close order
are covered by the browser suite.

## Release evidence (14 October 2026)

In [PR #1047](https://github.com/eyupio/zoomies/pull/1047).
First run, loading, a failed fleet fetch, an empty pool list and a filter that
matches nothing are now captured from the real binary, in dark and light and in
both styles. In Mission Control they are in `docs/screenshots/mission-control`;
the Classic ones sit in `docs/screenshots` beside the existing captures, and
`docs/ui-guidelines.md` links them.

To reach states the seeded fleet is never in, the screenshot harness gained two
fields: `stub`, which puts routes on the page before it loads (an empty list, a
failed fetch, a first run), and `noRows`, which stops `settle` waiting for a row
an intentionally empty grid will never have. Each stub covers only what the
state needs, so the rest of the page is still the real product.

A headless browser cannot raise a soft keyboard, so the phone-keyboard evidence
is not a picture: the assistant sheet's visual-viewport sizing, focus
containment and close order are covered by the browser suite.

## Component gallery (N-T4, 14 October 2026)

In [PR #1050](https://github.com/eyupio/zoomies/pull/1050), branch
`theme/state-gallery`. N-T4 asked for the states a healthy screenshot never
shows to be reviewable in one place, in both styles and modes. That is now
`web/gallery.html` plus `web/src/gallery/`: a development-only Vite entry
that renders the real components from `web/src/lib/components`, never a copy
of them, in the states the audit named and a few it did not.

It is not part of the product. `vite build` has one input and does not build
it, `internal/api/webdist` does not contain it, the router does not know it and
no page links to it. The shell budget is unchanged at 142.4 KB of 200 KB and the
status document at 19.1 KB of 30 KB; a `gallery.html` in the embedded output is
a mistake, not a feature.

The gallery covers buttons, icon buttons, segmented controls, form fields,
choice controls, sliders and the quantity field, the status vocabulary, the
surface and text tokens, skeletons, empty and error states, the loading
boundary, toasts, a drawer, a destructive confirmation, a rich tooltip, notices
and all four data-grid states. Refused controls keep their reason in the
accessible description and stay focusable; a plain disabled control is shown
beside them so the two read differently. The style and mode come from the query
string (`?style=mission-control&mode=dark`), so a combination is something a
capture or a test can name, and opening the gallery writes no browser
preference.

`make gallery` serves it on `:5173/gallery.html`. `make screenshots-gallery`
refreshes the eight committed captures, four combinations at 1440px and four at
375px, in `design/theme-review/gallery/` with a README that says what they are.
The encoder was extracted to `tests/support/webp.mjs` so the gallery script and
the documentation script share one Pillow invocation; a subset of
`tests/support/screenshots.mjs` was re-run after the change and still encodes.

`web/tests/gallery.spec.ts` is the automated half, against a Vite dev server
rather than the binary (the gallery is not in the binary to drive): 16 checks
pass. They cover all four combinations, the root attributes, the refusal
descriptions, the focus ring, retry actions, live contrast of every status
badge and state label against the surface it sits on, unknown values falling
back to Classic and System, a system-mode change that keeps Mission Control,
an in-place style switch that writes no preference, the fresh phone grid's
Cards/Rows fallback in each style, the drawer's focus trap and Escape order,
the confirmation's typed-name gate, the grid's ready, empty, loading and
failed states, the toast tones, and no page-level overflow at 1440 or 375
pixels. The spec project is excluded from the `chromium` and `mobile`
projects, which drive the real binary, and joins the Playwright matrix's
`other` group in CI so it runs at all.

One component reads the style store rather than the root attribute: a data
grid's fresh phone fallback is Cards in Mission Control and Rows in Classic.
Setting the attribute alone would have shown a Classic decision inside Mission
Control tokens, so `visualStyle` gained `preview`, which makes the rune agree
with an attribute the caller owns and writes no preference.

Building the gallery found one real defect in its own layout before it found
any in the components: a `minmax(22rem, 1fr)` form column has a 352px floor,
which is wider than a 375px phone once the page and panel padding are taken
out, and pushed the document to 389px. Both gallery grids now use
`minmax(min(22rem, 100%), 1fr)`.

There is one honest limit in the spec: a data grid keeps last-known rows on
screen while a later fetch is in flight, by design, so a filter change cannot
reach the loading state. The gallery keys the grid by its state to force a
fresh mount, and says so where the key is.

## Next coding-agent actions

1. The theme, the operational layout, first-job activation, the release
evidence and the development-only component gallery (N-T4, PR #1050) are
done. The one evidence item that is not a capture is the assistant sheet's
phone-keyboard behaviour, which the browser suite covers.
2. Keep the design specification in `roadmap/archive/theme-implementation-plan.md` as the
   intended behaviour; use this file for current implementation status.
3. Treat first-job repository assistance as a separate feature. Before adding
   discovery/dispatch, scope API contracts, permissions, protected branches
   and the manual workflow path. Do not add a dummy dispatch button.
4. Keep Classic opt-out, browser-local style and every explicit saved choice
   intact in any later changes. Preserve the original artwork and quirks.
