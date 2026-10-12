# The fleet as it moves: a recent window and analyses over MCP

Status: design approved in conversation, 12 October 2026. Two packages,
ZF-240 (the window) and ZF-241 (the analyses), each its own plan and pull
requests. ROADMAP.md section 8 carries the pointers.

## 1. Intent

An agent connected over MCP, and Eli inside the UI, can read what the fleet
is now and what it did over a period, but not what is happening *as it
happens*, and not why a period was slow or a release worse. The REST API has
the material: the live event stream with a replay ring, the scheduler's
decisions with their reasons, the Overview's minute samples per fleet and per
host, a runner's state history, workflow runs, webhook deliveries, the audit
log and the support bundle. None of it is offered to a model, and the
interesting questions (what changed in the last ten minutes, why are jobs
waiting right now, did the last release make things worse, is this host
unwell when it is busy) take more tool calls than Eli may spend and more
context than an answer is worth.

Two things fix that, in order:

1. **ZF-240, the window.** The recent past over MCP, bounded, with one
   tool that blocks briefly until something changes. No push, no sessions:
   the transport stays stateless, as `internal/mcp` and `docs/connect-claude.md`
   promise.
2. **ZF-241, the analyses.** Answers computed on the controller, pure
   functions over a snapshot in the mould of `internal/scheduler`, each a
   REST route first and an MCP tool second so the UI, the CLI, Eli and an
   agent render one answer and cannot disagree.

A platform team operating an instance for a product team asks these
questions from an agent, and the product team's developers ask Eli; both
are served by the same tools.

## 2. Decisions taken with the owner

- **Pull, not push.** MCP resource subscriptions and a server-initiated event
  stream are deferred and recorded in section 9. Today's clients only
  invalidate a cache on `notifications/resources/updated`, and none wakes a
  model on one; the cost would be sessions and per-session buffers in a
  transport that is stateless on purpose.
- **Eli is offered the window and the analyses, not the operator's
  reads.** The audit log and the support bundle are administrator routes;
  webhook deliveries any viewer may read, but a delivery's body is GitHub's
  whole payload and nothing a question to Eli needs. All three stay off Eli's
  list, named as left off in `TestEveryReadToolIsOnElisListOrLeftOffOnPurpose`.
- **All four analyses:** queue-wait breakdown, release comparison, host
  pressure against failures, repeated runner-startup failures.
- **Two packages**, so the window ships and is accepted before an analysis
  is argued over.

## 3. What exists and is reused

| Source | Route | Store read | Notes |
| --- | --- | --- | --- |
| Live events | `GET /events` (SSE) | `events.Bus` ring, 256 deep, ids `<epoch>.<seq>` | Replays from `Last-Event-ID`; a `resync` frame when it cannot |
| Scaling decisions | `GET /scaling-events` | `ListScalingEvents` | The scheduler's reason sentences, kept for `retention.scaling_events` |
| Fleet samples | `GET /samples` | `ListSamples` | One row a minute, `retention.samples` |
| Host samples | `GET /hosts/samples` | `ListHostSamples` | CPU, load, memory, disk, reserved; gaps not zeros |
| Runner timeline | `GET /runners/{id}/timeline` | `RunnerTimeline` | States and stages with durations |
| Workflow runs | `GET /workflow-runs` | `ListWorkflowRuns` | Derived from jobs, never stored |
| Webhook deliveries | `GET /webhook-deliveries` | | viewer; whole payloads |
| Audit | `GET /audit` | `ListAudit` | admin |
| Support bundle | `GET /diagnostics/bundle` | | admin, sectioned, capped |
| Job statistics | `GET /jobs/stats` | `JobStats` | Percentiles by six group keys, peaks |
| Explanation | `GET /jobs/{id}/explanation` | | The failure classes ZF-231 made |

Every new MCP tool is one call to one of these routes, or to a new route
under section 5, through the `API` interface, with the caller's own
credential. Nothing in `internal/mcp` learns about the store or the bus.

## 4. ZF-240: the window

### 4.1 One new route

`GET /events/recent` answers the ring as JSON rather than as a stream, so a
stateless MCP tool, the CLI and a test can read it. Parameters: `since`
(an event id as the stream sends it, or a duration such as `10m`), `kinds`,
`topic` and `limit` (at most 200). The answer is `{items, next, complete}`:
`next` is the id to continue from, and `complete` is false when the ring no
longer reaches back to `since` or the id is from another run of the process,
which is the same judgement the stream makes before sending `resync`. The
bus gains a `Recent(since uint64, opts) ([]Event, complete bool)` read over
the ring under its lock; the stream's replay and this route call the one
function, so they cannot disagree about what was lost. Viewer role, the same
`events.read` the stream requires.

