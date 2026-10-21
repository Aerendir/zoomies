# Working room on disk: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A runner is never placed on a host, real or a provider's next machine, that has less than a job's working room on disk, and a provider learns the disk its machines have so "the template decides" is judged by what the template gives.

**Architecture:** One constant in the pure scheduler, `RunnerDiskMB`, becomes the disk a pool is charged where it types none, so the existing `fits` gate, pass accounting, `MachineFor` and reason text all act on it. The provider side extends the mechanism migration 0103 built for processors and memory to disk: the preflight reports the template's boot disk, the store keeps `seen_disk_mb`, and the synthetic host the renting decision is asked about carries it. The provider's check adds a finding when a shape is too small for its slots.

**Tech Stack:** Go 1.27 (standard library `testing`, table-driven), SQLite migrations embedded in `internal/store/migrations`, Svelte 5 and TypeScript for the form hint with `node:test` unit tests.

**Spec:** `roadmap/runner-working-room.md`

## Global Constraints

* British spelling in prose; no em dashes and no spaced `--` anywhere (`internal/docs` fails the build on both).
* Comments say why, not what. Test names are sentences about behaviour.
* `internal/scheduler` stays pure: no clock, store or network (`purity_test.go`).
* SQL only in `internal/store`. A new column is a new numbered migration file, and `internal/store/migrations_test.go` lists every file by name.
* The working room is **10 GiB** (`10 * 1024` MB) and the host reserve floor is `store.MinHostReserveDiskMB` (**2048** MB). A zero on any disk figure means "not measured", never "no disk".
* Every `*.updated` event is the resource's GET shape; publish through the controller's helpers only.
* `make build-nogui` once before any plain `go` command; `make test` before the push.
* Commit messages are imperative sentences in plain prose, no prefix.

## Review Focus

1. A pool that types `resources.disk_gb` **smaller** than the working room is charged what it typed and places on a host with that much room (Task 1, test 3).
2. A host whose agent never reported a disk total constrains nothing, so an upgrade cannot empty a fleet (Task 1, test 4).
3. A provider whose shape types a disk is judged by the shape even when its machines report a smaller filesystem; the seen figure only answers where the shape is silent (Task 4, test 2).
4. A docker-in-docker pool with no typed disk is charged one working room for the pair, not two (Task 1, test 2).
5. The check's finding is silent when neither the shape nor the template says a figure, and treats a shape with zero slots as one (Task 5, tests 2 and 3).

---

### Task 1: The scheduler charges a working room where the pool types none

**Files:**
- Modify: `internal/scheduler/recommend.go:19-24` (the constant), `:225` (its use)
- Modify: `internal/scheduler/resources.go:85-130` (`Reserve`)
- Modify: `internal/scheduler/scheduler.go:1937-1941` (the "low on disk" fix text)
- Test: `internal/scheduler/resources_test.go`, `internal/scheduler/recommend_test.go`

**Interfaces:**
- Produces: `const RunnerDiskMB int64 = 10 * 1024` in `internal/scheduler/recommend.go`, replacing `AutoAdjustRunnerDiskMB` (rename every use; nothing outside the package uses it). `Reserve(p *store.Pool, h *store.Host) Reservation` returns `DiskMB == RunnerDiskMB` where `p.Resources.DiskGB == 0`.

- [ ] **Step 1: Write the failing tests in `resources_test.go`**

