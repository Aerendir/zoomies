# Theme review artefacts

14 October 2026. Start with [the interactive mock](mockups.html) and [the implementation plan](../../THEME_IMPLEMENTATION_PLAN.md).

The mock is a standalone HTML design artefact, not a replacement app. It uses illustrative data. Screen selection, visual-style choice, light/dark preview, Jobs search, Problems drawer, assistant placeholder and first-job success preview are interactive. It sends no API requests. The System button previews the current OS mode once; the production implementation must use the existing live system listener. Choice persistence, permissions, SSE and actual mutation flows belong to production, not this mock. No production fixes have been implemented.

Open the HTML in a browser, or serve this folder on loopback with `python3 -m http.server 8101 --bind 127.0.0.1` and browse `http://127.0.0.1:8101/mockups.html`. The browser used for the review required HTTP rather than file URLs.

## Concepts

| Screen | Desktop, 1440px | Phone, 375px |
| --- | --- | --- |
| Overview | [Preview](concept-overview-1440.png) | [Preview](concept-overview-375.png) |
| Pools | [Preview](concept-pools-1440.png) | [Preview](concept-pools-375.png) |
| Jobs | [Preview](concept-jobs-1440.png) | [Preview](concept-jobs-375.png) |
| Appearance | [Preview](concept-appearance-1440.png) | [Preview](concept-appearance-375.png) |
| First job | [Preview](concept-onboarding-1440.png) | [Preview](concept-onboarding-375.png) |
| Problems drawer | [Preview](concept-problems-1440.png) | [Preview](concept-problems-375.png) |

The review strip is not product UI. Symbols are placeholders; use the existing Lucide icons and original Zoomies logo. Navigation is abbreviated and several entries route to a representative mock screen. Use `shell/sections.ts`, not this mock, as the production route and bottom-navigation contract. The phone mock shows readable cards, while production must respect saved Rows/Cards choices. The heatmap is illustrative, not the current chart's full interaction model. These captures were inspected and the five main mock screens had no page-level overflow at either width.

## Current app evidence

| Screen | Desktop | Phone |
| --- | --- | --- |
| Overview | [Current app](current-overview-1440.png) | [Current app](current-overview-375.png) |
| Jobs | [Current app](current-jobs-1440.png) | [Current app](current-jobs-375.png) |
| Appearance | [Current app](current-appearance-1440.png) | [Current app](current-appearance-375.png) |

Current-app captures are from the locally built real controller using its temporary seeded fixture with authentication disabled. Counts and warnings are demonstration state. They establish current layout, not real workload performance. Full-page phone captures include the fixed bottom navigation at its viewport position; it moves with the viewport during normal scrolling.

[viewport-review.json](viewport-review.json) records the 14 controller route checks at both widths and the concept overflow checks. It is a smoke inspection, not a full accessibility or end-to-end test report. External billing, email, GitHub actions and real host operations were not executed. An additional first-run browser capture was blocked by automatic approval service usage limits; no bootstrap screenshot is claimed.

## Component gallery

The states a healthy fleet never shows (disabled and refused controls,
loading, empty, failed and retry states, focus rings, every status shape and
all four data-grid states) are reviewed in the development-only component
gallery rather than in a concept or a product page. Its captures in both
styles and both colour modes, at 1440px and 375px, are in
[the gallery folder](gallery/README.md); the live page, the refresh command and
the browser checks are described there.

## Required Zoomies identity

Retain the real Zoomies logo, existing logo treatments, dog/paw artwork, Eli identity, animations and quirky vocabulary choices in production. The mock symbols are layout placeholders, not proposed replacement branding. Preserve saved Off/Cute/Standard choices and reduced motion when changing visual style.
