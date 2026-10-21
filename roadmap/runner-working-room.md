# Working room on disk: a runner is never placed where a job cannot fit

Design, 20 October 2026. The plan that builds it is in `plans/` once written.

## The problem

On 20 October three CI jobs on the eyupio fleet failed with `no space left on
device` while downloading Go modules: `Upgrade` and `Cross-compile (linux,
arm64)` on main's run after #1182 merged, and `Go (controller-2)` on #1185.
All three ran on provider-rented machines, three different fresh Proxmox
clones. Every job on a fixed host passed. Nothing had silted up: a newly
cloned machine does not have room for one of this repository's Go jobs, and
the fleet let a runner start on it anyway.

What the fleet does today, and where the gaps are:

* The scheduler refuses to place on a host at its disk reserve (`fits` in
  `internal/scheduler/resources.go`), and the reserve floor is 2 GB
  (`store.MinHostReserveDiskMB`). A pool's per-runner disk ask comes only from
  `resources.disk_gb`, which defaults to zero, so nothing stops a runner
  starting on a machine with 3 GB free. One `go mod download` then fills it.
* The capacity recommender assumes 10 GiB of working room per runner
  (`scheduler.AutoAdjustRunnerDiskMB`), but placement does not charge it, so
  the two disagree about the same host.
* A provider's machine shape leaves disk to the template by default (the form
  says "the template decides"), which hands a clone the cloud image's few
  gigabytes. Migration 0103 made a provider learn the processors and memory
  its machines turn out to have, from the template at a check and from the
  first machine that joins, exactly because a machine of unknown size fits
  every pool. Disk was left out of it, so the synthetic host that decides
  whether a provider can serve a pool has an unknown disk, and an unknown
  figure constrains nothing.
* A runner's `_work`, Go module cache and build cache land in the container's
  writable layer under Docker's data root, with no size limit, on the
  machine's root disk.

Nothing reclaims Docker disk on a host either, but that is a different
problem (a fixed host that silts up over months) and is deliberately out of
scope here; see "Left out".

## What success looks like

* A runner is never placed on a host, real or a provider's next machine,
  that has less room than a job needs, and the reason the fleet gives names
  the figure and what changes it.
* A provider whose machines are too small for its slots is told so by its
  own check, before it rents anything, and the fix names the disk to set.
* The recommender and placement agree on what a runner needs.

## The design

### 1. Working room, charged by the scheduler

`Reserve(p, h)` returns a disk charge of `resources.disk_gb` today and zero
where the pool typed nothing. It will return a working room of 10 GiB where
the pool typed nothing: one constant, `scheduler.RunnerDiskMB`, which replaces
`AutoAdjustRunnerDiskMB` so that the recommender and placement read the same
figure. A docker-in-docker pool doubles only a typed figure, as it does today:
the pair shares one slot's working room.

Everything downstream already exists and needs no second copy of the rule:

* `fits` refuses a host whose allocatable disk is under the ask, on the
  empty-host question (`HostFits`) and the room-left question in a pass alike.
* `newHostSet` seeds each host with its allocatable disk and the pass charges
  the working room for each runner it adds. Review changed one thing here:
  the first draft let free disk stand as the measurement for the runners
  already there, and that forgot a runner placed last pass that had not yet
  written its room, so a host could be promised the same gigabytes twice
  across passes. A live runner now holds its room until it is gone, charged
  with its CPU and memory in `hostTotals.add`; one that has written most of
  its room is counted for the whole of it, which errs towards not placing.
* `MachineFor` names the disk a too-small machine would need, and
  `machineSizeFix` already says where to set it ("raise its machine size" or
  "enlarge the template it clones").
* `hostSet.why` counts the host as "low on disk". Its fix text gains the
  figure: each runner is given 10 GB of working room above the host's
  reserve; a pool that needs less sets `resources.disk_gb`.

A constant rather than a setting. The pool's `resources.disk_gb` is already
the per-pool override for a fleet of small jobs, and a fleet-wide setting
would have to be threaded through `HostFits`, `HostCanRun`, `MachineFor`,
`Reserve`, `MinimumReserve` and `RecommendHost`, all pure and all called from
the controller without a snapshot. If a fleet turns out to need a different
figure for every pool at once, a setting can be added then, with this
constant as its default.

What stays as it is: a host whose agent never measured its disk constrains
nothing, so an upgrade cannot empty a fleet; the 2 GB reserve floor and the
operator's `reserve_disk_mb`; the pool's typed `resources.disk_gb`, which
wins where set.

Behaviour that changes on purpose: a fixed host with less than about 12 GB
free that ran light jobs takes no new runner until space is freed, the
operator raises nothing, or the pool lowers its ask. The reason text says
which. That is the trade accepted: the alternative is a job that dies
part-way through with the same message these three did.

