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
