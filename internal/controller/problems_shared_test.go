package controller

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// The problems list is worked out in sections, and the sections of one list share
// a single read of the pools, the hosts and the runners on them (fleet_reads.go).
// The list has to be the same list whether they share or each reads for itself,
// and the sections have to leave the rows they share as they found them.

// problemKeys is a problems list reduced to what it is about, in its order: the
// code, who it is about and how bad it is. The sentences are left out because
// some of them count the time since something, and two lists asked for a moment
// apart can say 59 seconds and a minute.
func problemKeys(ps []Problem) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = fmt.Sprintf("%s|%s|%s|%s", p.Code, p.TargetKind, p.TargetID, p.Severity)
	}
	return out
}

// everythingWrong is a fleet that raises a problem in a good many of the sections:
// pools that are dangerous, share a cache, or have nowhere to run, a host that is
// cordoned, one that stopped reporting, one the controller cannot talk to and one
// that reports no size, runners that failed and runners stuck coming up, jobs
// that wait for pools that cannot run them, and an installation that is not
// working. The clock stands still, so that two lists asked for at once are asked
// for at the same time.
func everythingWrong(t *testing.T) *harness {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	h := newHarnessAt(t, func() time.Time { return now })
	inst := h.installation()

	plain := h.pool(inst, "plain")
	risky := h.pool(inst, "risky")
	risky.DockerMode, risky.Ephemeral = store.DockerHostSocket, false
	shared := h.pool(inst, "widgets")
	shared.Cache = store.CacheConfig{Enabled: true, Scope: store.CacheScopeRepository, Repository: "acme/widgets"}
	nowhere := h.pool(inst, "nowhere", "self-hosted", "linux", "x64", "gpu")
	nowhere.HostSelector = store.StringMap{"gpu": "yes"}
	for _, p := range []*store.Pool{risky, shared, nowhere} {
		if err := h.st.UpdatePool(h.ctx, p); err != nil {
			t.Fatalf("UpdatePool: %v", err)
		}
	}

	healthy := h.host("vm-healthy")
	cordoned := h.host("vm-cordoned")
	if err := h.st.SetHostCordoned(h.ctx, cordoned.ID, true); err != nil {
		t.Fatalf("SetHostCordoned: %v", err)
	}
	silent := &store.Host{Name: "vm-silent", Capacity: 4, Backends: store.StringSlice{"docker"}, Labels: store.StringMap{},
		OS: "linux", Arch: "amd64", CPUs: 8, MemoryMB: 16 * 1024, LastHeartbeat: now.Add(-3 * time.Hour)}
	unsized := &store.Host{Name: "vm-unsized", Capacity: 4, Backends: store.StringSlice{"docker"}, Labels: store.StringMap{},
		OS: "linux", Arch: "amd64", LastHeartbeat: now}
	old := &store.Host{Name: "vm-old", Capacity: 4, Backends: store.StringSlice{"docker"}, Labels: store.StringMap{},
		OS: "linux", Arch: "amd64", CPUs: 8, MemoryMB: 16 * 1024, LastHeartbeat: now}
	for _, host := range []*store.Host{silent, unsized, old} {
		if err := h.st.CreateHost(h.ctx, host); err != nil {
			t.Fatalf("CreateHost: %v", err)
		}
	}
	if err := h.st.SetHostProtocol(h.ctx, old.ID, 1, true); err != nil {
		t.Fatalf("SetHostProtocol: %v", err)
	}

	failed := h.runnerRow(plain, healthy, store.RunnerProvisioning)
	if _, err := h.st.TransitionRunner(h.ctx, failed.ID, store.RunnerFailed, "the image could not be pulled"); err != nil {
		t.Fatalf("TransitionRunner: %v", err)
	}
	h.runnerRow(plain, healthy, store.RunnerProvisioning)
	h.runnerRow(plain, healthy, store.RunnerIdle)
	h.runnerRow(risky, unsized, store.RunnerBusy)
	h.runnerRow(shared, cordoned, store.RunnerIdle)

	for i := range 4 {
		h.deliverJob(jobEvent{Action: "queued", JobID: int64(100 + i), RunID: 50, Labels: []string{"self-hosted", "linux", "x64", "gpu"}})
	}
	h.deliverJob(jobEvent{Action: "queued", JobID: 200, RunID: 51, Labels: []string{"self-hosted", "mystery"}})
	if err := h.st.SetInstallationHealth(h.ctx, inst.ID, "the App is not installed on acme"); err != nil {
		t.Fatalf("SetInstallationHealth: %v", err)
	}
	return h
}

