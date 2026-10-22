# Kennel Club layout review

A design review and clickable mock for the Kennel Club pages in both visual
styles. Nothing here ships as code; it was approved on 19 October 2026 and the
changes are recorded in
[decision 0013](../../roadmap/decisions/0013-the-kennel-pages-answer-first-and-fold-the-depth.md).

Open [mockups.html](mockups.html) in a browser. The review bar switches screen,
Classic or Mission Control, light or dark, and turns the numbered callouts on.
It uses illustrative data and sends no requests.

Captures are in [screens](screens) for a quick look without opening the mock.
The shipped Classic captures live in `docs/screenshots/kennel-*.webp`. Mission
Control is captured in [current](current), because the shipped screenshot pass
still writes Kennel Club in Classic only.

Both visual styles were reviewed at 1440 and 375, in light and dark. The
structure is identical between styles, so the proposals apply to both; Mission
Control is taller and is the layout that will break first.

## What the review found

The current pages are careful and well written, but the layout makes the reader
do the ranking. The same information is often repeated in a heading, a panel
description and a card detail.

1. **The Overview answers with seven equal cards.** The one question an
   operator has, "is anything wrong?", is one card among Repositories, Best in
   show, Partly checked, Errors, Warnings and Waived. The seventh and eighth
   cards wrap onto a second row with empty space beside them. The answer
   should be the first thing on the page, not a number to compare.
2. **Every section is open at once.** "By check" lists all checks with equal
   weight, including the ones with nothing open. "What Kennel Club can see"
   occupies a full panel even when every source was read. The page is long
   because nothing is folded, not because the information is large.
3. **The Repositories filter bar stacks.** `Select` is `width: 100%`, so five
   selects become five full-width rows. The table starts below the fold. The
   page already knows the five questions people ask, because the Overview
   counts them, but it does not offer them as quick filters.
4. **The section has three navigation layers and repeats itself.** The global
   sidebar, the Kennel rail, the page title and the breadcrumb all name the
   same place. On the repository page, five tabs then split again into
   sections. The in-page map is missing exactly where the pages are longest.
5. **Findings are full cards by default.** Each finding repeats severity,
   title, summary, "What to change", prompt, evidence, code, Ask Eli and
   Waive. Two findings fill more than a screen. Coverage and waivers are
   pushed below them, and the default tab is fleet activity rather than the
   findings that brought the reader here.
6. **AI Context leads with marketing.** Three benefit columns and two long
   explainers sit above the repositories. A configured card then offers six or
   more buttons. The page's job, managing repositories, starts below the fold.
7. **The rail switch dominates the section.** It is the largest control in the
   rail, it wraps onto two or three lines, and its status sentence competes
   with the navigation. The switch matters, but it is not the page.
8. **Theme parity is structural, not visual.** Classic and Mission Control
   render the same components. Mission Control's larger type makes every long
   panel and every wrapped filter row more expensive vertically, so the
   simplification helps both styles and helps Mission Control most.

## The proposal

Five principles:

- **Answer first.** A verdict band says what needs attention and what is
  clean. Supporting totals are still there, but they are secondary.
- **One map per page.** A sticky in-page anchor row carries counts and keeps
  its current section lit. On a repository, the three tabs are the map and the
  finding areas are the filter chips.
- **Fold depth, not importance.** A check with nothing open, a source that was
  read, and a prompt are all one disclosure away. Nothing is removed.
- **Put controls where the decision is.** Quick filters carry counts. The
  destructive tracking action leaves the primary button row. Each AI Context
  card has one contextual primary action and an overflow menu.
- **Keep the contract.** Status colours, shapes and labels, tokens, URLs,
  permissions and API routes are unchanged. The mock uses the existing
  Classic and Mission Control values.

## Screen notes

### Overview

- A verdict band replaces the seven equal cards: one attention sentence, one
  primary action, then Best in show, Partly checked and Repositories.
- All counts move into one pill disclosure.
- A two-column body puts the action list and checks on the left, and read
  confidence and scope on the right. This uses the width instead of leaving a
  second card row half empty.
- "By check" shows the two checks that are affecting a repository. The ten
  quiet checks fold away.
- Read confidence is an aside with a five-segment bar and a source list. A
  partial read says the counts are a minimum next to the counts.
