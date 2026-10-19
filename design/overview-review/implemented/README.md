# Calm operations implementation

19 October 2026. Captures from the built Zoomies controller and its temporary
seeded demonstration fleet, not generated mockups. Authentication was disabled
on loopback for the browser fixture. Values and warnings are demonstration
state and can change as the fixture runs.

| Style and mode | Desktop, 1440 by 900 | Phone, 375 by 900 |
| --- | --- | --- |
| Classic light | [Capture](classic-light-1440.png) | [Capture](classic-light-375.png) |
| Classic dark | [Capture](classic-dark-1440.png) | [Capture](classic-dark-375.png) |
| Mission Control light | [Capture](mission-control-light-1440.png) | [Capture](mission-control-light-375.png) |
| Mission Control dark | [Capture](mission-control-dark-1440.png) | [Capture](mission-control-dark-375.png) |

These captures show only the visible viewport, so the position of floating
controls is unambiguous. Eli starts at the bottom left and stays there when
the page scrolls, above the phone navigation. An explicitly saved side remains
available. Compare the Mission Control dark captures with the
[desktop after scrolling](mission-control-dark-1440-scrolled.png) and
[phone after scrolling](mission-control-dark-375-scrolled.png).

## Implemented behaviour

- Eli defaults to the bottom left of the viewport, retaining saved side choices.
- Both styles share the order: first-run guidance when needed, attention,
  four live metrics, pools, active jobs, recent events and supporting detail.
- One page scope selector controls job figures and links. Infrastructure and
  the activity matrix explicitly retain their fleet scope.
- Sticky page links provide Now, Pools, Jobs, History and Diagnostics. The
  phone has a separately named More overview sections control, with keyboard
  access and Escape dismissal. It leaves the app's More menu independent.
- Jump links open closed destinations, set a bookmarkable fragment and move
  focus below the sticky navigation. Current-section indication follows jumps
  and scrolling, including destinations near the end of the document.
- History, host capacity and startup/registration details start closed, retain
  live summaries and remember explicit expansion choices in this browser.
  Expanding history preserves its range, matrix navigation and day details.
- The recent-events preview shows three entries. Expanding it restores the
  longer feed, category preferences and original controller reason strings.
- Failure attention remains visible with its count and review link. Expanding
  its preview retains inspection, acknowledgement, undo and eligible Eli links.
- Pool rows distinguish busy/live utilisation from the live/maximum ceiling.
  Compact phone metrics keep metadata out of narrow tiles.
- Failed initial fetches retain the retry state. Loading figures do not claim
  zero; interrupted updates are labelled as potentially stale. A history fetch
  error is visible in the closed summary.

## Verification

The production UI build and Go binary build passed. UI type checks reported no
errors or warnings; Prettier, ESLint and the block-spacing check passed.
The unit suite passed 1,150 tests, with one existing environment-dependent
skip because the live-output test requires an awk implementation other than
this environment's mawk.

A clean run of 45 focused browser checks passed, followed by two additional
empty-preview checks. Browser verification used the real built controller. The selected suites cover
overview data and scopes, failure triage and repair eligibility, theme switching,
first-run guidance, capacity preferences, matrix keyboard interaction, zoom and
text enlargement. New Calm operations checks cover both styles and both colour
modes at desktop and phone widths, saved disclosures, deep links, page navigation,
loading and failed fetches. The initial test run exposed a page/app More naming
collision, which was corrected. The Eli follow-up passed 12 browser checks (two desktop-only panel tests were
skipped on phones), verifying viewport anchoring before and after scrolling,
saved side preferences and panel dragging. Tests that relied on former layouts, disclosure
visibility or error-control labels were updated and rerun.

`web/tests/calm-overview.spec.ts` records the new behaviour. Existing browser
specifications were updated to open supporting detail explicitly. Browser
captures verify layout, not real infrastructure performance or a usability study.
