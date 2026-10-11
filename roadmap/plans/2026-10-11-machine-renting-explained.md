# Machine Renting Explained and Switched On Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A pool with work waiting rents a machine in a mixed fleet, and wherever an operator would wonder why nothing is rented the product says so in one sentence and lets an administrator fix the usual cause in one click, with no restart.

**Architecture:** The scheduler's "at capacity" test stops counting hosts that can never run the pool. `DecideMachines` additionally returns a verdict per pool, which the machine loop (now always running) keeps even while renting is off. One read route, one problem code, the pool and job text, MCP and a Providers page notice all render that single verdict.

**Tech Stack:** Go (scheduler, controller, API, MCP), OpenAPI with generated TypeScript types, Svelte 5 with Playwright and `node:test`, MkDocs prose.

**Spec:** `roadmap/plans/2026-10-11-machine-renting-explained-design.md` (read it first; this plan only records what it leaves to the implementer).

## Global Constraints

- British spelling. No em dash (U+2014) and no `--` standing in for one, in code, tests, docs, UI strings or commit messages. `internal/docs/no_em_dash_test.go` walks the whole repository.
- Commit messages are imperative sentences in plain prose, sentence case, no Conventional Commits prefix. Every commit ends with the attribution trailer lines the coordinating session supplies in the task prompt; do not write a model name into any file.
- Defaults do not change: `provider.enabled` stays false and `provider.max_machines` stays zero. Nothing may rent a machine that an administrator did not switch on.
- `internal/scheduler` stays pure: no clock, database or network. `internal/store` stays the only place SQL is written.
- Every sentence that reaches the problems list carries no clock and no counter that moves while the fleet stays as it is (the list is re-sent whenever its JSON differs). Counts of jobs, slots and machines are allowed, because they move only when the fleet does, as the queue count in the `pool.no_capacity` title already does; a duration, a pass number or an age is never allowed. Times travel as fields.
- Demo fixtures (`controller.IsDemoID`) are never counted: not in the renting view, not in the problem.
- Generated files are regenerated, never hand-edited: `go run internal/api/gen_openapi.go` (writes `internal/api/openapi_spec.go`), `make openapi` (writes `web/src/lib/api/schema.d.ts`, needs `cd web && npm ci --no-audit --no-fund`), `make generate` (catalogue, command and MCP references). Run `make build-nogui` once before any command that compiles `internal/api`.
- UI: design tokens only (`web/src/lib/styles/tokens.css`), no new library, logic in pure `.ts` so `node:test` can reach it, a pure explanation uses `--z-accent-subtle` and `--z-accent-border`.
- Branch `claude/adoring-darwin-2d4bdn`, draft PR eyupio/zoomies#880. Commit per task; push after Tasks 1, 4, 8, 10 and 11.

## Review Focus

Inputs and conditions the spec implies and the obvious tests would miss, each pinned in the owning task:

1. A provider row that is itself disabled must not produce the notice, the problem or any fleet state but `none` (Tasks 4 and 6).
2. Two providers that both serve one pool give exactly one verdict, charged to one provider (Task 2).
3. Before the first machine pass the view has the switches and an empty `pools` array, never `null` (Tasks 4 and 5).
4. A pool short of its `min_runners` with nothing queued must not say "with 0 jobs queued" (Task 2).
5. An environment-pinned `provider.enabled` makes the click fail: the notice stays and shows the server's own refusal (Task 10).

## Shared copy

Every task that asserts a sentence asserts exactly these. `{n machine(s)}` uses `plural` from `internal/controller/problems.go` (`1 machine`, `2 machines`).

| Id | Sentence |
| --- | --- |
| `fleet.off` | `Renting is switched off: this controller is not allowed to rent machines, so work with nowhere to run waits for the hosts it has.` |
| `fleet.offFix` | `Turn on Rent machines in Settings, and set Machines the fleet may rent to the most you are willing to pay for.` |
| `fleet.noCeiling` | `Renting is on, but the fleet may rent no machines: a maximum of none is none.` |
| `fleet.noCeilingFix` | `Set Machines the fleet may rent to the most machines you are willing to pay for at once.` |
| `fleet.on` | `Renting is on.` |
| `fleet.none` | `No provider is configured, so nothing is rented.` |
| `held.off` | `renting is switched off for this controller (provider.enabled), so no machine is created from any provider; nothing is drained, deleted or checked either` |
| `pool.renting` | `Renting {n machine(s)} from {provider}, which covers this pool: {reason}.` |
| `pool.rentingPartial` | `Renting {create} of the {want} machines it needs from {provider}, which covers this pool: {reason}. The rest are held back: {because}.` |
| `pool.rentingShared` | `Renting {create} of the {want} machines this pool and others need from {provider}: {reason}. The rest are held back: {because}.` |
| `pool.onItsWay` | `{n machine(s)} from {provider} {is or are} on the way for this pool, so the next runners start there.` |
| `pool.waiting` | `Not renting yet: {scheduler sentence}. A machine is rented only once every host that could run this pool is full.` |
| `pool.waitingTail` | `A machine is rented only once every host that could run this pool is full.` |
| `pool.waitingFix` | `Wait for those hosts to clear, or uncordon, repair or remove the ones that are unavailable.` |
| `pool.ceiling` | `Not renting: {because}.` |
| `pool.unservedSelector` | `No provider will rent for this pool. {Pairing.Why as a sentence}` |
| `pool.unservedFit` | `No provider builds a machine this pool could use. {provider}: {machineFit}` |
| `pool.unservedFitFix` | `Give a provider machines that suit this pool, or change the pool.` |
| `pool.held` | `Not renting: {held}.` |
| `pool.off` | `Would rent {n machine(s)} for this pool, but renting is switched off.` |
| `pool.offCeiling` | `Renting is switched off, and {because}.` |
| `pool.offOnItsWay` | `A machine for this pool was being built, but renting is switched off, so nothing is finishing it.` |

`{reason}` is `{plural(slots, "runner slot")} short`, then ` with {plural(queued, "job")} queued` only when `queued > 0`, then `, and every host that can run it is full` (or `, and no host in the fleet can run it` when the pool is blocked with nowhere to go). `{because}` and its fix come from the existing ceiling helpers in `machinedemand.go`, the fix passed through `sentence()` from `pairings.go`.