```go
// A pool that says nothing about disk used to be charged nothing, so one
// runner could start on a machine with 3 GB free and fill it part-way through
// its first download. Three CI jobs died that way on fresh machines.
func TestAPoolThatTypesNoDiskIsChargedAWorkingRoom(t *testing.T) {
	h := sized("host", 4, 16, 16*1024, 200*1024)
	if got := Reserve(testPool("plain", "plain"), h).DiskMB; got != RunnerDiskMB {
		t.Fatalf("charged %d MB, want the working room %d", got, RunnerDiskMB)
	}
}

func TestADockerInDockerPairSharesOneWorkingRoom(t *testing.T) {
	p := testPool("dind", "dind")
	p.DockerMode = store.DockerDinD
	if got := Reserve(p, sized("host", 4, 16, 16*1024, 200*1024)).DiskMB; got != RunnerDiskMB {
		t.Fatalf("charged %d MB, want one room %d: only a typed figure doubles", got, RunnerDiskMB)
	}
}

func TestATypedDiskFigureWinsOverTheWorkingRoom(t *testing.T) {
	p := testPool("small", "small")
	p.Resources.DiskGB = 1
	h := sized("host", 4, 16, 16*1024, store.MinHostReserveDiskMB+2048) // 2 GB to place on
	if got := Reserve(p, h).DiskMB; got != 1024 {
		t.Fatalf("charged %d MB, want the 1024 the pool typed", got)
	}
	if !HostFits(h, p) {
		t.Fatal("a pool that asks for 1 GB does not fit a host with 2 GB to place on")
	}
}

func TestAnUnmeasuredDiskConstrainsNothing(t *testing.T) {
	h := sized("host", 4, 16, 16*1024, 0)
	h.DiskTotalMB, h.DiskFreeMB = 0, 0
	if !HostFits(h, testPool("plain", "plain")) {
		t.Fatal("a host that never measured its disk must place by slots alone")
	}
}

// Free disk is a measurement, so the runners already there are in the figure;
// the ones this pass adds are not, and each is charged its room.
func TestAPassChargesEachRunnerItAddsAWorkingRoom(t *testing.T) {
	h := sized("host", 4, 16, 16*1024, store.MinHostReserveDiskMB+RunnerDiskMB+1024) // room for one
	hs := newHostSet([]*store.Host{h}, []*store.Pool{testPool("plain", "plain")}, nil, now)
	if got := len(hs.place(testPool("plain", "plain"), 3)); got != 1 {
		t.Fatalf("placed %d runners on a host with room for one", got)
	}
}

func TestTheLowOnDiskFixNamesTheWorkingRoom(t *testing.T) {
	h := sized("host", 4, 16, 16*1024, store.MinHostReserveDiskMB+1024)
	b := newHostSet([]*store.Host{h}, []*store.Pool{testPool("plain", "plain")}, nil, now).why(testPool("plain", "plain"))
	for _, want := range []string{"free space", "10 GB", "disk_gb"} {
		if !strings.Contains(b.fix, want) {
			t.Errorf("the fix %q does not say %q", b.fix, want)
		}
	}
}
```

And in `recommend_test.go`, replace `AutoAdjustRunnerDiskMB` with `RunnerDiskMB` in `TestASmallDiskLimitsHowManyRunnersAHostIsToldToTake`, and add:

```go
// The recommender and placement read one constant, so a host told it can
// hold N runners is a host the pass will place N runners on.
func TestPlacementAndTheRecommenderAgreeOnAWorkingRoom(t *testing.T) {
	h := &store.Host{Capacity: 8, Backends: store.StringSlice{"docker"}, CPUs: 36, MemoryMB: 131072,
		DiskTotalMB: 40 * 1024, DiskFreeMB: 40 * 1024}
	r, ok := RecommendHost(h, DefaultRunnerAsk)
	if !ok || r.LimitedBy != "disk" {
		t.Fatalf("the disk should set the capacity: %+v", r)
	}
	if got := Reserve(&store.Pool{Enabled: true, Backend: store.BackendDocker}, h).DiskMB; got != RunnerDiskMB {
		t.Fatalf("placement charges %d MB, the recommender assumed %d", got, RunnerDiskMB)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -run 'WorkingRoom|TypedDiskFigure|UnmeasuredDisk|LowOnDiskFix|Agree' ./internal/scheduler/`
Expected: FAIL; the first on `charged 0 MB`, the last on `undefined: RunnerDiskMB`.

- [ ] **Step 3: Implement**

