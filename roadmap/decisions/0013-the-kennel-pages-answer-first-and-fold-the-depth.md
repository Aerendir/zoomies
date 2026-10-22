# 0013: The Kennel Club pages answer first and fold the depth

**Status**: accepted. Taken on 19 October 2026 on the owner's instruction, after
approving the mock and review in `design/kennel-club-review/`.

## Context

The pages had grown sideways rather than down. The Overview opened on seven equal
metric cards, so the answer to "is anything wrong?" was not the first thing read.
The checks table listed every check at the same weight, quiet and turned off alike.
The repository page had five tabs, opened on a fleet-activity tab even when
something was open, and rendered every finding as a full card. AI Context put its
benefits and explainers above the repository list, and every card offered six or
more equal actions. The review and the two-theme mock are the evidence; the owner
approved the direction.

## Decision

Every Kennel Club page answers its own question first, and the depth moves behind
a press or below the fold.

* The **Overview** opens on a verdict: what needs attention, the worst repository
  named, and the primary action. The full counts follow as one compact strip, and
  the checks list shows the affected checks first; the quiet checks and the
  turned-off checks each fold into their own disclosure, because "nothing open"
  and "not running" are not the same sentence. The sources that were read sit
  beside the conclusion, and a page nav marks the sections.
* The **Repositories** list carries the Overview's five questions as quick views
  with counts, and its filters are one row. Visibility is hidden by default and
  stays in the Columns chooser.
* A repository's sections are **Findings**, **Fleet activity** and **AI Context**.
  Findings leads when the repository has something open, and usually Fleet
  activity leads when it does not. **Protection** and **Agent guidance** are areas
  of Findings, reachable as filter chips, with their old addresses
  (`/protection`, `/agent-guidance`) opening the area they name. Tracking moves
  out of the header actions into the meta line; Recheck stays the primary action.
* **AI Context** lists repositories before it explains itself. A summary band and
  views count what is in setup, ready and needing attention, and each card keeps
  one action tied to its state. Reinstall, amend and remove fold under More, and
  the benefits and installation owners move below the list.

## Consequences

This supersedes decision 0008's "Overview is the default tab": the default is
Findings when something is open, and Fleet activity when nothing is. Decisions
0007 and 0010 still hold: the repository page hosts AI Context without a switch,
and the section keeps its side menu and its on-page switch.

Every old address still lands on what it named. `/ci` opens Findings with all
areas; `/protection` and `/agent-guidance` open Findings on that area; an unknown
section is replaced by the repository's first section. The repository page's
"Open the CI tab" link became "Open findings".

The repository list's quick views ask for every repository, like the Overview's
cards, so a count and the rows behind it agree. On a phone the views scroll
themselves rather than wrapping and pushing the filters below the fold.

The tests that named the old tabs and the old default path were updated with the
pages, and the Kennel screenshots in `docs/` were recaptured. The design review
stays in `design/kennel-club-review/` as the record of the mock that was approved.
