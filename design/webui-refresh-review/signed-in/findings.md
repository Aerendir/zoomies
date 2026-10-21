# Findings from the signed-in pages today

21 October 2026. Working notes for the signed-in README. Every number here is in
[current-measurements.json](current-measurements.json), written by
`tools/capture-signed-in.mjs` against the real controller: the seeded demo fleet,
authentication off on loopback, Mission Control dark, at 1440x900 and 375x812. A
picture is the whole page, capped at 2400px, and the height is recorded beside it.
Nothing was stubbed. The demo fleet is small (two pools, three hosts, ten runners),
so a real fleet's pages are taller than these.

## What the pages are like

| Page | Height at 1440 | Height at 375 | Tables | Columns in the first |
| --- | --- | --- | --- | --- |
| overview | 1238 | 2188 | 1 | 0 |
| pools | 1117 | 2086 | 1 | 14 |
| runners | 1854 | 2914 | 1 | 11 |
| queue | 1609 | 2584 | 1 | 9 |
| jobs | 1251 | 1532 | 1 | 12 |
| workflows | 900 | 1616 | 1 | 11 |
| comparisons | 1191 | 1979 | 0 | 0 |
| usage | 2692 | 6587 | 3 | 0 |
| hosts | 1681 | 3515 | 0 | 0 |
| providers | 1534 | 2346 | 0 | 0 |
| installations | 1276 | 2394 | 0 | 0 |
| repairs | 900 | 947 | 0 | 0 |
| migrate | 900 | 950 | 0 | 0 |
| kennel | 4423 | 7709 | 1 | 2 |
| ai-context | 977 | 1399 | 0 | 0 |
| audit | 900 | 1384 | 1 | 5 |
| settings | 900 | 1474 | 0 | 0 |
| pool-new | 2265 | 2582 | 0 | 0 |
| host-new | 1365 | 1929 | 0 | 0 |
| provider-new | 1023 | 1462 | 0 | 0 |
| pool-detail | 2389 | 5247 | 0 | 0 |
| host-detail | 1069 | 1616 | 0 | 0 |
| runner-detail | 2277 | 4796 | 0 | 0 |

* **Nothing scrolls sideways at 375.** The grids scroll inside their own region, as
  the guidelines ask, so the cost on a phone is height and not width.
* **Five pages are more than three screens tall on a phone**: Usage (6,587px),
  Kennel Club (7,709px), a pool (5,247px), a runner (4,796px) and Hosts (3,515px).
  These are the pages whose structure the look has most to say about.
* **Most list pages open the same way** (Pools, Queue, Jobs, Runners, Hosts and
  Providers; Workflows is the exception): a title, a sentence, four to six small
  stat tiles, a paragraph that explains them, filters, and then the grid. On
  Runners that is six tiles, a provisioning strip, a lifecycle diagram, two
  paragraphs and a disclosure before the first row at 1,000px down, and the grid
  then truncates names ("zoomies-2vcpu-4gb...") because its twelve columns share
  1,160px. The first row is below the fold of a 900px screen.
* **Grids are wide.** Pools has 14 columns, Jobs 12, Runners and Workflows 11,
  Queue 9; at 1440 the columns that name things (name, pool, host) are the ones
  that lose room.
* **The sidebar already does what the portal's does**: groups, one entry filled,
  a footer. What differs is everything inside the page.
* **Each page repeats the same pieces by hand**: stat tiles, a filter row, a
  status cell, a page header. That is the structural finding behind the look: a
  kit of a few parts, used everywhere, is what lets them agree.

## What the look would change, page by page

The portal's pages say one thing at a time and put the evidence under it. The
mocks do the same: a calm header, one summary where a page has one, the data in a
card of its own, and the explanation on request rather than in front of the
content. A table stays a table.
