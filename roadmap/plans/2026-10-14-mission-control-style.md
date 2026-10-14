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
- Operational layout (P5), verification and captures: [PR #1038](https://github.com/eyupio/zoomies/pull/1038), awaiting review.
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
First-job repository assistance (P8) remains separately scoped. The public
status page remains Classic.

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

## Next coding-agent actions

1. Review [PR #1038](https://github.com/eyupio/zoomies/pull/1038) and merge after its checks pass.
2. Keep the design specification in `THEME_IMPLEMENTATION_PLAN.md` as the
   intended behaviour; use this file for current implementation status.
3. Treat first-job repository assistance as a separate feature. Before adding
   discovery/dispatch, scope API contracts, permissions, protected branches
   and the manual workflow path. Do not add a dummy dispatch button.
4. Keep Classic opt-out, browser-local style and every explicit saved choice
   intact in any later changes. Preserve the original artwork and quirks.
