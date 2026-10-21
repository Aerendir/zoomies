# Web UI refresh review, pack 2: the signed-in pages

The owner chose the portal's look for the pages before sign in, and asked for the
signed-in pages "using this design as the approach and concept". This pack is
that: a mock of every signed-in page in the portal's look, each beside a capture
of the page today, built from one small kit of parts and one fleet of data, and
judged by the same checker as [pack 1](../public/README.md). It is a design to
react to. **Nothing under `web/src` has changed**, and none of it is implemented.

The method, the direction and the reasons are in
[SPEC.md](../SPEC.md) ("Direction chosen after pack 1") and the plan that built
this pack is [PLAN-signed-in.md](../PLAN-signed-in.md). What today's pages are
like, measured, is in [findings.md](findings.md).

## How to look at it

```sh
node design/webui-refresh-review/tools/serve.mjs
# then open http://127.0.0.1:8101/signed-in/mockups.html
```

The page has five controls. **Screen** and **State** pick what is drawn;
**Concept** is Current (a capture of the page today) or the portal style;
**Style** and **Mode** are the product's own two styles in light and dark. The
state of a list is one of rows, loading, empty, error with a retry, or stale rows
(the refresh failed and what is shown is from before). Narrow the window under
800px to see the phone's bottom bar and its More sheet.

The data is one frozen fleet in [fixture.js](fixture.js), four pools on four
hosts. Every field in it is one the REST API returns, which
[fixture.test.mjs](fixture.test.mjs) holds against `api/openapi.yaml`, and every
total a page shows is computed from its rows, so the Overview and the page it
links to cannot disagree.

## The pictures

Each page is photographed whole, in the state that shows it best. Today's pages
are from the real controller with the seeded demo fleet.

