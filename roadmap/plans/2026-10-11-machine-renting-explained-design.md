# Explain, and enable, machine renting: design

**Status:** design approved in conversation (approach A); this written spec awaits the owner's review before any plan or code.
**Date:** 11 October 2026.

## Goal

When a pool has work waiting and no machine is being rented for it, the operator
never has to guess why. The product says so in one plain sentence, in every
place somebody would wonder, with the one thing to change. An administrator can
fix the usual cause with one click and no restart. And a fleet like the reporter's
(four Linux Docker hosts and one Windows host, one Proxmox provider) rents a
machine for a full Linux pool without any extra configuration.

### Success criteria

1. In a fleet whose Linux Docker hosts are all full and which also holds a host
   that can never run the pool (a Windows host offering only `process`), a queued
   job for the Linux pool makes the machine planner buy a machine, up to the
   ceilings.
2. With renting switched off, or the fleet ceiling at zero, the Providers page,
   the provider's Pools tab, the pool page, the queued job's explanation and
   verdict, the problems drawer and the MCP `list_providers` tool all give the
   same sentence and the same fix. None of them says "would rent" without saying
   what stops it.
3. An administrator turns renting on from the Providers page in one click. It
   takes effect on the next machine pass, without a restart. Everyone else sees
   who to ask.
4. Nothing here spends money that was not asked for. `provider.enabled` still
   defaults to false and `provider.max_machines` still defaults to zero. The click
   is the explicit act, and it sets both.

## What was wrong

Two independent faults, found by reading the code and reproducing the second with
a model of the reporter's fleet (`scheduler.Decide` into `DecideMachines`, with
and without the Windows host).

**Renting was switched off, and nothing said so.** `provider.enabled` is off by
default and applied only at start (`internal/config/settings.go`), the machine
loop is only spawned when it is on (`internal/controller/controller.go`), and
every explanation path is silent while it is off:

- `provisioningHeld` builds the "held" sentence a provider card and the MCP tool
  present as "why no machine may be bought right now". It never consults the
  setting, so `held` was empty.
- `machineProblems` and the configuration validator return before raising
  anything while the setting is off, by design.
- `GET /meta` carries `providers_available`, but nothing in `web/src` reads it.
- The Pools tab's "Would rent" answers only the two selectors and the machine's
  shape. It says nothing about whether renting is on.
- `DecideMachines` already computes why a machine is not bought (a ceiling, no
  provider serving the pool), but the pass only logs it, and the ceiling
  sentences are not even logged.

**A host that can never run the pool stopped the fleet being called full.**
`hostSet.why` set `atCapacity` to `full + throttled + committed == len(hs.hosts)`,
counting hosts that can never run the pool (wrong backend, platform, selector or
size) in the denominator. `DecideMachines` buys only for a pool whose plan is
`BlockedAtCapacity` or `BlockedNoEligibleHost`, so one Windows host beside a Linux
fleet meant a Linux pool was never reported full and a machine was never bought.
History shows this was a side effect, not a decision: the `len(hs.hosts)`
denominator was written on 4 September, before any machine planner existed, and the
two test rows that pin the mixed case justify only the other flag
(`noEligibleHost`).

## Decisions

- **Consent model (A).** Renting stays opt-in. An administrator turns it on with
  one click that sets `provider.enabled` and `provider.max_machines` together.
  Rejected: turning it on when a provider is added (B), which spends money without
  a second confirmation and reverses the documented default; and explanations
  only (C), which leaves a hidden setting and a restart in the way.
- **Hosts that could run the pool but are cordoned, silent or incompatible keep
  blocking a purchase, as today.** A blip must not buy a machine, and there is no
  grace period on the buy side. Ignoring them is a separate decision that needs a
  grace period; it is a listed follow-up.
- **The fleet ceiling is shown to viewers.** The per-provider ceilings already are
  on `GET /providers`; the fleet figure is the same order of sensitivity.
- **Turning renting off still stops the whole loop**, deletes and ownership checks
  included, exactly as before. `provider.paused` is the gentle switch and the
  setting's text says so. A warning problem is raised when renting is off while
  machines are still owned, because nothing is then tending them.

## Design

### 1. At capacity means every host that could run the pool

In `hostSet.why` (`internal/scheduler/scheduler.go`), count how many hosts pass
`HostCouldRun` (the configured-compatible test, with the weather left out) and how
many of those are out of slots (full, throttled or committed, as the ladder already
classifies them). Then:

```go
atCapacity := full+throttled+committed == len(hs.hosts) ||
    (candidates > 0 && waiting == candidates)
```

The first disjunct is today's rule and is kept so that nothing that was reported
full stops being reported full (a too-small host the ladder called throttled before
it asked whether the pool fits at all is still out of slots). The second admits the
mixed fleet. `onlyFull`, which chooses the fix text, takes the same shape over
full and throttled hosts only, so a mixed fleet gets "wait for a job to finish,
raise a host's capacity, or add a host" rather than the generic fallback.
`noEligibleHost` is unchanged. Held and warming hosts that could run the pool still
count as candidates that are not out of slots, so a pressure hold never buys a
machine.