## File Structure

Create:
- `internal/controller/renting.go`: verdict and view types, the sentences above, `buildRentingView`, `Controller.Renting`, `Controller.rentingFor`, the stored report.
- `internal/controller/renting_test.go`, `renting_view_test.go`, `machines_live_test.go`, `machines_renting_test.go`, `problems_renting_test.go`.
- `web/src/lib/providers/renting.ts`, `web/src/lib/providers/RentingNotice.svelte`, `web/unit/renting.test.ts`, `web/tests/renting.spec.ts`.

Modify (all tasks): `internal/scheduler/scheduler.go`, `internal/controller/{machinedemand,machines,controller,problems,status,explain,why}.go`, `internal/config/{settings,config}.go`, `internal/api/{router,handlers_providers}.go`, `internal/mcp/providers.go`, `api/openapi.yaml`, `web/src/lib/api/{client,types}.ts`, `web/src/routes/{Providers,ProviderDetail}.svelte`, `web/src/lib/providers/PoolsTab.svelte`, and the docs named in each task.

## Tasks

### Task 1: A host that can never run a pool no longer hides that its fleet is full

**Files:** modify `internal/scheduler/scheduler.go` (`hostSet.why`, the `PoolPlan.BlockedAtCapacity` comment); test `internal/scheduler/scheduler_test.go`; docs `docs/hosts-and-pools.md` (the `at capacity` row near line 1510), `docs/problem-codes.md` (the `pool.no_capacity` row near line 282), `docs/capacity-demand-receiver.md` (the sentence near line 16).

**Interfaces:** Produces no new signature. `PoolPlan.BlockedAtCapacity` now means every host that could run the pool is out of slots. `blockage.onlyFull` takes the same shape.

- [ ] **Step 1: Extend the table test.** In `TestAFullFleetIsStillDistinguishedFromAFleetThatCanNeverRunThePool` add two optional fields to the row struct, `neverRunnable` and `couldRun` (host ids), and assert in the loop that `HostCouldRun(host, pool)` is false for the first list and true for the second, before asserting the flags. Change the rows "one host busy and one that could never run it" and "one host throttled and one that could never run it" to `atCapacity: true` and rewrite their comments to say a host that could never run the pool is not part of the question and the other host is out of slots. Add rows, each with `testHost`/`sized`, `h.Cordoned = true`, `h.LastHeartbeat = ago(10 * time.Minute)` or `h.Incompatible = true` as named:
  - "one host busy and one cordoned that could run it": neither flag, `couldRun` both.
  - "one host busy and one silent that could run it": neither flag.
  - "one host busy and one incompatible that could run it": neither flag.
  - "one host busy and a silent host that could never run it" (process backend, silent): `atCapacity: true`, `neverRunnable` the silent one.
  - "a throttled host too small for the pool, alone" (a `limited("linux", 0, 6*1024)` pool, a 4 GB host, `ActiveRunners: 1`, `Throttle: store.HostThrottle{Level: store.MaxThrottleLevel}`): `atCapacity: true`, `neverRunnable` it. This pins the kept first disjunct.
  - "one host busy and one with no disk left" (a `sized` host with `DiskFreeMB` below the disk reserve): `atCapacity: true`, `neverRunnable` it. If the fixture passes `HostCouldRun`, adjust it until the precondition holds.
- [ ] **Step 2: Add `TestAMixedFleetAdvisesWaitingOrAddingAHost`.** One busy docker host and one process-only host, one queued job: `strings.Contains(pp.BlockedFix, "wait for a job to finish")` and `pp.BlockedAtCapacity`.
- [ ] **Step 3: Run `go test ./internal/scheduler -run 'TestAFullFleetIsStillDistinguished|TestAMixedFleetAdvises' -count=1`.** Expected: FAIL on the changed and new rows.
- [ ] **Step 4: Implement in `hostSet.why`.** For every host also record `could := HostCouldRun(h, p)`, and set `waits`/`outOfSlots` beside the existing `full++`, `throttled++` and `committed++` (committed is `waits` only). Count `candidates` (could), `waiting` (could and waits) and `slotsOut` (could and outOfSlots). Then `atCapacity: full+throttled+committed == len(hs.hosts) || (candidates > 0 && waiting == candidates)` and `onlyFull := full+throttled == len(hs.hosts) || (candidates > 0 && slotsOut == candidates)`. Leave `noEligibleHost` alone. Rewrite the `blockage.atCapacity` comment and the `PoolPlan.BlockedAtCapacity` comment (scheduler.go near 278) to the new definition, keeping the why: a host that could never run the pool would be the same host however long anything waited.
- [ ] **Step 4a: Run `go test ./internal/scheduler -count=1`.** Expected: PASS.
- [ ] **Step 5: Run the dependents.** `make build-nogui && go test ./internal/controller -count=1 -run 'Problem|Capacity|Why|Seed|Demo'`. Expected: PASS. If a test encoded the old mixed-fleet result (the demo fleet in `internal/controller/seed.go` is itself mixed, so its seeded `pool.no_capacity` may become a warning), update that expectation and say why in a comment.
- [ ] **Step 6: Docs.** `docs/hosts-and-pools.md`: add one sentence under the `at capacity` row that a host that could never run the pool (another backend, platform or selector, or too small) is not counted when judging whether the fleet is full. `docs/problem-codes.md` (`pool.no_capacity`): "every host that could run it is full". `docs/capacity-demand-receiver.md`: "every host that could run the pool is full, however many other hosts the fleet has". Then `make generate && go test ./internal/catalog ./internal/docs -count=1`. Expected: PASS.
- [ ] **Step 7: Commit** "Stop a host that can never run a pool hiding that its fleet is full".

### Task 2: The machine planner says what it decided for each pool

**Files:** create `internal/controller/renting.go` (types and sentence helpers only in this task), `internal/controller/renting_test.go`; modify `internal/controller/machinedemand.go`.