| Page | Today, 1440 | Mock, Mission Control dark, 1440 | Mock, phone | Mock, Classic light, 1440 |
| --- | --- | --- | --- | --- |
| Overview | [current-overview-1440.png](current-overview-1440.png) | [concept-overview-1440.png](concept-overview-1440.png) | [concept-overview-375.png](concept-overview-375.png) | [concept-overview-classic-light-1440.png](concept-overview-classic-light-1440.png) |
| Queue | [current-queue-1440.png](current-queue-1440.png) | [concept-queue-1440.png](concept-queue-1440.png) | [concept-queue-375.png](concept-queue-375.png) | [concept-queue-classic-light-1440.png](concept-queue-classic-light-1440.png) |
| Jobs | [current-jobs-1440.png](current-jobs-1440.png) | [concept-jobs-1440.png](concept-jobs-1440.png) | [concept-jobs-375.png](concept-jobs-375.png) | [concept-jobs-classic-light-1440.png](concept-jobs-classic-light-1440.png) |
| Runners | [current-runners-1440.png](current-runners-1440.png) | [concept-runners-1440.png](concept-runners-1440.png) | [concept-runners-375.png](concept-runners-375.png) | [concept-runners-classic-light-1440.png](concept-runners-classic-light-1440.png) |
| Runner detail | [current-runner-detail-1440.png](current-runner-detail-1440.png) | [concept-runner-detail-1440.png](concept-runner-detail-1440.png) | [concept-runner-detail-375.png](concept-runner-detail-375.png) | [concept-runner-detail-classic-light-1440.png](concept-runner-detail-classic-light-1440.png) |
| Workflows | [current-workflows-1440.png](current-workflows-1440.png) | [concept-workflows-1440.png](concept-workflows-1440.png) | [concept-workflows-375.png](concept-workflows-375.png) | [concept-workflows-classic-light-1440.png](concept-workflows-classic-light-1440.png) |
| Pools | [current-pools-1440.png](current-pools-1440.png) | [concept-pools-1440.png](concept-pools-1440.png) | [concept-pools-375.png](concept-pools-375.png) | [concept-pools-classic-light-1440.png](concept-pools-classic-light-1440.png) |
| Pool detail | [current-pool-detail-1440.png](current-pool-detail-1440.png) | [concept-pool-detail-1440.png](concept-pool-detail-1440.png) | [concept-pool-detail-375.png](concept-pool-detail-375.png) | [concept-pool-detail-classic-light-1440.png](concept-pool-detail-classic-light-1440.png) |
| Hosts | [current-hosts-1440.png](current-hosts-1440.png) | [concept-hosts-1440.png](concept-hosts-1440.png) | [concept-hosts-375.png](concept-hosts-375.png) | [concept-hosts-classic-light-1440.png](concept-hosts-classic-light-1440.png) |
| Host detail | [current-host-detail-1440.png](current-host-detail-1440.png) | [concept-host-detail-1440.png](concept-host-detail-1440.png) | [concept-host-detail-375.png](concept-host-detail-375.png) | [concept-host-detail-classic-light-1440.png](concept-host-detail-classic-light-1440.png) |
| Providers | [current-providers-1440.png](current-providers-1440.png) | [concept-providers-1440.png](concept-providers-1440.png) | [concept-providers-375.png](concept-providers-375.png) | [concept-providers-classic-light-1440.png](concept-providers-classic-light-1440.png) |
| Usage | [current-usage-1440.png](current-usage-1440.png) | [concept-usage-1440.png](concept-usage-1440.png) | [concept-usage-375.png](concept-usage-375.png) | [concept-usage-classic-light-1440.png](concept-usage-classic-light-1440.png) |
| Comparisons | [current-comparisons-1440.png](current-comparisons-1440.png) | [concept-comparisons-1440.png](concept-comparisons-1440.png) | [concept-comparisons-375.png](concept-comparisons-375.png) | [concept-comparisons-classic-light-1440.png](concept-comparisons-classic-light-1440.png) |
| Kennel Club | [current-kennel-1440.png](current-kennel-1440.png) | [concept-kennel-1440.png](concept-kennel-1440.png) | [concept-kennel-375.png](concept-kennel-375.png) | [concept-kennel-classic-light-1440.png](concept-kennel-classic-light-1440.png) |
| Eli repairs | [current-repairs-1440.png](current-repairs-1440.png) | [concept-repairs-1440.png](concept-repairs-1440.png) | [concept-repairs-375.png](concept-repairs-375.png) | [concept-repairs-classic-light-1440.png](concept-repairs-classic-light-1440.png) |
| AI context | [current-ai-context-1440.png](current-ai-context-1440.png) | [concept-ai-context-1440.png](concept-ai-context-1440.png) | [concept-ai-context-375.png](concept-ai-context-375.png) | [concept-ai-context-classic-light-1440.png](concept-ai-context-classic-light-1440.png) |
| Settings | [current-settings-1440.png](current-settings-1440.png) | [concept-settings-1440.png](concept-settings-1440.png) | [concept-settings-375.png](concept-settings-375.png) | [concept-settings-classic-light-1440.png](concept-settings-classic-light-1440.png) |
| Audit | [current-audit-1440.png](current-audit-1440.png) | [concept-audit-1440.png](concept-audit-1440.png) | [concept-audit-375.png](concept-audit-375.png) | [concept-audit-classic-light-1440.png](concept-audit-classic-light-1440.png) |
| Installations | [current-installations-1440.png](current-installations-1440.png) | [concept-installations-1440.png](concept-installations-1440.png) | [concept-installations-375.png](concept-installations-375.png) | [concept-installations-classic-light-1440.png](concept-installations-classic-light-1440.png) |
| Migrate | [current-migrate-1440.png](current-migrate-1440.png) | [concept-migrate-1440.png](concept-migrate-1440.png) | [concept-migrate-375.png](concept-migrate-375.png) | [concept-migrate-classic-light-1440.png](concept-migrate-classic-light-1440.png) |
| New pool | [current-pool-new-1440.png](current-pool-new-1440.png) | [concept-pool-new-1440.png](concept-pool-new-1440.png) | [concept-pool-new-375.png](concept-pool-new-375.png) | [concept-pool-new-classic-light-1440.png](concept-pool-new-classic-light-1440.png) |
| Add a host | [current-host-new-1440.png](current-host-new-1440.png) | [concept-host-new-1440.png](concept-host-new-1440.png) | [concept-host-new-375.png](concept-host-new-375.png) | [concept-host-new-classic-light-1440.png](concept-host-new-classic-light-1440.png) |
| Add a provider | [current-provider-new-1440.png](current-provider-new-1440.png) | [concept-provider-new-1440.png](concept-provider-new-1440.png) | [concept-provider-new-375.png](concept-provider-new-375.png) | [concept-provider-new-classic-light-1440.png](concept-provider-new-classic-light-1440.png) |

## What the look changed, page by page

The portal's pages say one thing at a time and put the evidence under it. The
mocks do the same: a calm header, one answer in the stage where a page has one,
the data in a card of its own, and the explanation on request and not in front
of the content. A table stays a table and scrolls inside its card, never the
page. Heights are the stage alone, the way the capture frames it, so a page
shorter than the window is as tall as the window. Today's fleet is smaller than
the mock's (the demo has two pools and ten runners, the fixture four pools and
eleven), so the heights compare layouts and not what a real fleet would do.