`wait` on the same route is the blocking form: `?wait=20s` holds the request
until an event matching the filter arrives or the time is up, and answers at
once when `since` already has events past it. It subscribes to the bus for
the wait, counted against `limits.event_subscribers` like a stream, and
answers `{items: [], next, complete: true, timed_out: true}` on the deadline.
The wait is capped at 25 seconds so one call sits inside Eli's 30-second
tool timeout and inside any proxy's patience.

### 4.2 The tools

| Tool | Reads | Role | Eli |
| --- | --- | --- | --- |
| `recent_events` | `/events/recent` | viewer | yes |
| `wait_for_change` | `/events/recent?wait=` | viewer | yes, live |
| `scaling_decisions` | `/scaling-events` | viewer | yes |
| `fleet_samples` | `/samples` | viewer | yes |
| `host_samples` | `/hosts/samples` | viewer | yes |
| `runner_timeline` | `/runners/{id}/timeline` | viewer | yes |
| `list_workflow_runs` | `/workflow-runs` | viewer | yes |
| `list_webhook_deliveries` | `/webhook-deliveries` | viewer | left off |
| `list_audit` | `/audit` | admin | left off |
| `support_bundle` | `/diagnostics/bundle` | admin | left off |

Each answers two content blocks: a short rendering for a model (one line
per event or row: the time, the kind, the resource's name and the sentence
the frame carries, never the whole resource JSON) and the JSON under a
`structuredContent` object with an `outputSchema` on the tool, which the
2025-06-18 revision added and the server already answers to. The rendering
is the block Eli reads inside its 16 KiB ceiling; the JSON is what an agent
with room parses.

Workflow-written text (job, step and workflow names, branch names, commit
text, log lines in a frame) goes in the untrusted block the log tools
already use, with the same notice before it. `recent_events` therefore
renders an event by its resource's *id and state* and quotes the name in
the untrusted block only.

Every window is capped: at most 200 events or rows and 64 KiB of rendered
text per call, truncated with `truncated: true` and the id to continue
from, the way `get_runner_log` caps a log.

`support_bundle` takes `sections` and returns only those, because the whole
document is larger than any model should be handed; `sections` left out
returns the index of sections and their sizes.

### 4.3 Eli

- `AssistantFleetTools` gains the seven tools marked yes. The three marked
  left off are named in the left-off map with their reason.
- A tool definition gains `live bool` (`internal/mcp`), carried through
  `assistant.Tool` as `Live`. `runTool` skips the `made` cache for a live tool,
  so a second `wait_for_change` or `recent_events` with the same arguments
  runs again; `callKey` is unchanged for every other tool.
- `wait_for_change` with no `since` answers the current `next` id at once
  and nothing else, so a model's first call establishes a cursor and its
  second waits. The description says so.
- Three recorded scenarios in `eli_eval_test.go`: *what changed in the last
  ten minutes* (rests on `recent_events`, at most 2 calls), *why are jobs
  waiting right now* (rests on `scaling_decisions` and `list_problems`),
  *is this host unwell when it is busy* (rests on `host_samples`). Each
  plants a job name carrying an instruction and asserts it is not followed.
- The opening suggestions on a fleet with a problem gain *What changed in
  the last ten minutes?* when Eli may read the fleet.

### 4.4 Prompts and resources

- Prompt `what_changed` (argument: a duration, default `10m`): read
  `recent_events`, then `scaling_decisions` for the same period, then
  `list_problems`; say what moved and what is still open.
- Resource `zoomies://fleet/now`: the `fleet_status` document, so a client
  can load the shape of the fleet without a tool call. Read-only, same
  route, same token.

### 4.5 Documentation and generated files

`docs/connect-claude.md` gains a section *Watching the fleet move* and
`docs/eli.md` names the new reads under *What Eli can see*. `api/openapi.yaml`
gains the route; `internal/api/openapi_spec.go` and the UI's schema are
regenerated; the CLI gains `zoomies events recent [--since ID|DUR] [--wait]`
over the same route, and `skills/` regenerates the command reference.

### 4.6 Tests seen to fail first

- `TestRecentEventsAgreeWithTheStreamsReplay`: the route and a stream given
  the same `since` return the same events, and both say incomplete past
  the ring.
- `TestWaitForChangeAnswersAtOnceWhenEventsAlreadyExist` and
  `TestWaitForChangeTimesOutWithoutBlockingPastTheCap`.
- `TestALiveToolIsNotAnsweredFromTheCache` in `internal/controller`.
- `TestEveryReadToolIsOnElisListOrLeftOffOnPurpose` fails until the list and
  the left-off map are updated, which is the decision being recorded.
- `TestAWindowRendersAJobNameOnlyInTheUntrustedBlock`.
- The three Eli scenarios play under the scripted model in every run.

### 4.7 Accept when

An agent with a viewer token asks what changed in the last ten minutes and
is answered from one call; a second call with the returned cursor blocks
until a seeded event arrives and returns only it; Eli answers the three
scenarios with the tools they name and never follows the planted text; an
operator's agent reads the audit log and a product team's Eli cannot.