**Interfaces:**
- Produces:
```go
type RentingState string
const (
	RentingBuying   RentingState = "renting"
	RentingOnItsWay RentingState = "on_its_way"
	RentingWaiting  RentingState = "waiting"
	RentingCeiling  RentingState = "ceiling"
	RentingUnserved RentingState = "unserved"
	RentingHeld     RentingState = "held" // set by the overlay in Task 4
	RentingOff      RentingState = "off"  // set by the overlay in Task 4
)
type PoolRenting struct {
	PoolID       string       `json:"pool_id"`
	PoolName     string       `json:"pool_name"`
	State        RentingState `json:"state"`
	Summary      string       `json:"summary"`
	Fix          string       `json:"fix,omitempty"`
	Setting      string       `json:"setting,omitempty"`
	ProviderID   string       `json:"provider_id,omitempty"`
	ProviderName string       `json:"provider_name,omitempty"`
	Queued       int          `json:"queued"`
	Short        int          `json:"short"`
	Machines     int          `json:"machines"` // for renting, the machines this pool alone needs, at most what the provider rents this pass (pools that share a machine each count it, so never add them up); for on_its_way, how many of the named provider's machines are coming
	Since        *time.Time   `json:"since,omitempty"`
	Because      string       `json:"-"` // for ceiling only: the reason without its prefix or full stop, which Task 4's overlay needs to build pool.offCeiling
}
func (v PoolRenting) Aside() string // pool.waitingTail for waiting, otherwise Summary
func poolReason(d poolDemand) string // the {reason} in Shared copy
// MachinePlan gains: Pools []PoolRenting `json:"pools,omitempty"`, sorted by pool name.
```
- Consumes: `scheduler.Plan`, `scheduler.Pair`, `machineFit`, `sentence`, `plural`, the ceiling helpers.

- [ ] **Step 1: Write the failing tests in `renting_test.go`.** Reuse `demandPool`, `demandProvider`, `demandHost`, `demandMachine`, `demandPoolPlan`, `demandJobs`, `demandLimits` from `machinedemand_test.go`.
  - `TestAMixedFleetBuysAMachineForABusyPool`: build the reporter's fleet with `scheduler.Decide` then `DecideMachines`. Pool `zoomies-linux-x64` (docker, labels `zoomies`, `zoomies-linux-x64`, `MinRunners: 2`, `MaxRunners: 15`); hosts: docker capacity 2 with 2 busy runners, docker capacity 1 with 1, docker capacity 2 with 2, docker capacity 4 with 2 and `Throttle: store.HostThrottle{Level: store.MaxThrottleLevel}`; one Windows host (`Backends: {"process"}`, `OS` and `Distro` `windows`, capacity 4, none active); busy runners are `store.Runner{State: store.RunnerBusy}` rows filed under the pool; one queued job labelled `zoomies-linux-x64`; provider `proxmox-lab` (docker, `MachineCapacity: 2`, `MaxMachines: 2`, `MaxCreatesInFlight: 1`); limits 2 and 2; scheduler `Policy{MaxRunnerLifetime: 6*time.Hour, ProvisionTimeout: 5*time.Minute, DrainTimeout: 15*time.Minute, MaxCreatesPerTick: 10}`. Assert with the Windows host: `plan.Pools[0].BlockedAtCapacity`, the provider plan `Create == 1`, its `Reason == "buy 1 machine on proxmox-lab: pool zoomies-linux-x64 is 1 runner slot short with 1 job queued, and every host that can run it is full"`, and one verdict with `State == RentingBuying`. Run the same without the Windows host and assert the same.
  - `TestEveryVerdictStateSaysItsSentence`: a table over hand-built snapshots, asserting `State`, `Summary`, `Fix`, `Setting`, `Machines` per Shared copy:
    - renting: one provider, a pool blocked `"nowhere"` with 1 queued, no hosts. Summary `Renting 1 machine from lab, which covers this pool: 1 runner slot short with 1 job queued, and no host in the fleet can run it.`
    - on its way: a `MachineCreating` machine of capacity 2 covering the shortfall. Summary `1 machine from lab is on the way for this pool, so the next runners start there.`, `Machines: 1`, `Since` equal to the machine's `CreatedAt`.
    - waiting: a plan with `Blocked: "no host can take a new docker runner (1 cordoned, 3 at capacity)"`, neither flag, `Desired: 2`, `Current: 1`. Summary per `pool.waiting`, Fix `pool.waitingFix`.
    - ceiling, fleet: `Limits.MaxMachines = 0`. Summary `Not renting: provider.max_machines is 0, so this fleet rents nothing.`, Fix `Set provider.max_machines to the number of machines this fleet may pay for.`, Setting `provider.max_machines`.
    - ceiling, provider: a provider with `MaxMachines: 1` that owns one ready machine and a pool still short. Summary `Not renting: lab owns 1 machine and its maximum is 1.`, Setting empty.
    - unserved, selector: provider `PoolSelector{"tier": "large"}`, pool without the label. Summary `No provider will rent for this pool. Provider lab only rents for pools matching tier=large, and pool alpha does not have tier=large.`, Fix `Edit the provider's pool selector, or give the pool the label tier=large.`
    - unserved, fit: provider builds docker, pool needs process. Summary `No provider builds a machine this pool could use. lab: The machines it builds run docker, and this pool needs process.`, Fix `pool.unservedFitFix`.
  - `TestTwoProvidersThatServeOnePoolGiveOneVerdict`: providers `alpha-lab` and `beta-lab`, one pool: `len(plan.Pools) == 1` and `plan.Pools[0].ProviderName == "alpha-lab"`.
  - `TestAPoolShortOfItsMinimumWithNothingQueuedDoesNotMentionAQueue`: `Desired: 2`, `Current: 0`, `QueuedMatched: 0`, blocked nowhere. Summary `Renting 1 machine from lab, which covers this pool: 2 runner slots short, and no host in the fleet can run it.`
  - `TestAPoolTheSchedulerCanPlaceItselfGetsNoVerdict` (not blocked) and `TestAPoolCoveredByFreeSlotsGetsNoVerdict`.
  - The existing `machinedemand_test.go` tests are not edited and must still pass: decisions are unchanged.
