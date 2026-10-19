# Overview design review

19 October 2026. Original concept review, prepared before application changes.
Concept A has since been implemented; see the [implementation captures and
validation record](implemented/README.md).

## Visual concepts

- [A: Calm operations](concept-a-calm-operations.png), the recommended continuous
  page, shown in Mission Control dark, Classic light and a phone layout.
- [B: Focus workspace](concept-b-focus-workspace.png), the view-based alternative,
  with a desktop, phone and previews of the supporting views.

The boards show hierarchy, disclosure and visual direction. View them at their
original size. Actual interface text, type sizes and accessible controls should
follow this review and the existing UI contract rather than the generated
image's small text. In A, the event preview's 'View events' means expanding the
existing feed in place, not introducing another route.

## Recommendation

Use **A, Calm operations** for both Classic and Mission Control. Give both
styles the same information architecture, with their existing palettes and
density. Answer three questions in order: does anything need attention, what
is happening now, and where should I investigate?

**B, Focus workspace** is an alternative for people who frequently investigate
history or hosts. It keeps the live summary visible and puts supporting detail
in separate views. It reduces scrolling further, but makes simultaneous
monitoring harder. The visual style and the layout concept are independent:
either concept can use either style, in light or dark mode.

The generated visual boards are illustrative design sketches. Their sample
numbers, small text and icons are not specifications or validated product
screens. The behaviour and data rules below govern any later implementation.

## Evidence reviewed

- Current `web/src/routes/Overview.svelte` and its overview components, including
  metrics, problem summary, recent failures, repair offers and the host chart.
- `web/AGENTS.md`, `docs/ui-guidelines.md` and the theme token definitions.
- Existing real-product desktop and phone captures in `docs/screenshots` and
  `docs/screenshots/mission-control`, including light and dark examples.
- The previous concepts in `design/theme-review`, to distinguish earlier
  proposals from the current implementation.

This review uses checked-in captures and current source, rather than a new live
browser session. No usability study, runtime accessibility audit or measured
performance improvement is claimed. There is no dedicated UX design skill in
the available skill catalogue; the repository's design guidance and the
available image-generation tool support this review.

## What needs simplifying

| Finding | User cost | Proposed change |
| --- | --- | --- |
| Classic leads with the activity matrix and six metric tiles. | History and timings delay current issues and work. | Put attention and four live metrics first in both styles. |
| Mission Control already prioritises issues and collapses timings, but its timing disclosure still precedes current work. | Supporting diagnostics interrupt the main task. | Move timing detail below pools and active jobs. |
| The same page has different section order by style. | Switching appearance changes where information lives. | Keep a stable order; let style change appearance and density. |
| The header repeats Overview and explains several time windows in prose. | A long introduction takes valuable phone space. | Use a short page introduction and local time-window labels. Retain accessible page and shell titles. |
| Scope switches recur in the page and panels. | People must work out whether they control the same data. | One clearly labelled scope control, with specific scope notes where a metric cannot widen. |
| Cards and verbose panel introductions have similar visual weight. | Everything competes for attention. | Compact metric strip, shorter headings and restrained panel borders. |
| Pool ceiling is shown next to a busy-versus-live utilisation bar. | A low busy percentage can appear to contradict an at-limit warning. | Show busy/live separately from the live/max ceiling and explain the queue. |
| Full history and multi-metric host charts share the monitoring page. | Exploration controls lengthen a page intended for glancing. | Keep useful summaries visible and disclose charts on request. |
| There is no page-level orientation or jump assistance. | Returning to a section means searching and scrolling. | Add a small sticky section navigation with current-section indication. |
| The phone captures spend most of the first viewport before pools. | An operator cannot quickly inspect work while away from a desk. | Two-by-two compact metrics, wrapped capacity metadata and fewer introductory lines. |

## A: Calm operations

One continuous page with sticky jump links: **Now, Pools, Jobs, History,
Diagnostics**. Links move within the page; they do not hide other sections.

Reading order:

1. First-run checklist when setup is incomplete. Existing critical problems
   remain visible; do not call an unfinished fleet healthy.
2. Compact attention area: fleet problems, failures needing review and eligible
   Eli offers remain distinct. Show the relevant counts and one action per
   category. Keep the existing maximum of two failure previews, with the full
   list accessible through its existing attention-filtered Jobs view.
3. Four live figures: queued jobs, running jobs, live runners and median queue
   wait with p95. Metrics remain links to the corresponding scoped records.
4. Pools and active jobs in the main desktop column. A short recent-events
   preview occupies a narrower supporting column. Mobile stacks pools, jobs,
   then events. Use ordinary page scrolling for the preview; expanding the
   feed can reveal its existing filters and longer list in place.
5. History, host capacity and startup/registration detail as collapsible
   sections. A closed section still communicates useful context.

Proposed closed summaries:

| Section | Example visible summary | Expanded content |
| --- | --- | --- |
| History | Today: 45 finished, 10 failed | Existing matrix, range, outcome selector and accessible cell interaction. |
| Host capacity | 3 connected hosts; 8 of 10 slots used | Existing host chart, per-host/overlay views, ranges and metric selection. |
| Startup and registration | p50 startup 4.0s; registration 9.0s | Existing p50 and p95 values, their period and runner links. |

History's selected range and the live figures' one-hour trends are independently
labelled. Do not imply one page-wide range changes every section. Summary
values must come from existing authoritative data, with missing or failed data
identified explicitly. Slot capacity, live runners and busy runners are
different quantities and retain different labels.

## B: Focus workspace

Keep attention and the four live figures persistent. Below them, offer
**Work, History, Diagnostics** views. Work contains pools, active jobs and the
short events preview; History contains the matrix; Diagnostics contains host
capacity and timings. This is a view switcher, unlike A's jump navigation.

Benefits: less vertical scanning, more space for analytical controls, and a
clear focus for occasional investigation. Costs: hidden information requires
another action, fewer signals can be read simultaneously, and view-specific
state and keyboard interaction add complexity. Attention counts and loss of
the live connection remain visible regardless of view.

Prefer A for a dashboard left open all day. Choose B if operator feedback shows
that focused investigation matters more than simultaneous visibility. Do not
ship both layouts as another preference without evidence that it is needed.

## Disclosure and navigation rules

- Keep attention and the core live figures visible. Pools and active jobs open
  by default. Collapse supporting detail, rather than every panel.
- Remember explicit expansion choices in this browser, independently of visual
  style. Avoid unexpected scrolling or reordering when data updates.
- Closed sections continue to show live summary state. A newly detected host
  fault appears in the attention area even when its chart is closed.
- Keep categories separate: fleet warnings, unreviewed job failures and Eli
  offers can concern the same underlying event. Never add their counts into a
  fabricated total of unique issues. Marking a failure known is not resolving it.
- Jumping to a collapsed destination opens it and positions its heading below
  the sticky bars. Give sections stable fragments so they can be bookmarked.
- On phones, use **Now, Pools, Jobs, More**; More contains labelled links to
  Events, History and Diagnostics. This menu assists page navigation and does
  not replace the existing bottom app navigation.
- Give jump links visible current-section styling and `aria-current=location`.
  Use native disclosure controls where practical, labelled navigation, visible
  focus, keyboard access and 44px touch targets. Reduced motion applies to
  scrolling and existing animations.
- One page scope control preserves the existing managed/all-runners meaning.
  Queue and Jobs links must match their counts. Host capacity and live runners
  remain this fleet's infrastructure even when external jobs are included;
  a label should make that boundary clear. Changing scope must update a closed
  summary as well as any open list.
- Show a labelled Live, Reconnecting or Disconnected state. Retain timestamps
  or a stale-data message after a connection gap. Do not rely on a green dot.
- Preserve the existing detailed matrix keyboard interaction, read/dismiss
  behaviour, failure distinction, permissions, Eli review flow, saved table
  choices, artwork, vocabulary and reduced-motion preference.

## Visual direction

Classic keeps its compact neutral surfaces; Mission Control keeps its navy or
cool light surfaces and slightly larger type. Both use existing tokens and
status semantics. Blue identifies interaction, cyan running work, amber
pending/warnings and the existing danger colour failures. Decoration does not
carry meaning. Retain original Zoomies branding and Eli artwork; any generated
icon is a placeholder, not a proposed replacement asset.

Use fewer large card enclosures, align section actions consistently and reserve
large numerals for the four live figures. Keep capacity metadata outside narrow
metric cells on phones so it can wrap. Keep long pool names available through
wrapping or an accessible expansion, rather than relying on hover. Keep the
assistant trigger and bottom navigation clear of record actions and safe areas.

## Review targets before implementation

- At 1440 by 900, attention, metrics and useful pool/job rows should be visible
  without opening diagnostics. Check a state with two failure previews as well
  as a quiet fleet.
- At 375 by 900, aim to show the first pool within the initial viewport after
  the compact metric grid, with no cropped capacity text or horizontal overflow.
  This is a target to validate later, not a measured result of these sketches.
- At 200% zoom, allow one-column reflow without lost actions. Check keyboard
  order, focus visibility, sticky offsets, touch controls and reduced motion.
- Verify both concepts' chosen implementation in Classic and Mission Control,
  each in light and dark mode. Do not shrink typography to make a board fit.
- Validate first run, loading, failed fetch, reconnecting/stale data, no work,
  many pools, long names and permissions. Zero and unavailable data differ.
- Usability check: can someone find queued work, identify a pool at its live
  limit, review an unacknowledged failure and reach host diagnostics quickly?

The original concept review included no application changes. The subsequent
implementation follows Concept A and is recorded separately above.