func TestTheProblemsListIsTheSameWhetherItsSectionsShareTheirReadsOrNot(t *testing.T) {
	h := everythingWrong(t)

	shared, err := h.c.Problems(h.ctx)
	if err != nil {
		t.Fatalf("Problems: %v", err)
	}
	alone, err := h.c.Problems(noFleetReads(h.ctx))
	if err != nil {
		t.Fatalf("Problems without sharing: %v", err)
	}
	// The whole of each problem, sentences and all: the clock stands still.
	if !reflect.DeepEqual(shared, alone) {
		t.Errorf("the list differs when its sections share their reads\n shared: %v\n alone:  %v", problemKeys(shared), problemKeys(alone))
	}
	// The comparison says something only if the fleet raised problems in the
	// sections that share: those about pools, those about hosts, and those about
	// the runners on them.
	keys := problemKeys(shared)
	for _, want := range []string{"pool.dangerous", "pool.cache_shared", "host.unhealthy", "host.resources_unknown", "runners.failed", "installation.unhealthy"} {
		if !slices.ContainsFunc(keys, func(k string) bool { return len(k) >= len(want) && k[:len(want)] == want }) {
			t.Errorf("the fleet raised no %s, so the comparison says nothing about the section that does: %v", want, keys)
		}
	}
	if len(shared) < 8 {
		t.Errorf("the fleet raised %d problems; want at least 8 for the comparison to be worth making: %v", len(shared), keys)
	}
}

// The sections read the rows they share and nothing else: a list worked out
// and the figures that follow it leave every pool, host and runner as the store
// has it.
func TestTheRowsTheSectionsShareAreLeftAsTheyWereFound(t *testing.T) {
	h := everythingWrong(t)
	ctx := withFleetReads(h.ctx)

	if _, err := h.c.Problems(ctx); err != nil {
		t.Fatalf("Problems: %v", err)
	}
	if _, err := h.c.Stats(ctx, statsEventWindow); err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if _, err := h.c.UpdatesView(ctx); err != nil {
		t.Fatalf("UpdatesView: %v", err)
	}

	r := readsIn(ctx)
	if !r.pools.done || !r.hosts.done || !r.runners.done {
		t.Fatalf("the sections did not read the pools, the hosts and their runners through the shared reads: %v %v %v", r.pools.done, r.hosts.done, r.runners.done)
	}
	pools, err := h.st.ListPools(h.ctx)
	if err != nil {
		t.Fatalf("ListPools: %v", err)
	}
	if !reflect.DeepEqual(r.pools.val, pools) {
		t.Errorf("a section changed a pool it shared")
	}
	hosts, err := h.st.ListHosts(h.ctx)
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if !reflect.DeepEqual(r.hosts.val, hosts) {
		t.Errorf("a section changed a host it shared")
	}
	runners, err := h.st.ListRunnersByHost(h.ctx, store.RunnerIdle, store.RunnerBusy)
	if err != nil {
		t.Fatalf("ListRunnersByHost: %v", err)
	}
	if !reflect.DeepEqual(r.runners.val, runners) {
		t.Errorf("a section changed a runner it shared")
	}
}

// What sharing is for is the reading, and the reading is allocation: the pools
// and the hosts of a fleet decoded again for every section. A list worked out
// with its reads shared allocates well under what it does with each section
// reading for itself.
func TestAProblemsListWithSharedReadsAllocatesWellUnderOneWithout(t *testing.T) {
	h := newHarness(t)
	_, pools := benchPools(h, 20)
	for i := range 10 {
		host := h.host(fmt.Sprintf("vm-%03d", i))
		h.seedRunner(t, pools[i], host, store.RunnerIdle)
	}

	shared := testing.AllocsPerRun(3, func() {
		if _, err := h.c.Problems(h.ctx); err != nil {
			t.Fatalf("Problems: %v", err)
		}
	})
	alone := testing.AllocsPerRun(3, func() {
		if _, err := h.c.Problems(noFleetReads(h.ctx)); err != nil {
			t.Fatalf("Problems without sharing: %v", err)
		}
	})
	if shared > alone*0.6 {
		t.Errorf("a list with its reads shared allocated %.0f times and one without %.0f; want the shared list under 60%% of the other", shared, alone)
	}
}

// The same holds for everything a listener is told after a pass, which is the
// list, the statistics, the update status and the host views together: they read
// the fleet once between them.
func TestTheFiguresAfterAPassAllocateWellUnderTheSameWithoutSharedReads(t *testing.T) {
	h := newHarness(t)
	_, pools := benchPools(h, 20)
	for i := range 10 {
		host := h.host(fmt.Sprintf("vm-%03d", i))
		h.seedRunner(t, pools[i], host, store.RunnerIdle)
	}
	sub := h.listen()
	go func() {
		for range sub.C {
		}
	}()

	shared := testing.AllocsPerRun(3, func() { h.c.publishDerived(h.ctx) })
	alone := testing.AllocsPerRun(3, func() { h.c.publishDerived(noFleetReads(h.ctx)) })
	// The problems list alone, sharing among its own sections, comes to 0.29 of what
	// each reading for itself does; the whole set sharing comes to 0.19.
	if shared > alone*0.25 {
		t.Errorf("the figures after a pass allocated %.0f times with their reads shared and %.0f without; want the shared ones under a quarter of the other", shared, alone)
	}
}