### Watching the fleet

| Page | Today, 1440 | Columns today | Mock, 1440 | Columns in the mock | Today, phone | Mock, phone |
| --- | --- | --- | --- | --- | --- | --- |
| Overview | 1238 | none | 1611 | none | 2188 | 2627 |
| Queue | 1609 | 9 | 971 | 6 | 2584 | 1269 |
| Jobs | 1251 | 12 | 1393 | 8 | 1532 | 1541 |
| Runners | 1854 | 11 | 1123 | 8 | 2914 | 1425 |
| Runner detail | 2277 | none | 872 | none | 4796 | 1227 |
| Workflows | 900 | 11 | 1255 | 7 | 1616 | 1403 |

Today's Overview is six tiles, a lifecycle diagram and two paragraphs. The mock
answers the question the page is opened for, whether anything is wrong, in the
stage, and puts four numbers, a card per pool and the problems with what to do
about each under it. It is **taller** than today's, because it carries more of
the fleet on the page; the cost is height, and the gain is that nothing needs a
click to be known. The list pages drop the tiles and the diagram and put the
filters and the rows first, and hold a long name to a line with the whole of it
in the title.

### Managing the fleet

| Page | Today, 1440 | Columns today | Mock, 1440 | Columns in the mock | Today, phone | Mock, phone |
| --- | --- | --- | --- | --- | --- | --- |
| Pools | 1117 | 14 | 872 | 7 | 2086 | 942 |
| Pool detail | 2389 | none | 1254 | 5 | 5247 | 1564 |
| Hosts | 1681 | none | 872 | 8 | 3515 | 890 |
| Host detail | 1069 | none | 986 | 4 | 1616 | 1280 |
| Providers | 1534 | none | 872 | 7 | 2346 | 692 |

A pool, a host and a runner each get a page that is a header, a few numbers, the
facts, and the things that belong to it. A host that is not reporting says so in
words above the numbers, and a runner that failed says what happened and does
not claim a job.

### Insight and repairs

| Page | Today, 1440 | Columns today | Mock, 1440 | Columns in the mock | Today, phone | Mock, phone |
| --- | --- | --- | --- | --- | --- | --- |
| Usage | 2692 | none | 1332 | 5 | 6587 | 1626 |
| Comparisons | 1191 | none | 1182 | 4 | 1979 | 1288 |
| Kennel Club | 4423 | 2 | 1335 | 5 | 7709 | 1799 |
| Eli repairs | 900 | none | 1038 | 6 | 947 | 1292 |
| AI context | 977 | none | 937 | 6 | 1399 | 1061 |

Each page leads with the one answer it exists to give (how did the week go, did
the change help, is anything wrong, what did Eli do) in the stage, and the stage's
sentence is computed from the rows under it. Kennel Club is the page with most to
gain, since today it is the tallest page in the capture.

### Settings and setup

| Page | Today, 1440 | Columns today | Mock, 1440 | Columns in the mock | Today, phone | Mock, phone |
| --- | --- | --- | --- | --- | --- | --- |
| Settings | 900 | none | 931 | none | 1474 | 1525 |
| Audit | 900 | 5 | 952 | 6 | 1384 | 1084 |
| Installations | 1276 | none | 872 | 7 | 2394 | 692 |
| Migrate | 900 | none | 899 | none | 950 | 1221 |
| New pool | 2265 | none | 872 | none | 2582 | 1127 |
| Add a host | 1365 | none | 872 | none | 1929 | 873 |
| Add a provider | 1023 | none | 872 | none | 1462 | 995 |

Settings is sectioned cards with their icon tiles and a bar that says a change
has not been saved. The setup flows (a pool, a host, a provider) become the
portal's numbered steps, one step on screen at a time, with a wrong field named
in the field's own words.

## What every page shares

* **A shell**: the product's own sidebar, in its four groups and its order, with
  the Overview alone above them; a top bar; and on a phone a bar of the four
  sections an operator watches a fleet with and a More sheet for the rest. The
  sheet is a native popover, so it opens with focus on its Close button, closes
  on Escape, gives focus back and dims the page behind it. A native popover does
  not hold Tab inside it: the product's own menu on a phone (`NavMenu`) does, by
  making the page behind it inert, and the real build keeps that behaviour.
* **A page header**: small capitals, the title, a short blue rule, one sentence.
* **A card, a stat, a stage, a table card, steps and a checklist.**
* **A pill**: a word and a shape (disc, diamond, ring, triangle, square, bar),
  so a state survives greyscale and forced colours. Colour is the third cue.