Visible effects: `pool.no_capacity` for a merely full mixed fleet becomes a warning
("waiting for a host with room") instead of an error ("nowhere to run them"); the
capacity-demand webhook fires for such a fleet; usage records count its minutes at
capacity.

### 2. The planner returns a verdict for each pool with unmet demand

`DecideMachines` (`internal/controller/machinedemand.go`) gains `MachinePlan.Pools
[]PoolRenting`. Its decisions do not change. A verdict is recorded at each point it
already branches, for every pool the scheduler reports blocked whose `Desired`
exceeds its `Current`. A pool the scheduler can place itself, or whose shortfall is
covered by free slots on hosts that can run it, gets no verdict:

| State | When | Setting to change |
| --- | --- | --- |
| `renting` | this pass reserves a machine that covers it (possibly only part of it) | none |
| `on_its_way` | machines already being built cover it | none |
| `waiting` | the scheduler is not blocked in a way a machine fixes: a candidate host is held, starting runners, cordoned, silent or incompatible | none |
| `ceiling` | a limit stops the machine it needs | `provider.max_machines` for the fleet limit, none for a provider's own |
| `unserved` | no provider may rent for it (a selector says no) or none builds a machine that suits it | none |

Two more states are overlaid by the controller, because the planner is pure and
does not know them: `held` (the provider could, and `provisioningHeld` says it may
not) and `off` (renting is switched off; every verdict that would have been
`renting` or `ceiling` becomes `off` and says how many machines it would rent).

A verdict carries `state`, a one-sentence `summary`, a `fix`, the `setting` when
one setting changes it, the provider, `queued`, `short`, `machines` and `since`
(when the oldest machine on its way was asked for). Sentences never contain a
running clock or a count that changes between passes, because anything that reaches
the problems list is re-sent whenever its JSON differs. Times travel as `since`.

### 3. The loop keeps the verdict, and renting applies live

- `provider.enabled` becomes live (`Live: true`, no `RestartReason`). The machine
  loop is always spawned, as the kennel loop is, and `ReconcileMachines` still does
  nothing while renting is off: no recovery, sweep, drain, delete or create.
- While renting is off, or this controller may not act, the pass still reads the
  snapshot, runs `DecideMachines`, and stores the result. This is the rule
  `Controller.apply` already follows for a fenced fleet: decide and publish, but do
  not act, so an operator can tell "nothing to do" from "not allowed to".
- The stored report is replaced under its own lock, never `machines.mu`, which is
  held for a whole pass. The report is as old as the last pass, at most one
  `provider.interval`, and a settings change or a provider change wakes the loop.
- `provisioningHeld` gains a clause for the switch, after the recovery fence, so
  `ProviderView.Held` and `zoomies providers list` stop saying a provider is fine
  when nothing can be bought.
- When a setting under `provider.` changes through the settings API, every
  provider's view is republished (`provider.updated`), as `KickKennel` does for
  its feature, so open pages move without a reload.

### 4. One verdict, every surface

