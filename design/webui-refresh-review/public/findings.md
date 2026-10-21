# Findings from the current public pages

21 October 2026. Working notes for the pack-1 README. Every number here is in
[current-measurements.json](current-measurements.json), written by
`tools/capture-current.mjs` against the real controller: the first-run
controller for the first account, sign in and the MCP consent screen, the status
fixture for the public status page, and the demo fleet for a wrong address. The
captures are Mission Control, dark, at 1440x900 and 375x812. Nothing was stubbed.

## The six observations from the source

| # | Observation (from reading the source) | Evidence | Verdict |
| --- | --- | --- | --- |
| 1 | Three brand treatments on three screens | Sign in draws the `lockup` at 352x352 (1440px) and 295x295 (375px) on a black stage. The first-account form draws the `mark` at 72x72. Consent draws the `full` logo at 121x28. Three variants and three sizes. | **Confirmed** |
| 2 | One banner, written three times | Measured on real failure states: border radius 4px on the first-account banner and 8px on sign in and consent; icon top offset 1px, 3px and 2px; icon size 15px, 15px and 16px. The consent banner measured is the loopback warning, which shares a CSS block with its failure banner. `web/src/lib/components` has no shared callout. | **Confirmed** |
| 3 | The hardest field comes first | The setup token is the first input. It starts 425px down at 1440x900 and 511px down at 375x812, below a three-sentence introduction and a two-sentence "Then:" paragraph. Only the Docker Compose command is visible as a hint; the other four install types are in the `?` help. | **Confirmed, with a correction below** |
| 4 | The widest audience has the least brand | The status page has 0 logos and 0 images, carries no `data-ui-style` (so it is Classic), and is dark. | **Confirmed** |
| 5 | Sign in carries four states in one 983-line file | `Login.svelte` is 983 lines. Its branches are authentication off, two-step verify, two-step enrolment with recovery codes, and the password and single sign-on form. A visitor cannot see this; it is the cost of changing the page. | **Confirmed** (code structure, not visible) |
| 6 | Whether the loopback warning's tone is alarm fatigue | Still a question. The capture shows a red-tinted card with a red icon. With it showing, the Allow button's bottom edge is at 905px at 1440x900, 5px below the fold; without it, 820px. | **Open, for the concepts** |

### Correction to observation 3

The first-account form's own source comment says an earlier lockup pushed the
Create button below the fold of a 1440x900 screen. That is fixed: the button's
bottom edge is at 851px, inside the fold. At 375x812 it is at 1017px, 205px
below it. No concept may claim to fix the 1440px case.

## Observations the captures added

| Observation | Evidence |
| --- | --- |
| On a phone, the sign-in button is below the fold | At 375x812 the logo stage is 295px tall from y=80, the first input starts at 601px and the Sign in button's bottom edge is at 839px, 27px below the fold. At 1440x900 everything fits, with the button's bottom edge at 590px. |
| The consent screen's primary action is below the fold on a phone | Allow's bottom edge is at 963px at 375x812. At 1440x900 it is at 820px. |
| The status page of a blocked fleet is long | The stuck fixture lists 12 reasons: the page is 1724px tall at 1440 and 2080px at 375, about two and a half screens, with the state word at the top and the reasons doing the rest of the work. |
| Two links on the not-found page are called "Go to the overview" | The shell's brand link and the empty state's button. Not a defect (same target), but a concept should not add a third. |

## Reading the captures

* The Not found capture sits inside the full app shell. The "Zoomies demo system"
  banner and the "Authentication disabled" chip in it come from the demo fixture,
  not from the product, and a concept should not copy them.
* The status capture is the stuck fixture, which is close to the longest page the
  product shows. It is the case to design for, not the typical one.
* The consent capture is a real registration of a client called Claude with the
  redirect `claude.ai`. The loopback capture is a second real client with a
  redirect on `127.0.0.1`.

## What the first account concepts change, measured

Positions are in pixels from the top of the page, with the Mission Control dark
captures and mockups at 1440x900 and 375x812, and the empty state. Today's page
is from `current-measurements.json`.

| | Setup token field, 1440 | Create button's bottom edge, 1440 | Setup token field, 375 | Create button's bottom edge, 375 |
| --- | --- | --- | --- | --- |
| Today | 425 to 457 | 851 | 511 to 543 | 1017 |
| A, the same card | 362 to 394 | 884 | 364 to 396 | 1004 |
| B, two columns | 295 to 343 | 821 | 380 to 428 | 926 |
| C, a rail of steps | 217 to 265 | 839 | 387 to 435 | 1107 |

* Every concept asks for the token higher up than today, by 63 to 208px on a
  desktop and by 124 to 147px on a phone.
* A is the only concept whose Create button is lower than today's on a desktop,
  by 33px, because the commands that find the token now sit under the token
  field. It still ends inside the fold, and a test holds it there.
* B and C use the product's 48px touch fields, where A keeps today's 32px ones,
  which is most of why their phone pages are longer.

## What the connect an app concepts change, measured

The bottom edge of the Allow button, in pixels from the top of the page, in
Mission Control dark at 1440x900 and 375x812. Today's page is from
`current-measurements.json`. The fold is 900px on a desktop and 812px on a phone.

| | Normal, 1440 | With the warning, 1440 | Normal, 375 |
| --- | --- | --- | --- |
| Today | 820 | 905 (below the fold) | 963 (below the fold) |
| A, the same card | 769 | 873 | 949 (below the fold) |
| B, role cards | 732 | 818 | pinned to the screen's bottom edge |
| C, a diagram | 576 | 657 | 749 |

* All three put Allow inside the fold on a desktop, with or without the warning.
  Tests hold that.
* A's first version put Allow at 995px with the warning, worse than today, and
  the fix was to drop a decorative icon, merge two notes into one and position
  the appearance toggle, which had been taking a row of its own.
* On a phone only B and C keep Allow in reach: B pins the buttons, and C's page
  is the shortest. A is a tidy card, and a card this long scrolls.
* How the warning about this computer should sound stays a question. A keeps the
  danger colour with fewer words, B uses a neutral note with the laptop icon,
  and C labels the address in its diagram as this computer with no alarm colour.

## What the fleet status concepts change, measured

A blocked fleet with twelve reasons, in Classic dark at 1440x900 and 375x812. Today's page
is from `current-measurements.json`: 1724px tall at 1440 and 2080px at 375. Its first
reasons are already on the first screen, so what is long is the list, in which the two
reasons that block and the ten that only slow are one undivided stack.

| | Page, 1440 | Page, 375 | First blocking cause ends, 1440 | First blocking cause ends, 375 |
| --- | --- | --- | --- | --- |
| Today | 1724 | 2080 | not measured | not measured |
| A, banner, tiles, grouped | 888 | 912 | 654 | 678 |
| B, one sentence | 823 | 559 | behind a disclosure | behind a disclosure |
| C, a board | 1440 | 2090 | 222 | 612 |

* A halves the page on both widths by grouping the reasons by how serious they are
  and folding the ten that only slow. Its two blocking causes are on the first screen.
* B is the shortest and the plainest, and its price is that the cause is one tap away.
  For a developer who only wants to know whether to wait, that may be the right price.
* C helps on a desktop, where the cause sits beside the word Blocked, and not on a phone,
  where its page is as long as today's. It is a desktop idea, and needs A's folding to
  help a phone.
* The state survives forced colours in all three: the word is on the page, and a
  triangle, a dashed ring and a hollow ring differ without any colour.
* The status page ignores the visual style today, because its policy allows no inline
  script to apply one. A Mission Control status page would need the server to write the
  style into the HTML.
