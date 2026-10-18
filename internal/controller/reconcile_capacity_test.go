package controller

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// Every reconcile pass records, for each pool, whether the pool was held at its
// limit, and it used to do it with a write of its own per pool. The number of
// writes in a pass that changed nothing therefore grew with the size of the
// fleet, and in a profile of a quiet pass at a hundred pools they were a
// quarter of it. These hold the pass to recording the same observations, and to
// recording them without a write for each pool.

// sampledCapacity is what the capacity history holds for a pool: how many
// minutes were observed, and in how many of them the pool was held at its limit.
func sampledCapacity(t *testing.T, h *harness, pool *store.Pool, around time.Time) (samples, reached int) {
	t.Helper()
	rows, err := h.st.Usage(h.ctx, around.Add(-2*time.Hour), around.Add(time.Hour), store.UsageByPool)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	for _, r := range rows {
		if r.Key != pool.ID {
			continue
		}
		for _, b := range r.History {
			samples += b.CapacitySamples
			reached += b.CapacityReached
		}
	}
	return samples, reached
}

func TestAReconcilePassRecordsWhetherEachPoolWasHeldAtItsLimit(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	h.host("vm-1") // room for four runners

	wide := h.pool(inst, "wide", "self-hosted", "linux", "x64", "wide")
	wide.MaxRunners = 10
	if err := h.st.UpdatePool(h.ctx, wide); err != nil {
		t.Fatalf("UpdatePool: %v", err)
	}
	idle := h.pool(inst, "idle", "self-hosted", "linux", "x64", "idle")

	// Six jobs for a pool that may have ten runners, on a host with room for
	// four: two of them have nowhere to go, which is what "held at its limit"
	// means. The other pool has no work and is not.
	for i := 1; i <= 6; i++ {
		rec := h.deliverJob(jobEvent{Action: "queued", JobID: int64(i), RunID: int64(i), Name: fmt.Sprintf("build-%d", i),
			Labels: []string{"self-hosted", "linux", "x64", "wide"}})
		if rec.Code != 200 && rec.Code != 202 {
			t.Fatalf("delivering job %d: %d %s", i, rec.Code, rec.Body)
		}
	}

	if err := h.c.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	now := h.c.Now()
	if samples, reached := sampledCapacity(t, h, wide, now); samples != 1 || reached != 1 {
		t.Errorf("the pool that jobs could not be placed for has %d samples of which %d held at its limit, want 1 and 1", samples, reached)
	}
	if samples, reached := sampledCapacity(t, h, idle, now); samples != 1 || reached != 0 {
		t.Errorf("the pool with no work has %d samples of which %d held at its limit, want 1 and 0", samples, reached)
	}
}

// writesInAQuietPass is how many times the pass takes the writer when it has
// nothing to change, for a fleet of the given number of pools. The first pass is
// let go by: it is the one that settles whatever a fresh fleet settles.
func writesInAQuietPass(t *testing.T, pools int) int32 {
	t.Helper()
	h := newHarness(t)
	inst := h.installation()
	h.host("vm-1")
	for i := range pools {
		name := fmt.Sprintf("pool-%02d", i)
		h.pool(inst, name, "self-hosted", "linux", "x64", name)
	}
	if err := h.c.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile (settling): %v", err)
	}

	var writes atomic.Int32
	h.st.ObserveWrites(func(_, _ time.Duration) { writes.Add(1) })
	defer h.st.ObserveWrites(nil)
	if err := h.c.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile (quiet): %v", err)
	}
	return writes.Load()
}

func TestAQuietReconcilePassTakesTheWriterNoMoreOftenForABiggerFleet(t *testing.T) {
	small, large := writesInAQuietPass(t, 2), writesInAQuietPass(t, 12)
	if small == 0 {
		t.Fatal("a quiet pass took the writer 0 times; the pass records capacity, so the count means nothing")
	}
	if large != small {
		t.Errorf("a quiet pass over 12 pools took the writer %d times and over 2 pools %d: the capacity of every pool is recorded in one write, whatever the number of pools", large, small)
	}
}