## 5. ZF-241: the analyses

### 5.1 The package

`internal/analysis` is pure: a function per analysis over a snapshot the
caller assembled, returning a `Finding` with `summary` (one sentence an
operator reads), typed `evidence`, `confidence` (`high`, `medium`, `low`
with the reason when not high), `next_steps` in order, and the catalog
codes to read on. It reads no clock, no store and no network;
`purity_test.go` enforces it as `internal/updates` does. The controller
assembles the snapshot from the store and renders the finding; the REST
route and the MCP tool serve that rendering.

### 5.2 The four

| Analysis | Route and tool | Snapshot | Answer |
| --- | --- | --- | --- |
| Queue-wait breakdown | `GET /analysis/queue-wait`, `queue_wait_breakdown` | Jobs in a period with their timelines, the scaling decisions, the hosts' throttle episodes and runtime incidents, the pools | Waiting time by cause: no host matched, pool at its ceiling, hosts throttled, runner startup, held for review, GitHub's own delay; per pool, with the share of each and the one change that would have cut the most |
| Release comparison | `GET /analysis/compare`, `compare_periods` | Two `JobStats` groups, by controller version or by period, with the fleet's size in each | What regressed and what improved, each with the figures and whether the sample is large enough to say; cancelled and skipped left out and said so |
| Host pressure | `GET /analysis/host-pressure`, `host_pressure` | A host's samples and the jobs that ran on it in the period, with their outcomes and peaks | Whether failures, OOM kills and slow startups cluster in the minutes the host was under pressure, with the threshold that separates them; or that they do not |
| Repeated startup failures | `GET /analysis/startup-failures`, `startup_failures` | Runner timelines ending in a failed start, by host and pool, over the period | Which host or pool keeps failing to start runners, since when, the stage it fails at, and whether a throttle, an incident or an image pull explains it |

Each route takes `since` and `until` as the job routes do, defaults to the
last 24 hours, and is viewer role. Each tool is on Eli's list.

### 5.3 Shape of an answer

The MCP tool answers the summary and the ordered next steps as text, and
the finding as `structuredContent` with an `outputSchema`. Evidence that
quotes a job name or a log line goes in the untrusted block. A finding's
`next_steps` name settings and catalog codes, never commands to run, which
is the catalog's own rule.

### 5.4 The UI and the CLI

Each finding has one place in the UI that already asks its question: the
queue-wait breakdown on the Jobs page's waiting view, the comparison on the
Overview's version panel, host pressure on the host page's capacity map,
startup failures on the Runners page beside the failed filter. Each is a
`Why` card in the drawer's style, rendering the route's answer, so the UI
cannot say something Eli would not. `zoomies analyse <name>` prints the
same answer.

### 5.5 Tests seen to fail first

- A table test per analysis in `internal/analysis` with a seeded snapshot
  per cause, each asserting the summary sentence, the dominant cause and the
  confidence; a snapshot with too little data asserts `low` with the reason.
- `TestTheAnalysisPackageIsPure`.
- A route test per analysis against the fake fleet.
- An Eli scenario per analysis: *why were jobs slow this morning*, *did the
  last release make things worse*, *is build-1 failing because it is busy*,
  *which host keeps failing to start runners*.

### 5.6 Accept when

Each seeded cause in `zoomies demo` is named as the dominant one at high
confidence; a comparison with fewer than thirty jobs on either side says so
rather than ranking; the UI card and the tool show one answer; Eli answers
each scenario from one analysis call.

## 6. Limits and settings

No new setting. The window reads under `retention.scaling_events`,
`retention.samples` and `retention.jobs`, and the wait counts under
`limits.event_subscribers`. The analyses read the same retention, which is
why a comparison across more than `retention.jobs` falls back to the usage
roll-up for counts and says the percentiles are unavailable.

## 7. Security

- Every tool is the documented route with the caller's own credential; an
  admin tool asked for by a viewer is the route's 403 with its sentence.
- Workflow-written text is quoted only in untrusted blocks.
- The wait is bounded, counted and cancellable; a client that goes away
  cancels its context, which the bus watches.
- Nothing new leaves the machine through Eli that a viewer's token could not
  already read, and the three reads left off Eli's list stay off it.

## 8. Out of scope

Push notifications (section 9), a new Prometheus family, stored analyses,
alerting, and any analysis that needs data the fleet does not keep.

## 9. Deferred: push

`resources/subscribe` and a server-initiated SSE stream on `/mcp` would
give a client a `notifications/resources/updated` when the fleet moves.
Deferred because the transport is stateless on purpose, the clients in use
only invalidate a cache on the notification, and `wait_for_change` gives a
model the same thing one bounded call at a time. Revisit when a client in
use wakes a model on a notification.
