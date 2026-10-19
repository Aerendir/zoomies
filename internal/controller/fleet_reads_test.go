package controller

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// The figures that follow a pass share one read of the pools, the hosts and the
// runners on them (fleet_reads.go). These hold that to what it promises: each is
// read once for the set, a section on its own reads for itself, a failure is not
// kept, a section's slice is its own, and the runners on a host are the ones the
// store gives for it.

// noFleetReads is ctx told to carry no reads and not to be given any, which is how
// a test asks the question the way a section run on its own does.
func noFleetReads(ctx context.Context) context.Context {
	return context.WithValue(ctx, fleetReadsKey{}, (*fleetReads)(nil))
}

// readFleet is a fleet of three pools and three hosts with runners of several
// states on them.
func readFleet(t *testing.T) (*harness, []*store.Host) {
	t.Helper()
	h := newHarness(t)
	inst := h.installation()
	var pools []*store.Pool
	for i := range 3 {
		pools = append(pools, h.pool(inst, fmt.Sprintf("pool-%d", i)))
	}
	var hosts []*store.Host
	for i := range 3 {
		hosts = append(hosts, h.host(fmt.Sprintf("vm-%d", i)))
	}
	for i, host := range hosts {
		for _, state := range []store.RunnerState{store.RunnerIdle, store.RunnerBusy, store.RunnerRegistering}[:i+1] {
			h.seedRunner(t, pools[i], host, state)
		}
	}
	return h, hosts
}

func TestOneSetOfReadsReadsThePoolsTheHostsAndTheirRunnersOnce(t *testing.T) {
	h, hosts := readFleet(t)
	ctx := withFleetReads(h.ctx)

	for range 4 {
		if _, err := h.c.fleetPools(ctx); err != nil {
			t.Fatalf("fleetPools: %v", err)
		}
		if _, err := h.c.fleetHosts(ctx); err != nil {
			t.Fatalf("fleetHosts: %v", err)
		}
		for _, host := range hosts {
			if _, err := h.c.fleetRunnersUpOn(ctx, host.ID); err != nil {
				t.Fatalf("fleetRunnersUpOn: %v", err)
			}
		}
	}
	r := readsIn(ctx)
	if r.pools.reads != 1 || r.hosts.reads != 1 || r.runners.reads != 1 {
		t.Errorf("a set of reads asked for the pools, the hosts and their runners again and again read them %d, %d and %d times; want once each",
			r.pools.reads, r.hosts.reads, r.runners.reads)
	}

	pools, _ := h.c.fleetPools(ctx)
	fresh, err := h.st.ListPools(h.ctx)
	if err != nil {
		t.Fatalf("ListPools: %v", err)
	}
	if !reflect.DeepEqual(pools, fresh) {
		t.Errorf("the pools a set of reads gave are not the pools the store has")
	}
}

func TestASectionRunOnItsOwnReadsForItself(t *testing.T) {
	h, _ := readFleet(t)

	for name, ctx := range map[string]context.Context{"a context with no reads": h.ctx, "a context told to carry none": noFleetReads(h.ctx)} {
		first, err := h.c.fleetPools(ctx)
		if err != nil {
			t.Fatalf("fleetPools: %v", err)
		}
		second, err := h.c.fleetPools(ctx)
		if err != nil {
			t.Fatalf("fleetPools: %v", err)
		}
		if first[0] == second[0] {
			t.Errorf("%s: two reads gave the same row, so something was kept between them", name)
		}
	}
}

func TestWithFleetReadsLeavesAContextAsItFindsIt(t *testing.T) {
	h, _ := readFleet(t)

	ctx := withFleetReads(h.ctx)
	if again := withFleetReads(ctx); readsIn(again) != readsIn(ctx) {
		t.Errorf("a context that had reads was given others")
	}
	if ctx := withFleetReads(noFleetReads(h.ctx)); readsIn(ctx) != nil {
		t.Errorf("a context told to carry no reads was given some")
	}
}

// A read that failed says nothing about the next, so the next section asks again
// and one failure costs only the section it hit.
func TestAFailedReadIsAskedForAgain(t *testing.T) {
	h, _ := readFleet(t)
	reads := &fleetReads{}
	failing, cancel := context.WithCancel(context.WithValue(h.ctx, fleetReadsKey{}, reads))
	cancel()

	if _, err := h.c.fleetHosts(failing); err == nil {
		t.Fatal("reading the hosts on a cancelled context did not fail")
	}
	working := context.WithValue(h.ctx, fleetReadsKey{}, reads)
	hosts, err := h.c.fleetHosts(working)
	if err != nil || len(hosts) != 3 {
		t.Fatalf("the hosts, asked for again on a context that works, were %d with error %v; want the 3 the fleet has", len(hosts), err)
	}
	if reads.hosts.reads != 2 {
		t.Errorf("the hosts were read %d times; want twice, the failure and then the read that worked", reads.hosts.reads)
	}
}

// The rows are shared and the slice is not, so a section that puts the hosts in an
// order of its own does not put them there for the next.
func TestTheSliceASectionIsGivenIsItsOwn(t *testing.T) {
	h, _ := readFleet(t)
	ctx := withFleetReads(h.ctx)

	hosts, err := h.c.fleetHosts(ctx)
	if err != nil {
		t.Fatalf("fleetHosts: %v", err)
	}
	pools, err := h.c.fleetPools(ctx)
	if err != nil {
		t.Fatalf("fleetPools: %v", err)
	}
	hostsWere, poolsWere := slices.Clone(hosts), slices.Clone(pools)
	slices.Reverse(hosts)
	hosts[0] = nil
	slices.Reverse(pools)
	pools[0] = nil

	again, err := h.c.fleetHosts(ctx)
	if err != nil {
		t.Fatalf("fleetHosts: %v", err)
	}
	if !slices.Equal(again, hostsWere) {
		t.Errorf("a section that reversed the hosts and cleared one left the next section with a different list")
	}
	again2, err := h.c.fleetPools(ctx)
	if err != nil {
		t.Fatalf("fleetPools: %v", err)
	}
	if !slices.Equal(again2, poolsWere) {
		t.Errorf("a section that reversed the pools and cleared one left the next section with a different list")
	}
}

func TestTheRunnersUpOnAHostAreTheOnesTheStoreGivesForIt(t *testing.T) {
	h, hosts := readFleet(t)
	ctx := withFleetReads(h.ctx)

	up := []store.RunnerState{store.RunnerIdle, store.RunnerBusy}
	for _, host := range append(hosts, &store.Host{ID: "host_that_is_not_there"}) {
		shared, err := h.c.fleetRunnersUpOn(ctx, host.ID)
		if err != nil {
			t.Fatalf("fleetRunnersUpOn: %v", err)
		}
		alone, err := h.st.ListRunnersForHost(h.ctx, host.ID, up...)
		if err != nil {
			t.Fatalf("ListRunnersForHost: %v", err)
		}
		if len(shared) == 0 && len(alone) == 0 {
			continue
		}
		if !reflect.DeepEqual(shared, alone) {
			t.Errorf("host %s: %d runners from the shared read and %d from the store, or in another order", host.ID, len(shared), len(alone))
		}
	}
}