- A sticky anchor row gives Summary, Attention, Checks and Read confidence.

### Repositories

- Quick filters for All, Needs attention, No open findings, Partly checked and
  Not tracked carry the counts from the Overview.
- Search, Filters and the activity scope sit on one row. The filters move into
  one popover, with removable chips when applied.
- The table keeps Repository, Standing, Open findings and Last read. Visibility
  moves into Columns.
- The scope sentence says how many are shown and makes the hidden count a
  link.
- On a phone the table becomes cards and the selection column waits for a
  deliberate select mode rather than adding a lone tick box to every card.

### Repository

- Three tabs: Findings, Fleet activity and AI Context. Protection and Agent
  guidance become areas inside Findings, with their own filter chips and
  deep-linkable addresses. This is the biggest structural proposal; see
  "Decisions to confirm".
- Tracking is a sentence with a link, not a switch in the primary action row.
  Recheck is the one primary action. Refresh is an icon. Stop tracking moves
  into the overflow menu.
- Findings use a list and detail workbench on desktop. The list is scannable,
  the selected finding gets the room, and prompt, evidence and technical
  details fold away. A single-column accordion is the alternative and the
  phone layout.
- Coverage is a line under the selected finding with details on demand.
- Waived findings remain a disclosure at the foot.

### AI Context

- Repositories come first. The summary band counts repositories in setup,
  ready and needing attention.
- The card shows branch, output, installation and last check in one compact
  row. The action that fits the state is the button; reinstall, amend, remove,
  instructions and badge move into the overflow menu.
- The benefits and setup explanations become one "How AI Context works"
  disclosure below the list. The empty state can still carry the full version.

## Decisions to confirm before code

1. **Findings first on a repository.** Make Findings the default tab when the
   repository has open findings, and Fleet activity the default when it does
   not. This changes decision
   [0008](../../roadmap/decisions/0008-the-repository-page-leads-with-what-the-fleet-knows.md).
2. **Merge Protection and Agent guidance into Findings.** Keep their routes as
   aliases that open Findings with the matching area selected. This changes the
   tab list but keeps every finding, waiver and permission surface.
3. **Keep the section rail.** Decision
   [0010](../../roadmap/decisions/0010-kennel-club-has-a-side-menu-and-the-switch-is-in-it.md)
   is still the right shape. The proposal only makes the switch calmer and
   removes the duplicate breadcrumb, not the rail.
4. **Tracking moves out of the header.** The track switch becomes a state
   sentence with a link and the destructive action moves to overflow. This is
   a workflow change for administrators, not a permission change.
5. **AI Context explainers move below the list.** The text is kept, but it is
   no longer the first thing a returning operator sees.

## Implementation notes

- Use native `details` and `summary` for folding. Keep the disclosure state in
  the URL only where a link should open it, as the current waivers do.
- Give the anchor row `scroll-margin-top` equal to the sticky bar height so a
  focused heading is not obscured.
- The in-page nav is a `nav` with `aria-current` on the active item. The counts
  are text inside the link, not separate badges.
- Keep status colour plus shape plus label. The workbench list uses the same
  severity badge as the detail.
- Touch targets are 44px under a coarse pointer. The mobile table becomes
  cards, and the selection column is not shown until select mode.
- Test both visual styles in both colour modes at 1440 and 375. Mission Control
  is the taller of the two, so it is the one that will break first.
- No new dependency, API route, token or permission is needed.

## Files

- `mockups.html`: the standalone, interactive mock.
- `screens/overview-mission-dark.png`: Overview in Mission Control, dark.
- `screens/overview-classic-light.png`: Overview in Classic, light.
- `screens/repositories-mission-dark.png`: Repositories in Mission Control.
- `screens/repository-mission-dark.png`: Findings workbench.
- `screens/repository-single-mission-dark.png`: Findings single column.
- `screens/ai-context-mission-dark.png`: AI Context.
- `screens/overview-mobile-mission-dark.png`: Overview at 375.
- `screens/repository-mobile-classic-dark.png`: Repository at 375.
- `current/mission-control-overview-dark.webp`: the current Overview.
- `current/mission-control-repositories-dark.webp`: the current list.
- `current/mission-control-repository-dark.webp`: the current repository page.
- `current/mission-control-ai-context-dark.webp`: the current AI Context page.