In `recommend.go` rename `AutoAdjustRunnerDiskMB` to `RunnerDiskMB`, keeping its comment and adding that placement charges it where a pool types nothing. In `Reserve` (`resources.go`), after `res` is built: `diskFromHost := res.DiskMB <= 0; if diskFromHost { res.DiskMB = RunnerDiskMB }`, and double `res.DiskMB` under DinD only when `!diskFromHost`. Rewrite the comment above the doubling, which says disk has no fallback. In `scheduler.go:1941` make the fix: `"free space on those hosts' work directories, set the pool's resources.disk_gb below the 10 GB of working room a runner is given, or add a host"`.

- [ ] **Step 4: Run the package**

Run: `go test -race -count=1 ./internal/scheduler/ && go test -count=1 -run 'Disk|Machine|Fit|Capacity' ./internal/controller/`
Expected: PASS. Existing tests that size a host with under 12 GB free and are not about disk now refuse; raise their free disk (the `sized` helper's last argument, or a controller test host's `DiskFreeMB`) rather than the constant. `TestAHostBelowItsDiskReserveTakesNothing` and `TestTheShortfallNamesTheDiskAPoolAskedFor` must still pass as they are.

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler
git commit -m "Charge every runner a working room on disk where its pool types none"
```

---

### Task 2: The Proxmox preflight reports the disk the template builds

**Files:**
- Modify: `internal/provider/provider.go:560-571` (`Report`)
- Modify: `internal/provider/proxmox/preflight.go:490-497`
- Test: `internal/provider/proxmox/preflight_test.go:466-474`

**Interfaces:**
- Produces: `Report.BuildsDiskMB int64`, the template's boot disk in MB as read by `bootDisk` and `diskSizeMB`, zero where it cannot be read.

- [ ] **Step 1: Extend the failing test**

In `TestPreflightReportsTheSizeTheTemplateBuilds`, assert `report.BuildsDiskMB == 32*1024` (the fake's template line is `size=32G`; if the prerequisites the test uses point at a template with another size, assert that one). Add a case: a template whose disk line carries no `size=` reports `BuildsDiskMB == 0`, using the fake's `disk` field on the template VM.

- [ ] **Step 2: Run it to see it fail**

Run: `go test -run TestPreflightReportsTheSizeTheTemplateBuilds ./internal/provider/proxmox/`
Expected: FAIL on `report.BuildsDiskMB undefined`.

- [ ] **Step 3: Implement**

Add `BuildsDiskMB int64` to `Report` beside `BuildsMemoryMB`, extending that comment to disk. In `preflight.go` after `pf.report.BuildsMemoryMB = int64(cfg.Memory)`: read `_, spec := bootDisk(cfg)` and set `pf.report.BuildsDiskMB` when `diskSizeMB(spec)` reports a size.

- [ ] **Step 4: Run the package**

Run: `go test -race -count=1 ./internal/provider/... `
Expected: PASS, including the contract tests the fake provider runs.

- [ ] **Step 5: Commit**

```bash
git add internal/provider
git commit -m "Report the disk a template builds from the preflight, beside its processors and memory"
```

---

### Task 3: The store keeps the disk a provider's machines were seen to have

**Files:**
- Create: `internal/store/migrations/0115_provider_machine_disk_seen.sql`
- Modify: `internal/store/machines.go:118-125`, `internal/store/queries_providers.go:24-46, 72-90, 143-170, 221-243`
- Modify: `internal/store/migrations_test.go:128` (the file list)
- Modify: `internal/controller/provideradmin.go:228-233`, `internal/controller/machines.go:331-358` (the two callers)
- Test: `internal/store/provider_machine_size_test.go`

**Interfaces:**
- Produces: `Provider.SeenDiskMB int64` (`json:"seen_disk_mb,omitempty"`); `SetProviderMachineSizeSeen(ctx context.Context, id string, cpus int, memoryMB, diskMB int64, from string, at time.Time) (bool, error)`.
- Consumes: `Report.BuildsDiskMB` (Task 2).

- [ ] **Step 1: Extend the failing tests**

In `TestAProvidersSeenMachineSizeIsOnlyEverReplacedByNewerEvidence` add a `diskMB int64` column to the steps (`3915` memory rows carry `20480`, the check's row `32768`), pass it, and assert `got.SeenDiskMB == 32768`. In the reset test below it (`SetProviderMachineSizeSeen(ctx, p.ID, 8, 15800, ...)`) pass `40960` and include `SeenDiskMB` in both the `forgot` and `kept` conditions. Add the migration's file name to the list in `migrations_test.go`.

- [ ] **Step 2: Run them to see them fail**

Run: `go test -run 'SeenMachineSize|Migration' ./internal/store/`
Expected: FAIL on the signature, then on the missing file.

- [ ] **Step 3: Implement**

The migration, with a header in the voice of `0103`: why disk was left out and what it cost (three jobs on fresh clones):

```sql
ALTER TABLE providers ADD COLUMN seen_disk_mb INTEGER NOT NULL DEFAULT 0;
```

Add the column to `providerCols` after `seen_memory_mb`, to `scanProvider`, to the insert (one more `?`), to the shape-edit reset (`seen_disk_mb = CASE WHEN ... THEN 0 ELSE seen_disk_mb END`) and to `SetProviderMachineSizeSeen`'s `SET` and its "already holds" predicate. Update the two controller callers: `provideradmin.go` passes `report.BuildsDiskMB`; `noteMachineSizes` passes `h.DiskTotalMB`, includes `row.SeenDiskMB == h.DiskTotalMB` in its "same evidence" check, and sets `row.SeenDiskMB` when it wrote. The `h.CPUs <= 0 || h.MemoryMB <= 0` guard stays as it is: a machine that measured no disk records zero, which means unmeasured.

- [ ] **Step 4: Run the packages**

Run: `go test -race -count=1 ./internal/store/ && go build ./... && go test -race -count=1 -run 'MachineSize|Seen|Template' ./internal/controller/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store internal/controller/provideradmin.go internal/controller/machines.go
git commit -m "Keep the disk a provider's machines were seen to have, with their processors and memory"
```

---

### Task 4: The provider's next machine is judged by its disk

**Files:**
- Modify: `internal/controller/machinedemand.go:421-475` (`providerSynthHost`, `providerMachineSize`)
- Modify: `internal/controller/pairings.go:130-216` (`machineSizeSaid`, `machineSizeFix`, `machineSize`)
- Test: `internal/controller/machine_size_test.go`

**Interfaces:**
- Produces: `providerMachineDisk(p *store.Provider) int64` (the shape's `MachineDiskMB`, else `SeenDiskMB`); `machineSize(cpus int, memoryMB, diskMB int64) string`.
- Consumes: `Provider.SeenDiskMB` (Task 3), `scheduler.RunnerDiskMB` (Task 1).

- [ ] **Step 1: Write the failing tests in `machine_size_test.go`**

Model them on the tests there that set `SeenCPUs, SeenMemoryMB, SeenFrom`:

```go
// "The template decides" used to mean a machine of unknown disk, which fits
// every pool. A provider whose template gives a cloud image's few gigabytes
// is now refused for any pool with a working room, before it rents anything.
func TestAProviderWhoseTemplateGivesASmallDiskIsUnservable(t *testing.T) {
	// p: a provider with MachineDiskMB 0, SeenDiskMB 4096, SeenFrom template,
	// SeenCPUs 2, SeenMemoryMB 4096, capacity 1; a pool typing no disk.
	// Assert providerSynthHost(p, now).DiskTotalMB == 4096, that
	// scheduler.HostCanRun(synth, pool, now) is false, and that the
	// provider.unservable problem's Fix names a disk figure in GB.
}