- [ ] **Step 2: Run `go test ./internal/controller -run 'TestAMixedFleet|TestEveryVerdict|TestTwoProviders|TestAPoolShort|TestAPoolThe|TestAPoolCovered' -count=1`.** Expected: FAIL to compile (`PoolRenting` undefined).
- [ ] **Step 3: Add the types and helpers** in `renting.go` as above.
- [ ] **Step 4: Record verdicts in `DecideMachines`** without changing any decision. Append to a local `[]PoolRenting`: `waiting` when the scheduler flags are both false but `pp.Blocked != ""` and `Desired > Current`, unless `providerFor` returns nil for the pool, in which case no machine will ever follow and the `unserved` verdict is recorded instead (a verdict only: never `MachinePlan.Unserved`, the list of pools whose work a machine would answer, which feeds a once-per-change warning); nothing when the pool is not blocked or the free-slot ledger covers the shortfall; `on_its_way` (count of the pending machines of the provider it names, and the oldest `CreatedAt` of those whose credit was consumed) when only the credit covered it; `unserved` when `providerFor` returns nil (selector reason from `unservedBy`, otherwise the fit reason of the first provider by name); and, after the ceilings are applied, `renting` or `ceiling` (with `Because` set to the `because` text) for each pool charged to a provider, from `create` and the `because`, `fix` that `bite` already holds. A pool's own need is `ceilDiv(d.slots, providerSlots(p))` and its own `Machines` is `min(create, own need)` in every case. `renting` when `create >= want` (the whole purchase is made, so each pool quotes the machines it alone needs, never the provider's whole order); when `0 < create < want` the purchase is held back, and `rentingPartial` (create of want) is used for a provider charged with one pool, `rentingShared` (create of want, "this pool and others") for one charged with several, because a held-back purchase cannot be said to cover any one of them; `ceiling` only when `create` is 0. Extend `bite` with a `setting` argument (`provider.max_machines` for the fleet machine ceiling, `provider.max_creates_in_flight` for the fleet create ceiling, empty for a provider's own). Sort the result by pool name and set `MachinePlan.Pools`.
- [ ] **Step 5: Run the Step 2 command, then `go test ./internal/controller -run 'Demand|Machine' -count=1`.** Expected: PASS.
- [ ] **Step 6: Commit** "Let the machine planner say what it decided for each pool".

### Task 3: The machine loop always runs, and renting switches on without a restart

**Files:** modify `internal/config/settings.go` (the `provider.enabled` entry), `internal/config/config.go` (stale comment near line 416), `internal/controller/controller.go` (spawn and its comment), `internal/controller/machines.go` (`provisioningHeld`, the guard comment), `internal/controller/machines_pause_test.go` (stale comment near line 201), `internal/api/admin_test.go` (`TestTheProviderDeadlinesNeedARestart`), `docs/configuration.md` (the `provider.enabled` row near line 609 and the paragraph near line 1951); test `internal/controller/machines_live_test.go`, plus an API test beside the existing `pending_restart` tests.

**Interfaces:**
- Produces: `provisioningHeld(p, now)` returns `held.off` (Shared copy) when `!c.cfg().Provider.Enabled`, after the recovery-fence clause and before the transfer, pause and row clauses, including for a nil `p`.

- [ ] **Step 1: Write the failing tests.**
  - `TestHeldSaysRentingIsOffWhenItIs`: `h := newHarness(t)`, `row := h.providerRow(t, "lab")`; `h.c.provisioningHeld(row, h.c.Now())` equals `held.off`; after `h.enableProviders(t)` it is `""`.
  - `TestRentingSwitchedOnWhileTheControllerRunsStartsRenting`: build the fleet without enabling (pool, `providerRow`, `queueWork`, `MaxMachines` left at zero), start the controller as `TestANewSchedulerIntervalTakesEffectWithoutARestart` does (`h.c.Start(ctx)`, `t.Cleanup(Stop)`), assert `len(h.machines()) == 0`, call `h.c.UpdateConfig` setting `Provider.Enabled = true` and `MaxMachines = 10`, then `eventually(t, 5*time.Second, "a machine is planned", func() bool { return len(h.machines()) > 0 })`.
  - The existing `TestNothingIsRentedUntilProvidersAreSwitchedOn` must still pass unchanged (no machine, no provider call while off).
  - API: PATCH `/api/v1/settings` with `{"provider.enabled": true}` is accepted and `provider.enabled` is absent from `pending_restart`. Find the file with `grep -rn pending_restart internal/api/*_test.go`.
  - `TestTheProviderDeadlinesNeedARestart`: remove `provider.enabled` from its restart-only list and assert it in the live list the same file keeps for `provider.max_machines`.
- [ ] **Step 2: Run** `go test ./internal/controller -run 'TestHeldSays|TestRentingSwitchedOn|TestNothingIsRented' -count=1 -race` and the API and config tests. Expected: FAIL for the new tests.
- [ ] **Step 3: Implement.** `settings.go`: add `Live: true`, delete `RestartReason`, and end the `Summary` with "Turning it off stops the whole loop, including draining and deleting machines already rented; use Pause new machines to stop buying and keep tending them." `controller.go`: spawn `machines` unconditionally (as `kennel` is), and rewrite the comments that say the loop starts only when enabled. `machines.go`: add the `held.off` clause; update the "Off is off" comment to say the loop runs and does nothing. Correct the stale comments in `config.go` and `machines_pause_test.go`.
- [ ] **Step 4: Docs.** `docs/configuration.md`: the `provider.enabled` row's column becomes `at once` with the new Summary, and the paragraph near line 1951 says it applies at once and what turning it off stops.
- [ ] **Step 5: Run** `go test ./internal/config ./internal/controller ./internal/api -count=1 -race`. Expected: PASS (includes `TestALiveSettingGivesNoReasonItNeedsARestart` and `TestEveryConfigurationKeyIsDocumented`).
- [ ] **Step 6: Commit** "Let renting be switched on without a restart".

### Task 4: The controller keeps a renting report and can say it as one view

**Files:** modify `internal/controller/renting.go`, `internal/controller/machines.go` (`ReconcileMachines`, `machineRuntime`), `internal/controller/controller.go` (fields); test `internal/controller/renting_view_test.go`, `internal/controller/machines_renting_test.go`.

**Interfaces:**
- Produces:
```go
type RentingView struct {
	State                string        `json:"state"` // "none", "off", "no_ceiling", "on"
	Summary              string        `json:"summary"`
	Fix                  string        `json:"fix,omitempty"`
	Setting              string        `json:"setting,omitempty"`
	Enabled              bool          `json:"enabled"`
	Paused               bool          `json:"paused"`
	MaxMachines          int           `json:"max_machines"`
	SuggestedMaxMachines int           `json:"suggested_max_machines,omitempty"`
	Owned                int           `json:"owned"`
	DecidedAt            *time.Time    `json:"decided_at,omitempty"`
	Pools                []PoolRenting `json:"pools"` // never nil
}
type rentingInput struct {
	Report      *rentingReport // nil before the first pass
	Usable      []*store.Provider // non-demo, row enabled
	Enabled     bool
	Paused      bool
	MaxMachines int
	Held        map[string]string // provider id to provisioningHeld, computed only while enabled
}
func buildRentingView(in rentingInput) RentingView // pure
func (c *Controller) Renting(ctx context.Context) (RentingView, error) // reads providers only when there is no report yet
func (c *Controller) rentingFor(poolID string) *PoolRenting // overlay applied, no database, nil when unknown
func (c *Controller) noteRenting(env *machineEnv, plan MachinePlan)
func (c *Controller) usableProviders(ctx context.Context) ([]*store.Provider, error) // non-demo, row enabled; from the report when there is one
```
- `rentingReport` holds `At`, `Pools []PoolRenting`, `Usable []*store.Provider` and `Owned int` (machines for which `Owns()` is true). It is replaced under `c.mu`, never `machines.mu`.
- State rules: `none` when `Usable` is empty; else `off` when `!Enabled`; else `no_ceiling` when `MaxMachines <= 0`; else `on`. `SuggestedMaxMachines` is the sum of `Usable` ceilings, at least 1. Overlay, in this order: when off, `renting` becomes `off` (`pool.off`, `Machines` kept), `ceiling` becomes `off` (`pool.offCeiling`, built from the verdict's `Because`), `on_its_way` becomes `off` (`pool.offOnItsWay`), `waiting` and `unserved` stay; when enabled, a `renting` verdict whose provider has a non-empty `Held` entry becomes `held` (`pool.held`). Every overlaid verdict that is `off` carries `Setting: "provider.enabled"` and `Fix: fleet.offFix`.

- [ ] **Step 1: Write the failing tests.**
  - `TestTheRentingViewSaysWhichSwitchIsHoldingRentingBack` (pure, table): off/zero gives `State "off"`, `Summary == fleet.off`, `Fix == fleet.offFix`, `Setting "provider.enabled"`; enabled/zero gives `no_ceiling` with `fleet.noCeiling`, `fleet.noCeilingFix`, `Setting "provider.max_machines"`; enabled/2 gives `on` with `fleet.on`; no usable providers gives `none` with `fleet.none`; a row-disabled provider is not in `Usable`, so it gives `none` (Review Focus 1); two usable providers with ceilings 2 and 3 suggest 5, one with ceiling 0 suggests 1.
  - `TestTheOverlayTurnsWhatWouldBeBoughtIntoWhatRentingBeingOffPrevents` (pure): the three off overlays with their exact sentences, `waiting` unchanged, and the `held` overlay while enabled.
  - `TestTheViewBeforeTheFirstPassHasTheSwitchesAndNoPools` (Review Focus 3): `Report == nil`, `Pools` is a non-nil empty slice, `DecidedAt == nil`.
  - Harness `TestAPassWhileRentingIsOffDecidesAndStoresButBuysNothing`: pool, `providerRow`, `queueWork` with `Provider.MaxMachines = 10` but `Enabled` false; `h.machinePass(t)`; `len(h.machines()) == 0`, `h.callsTo("allocate") == 0`; `h.c.Renting(h.ctx)` is `off` with one pool verdict `off` whose Summary is `Would rent 1 machine for this pool, but renting is switched off.`
  - `TestTheVerdictFollowsTheSwitchWithoutWaitingForAPass`: continue from the above, `UpdateConfig` enabling renting, then `h.c.Renting` before another pass shows `on` and the pool verdict `renting`.
  - `TestTheDemoFixtureIsNotInTheRentingView`: with the demo provider fixture only, `State == "none"` (extend the existing `TestTheDemoProvidersFixtureIsNeverRentedFrom`).
  - `TestAChangeOfWhatIsHeldRepublishesTheProviders`: run one `h.machinePass(t)` while renting is off (so there is an earlier held sentence to differ from), then `sub := h.listen(events.KindProviderUpdated)`, flip renting on via `UpdateConfig`, `h.machinePass(t)`; a `provider.updated` frame arrives whose `held` no longer contains "renting is switched off".
- [ ] **Step 2: Run** `go test ./internal/controller -run 'TestTheRenting|TestTheOverlay|TestTheViewBefore|TestAPassWhile|TestTheVerdictFollows|TestTheDemoFixtureIsNot|TestAChangeOfWhatIsHeld' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement the view builder and accessors** in `renting.go`; add `renting *rentingReport` to `Controller` and `lastHeld map[string]string` to `machineRuntime`.
- [ ] **Step 4: Restructure `ReconcileMachines`.** Order: take `machines.mu`; list providers and drop demo fixtures; none means clear the report and return; if `!c.mayAct() || !c.cfg().Provider.Enabled`, build `machineSnapshot`, `noteRenting(env, DecideMachines(env.demand()))`, call `c.publishDerived(ctx)` and return (no recovery, sweep, apply or step); otherwise the existing sequence unchanged, with `noteRenting(env, plan)` called once on the same plan that `applyMachinePlan` receives. `noteRenting` builds the report, stores it, and republishes (`publishProvider`) every provider whose `provisioningHeld` sentence differs from `lastHeld`.
- [ ] **Step 5: Run** the Step 2 command, then `go test ./internal/controller -count=1 -race`. Expected: PASS.
- [ ] **Step 6: Commit** "Keep the machine pass's verdict so renting can be explained while it is off". Push.

### Task 5: The renting route and its contract

**Files:** modify `internal/api/handlers_providers.go`, `internal/api/router.go` (next to `/pairings`, before `/{id}`), `api/openapi.yaml`, `docs/api-surface.md` (near lines 322, 327 and 388); regenerate `internal/api/openapi_spec.go` and `web/src/lib/api/schema.d.ts`; test `internal/api/providers_test.go`, `internal/api/api_test.go` (the route table near line 928), `internal/api/shapes_test.go`.

**Interfaces:**
- Consumes: `Controller.Renting(ctx) (RentingView, error)`.
- Produces: `GET /api/v1/providers/renting`, operationId `getProviderRenting`, `x-zoomies-role: viewer`, action `auth.ActionProvidersRead`; schemas `Renting` (required `state`, `summary`, `enabled`, `paused`, `max_machines`, `owned`, `pools`; `state` enum `none`, `off`, `no_ceiling`, `on`) and `PoolRenting` (required `pool_id`, `pool_name`, `state`, `summary`, `queued`, `short`, `machines`; `state` enum `renting`, `on_its_way`, `waiting`, `ceiling`, `unserved`, `held`, `off`).

- [ ] **Step 1: Write the failing tests.** `TestTheRentingRouteSaysWhyNothingIsRentedWhileRentingIsOff` (mirror `TestPairingsSayWhichSideRefusedAndWhy`): after forcing a machine pass with `ReconcileMachines` (the report is empty until one has run), a viewer GETs `/api/v1/providers/renting` with a provider and queued work and renting off, gets 200, `state == "off"`, `setting == "provider.enabled"`, a pool verdict with `state "off"`; with no provider, `state == "none"` and `pools` is `[]`. Add the route row `{method: "GET", path: "/api/v1/providers/renting", role: store.RoleViewer, action: auth.ActionProvidersRead}`. Add a `Renting` subtest to `TestResponsesMatchTheSpecShapes` whose fixture triggers the optional `setting`, `since` and `suggested_max_machines`.
- [ ] **Step 2: Run** `make build-nogui && go test ./internal/api -run 'TestTheRentingRoute|TestResponsesMatchTheSpecShapes|TestEmbeddedSpecMatchesTheSource' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement** `func (s *Server) handleProviderRenting(w http.ResponseWriter, r *http.Request)` writing `s.ctrl.Renting(r.Context())` as JSON, and the route. Add the path and both schemas to `api/openapi.yaml` in the style of `/providers/pairings` and `ProviderPairing`, then `go run internal/api/gen_openapi.go` and `(cd web && npm ci --no-audit --no-fund) && make openapi`.
- [ ] **Step 4: Docs.** `docs/api-surface.md`: list the route and say `providers_available` is the coarse flag and this route is the answer behind it.
- [ ] **Step 5: Run** `go test ./internal/api -count=1`. Expected: PASS.
- [ ] **Step 6: Commit** "Add the route that says why nothing is being rented".

### Task 6: The problem `provider.renting_off`

**Files:** modify `internal/controller/problems.go` (new `rentingProblems`, registered with `gather("machine renting", c.rentingProblems)` after the existing `gather("infrastructure providers", ...)`, and `problemAudience`), `internal/controller/status.go` (`publicSentences`), `docs/problem-codes.md`; regenerate `internal/catalog/catalog.json`; test `internal/controller/problems_renting_test.go`.

**Interfaces:**
- Produces: `func (c *Controller) rentingProblems(ctx context.Context, out *[]Problem) error`. Code `provider.renting_off`, `AudienceFleet`, `Setting: "provider.enabled"`, `TargetKind: "provider"`, `TargetID` the first usable provider by name (from `usableProviders`). Raised when `Renting(ctx).State == "off"`. Title `renting is switched off, so no provider can add a machine`. Detail: `{lab is | N providers are} configured, but this controller is not allowed to rent machines, so a pool that runs out of room waits for the hosts it has.`, then for each `off` pool verdict with `Machines > 0` (at most three, by name) ` Pool {name} would rent {n machine(s)} now.`, then when `Owned > 0` ` {1 machine is | N machines are} still rented, and nothing is draining, deleting or checking {it | them} while renting is off.` Fix `fleet.offFix`. Severity: warning when any pool verdict is `off` or `Owned > 0`, otherwise info.
- Public sentence: `Machines are not being rented, so jobs wait for the machines the fleet already has.`

- [ ] **Step 1: Write the failing tests** (harness): `TestAConfiguredProviderWithRentingOffRaisesAnInfoUntilWorkIsWaiting` (info with no queue, warning after `queueWork` and a pass; `Setting` and `TargetKind` as above), `TestRentingOffWithMachinesStillOwnedIsAWarning` (a ready machine, no queue; Detail contains `1 machine is still rented`), `TestADisabledProviderRowRaisesNoRentingProblem` (Review Focus 1), `TestTheDemoFixtureRaisesNoRentingProblem`, `TestNoRentingProblemWhileRentingIsOn`.
- [ ] **Step 2: Run** `go test ./internal/controller -run 'TestAConfiguredProviderWith|TestRentingOffWith|TestADisabledProviderRow|TestTheDemoFixtureRaises|TestNoRentingProblem' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement** as above. Use a `Code: "provider.renting_off"` string literal (a constant would hide it from the docs and audience tests).
- [ ] **Step 4: Docs.** `docs/problem-codes.md`: a row in "Runtime: infrastructure providers" (columns Code, Severity, What it means, What to do): severity `info, or **warning** with a pool waiting on it or machines still rented`; and the same sentence word for word under "What the status page says". Reword the "silent while provider.enabled is false" lines (near 230 and the intro under "Runtime: infrastructure providers") so they except this code. Then `make generate`.
- [ ] **Step 5: Run** `go test ./internal/controller ./internal/catalog ./internal/docs -count=1`. Expected: PASS (`TestEveryProblemCodeHasAnAudience`, `TestEveryFleetCodeHasAPublicSentence`, `TestEveryProblemCodeIsDocumented`, `TestTheCatalogIsCurrent`).
- [ ] **Step 6: Commit** "Raise a problem when a provider is configured and renting is off".

### Task 7: Pool, job and verdict text carry the renting sentence

**Files:** modify `internal/controller/problems.go` (`PoolCapacityProblems`), `internal/controller/explain.go` (`explainQueued`, and `whySnapshot` assembly near line 128), `internal/controller/why.go` (`whySnapshot`, `classifyQueued`); tests beside each existing test file.

**Interfaces:**
- Consumes: `Controller.rentingFor(poolID) *PoolRenting`, `PoolRenting.Aside()`.
- Produces: `whySnapshot.Renting *PoolRenting`.
- Rules: when a verdict exists for the blocked pool, `pool.no_capacity` Detail becomes `pp.Blocked + " " + v.Aside()` and, when `v.Fix != ""` and the state is not `waiting`, Fix becomes `v.Fix + " " + pp.BlockedFix`. `explainQueued` does the same to `out.Detail` and `out.Fix`. `classifyQueued` (blocked case) appends `Evidence{Kind: "plan_reason", Label: "renting", Value: v.Summary}` and, when `v.Fix != ""`, a `NextStep{Text: v.Fix, Kind: "change", Link: "/providers"}`. No verdict means no change, so fleets without providers read as before.

- [ ] **Step 1: Write the failing tests.** `TestAPoolWaitingForAHostSaysARentalIsOffWhenItIs` (renting off, queue, one scheduling pass so `PoolCapacityProblems()` has a plan, and one machine pass so there is a verdict; the `pool.no_capacity` entry for `pool.no_capacity` has Detail containing `Would rent 1 machine for this pool, but renting is switched off.` and a Fix starting `Turn on Rent machines in Settings`); `TestTheQueuedJobExplanationCarriesTheRentingSentence`; `TestTheJobVerdictCarriesTheRentingSentenceAsEvidenceAndANextStep` (evidence label `renting`, next step link `/providers`); `TestAWaitingVerdictAddsOnlyTheTailToTheSchedulersSentence`.
- [ ] **Step 2: Run** `go test ./internal/controller -run 'TestAPoolWaitingForAHost|TestTheQueuedJobExplanation|TestTheJobVerdictCarries|TestAWaitingVerdictAdds' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement** per the rules. The existing `why_test.go` builds a `whySnapshot` by hand, so the new field must be optional.
- [ ] **Step 4: Run** `go test ./internal/controller -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** "Say why no machine is rented where a pool and a waiting job are explained".

### Task 8: MCP `list_providers` includes the answer

**Files:** modify `internal/mcp/providers.go` (`listProviders`, the tool description), `docs/connect-claude.md` (near lines 192 to 207); regenerate `skills/zoomies/mcp-reference.md` with `make generate`; test `internal/mcp/providers_test.go` and `internal/api/mcp_providers_test.go`.

- [ ] **Step 1: Write the failing tests.** Beside the existing `providers_available` test: with a fake API answering `/providers`, `/meta` and `/providers/renting`, the tool's answer contains a `renting` key equal to the route's body; when `/providers/renting` fails the answer is still returned without it. Keep `TestProviderToolDescriptionsTellTheModelWhereToLookAndWhatNotToTrust` passing.
- [ ] **Step 2: Run** `go test ./internal/mcp ./internal/api -run 'Provider' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement.** After the `providers_available` splice, `Call(GET, "/providers/renting")` best effort and `list["renting"] = body`. Extend the description with: `renting is the answer to why nothing is being bought: whether renting is switched on and the fleet ceiling, and for each pool with work waiting what a provider will do about it and what to change.` Keep every phrase the description test pins.
- [ ] **Step 4: Docs and generated files.** `docs/connect-claude.md`: one sentence that `list_providers` now carries `renting`. Then `make generate`.
- [ ] **Step 5: Run** `go test ./internal/mcp ./internal/api ./cmd/zoomies -count=1`. Expected: PASS (`TestTheMCPReferenceMatchesTheBinary`).
- [ ] **Step 6: Commit** "Let an assistant ask why nothing is being rented in one call". Push.

### Task 9: The UI model, client and notice

**Files:** create `web/src/lib/providers/renting.ts`, `web/src/lib/providers/RentingNotice.svelte`, `web/unit/renting.test.ts`; modify `web/src/lib/api/client.ts` (add `getProviderRenting(signal?)` after `listProviderPairings`, `api.get<Result<'getProviderRenting'>>('/providers/renting', { signal })`) and `web/src/lib/api/types.ts` (`ProviderRenting = Schemas['Renting']`, `PoolRenting = Schemas['PoolRenting']`).

**Interfaces:**
```ts
export interface RentingNoticeModel {
  id: 'off' | 'no_ceiling';
  title: string;            // 'Renting is switched off' | 'Renting is on, but the fleet may rent no machines'
  body: string;             // renting.summary, unchanged: the server owns the sentence
  fix: string;              // renting.fix
  pools: PoolRenting[];     // verdicts in state 'off' or 'ceiling'
  action: { label: string; ceiling: number; keys: ('provider.enabled' | 'provider.max_machines')[] } | null;
  hint: string;             // for non-admins: `An administrator can change this under Settings, ${label}.` with label 'Rent machines' for off and 'Machines the fleet may rent' for no_ceiling
}
export function rentingNotice(renting: ProviderRenting | null, admin: boolean): RentingNoticeModel | null;
export function settingsFor(notice: RentingNoticeModel, ceiling: number): Record<string, boolean | number>;
export function clampCeiling(value: number, fallback: number): number;
export function poolRenting(renting: ProviderRenting | null, poolId: string): PoolRenting | null;
```
`RentingNotice.svelte` props: `renting: ProviderRenting | null`, `canAdmin: boolean`, `onchanged: () => void`; it renders nothing when `rentingNotice` returns null, so a page mounts it unconditionally. The button calls `updateSettings(settingsFor(...))`, shows the thrown `ApiError` message beside the button on failure, and calls `toasts.success` then `onchanged` on success.

- [ ] **Step 1: Write the failing unit tests** in `web/unit/renting.test.ts` (`node:test`, `assert/strict`, importing from `../src/lib/providers/renting.ts`):
```ts
test('renting off asks an administrator for one click that sets both settings', () => {
  const n = rentingNotice({ ...base, state: 'off', summary: 'S', fix: 'F', suggested_max_machines: 2 }, true);
  assert.equal(n?.title, 'Renting is switched off');
  assert.equal(n?.body, 'S');
  assert.equal(n?.action?.label, 'Turn on renting');
  assert.deepEqual(settingsFor(n!, 2), { 'provider.enabled': true, 'provider.max_machines': 2 });
});
test('someone who cannot change settings is told who can', () => {
  const n = rentingNotice({ ...base, state: 'off' }, false);
  assert.equal(n?.action, null);
  assert.equal(n?.hint, 'An administrator can change this under Settings, Rent machines.');
});
test('no ceiling offers the ceiling alone', () => {
  const n = rentingNotice({ ...base, state: 'no_ceiling', suggested_max_machines: 2 }, true);
  assert.equal(n?.title, 'Renting is on, but the fleet may rent no machines');
  assert.equal(n?.action?.label, 'Allow 2 machines');
  assert.deepEqual(settingsFor(n!, 2), { 'provider.max_machines': 2 });
});
test('there is nothing to say when nothing is wrong', () => {
  for (const state of ['none', 'on'] as const) assert.equal(rentingNotice({ ...base, state }, true), null);
  assert.equal(rentingNotice(null, true), null);
});
test('only pools renting cannot help are listed', () => { /* states off and ceiling kept, waiting and renting dropped */ });
test('a ceiling is a whole number of at least one', () => {
  assert.equal(clampCeiling(0, 2), 2);
  assert.equal(clampCeiling(Number.NaN, 2), 2);
  assert.equal(clampCeiling(3.9, 2), 3);
});
```
with `base = { state: 'on', summary: '', enabled: false, paused: false, max_machines: 0, owned: 0, pools: [] }`. Also assert `Allow 1 machine` for a suggestion of 1.
- [ ] **Step 2: Run** `cd web && npm run test:unit`. Expected: FAIL (module missing).
- [ ] **Step 3: Implement** `renting.ts` and the client and type additions; then the component (tokens only; `<section aria-labelledby>` with an `h2`; a number input for the ceiling labelled "Machines at once", defaulting to `action.ceiling`; the pool list shows each verdict's server `summary` and its queue count; non-admins see `hint`).
- [ ] **Step 4: Run** `cd web && npm run test:unit && npm run check && npm run lint`. Expected: PASS (`design-tokens.test.ts` fails on an undefined `--z-*`; `absent-value.test.ts` and the em dash test fail on a stray dash).
- [ ] **Step 5: Commit** "Add the renting notice and the model behind it".

### Task 10: The Providers pages show the notice and each pool's verdict

**Files:** modify `web/src/routes/Providers.svelte`, `web/src/routes/ProviderDetail.svelte`, `web/src/lib/providers/PoolsTab.svelte`; create `web/tests/renting.spec.ts`; docs `docs/ui.md` (the Providers section near line 553).

**Interfaces:**
- Consumes: `getProviderRenting`, `rentingNotice`, `poolRenting`, `RentingNotice`.
- Wiring: `Providers.svelte` loads the view with the other three requests (`.catch(() => null)`), shows `<RentingNotice>` above the `MetricGrid` when `rentingNotice(renting, canAdmin)` is non-null, and refetches it (debounced half a second) on `provider.updated`, `machine.updated` and `problems.updated`. `ProviderDetail.svelte` loads it beside the pairings and shows the same notice above the tabs. `PoolsTab` gains `renting: readonly PoolRenting[] | null` and, under each pool that has a verdict, a `<p class="verdict" data-testid="pool-renting">` with the verdict's `summary` (and `fix` when present), keeping the existing static reason for "Would not rent".

- [ ] **Step 1: Write the failing Playwright spec** `web/tests/renting.spec.ts` (copy the `goto` and `signedInAs` patterns from `providers.spec.ts` and `check-now.spec.ts`; route `**/api/v1/providers/renting` with `page.route`):
  - `the notice says renting is off and an administrator turns it on in one request`: route an `off` view with a pool verdict; assert the title, the body, the pool line; route `PATCH **/api/v1/settings`, capture `postDataJSON()`, click "Turn on renting"; `expect(body).toEqual({ 'provider.enabled': true, 'provider.max_machines': 2 })`; flip the renting route to `on` and assert the notice is gone.
  - `someone who cannot change settings is told who can`: signed in as a viewer, no button, the hint text visible.
  - `an environment-pinned setting shows the server's own refusal` (Review Focus 5): the PATCH returns 422 with a per-key error message; the message is visible and the notice stays.
  - `the Pools tab says what each pool's rental is doing now`: provider detail, Pools tab, `getByTestId('pool-renting')` contains the verdict summary.
  - `no notice appears when only the demo provider exists`: no mocking; `getByRole('heading', { name: 'Renting is switched off' })` has count 0 (the seeded provider is a fixture and the report calls it `none`).
  - an axe pass on the page with the notice shown, using the pattern in `a11y.spec.ts`.
- [ ] **Step 2: Run** `make build VERSION=dev && cd web && npx playwright test tests/renting.spec.ts --project=chromium`. Expected: FAIL.
- [ ] **Step 3: Implement** the wiring. Keep the page's existing deliberate choice of not using the fleet cache.
- [ ] **Step 4: Run** `cd web && npx playwright test tests/renting.spec.ts tests/providers.spec.ts tests/a11y.spec.ts tests/navigation.spec.ts tests/mobile.spec.ts`, then `npm run check && npm run lint && npm run test:unit`. Expected: PASS on both the `chromium` and `mobile` projects.
- [ ] **Step 5: Docs.** `docs/ui.md`: describe the notice and the per-pool verdict in the Providers section in two or three sentences.
- [ ] **Step 6: Commit** "Show why nothing is rented on the Providers pages". Push.

### Task 11: The Proxmox page, full verification and the PR

**Files:** modify `docs/proxmox.md` (the "Renting machines is off until you turn it on" note near line 32, and a new section "Why is no machine rented?"), `roadmap/plans/2026-10-11-machine-renting-explained-design.md` (Status line), the PR description.

- [ ] **Step 1: Docs.** Rewrite the note: renting is off until an administrator turns it on; the Providers page says so and offers one click; the setting applies at once; both `provider.enabled` and `provider.max_machines` must be set. Add "Why is no machine rented?": one short paragraph and a table of the verdict states (`renting`, `on_its_way`, `waiting`, `ceiling`, `unserved`, `held`, `off`) with what each means and what to change, the sentences taken from Shared copy, plus where each is shown (Providers page, pool page, job drawer, problems drawer, `list_providers`).
- [ ] **Step 2: Regenerate and format.** `make generate && make fmt`.
- [ ] **Step 3: Verify everything.** `make lint && make test`, then `make test-ui`. Expected: all PASS. Re-run the reporter's shape once more (Task 2's mixed-fleet test) and read its output.
- [ ] **Step 4: Update the spec Status** to implemented, and the PR description to list what shipped and what was left out.
- [ ] **Step 5: Commit** "Explain on the Proxmox page why no machine is rented". Push, mark the PR ready only after CI is green and the user has looked.
