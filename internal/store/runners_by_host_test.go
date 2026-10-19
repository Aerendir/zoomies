package store

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

// The problems list asks for the runners on a host once for each host. Asking for
// every host's in one query has to give each host what asking for it alone does,
// in the same order, for any set of states.
func TestRunnersListedByHostAreWhatListingEachHostsGives(t *testing.T) {
	ctx := context.Background()
	var tick atomic.Int64
	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	// Each runner is created a moment before the one before, so that created_at,
	// which each list is in the order of, never ties and is not the order the rows
	// were written in.
	s := newTestStoreAt(t, func() time.Time { return base.Add(-time.Duration(tick.Add(1)) * time.Second) })
	_, pool, _ := seedPool(t, s)

	var hosts []*Host
	for i := range 3 {
		h := &Host{Name: fmt.Sprintf("node-%d", i), Capacity: 8, Backends: StringSlice{"docker"}}
		if err := s.CreateHost(ctx, h); err != nil {
			t.Fatalf("CreateHost: %v", err)
		}
		hosts = append(hosts, h)
	}
	empty := &Host{Name: "node-empty", Capacity: 1, Backends: StringSlice{"docker"}}
	if err := s.CreateHost(ctx, empty); err != nil {
		t.Fatalf("CreateHost: %v", err)
	}

	states := []RunnerState{RunnerProvisioning, RunnerRegistering, RunnerIdle, RunnerBusy, RunnerDraining, RunnerFailed, RunnerRemoved}
	for n := range 3 * len(states) {
		// The runners are dealt out over the hosts in turn, and the states over the
		// runners in turn, so that each host has some of every state.
		r := &Runner{PoolID: pool.ID, HostID: hosts[n%len(hosts)].ID, Name: fmt.Sprintf("run-%02d", n), State: states[n%len(states)]}
		if err := s.CreateRunner(ctx, r); err != nil {
			t.Fatalf("CreateRunner: %v", err)
		}
	}

	for _, ask := range [][]RunnerState{nil, {RunnerIdle, RunnerBusy}, {RunnerFailed}, {RunnerRemoved}, {RunnerIdle, RunnerBusy, RunnerDraining}} {
		grouped, err := s.ListRunnersByHost(ctx, ask...)
		if err != nil {
			t.Fatalf("ListRunnersByHost(%v): %v", ask, err)
		}
		listed := 0
		for _, h := range append(hosts, empty) {
			alone, err := s.ListRunnersForHost(ctx, h.ID, ask...)
			if err != nil {
				t.Fatalf("ListRunnersForHost(%s, %v): %v", h.Name, ask, err)
			}
			listed += len(alone)
			if len(alone) == 0 && len(grouped[h.ID]) == 0 {
				continue
			}
			if !reflect.DeepEqual(grouped[h.ID], alone) {
				t.Errorf("states %v: host %s has %d runners listed with the others and %d alone, or in another order", ask, h.Name, len(grouped[h.ID]), len(alone))
			}
		}
		total := 0
		for _, rs := range grouped {
			total += len(rs)
		}
		if total != listed {
			t.Errorf("states %v: %d runners were listed by host and %d host by host", ask, total, listed)
		}
		if listed == 0 {
			t.Errorf("states %v: no runner matched, so the comparison said nothing", ask)
		}
	}
}