* **The four states of a list**, drawn by one function, so no page can forget one.

## Where it departs from the portal, and why

* **Density.** The portal's pages are marketing pages with room to breathe. A
  fleet page has to put rows on the first screen, so cards are smaller and the
  stage is shorter.
* **No decoration in status colours.** A pill's colour is only ever a state, and
  the stage carries neutral white and the brand blue only, because the panel does
  not change with the theme.
* **Tables scroll inside their card.** The portal has none.
* **No dog on every page.** The stage carries the mark once, at the top of a page
  that has a stage, and nowhere else.

## Tokens

The kit proposes **no new tokens**: [kit-tokens.css](kit-tokens.css) is empty on
purpose, and everything is drawn from `tokens.css` as it stands. What
[kit.css](kit.css) writes by hand is a handful of one-off layout measures, which
`web/AGENTS.md` allows and which are listed here so none is hidden.

| Length | Where it is used |
| --- | --- |
| `80rem` | the widest a page's content runs |
| `40rem` | the longest line of a page's subtitle |
| `32rem` | the longest line of a stage's sentence |
| `36rem` | the longest line of an empty state's explanation |
| `14rem` | the widest a truncated table cell grows before it clips, and the narrowest a stat is allowed |
| `20rem` | the narrowest a card in a two-up row is allowed |
| `10rem` | the narrowest a stat is allowed on a phone, and the widest the mark in a stage grows |
| `6rem` | the narrowest the mark in a stage shrinks to |
| `12rem` | the narrowest a list row's main text is allowed |
| `3.5rem` | the width of an outlined numeral |
| `5.5rem` | the mark's size in a stage on a phone |
| `1.4s` | the pulse of a loading placeholder, still for anyone who asks for less motion |
| `0.5` | the dimmest a loading placeholder pulses to |
| `100vh` | the tallest the sidebar grows before it scrolls inside itself |
| `80vh` | the tallest the phone's More sheet grows before it scrolls |

## What was checked

[viewport-review.json](viewport-review.json) is what `node
design/webui-refresh-review/tools/check.mjs --pack signed-in` wrote: 1,392
combinations of screen, state, style, mode and width (320, 375 and 1440), with
0 findings and 4 known, on the 18 pairs of tokens the public pack
checks and 12 the kit adds (the stage's words and accent on Zoomies Black, the
card surface, each pill's word on its tint, a filter's edge). What it looks for is
the same as in pack 1: a page that scrolls sideways or cuts a sentence off, a
control with no name, a focus ring that has gone, a colour pair under 4.5:1 for
text or 3:1 for an edge, a landmark that is not one.

The checker learned two things on the way, each with a test that failed first: a
table inside a region that scrolls is not "cut off", and a region that hides its
spill still is.

Tests hold the rest: the fixture against the API's schema and its totals against
its rows, the Overview's numbers against the fleet, every list page in all five
states and at 320, a 300-character name and a 120-character label held to a line
with all of the text in the title and shown whole while the row has keyboard focus, every row action, filter, page action and search at 44px on a phone, the More
sheet opening and closing and returning focus, the pill shapes being six
different shapes, and a wrong field being marked and described.

### One known issue no mock can fix

Classic's control edge, `--z-border-strong`, is about 1.62:1 against the page
surface and the raised card surface in light, and 1.48:1 on the card in dark,
under the 3:1 minimum for the edge of a control. Mission Control's is past 3:1.
It is the same token as in pack 1 and one change to it clears every instance.
The checker reports it as known every time, so a green run is never silent about
it.

## Not claimed

* **A usability study.** Nobody has used these pages to do anything.
* **A screen reader pass.** The checker looks for a name, a landmark and a focus
  ring; it does not read a page aloud.
* **Performance.** The mocks are plain HTML; what a built page weighs and how it
  paints is not measured. The status page's budget is untouched, since nothing
  here is built into the product.
* **End-to-end behaviour.** The mocks send no request. A retry, a refresh and a
  save go nowhere.
* **A real fleet's data.** The fixture is illustrative and small, so the heights
  compare layouts and not what a fleet of two hundred runners would do. The
  Comparisons figures are a design, not API output.

## What happens next

Nothing here is implemented. Each page is a separate task, in the order the owner
chooses, and starts from its mock. The kit is the first of them: the shell, the
page header, the card, the pill and the four states of a list are what every page
needs, and building them once in `web/src/lib` is what lets a page after that be
a small change.