func TestAShapeThatTypesADiskWinsOverWhatAMachineMeasured(t *testing.T) {
	// MachineDiskMB 32768, SeenDiskMB 4096 from a machine:
	// providerSynthHost(p, now).DiskTotalMB == 32768.
}

func TestTheSizeSentenceNamesTheDisk(t *testing.T) {
	// machineSizeSaid on a provider with SeenDiskMB 4096 from the template
	// contains "4 GB of disk" and "the template it clones gives".
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -run 'SmallDiskIsUnservable|WinsOverWhatAMachine|SentenceNamesTheDisk' ./internal/controller/`
Expected: FAIL; the synthetic host's `DiskTotalMB` is 0 and the sentence has no disk.

- [ ] **Step 3: Implement**

`providerMachineDisk` beside `providerMachineSize`; `providerSynthHost` sets `DiskTotalMB` and `DiskFreeMB` from it, keeping the comment that an unbuilt machine has all of its disk free. `machineSize` gains the disk as `scheduler.FormatMB(diskMB) + " of disk"`; `machineSizeSaid` passes `providerMachineDisk(p)` and counts `p.MachineDiskMB > 0` as a typed size; `machineSizeFix` passes `need.DiskMB` so the fix names the disk `MachineFor` found.

- [ ] **Step 4: Run the package**

Run: `go test -race -count=1 -timeout 45m ./internal/controller/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller
git commit -m "Judge a provider's next machine by the disk it will have"
```

---

### Task 5: The provider's check says when a shape is too small for its slots

**Files:**
- Modify: `internal/controller/provideradmin.go:210-215, 250-270`
- Modify: `docs/problem-codes.md` (the providers table near line 383 and the sentence table near line 539)
- Regenerate: `internal/catalog/catalog.json` with `make generate`
- Test: `internal/controller/provideradmin_test.go`

**Interfaces:**
- Produces: `machineDiskFinding(row *store.Provider, report provider.Report) (config.Finding, bool)`, code `provider.machine_disk_small`, severity warning, `Setting: "machine_disk_mb"`.
- Consumes: `Report.BuildsDiskMB` (Task 2), `providerSlots` (existing), `scheduler.RunnerDiskMB`, `store.MinHostReserveDiskMB`.

- [ ] **Step 1: Write the failing tests**

```go
func TestTheCheckWarnsWhenAShapeIsTooSmallForItsSlots(t *testing.T) {
	// row: MachineCapacity 2, MachineDiskMB 0; report.BuildsDiskMB 8192.
	// want a finding with Code "provider.machine_disk_small", Severity
	// warning, Setting "machine_disk_mb"; Title containing "8 GB" and
	// "22 GB" (2048 + 2 * 10240); Fix containing "machine's disk" and "slots".
}

func TestTheCheckIsSilentWhenNothingSaysTheDisk(t *testing.T) {
	// MachineDiskMB 0 and BuildsDiskMB 0: ok == false.
}

func TestTheCheckCountsAShapeWithNoSlotsAsOne(t *testing.T) {
	// MachineCapacity 0, MachineDiskMB 12288 (2048 + 10240): ok == false;
	// MachineDiskMB 12287: ok == true.
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -run 'TooSmallForItsSlots|SilentWhenNothingSaysTheDisk|NoSlotsAsOne' ./internal/controller/`
Expected: FAIL on `undefined: machineDiskFinding`.

- [ ] **Step 3: Implement**

`machineDiskFinding`: `disk := row.MachineDiskMB`, else `report.BuildsDiskMB`, else no finding. `need := store.MinHostReserveDiskMB + int64(providerSlots(row))*scheduler.RunnerDiskMB`. Below `need`, a finding whose Title is `fmt.Sprintf("the machines this provider builds have %s of disk, and %s need %s", FormatMB(disk), plural(slots, "runner"), FormatMB(need))`, whose Detail says each runner is given 10 GB of working room above the host's 2 GB reserve, and that a job on a machine with less fills the disk part-way through and fails with "no space left on device", and whose Fix is "raise the machine's disk to at least X, give the machine fewer slots, or enlarge the template it clones" (drop the template clause when the shape typed the disk). Append it in `CheckProvider` after `machineCertificateFinding`. Add the two rows to `docs/problem-codes.md` in the voice of `provider.machine_ca_same_as_cluster` ("Found by the provider's check."), then `make generate`.

- [ ] **Step 4: Run the packages**

Run: `go test -race -count=1 -run 'Check|Finding' ./internal/controller/ && go test -count=1 ./internal/catalog/ ./internal/docs/`
Expected: PASS; `TestTheCatalogIsCurrent` passes only after `make generate`.

- [ ] **Step 5: Commit**

```bash
git add internal/controller internal/catalog docs/problem-codes.md
git commit -m "Say from the provider's check when a machine shape is too small for its slots"
```

---

### Task 6: The form's Disk field says what the slots need

**Files:**
- Modify: `web/src/lib/providers/recommend.ts` (constants and helper)
- Modify: `web/src/lib/providers/ProviderForm.svelte:846-864` (the Disk `Field` gains a `hint`)
- Test: `web/unit/provider-recommend.test.ts`

**Interfaces:**
- Produces: `export const RUNNER_DISK_MB = 10 * 1024;`, `export const HOST_RESERVE_DISK_FLOOR_MB = 2048;` (mirrors of `scheduler.RunnerDiskMB` and `store.MinHostReserveDiskMB`, say so in their comments); `export function machineDiskNeededMb(slots: number): number` (`floor + max(slots, 1) * room`); `export function machineDiskHint(slots: number, diskMb: number): string`.

- [ ] **Step 1: Write the failing tests**

```ts
test('a machine needs the reserve floor plus a working room per slot', () => {
  assert.equal(machineDiskNeededMb(2), 2048 + 2 * 10240);
  assert.equal(machineDiskNeededMb(0), 2048 + 10240);
});

test('the disk hint names the figure and says when the disk is under it', () => {
  assert.match(machineDiskHint(2, 0), /22 GB/);
  assert.match(machineDiskHint(2, 8192), /8 GB.*22 GB/);
  assert.doesNotMatch(machineDiskHint(1, 32768), /under/);
});
```

- [ ] **Step 2: Run them to see them fail**

Run: `make test-unit`
Expected: FAIL on the missing exports.

- [ ] **Step 3: Implement**

The helper in `recommend.ts`; the hint copy: with no disk typed, "Each runner is given 10 GB of working room above a 2 GB reserve, so N slots need at least X GB; the template's disk counts once the check has read it." With a disk under the figure, "D GB is under the X GB N slots need." With enough, "N slots need at least X GB." Use the form's existing `memoryLabel` for GB figures. In `ProviderForm.svelte` pass `hint={machineDiskHint(Number(draft.machine_capacity) || 0, Number(draft.machine_disk_mb) || 0)}` on the Disk `Field`, as the Operating system field does.

- [ ] **Step 4: Run the UI checks**

Run: `make test-unit && cd web && npm run check && npm run lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "Say under the provider's Disk field what its slots need"
```

---

### Task 7: Docs

**Files:**
- Modify: `docs/hosts-and-pools.md:1561-1565` (the gate paragraph) and `:1668` (the "low on disk" row)
- Modify: `docs/proxmox.md:355` (the Machine shape row)
- Test: `internal/docs`

- [ ] **Step 1: Rewrite the paragraph**

"Free disk is a gate, and every runner is charged working room against it. A host at or below its disk reserve takes no new runner at all; above it, each runner a pass places is charged 10 GB of working room, or the figure the pool types as `resources.disk_gb`, so a job is never started on a machine that cannot hold its checkout and caches. Nothing is evicted to make room, because a runner's caches outlive it on purpose. Two things follow: disk is the one shortage that no job finishing will clear, and a host with less than about 12 GB free takes no new runner until space is freed or a pool asks for less." Update the "low on disk" row's fix to match, and the Proxmox row to say the disk a template gives is read by the check and used until a machine measures itself.

- [ ] **Step 2: Run the docs tests**

Run: `go test -count=1 ./internal/docs/`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add docs
git commit -m "Say that every runner is charged working room on disk, and where a provider's disk comes from"
```

---

### Task 8: The whole tree, then the pull request

- [ ] **Step 1: Run everything a contributor runs**

Run: `make lint && make test`
Expected: PASS (the controller package needs `-timeout` above ten minutes on a slow machine; `make test` sets it).

- [ ] **Step 2: Push and open a draft pull request**

Branch `claude/runner-working-room`. The body follows `.github/pull_request_template.md`: the problem (the three jobs and the three machines), the approach (sections 1 to 3 of the spec, the constant over a setting, what was left out), what was run, and the Impact list with the migration named under the deletion-path line (it adds a column and deletes nothing).