**Route.** `GET /api/v1/providers/renting` (a viewer's, action `providers.read`):

```json
{
  "state": "off",
  "summary": "Renting is switched off: this controller is not allowed to rent machines, so work with nowhere to run waits for the hosts it has.",
  "fix": "Turn on Rent machines in Settings and set Machines the fleet may rent.",
  "setting": "provider.enabled",
  "enabled": false, "paused": false, "max_machines": 0,
  "suggested_max_machines": 2, "owned": 0,
  "decided_at": "2026-10-11T13:12:00Z",
  "pools": [
    { "pool_id": "pool_example", "pool_name": "zoomies-linux-x64", "state": "off",
      "summary": "Would rent 1 machine for this pool, but renting is switched off.",
      "fix": "Turn on renting.", "setting": "provider.enabled",
      "provider_id": "prv_example", "provider_name": "proxmox-lab",
      "queued": 1, "short": 1, "machines": 0 }
  ]
}
```

The fleet `state` is `none` (no provider rows other than demo fixtures), `off`,
`no_ceiling` (renting is on and the fleet ceiling is zero) or `on`.
`suggested_max_machines` is the sum of the enabled providers' own ceilings, at
least one: what the click sets. `pools` is empty before the first pass.

**Held.** `ProviderView.Held` says renting is off (section 3).

**Problem `provider.renting_off`.** Raised wherever a non-demo provider exists and
renting is off, from a function that runs before `machineProblems` returns.
Severity is chosen in code: a warning when a pool's verdict is `off` or machines
are still owned, otherwise info. The setting is `provider.enabled`. Fleet audience,
a public-status sentence ("Machines are not being rented, so jobs wait for the
machines the fleet already has."), a row in `docs/problem-codes.md`, and a
regenerated catalogue.

**Pool, job and verdict text.** `pool.no_capacity` gains the pool's verdict summary
in its detail and puts the verdict's fix first. The queued-job explanation
(`explain.go`) and the job verdict (`why.go`) do the same, the latter as an
evidence line labelled "renting" and a next step linking to `/providers`.

**MCP.** `list_providers` splices in `renting` from the new route, best effort, as
it already does for `providers_available`. A single call then answers the original
question. The words the description tests pin stay.

### 5. The Providers page

- `getProviderRenting()` in the API client. Providers and provider-detail load it
  with their other data and reload on `provider.updated`, `machine.updated` and
  `problems.updated`.
- A new `RentingNotice` shows above the metrics for `off` and `no_ceiling`. For an
  administrator it carries **Turn on renting** (or **Allow N machines**) and a
  ceiling field defaulting to `suggested_max_machines`; one `PATCH /settings` sets
  `provider.enabled` and `provider.max_machines` together. Everyone else sees the
  sentence and who can change it. An environment-pinned setting shows the server's
  own refusal. For `on` with something being built it shows a single calm line.
- The Pools tab shows each pool's live verdict under "Would rent" and keeps the
  static reason for "Would not rent".
- Tokens only: the notice is a pure explanation (`--z-accent-subtle` and
  `--z-accent-border`), danger only for failure. No new library. Logic lives in
  `lib/providers/renting.ts` so it is unit-testable.

The notice, for tone:

> **Renting is switched off.** proxmox-lab passed its check, but this
> controller is not allowed to rent machines yet, so work with nowhere to run will
> wait. Right now zoomies-linux-x64 has 1 job waiting and every host that can run
> it is full: with renting on, 1 machine would be built for it straight away.
> [Turn on renting] up to [2] machines at once. You can pause or lower this at any
> time.

## Tests

- **Scheduler.** Two rows in `TestAFullFleetIsStillDistinguishedFromAFleetThatCanNeverRunThePool`
  change to `atCapacity: true` with their comments rewritten ("one host busy and
  one that could never run it", "one host throttled and one that could never run
  it"). New rows: busy beside a cordoned, a silent and an incompatible host (all
  still neither); a Windows host that is also silent; a too-small throttled host
  alone (still at capacity); a disk-starved host beside a busy one.
- **Planner.** A test drives a real `scheduler.Decide` into `DecideMachines` for the
  reporter's fleet shape, with and without the Windows host. One test per verdict
  state. The existing decisions are asserted unchanged.
- **Controller.** The loop starts disabled, `UpdateConfig` turns it on, and a pass
  follows. A disabled pass stores a verdict and writes nothing. The `held` clause.
  `provider.renting_off` severities. The pool, explanation and verdict text.
- **API.** The new route, its shape against the spec, the settings change not
  appearing in `pending_restart`, and the provider republish.
- **UI.** Unit tests for `renting.ts`; a Playwright spec that routes the new
  endpoint, asserts the notice, the admin request body and the non-admin text, and
  undoes the setting. The demo seed's provider is a fixture and is filtered out of
  the report, so existing specs, the accessibility pass and the phone overflow
  sweep are unaffected.

## Docs and generated files

`docs/proxmox.md` (the "off until you turn it on" note and a "Why is no machine
rented?" section), `docs/configuration.md` (`provider.enabled` applies at once),
`docs/problem-codes.md` (new row, the status sentence, the wording that says codes
are silent while renting is off), `docs/api-surface.md`, `docs/ui.md`,
`docs/hosts-and-pools.md`, `docs/capacity-demand-receiver.md`, `docs/connect-claude.md`.
Regenerate `internal/api/openapi_spec.go`, `web/src/lib/api/schema.d.ts`, the
catalogue and the skill references with `make generate`. Stale comments about the
loop starting only at startup are corrected.

## Risks and how each is bounded

- **A busy host beside a cordoned or silent one still buys nothing.** Kept on
  purpose (see Decisions). The `waiting` verdict now says why.
- **More fleets now ask for machines.** Bounded by the fleet and provider ceilings,
  `max_creates_in_flight`, the pending-machine credit, and idle release after about
  thirty minutes. A pool's `min_runners` shortfall now buys in mixed fleets, as it
  already did in uniform ones.
- **Turning renting off mid-build strands billed machines.** Unchanged behaviour,
  now stated in the setting's text and raised as a warning problem.
- **The verdict flapping into the problems stream.** Sentences carry no clock and no
  per-pass count; times travel as fields.
- **The always-running loop.** It does one provider listing and returns while there
  are no providers; the cost with a provider and renting off is one read-only
  snapshot every `provider.interval`.

## Not included

Wizard changes, a CLI table column, a navigation badge, new event kinds, a new MCP
tool, a remedy kind for settings, auto-enabling, and a grace period for silent
hosts. Each can follow without changing anything above.