### 2. A provider learns its machines' disk

The same mechanism as migration 0103, extended to the field it left out.

* `provider.Report` gains `BuildsDiskMB` beside `BuildsCPUs` and
  `BuildsMemoryMB`: the boot disk's size as the provider reads it from the
  template, zero where it cannot. The Proxmox preflight reads it with
  `bootDisk` and `diskSizeMB`, the parser `growDisk` already uses.
* `providers` gains `seen_disk_mb INTEGER NOT NULL DEFAULT 0` (migration
  0115). `SetProviderMachineSizeSeen` writes it with the other two; the
  shape-edit reset clears it with them.
* `provideradmin` records `BuildsDiskMB` from a check as
  `MachineSizeFromTemplate`, and `noteMachineSizes` records a machine's
  reported `DiskTotalMB` as `MachineSizeFromMachine`. A machine reports the
  guest filesystem it actually has, which is what we want to learn: a disk
  grown at the block layer whose filesystem never grew is judged at its real
  size.
* `providerSynthHost` takes `machine_disk_mb` from the shape, else the seen
  figure, as `providerMachineSize` does for the other two. A provider whose
  template gives 3.5 GB is then refused for every pool with a working room,
  `provider.unservable` names the pool and the disk to set, and nothing is
  rented for it.
* `machineSizeSaid` includes the disk in its sentence so the explanation says
  which figure to go and change.

A zero on any of these means "not read", never "no disk", exactly as for the
other two fields.

### 3. Saying it at configuration time

The gate above stops the damage; this says so earlier, on the provider's own
card, before any pool or renting is involved.

* The provider check adds a finding, `provider.machine_disk_small`, warning,
  when the disk its machines will have (the shape's, else the template's as
  the check read it) is under the host reserve floor plus slots times working
  room. It names both figures ("8 GB, and two runners need 22 GB") and where
  to change them: the shape's disk, or fewer slots, or a larger template.
  It is a finding of the check rather than a problem of its own, because it
  is about the configuration and goes away when the configuration changes.
* The form's Disk field gets a hint computed the same way from the slots the
  form holds, so a new provider on the defaults (two slots, "the template
  decides") reads what that implies before saving.

### 4. Docs

* `docs/hosts-and-pools.md`: the paragraph "Free disk is a gate rather than a
  budget" becomes gate plus working room, with the figure, the per-pool
  override and the trade stated.
* `docs/proxmox.md`: the machine-shape row says the disk a template gives is
  read by the check and used until a machine measures itself.
* `docs/problem-codes.md` and the generated catalog: the new finding.

## Left out, deliberately

* Reclaiming disk on a host (old runner image versions, dangling layers,
  per-image tool caches). Real, but a different failure: a fixed host that
  fills over months. It needs a backend contract, an agent loop and a
  setting, and the hosttune consent rules apply to it. A separate design.
* Measuring Docker's data root as well as the agent's work directory. On the
  machines that failed they are one filesystem. A host with a separate
  `/var/lib/docker` is still measured on the wrong one; a follow-up.
* An agent-side refusal to create a runner when the host is below the working
  room. The controller has the figure and can decline to place instead of
  failing a runner and spending a registration credential.
* A limit on a container's writable layer (`--storage-opt`). It needs a
  storage driver that supports it and would turn a full disk into a failed
  job rather than an unplaced one.

## Failure modes

* A provider that cannot read its template's disk reports zero, the synthetic
  host's disk is unknown, and the provider is judged as today. The finding in
  section 3 does not fire on an unknown figure.
* A machine whose agent predates disk reporting teaches the provider nothing;
  the template's figure stands.
* A migration that fails leaves the schema as it was; the store's migration
  runner is transactional.
* The constant is deliberately generous. A pool whose jobs need less sets
  `resources.disk_gb`, and the reason text says so on the host it refuses.

## Tests

Table-driven, standard library, as the rest.

* `internal/scheduler`: a pool with no disk figure is charged the working
  room; a typed figure wins; docker-in-docker does not double the default; a
  pass charges the room for each runner it adds and refuses the next when it
  runs out; `MachineFor` names the disk; `RecommendHost` and `Reserve` use
  the same constant (a test that reads both).
* `internal/store`: the column migrates; `SetProviderMachineSizeSeen` writes
  and the shape edit resets the disk with the other two.
* `internal/provider/proxmox`: the preflight reports the template's boot disk
  size, and zero for a template without one.
* `internal/controller`: the synthetic host falls back to the seen disk; a
  machine's report records it; a provider with a small template is unservable
  with a fix that names the disk; the check's finding fires on a small shape
  and not on an unknown one.
* `web/unit`: the Disk hint for a given slot count.
* `internal/docs`: the existing tests over the docs and the catalog.
