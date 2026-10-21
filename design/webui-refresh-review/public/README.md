# Web UI refresh review, pack 1: the pages you can see without an account

21 October 2026. Design review only. No application source, API, test or
documentation page was changed: this folder, its tools and the plan and
specification beside it are the whole branch. Choosing a concept starts a
separate implementation task, and each recommendation below says which Svelte
files that task would touch.

The owner asked for the whole controller web UI to be refactored or improved,
with variations and concepts rather than one answer. This is the first of five
packs. It covers the five pages reachable without signing in: Sign in, First
account, Connect an app, Fleet status and Not found. The four signed-in packs
follow, each using the direction recommended here as its baseline
([SPEC.md](../SPEC.md#rolling-on)).

## How to look at it

```sh
node design/webui-refresh-review/tools/serve.mjs
# then open http://127.0.0.1:8101/review/mockups.html
```

Run `npm ci` in `web/` first. The page links the real
[tokens.css](../../../web/src/lib/styles/tokens.css) and fonts instead of copying
their values, so it cannot drift from the product, and they come from there.

The page has five controls, and every combination is also a link you can send,
because the address carries it:

* **Screen** and **State**: every state the specification asked for, for example
  ten for Sign in and six for Fleet status.
* **Concept**: Current (a photograph of the real page), A, B or C.
* **Style** and **Mode**: Mission Control or Classic, light or dark.

It sends no request and the data is illustrative. Counts, names and the host
`fleet.acme-ci.example` are made up; the sentences are the product's own wherever
one exists (`Login.svelte`, `Bootstrap.svelte`, `McpConsent.svelte`,
`present.ts`, `NotFound.svelte`), so the concepts are compared on structure and
not on who wrote better copy. Proposed copy is said to be proposed below.

## The pictures

Each concept is shown in Mission Control, dark, at 1440 and at 375 (a phone), in
the state that tests the design hardest. The recommended concept is also shown in
Classic, light. Every picture is the whole page, not the first screen; where a
fold matters the text gives a measured number.

| Screen | State shown | Today | A | B | C | Recommended, Classic light |
| --- | --- | --- | --- | --- | --- | --- |
| Sign in | Password only | [1440](current-signin-1440.png), [375](current-signin-375.png) | [1440](concept-signin-a-1440.png), [375](concept-signin-a-375.png) | [1440](concept-signin-b-1440.png), [375](concept-signin-b-375.png) | [1440](concept-signin-c-1440.png), [375](concept-signin-c-375.png) | [B](concept-signin-b-classic-light-1440.png) |
| First account | Empty | [1440](current-first-account-1440.png), [375](current-first-account-375.png) | [1440](concept-first-account-a-1440.png), [375](concept-first-account-a-375.png) | [1440](concept-first-account-b-1440.png), [375](concept-first-account-b-375.png) | [1440](concept-first-account-c-1440.png), [375](concept-first-account-c-375.png) | [B](concept-first-account-b-classic-light-1440.png) |
| Connect an app | A request | [1440](current-connect-app-1440.png), [375](current-connect-app-375.png), [with the warning](current-connect-app-loopback-1440.png) | [1440](concept-connect-app-a-1440.png), [375](concept-connect-app-a-375.png) | [1440](concept-connect-app-b-1440.png), [375](concept-connect-app-b-375.png) | [1440](concept-connect-app-c-1440.png), [375](concept-connect-app-c-375.png) | [B](concept-connect-app-b-classic-light-1440.png) |
| Fleet status | Blocked, twelve reasons | [1440](current-fleet-status-1440.png), [375](current-fleet-status-375.png) | [1440](concept-fleet-status-a-1440.png), [375](concept-fleet-status-a-375.png) | [1440](concept-fleet-status-b-1440.png), [375](concept-fleet-status-b-375.png) | [1440](concept-fleet-status-c-1440.png), [375](concept-fleet-status-c-375.png) | [A](concept-fleet-status-a-classic-light-1440.png) |
| Not found | The one state | [1440](current-not-found-1440.png), [375](current-not-found-375.png) | [1440](concept-not-found-a-1440.png), [375](concept-not-found-a-375.png) | [1440](concept-not-found-b-1440.png), [375](concept-not-found-b-375.png) | [1440](concept-not-found-c-1440.png), [375](concept-not-found-c-375.png) | [B](concept-not-found-b-classic-light-1440.png) |

The "Today" pictures come from the real built controller, not a stub: the
first-run controller for First account, Sign in and Connect an app, the status
fixture for Fleet status, and the demo fleet for Not found. Their measurements
are in [current-measurements.json](current-measurements.json), and
[findings.md](findings.md) holds the working notes behind this page.

Two things in those photographs are not the product. The Not found capture sits
inside the app shell, and its "Zoomies demo system" banner and "Authentication
disabled" chip come from the demo fixture. The concepts draw only the page,
because the shell is the next pack's subject.

## What the review found

The specification listed six observations from reading the source. Each was
checked in a browser before it was kept; this is what survived.

1. **Three brand treatments on three screens. Confirmed.** Sign in draws the
   lockup at 352px (295px on a phone) on a black stage, First account draws the
   72px mark, and Connect an app draws the 121 by 28 full logo.
2. **One banner, written three times. Confirmed.** The failure banner is
   hand-written in `Login.svelte`, `Bootstrap.svelte` and `McpConsent.svelte`.
   Measured on real failure states, the radius is 4px on one and 8px on the
   others, and the icon sits 1px, 3px and 2px from the top. There is no shared
   callout in `web/src/lib/components`.
3. **The hardest field comes first. Confirmed, with a correction.** The setup
   token is the first input, 425px down at 1440x900 and 511px down on a phone,
   and only the Docker Compose command is visible as a hint. The form's own
   source comment says an earlier lockup pushed the Create button below the fold
   of a 1440x900 screen. That is fixed: the button ends at 851px. On a phone it
   ends at 1017px, 205px below the fold, and no concept claims otherwise at 1440.
4. **The widest audience has the least brand. Confirmed.** The status page has no
   logo and no image, carries no visual style, and so is always Classic.
5. **Sign in carries four states in one 983-line file. Confirmed**, as code
   structure: a visitor cannot see it, and it is what makes any visual change to
   the page expensive.
6. **The loopback warning's tone. Still open.** With the warning showing, the
   Allow button ends at 905px on a 900px screen, 5px below the fold. Whether the
   danger treatment causes alarm fatigue for a common, legitimate local client is
   a judgement the pack has no evidence on. The concepts answer it three ways, and
   it is the owner's call.

The captures added four more, each a number:

* On a phone the Sign in button ends at 839px of 812px, because the black stage
  is 295px tall.
* On a phone Allow ends at 963px of 812px.
* A blocked fleet's twelve reasons make the status page 1724px tall at 1440 and
  2080px at 375, about two and a half screens, in which the two reasons that
  block and the ten that only slow are one undivided stack.
* "Go to the overview" appears twice on the Not found page, as the shell's brand
  link and as the button, so a concept should not add a third.

## The concepts, and one recommendation for each screen

Every concept uses only tokens that exist today, except C, which needs the new
ones listed [below](#the-tokens-concept-c-would-add). Each works in both styles,
in light and dark, and has a phone layout.

* **A, evolution.** Keeps each page's structure and fixes its hierarchy, wording
  and spacing, so it is the least risky to ship. It is a tidy version of what is
  there.
* **B, evolution.** A different layout idea inside the same tokens and brand,
  and the one that gives the pages before sign in a frame they share.
* **C, bold.** A deliberately larger departure: the scene behind the Zoomies
  lockup becomes the page, and the state word or the tagline becomes the largest
  type in the product. It is the most memorable and costs the most.

### Sign in

Today the button ends at 590px on a 1440x900 screen and at 839px on a phone, and
the first input starts at 352px and 601px. Measured from the top of the page, in
Mission Control dark, in the password-only state:

| | First input, 1440 | Sign in button's bottom edge, 1440 | First input, 375 | Sign in button's bottom edge, 375 |
| --- | --- | --- | --- | --- |
| Today | 352 | 590 | 601 | 839 (below the fold) |
| A, the split layout | 337 | 571 | 244 | 478 |
| B, the shared card | 319 | 553 | 291 | 525 |
| C, a scene | 267 | 501 | 343 | 577 |

* **A** keeps the brand panel and the form side by side, and gives a phone one
  compact brand header in place of the 295px black stage. It is the smallest
  change and leaves the page unlike the two that follow it.
* **B** drops the brand panel for the centred card that First account and Connect
  an app also use, so every page before sign in shares one frame. All ten states
  fit in it, including a 128-character host name with nowhere to break.
* **C** makes the brand moment the page: a scene, the tagline in type larger than
  anything else in the product, and the form as a raised panel. It needs
  [six new tokens](#the-tokens-concept-c-would-add) and puts a picture behind
  the screen where people read "controller unreachable" and "rate limited".

**Recommendation: B.** It is the only concept that settles finding 1, the three
brand treatments, because it gives all three account screens one frame, and it
does so with less page than today on both a desktop and a phone. A leaves the
three pages unlike one another. C is the most memorable, but it spends the most
and does so on the screen that matters most when something has gone wrong.

Files a change to B would touch: `web/src/routes/Login.svelte`, split into the
frame and one component for each of the four states (the two-step pieces already
live in `web/src/lib/twostep/`); a new shared frame component, for example
`web/src/lib/components/AuthFrame.svelte`, that Sign in, First account and
Connect an app all use; and a new shared `Callout.svelte` that replaces the three
hand-written banners. `Logo.svelte` is used as it is, with its `mark` variant.
The Playwright specs for these pages in `web/tests/` and the screenshots in
`docs/screenshots` would be regenerated.

### First account

The first account is made once, by whoever deployed the controller, and the hard
part is finding the setup token in a log. Positions are in pixels from the top of
the page, in Mission Control dark, in the empty state:

| | Setup token field, 1440 | Create button's bottom edge, 1440 | Setup token field, 375 | Create button's bottom edge, 375 |
| --- | --- | --- | --- | --- |
| Today | 425 to 457 | 851 | 511 to 543 | 1017 |
| A, the same card | 362 to 394 | 884 | 364 to 396 | 1004 |
| B, two columns | 295 to 343 | 821 | 380 to 428 | 926 |
| C, a rail of steps | 217 to 265 | 839 | 387 to 435 | 1107 |

* **A** keeps the card, with a one-sentence introduction, the commands that find
  the token as tabs directly under its field, and the explanation behind a
  disclosure. It is the only concept whose Create button is lower than today's on
  a desktop, by 33px, and a test holds it inside the fold.
* **B** puts where to find the token beside the form from 960px, and folds that
  help into a disclosure above the field on a phone. It uses the product's 48px
  touch fields, as C does, where A keeps today's 32px.
* **C** adds a rail of what comes after the account, echoing the Overview's
  checklist, beside the form. Its phone page is the longest, 90px longer than
  today's.

**Recommendation: B.** The token is the one thing a visitor cannot guess, so the
page should show where it is, beside the field, without a click; B does that and
is shorter than today on both screens (821px against 851px, and 926px against
1017px). It also reuses the shared frame from Sign in. A's tabs keep the command
in view on a phone without a tap, and are worth borrowing if B's disclosure tests
badly.

Files: `web/src/routes/Bootstrap.svelte`, the shared frame and callout above, and
the install commands as tabs, which may use `web/src/lib/components/Tabs.svelte`
if it fits and a small new component if not.

### Connect an app

Somebody else's app sends a signed-in person here mid-task to say yes or no. The
bottom edge of the Allow button, in pixels from the top of the page, in Mission
Control dark, with the fold at 900px on a desktop and 812px on a phone:

| | A request, 1440 | With the loopback warning, 1440 | A request, 375 |
| --- | --- | --- | --- |
| Today | 820 | 905 (below the fold) | 963 (below the fold) |
| A, the same card | 769 | 873 | 949 (below the fold) |
| B, role cards | 732 | 818 | pinned to the screen's bottom edge |
| C, a diagram | 576 | 657 | 749 |

* **A** keeps the card, groups the destination and its warning in one summary,
  shortens the warning, and names the app on the primary button. It keeps the
  danger colour for the warning, with fewer words.
* **B** shows the role as two cards that say what each allows, lists what the app
  can never do, and pins the buttons to the bottom of a phone without covering
  anything that has focus. It shows the warning as a neutral note with the laptop
  icon.
* **C** draws Zoomies at one end and the app at the other, with the address it
  sends you back to as the checked destination. It labels that address as this
  computer and uses no alarm colour.

All three put Allow inside the fold on a desktop, with and without the warning,
and a test holds that. On a phone only B and C keep it in reach. A's first
version put Allow at 995px with the warning, worse than today, and the fix is
described in [findings.md](findings.md#what-the-connect-an-app-concepts-change-measured).

**Recommendation: B.** Two role cards make the decision the person came to take
the biggest thing on the page, and the pinned buttons keep Allow in reach on a
phone however long the request is. C gets there by being the shortest page, which
depends on the request staying short, and the review page's `stress` state shows a
300-character app name to test that. Take B's structure with A's shortened warning in
the danger colour. That combination is not drawn in the pack and so the checker
has not seen it, though both halves use tokens it has. The tone is a judgement
and the owner's decision: nothing here shows that a red warning causes alarm
fatigue, and a consent screen that stops looking like a warning for the one
redirect that reaches this computer is a risk of its own.

Files: `web/src/routes/McpConsent.svelte`, `web/src/lib/components/RadioGroup.svelte`
(or a role-card variant of it), the shared frame and the shared callout.

### Fleet status

A developer whose job has not started wants to know whether the fleet or their
workflow is at fault, usually on a phone. The page may use only what
`GET /api/v1/status` returns: a state, when it began, two counts as bands, two
waits, and the reasons with the server's sentence for each. No concept draws a
number the API does not send. Measured on a blocked fleet with twelve reasons, in
Classic dark, because that is the only style today's page has:

| | Page, 1440 | Page, 375 | First blocking cause ends, 1440 | First blocking cause ends, 375 |
| --- | --- | --- | --- | --- |
| Today | 1724 | 2080 | not measured | not measured |
| A, banner, tiles, grouped | 888 | 912 | 654 | 678 |
| B, one sentence | 823 | 559 | behind a disclosure | behind a disclosure |
| C, a board | 1440 | 2090 | 222 | 612 |

* **A** adds a brand header, a larger state banner and the four facts as tiles
  that stay two across on a phone, and groups the reasons by how serious they
  are, folding the ten that only slow.
* **B** says one plain sentence first, written for the person waiting, and puts
  everything else behind a disclosure. It is the shortest, and its price is that
  the cause is one tap away.
* **C** holds the state and the queue in view on one side while the reasons
  scroll on the other, with the state word at the largest size there is. It helps
  on a desktop and not on a phone, where its page is as long as today's.

In all three the state survives forced colours: the word is on the page, and a
triangle, a dashed ring and a hollow ring differ with no colour at all.

**Recommendation: A.** It cuts the page from 1724px to 888px at 1440 and from
2080px to 912px at 375, puts both blocking causes on the first screen, and keeps
every other reason one tap away, which matters on a page whose reader has nobody
to ask. B is the right trade for
a reader who only wants to know whether to wait, and could be offered as a
summary line over A. C is a desktop idea that needs A's folding to help a phone.

Files: `web/src/status/StatusPage.svelte` and `web/src/status/present.ts`, for
the grouping and the tiles. Two decisions sit outside the pack. The page ignores
the visual style today because its content security policy allows no inline
script to apply one, so giving it Mission Control means the server writes the
style into the HTML (`internal/api/handlers_status.go`). And a logo on a page for
people with no account is a product choice this pack only makes visible.

### Not found

A wrong address is most likely somebody else's stale link. All three concepts
share one rule, and it is a rule, not a style: none says what the address was.
A path taken from the URL is text anybody can put in a link on this controller's
domain, which is why this page never repeats what it was asked for.

* **A** keeps the page and adds a way back beside the way forward.
* **B** keeps the page and offers the three places most people want, Overview,
  Pools and Runners, with the product's own sentence for each from
  `web/src/lib/shell/sections.ts`.
* **C** makes the mark large and offers one calm action with nothing else to
  read.

**Recommendation: B.** The visitor does not know where they have landed, and B
gives them three places the phone's bottom bar already teaches them, without
adding a second "Go to the overview". A's "Go back" is worth keeping as a second
action, with one condition: it needs browser history, and a link opened from
another app has none, so the button must not render then. Nothing is measured
here, because the page is one screen in every concept.

Files: `web/src/routes/NotFound.svelte`, which is 23 lines today and would stop
using `EmptyState.svelte` for B's cards.

## The tokens concept C would add

Concept C defines its tokens in one file, [concept-c-tokens.css](concept-c-tokens.css),
scoped to the concept so the other two cannot lean on them. Choosing C would add
each to `web/src/lib/styles/tokens.css` and describe it in
`docs/ui-guidelines.md` in the section named here. The cost of C is this table.

| Token | Value | For | Guideline section it would change |
| --- | --- | --- | --- |
| `--z-gate-ground` | `#070b14` | The scene: a navy cast, deeper than the panel's pure black | 1.1 Colour |
| `--z-gate-edge` | `#4f6280` | The panel's edge against the scene | 1.1 Colour |
| `--z-gate-scene-opacity` | `0.55` | How much of the backdrop image shows behind the headline | 1.1 Colour |
| `--z-gate-shadow` | `0 24px 64px -16px rgb(0 0 0 / 0.6)` | The raised panel | 1.5 Elevation |
| `--z-gate-display` | `clamp(var(--z-text-3xl), 4.5vw, 3.75rem)` | The tagline, the largest type in the product, used once | 1.2 Type |
| `--z-gate-display-leading` | `1.1` | Its line height | 1.2 Type |
| `--z-hero-word` | `clamp(var(--z-text-3xl), 6vw, 4.5rem)` | The status word on Fleet status | 1.2 Type |
| `--z-hero-shape` | `5rem` | The shape drawn beside that word | 1.4 Radii, borders and geometry |

The pairs C relies on are held to the product's minimums: the existing panel
text on `--z-gate-ground` is 19.68:1, the muted panel text 10.34:1, and
`--z-gate-edge` against it 3.18:1, so the edge reads as a shape. They are in
[concept-c-pairs.json](concept-c-pairs.json) and the checker reads them.

## What was checked

[viewport-review.json](viewport-review.json) records 1,044 combinations, 0
findings and 6 known, from `node design/webui-refresh-review/tools/check.mjs`.
Each combination is a screen, a state, a concept, a style, a mode and a width of
320, 375 or 1440, and the checker looks for:

* a page that scrolls sideways, and text clipped by `overflow: clip` rather than
  scrolled;
* a console error or an uncaught exception;
* a control with no accessible name, a page with no `h1` or with two, a page with
  no `main` landmark or with two, a positive `tabindex`, and a control that hides
  its focus ring;
* contrast computed from the real token values in the browser, at 4.5:1 for text
  and 3:1 for the edge of a control.

It reports and never edits a concept to pass. Each rule was shown to fail on a
page built to break it before it was trusted. Every test in the pack passes under
`node --test`, and `go test ./internal/docs` holds the pack to the repository's
voice and placement rules.

### One known issue no concept can fix

Classic's control edge, `--z-border-strong` against `--z-surface`, is 1.62:1 in
both light and dark, under the 3:1 minimum for the edge of a control. Mission
Control's is past 3:1. The six known entries are this one pair, once for each
of the three concepts in Classic light and Classic dark. No concept built from tokens can change it, so the
checker records it with its reason and a floor, and it would fail if it got
worse. **Raising Classic's `--z-border-strong` is recommended as a token change
independent of any concept**, and would fix it on every page in the product, not
only these five.

## Not claimed

* No usability study. Nobody was watched using any of this, and "recommended"
  means argued from measurements and the source, not tested on people.
* No screen reader session. The checker reads the accessibility tree and proves
  names, headings and landmarks exist; it does not say what a person hears.
* No measured performance change. Page heights and positions are measured; load
  time, bundle size and render cost are not.
* No end-to-end test of the real sign-in, bootstrap or consent flows. The
  concepts are static pages with illustrative data, and nothing here drives a
  controller.
* Nothing in a mockup is a specification. Counts, names and host names are made
  up, and the copy that is new is proposed, for the owner to accept or reword.
* The Playwright MCP the web design skill prefers is not installed in this
  environment, so its automatic verification was skipped. The repository's own
  `@playwright/test` and the preinstalled Chromium did the work.

## What happens next

The owner decides the direction, and nothing changes in the product until they
do. The recommendation above is one choice per screen:

| Screen | Recommended | Decision it needs from the owner |
| --- | --- | --- |
| Sign in | B | Whether the three account pages should share one frame |
| First account | B | None beyond the frame |
| Connect an app | B | The loopback warning's tone |
| Fleet status | A | Whether the status page may take the visual style and a logo |
| Not found | B | None |

Once chosen, the signed-in pages follow in four packs, shell and monitoring
first, each with its own plan.

To regenerate any of this:

| Command | What it writes |
| --- | --- |
| `node design/webui-refresh-review/tools/capture.mjs` | the `concept-*.png` pictures |
| `node design/webui-refresh-review/tools/check.mjs --out design/webui-refresh-review/public/viewport-review.json` | the check record |
| `node design/webui-refresh-review/tools/capture-current.mjs` | the `current-*.png` pictures and their measurements; needs `make build` first |
| `node design/webui-refresh-review/tools/make-icons.mjs` | `icons.js` from Lucide and the product's GitHub mark |
| `node --test "design/webui-refresh-review/tools/**/*.test.mjs" "design/webui-refresh-review/public/**/*.test.mjs"` | nothing: the pack's tests |
