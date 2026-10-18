package controller

import (
	"reflect"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// The webhook and the poller read the pools to claim a job, and then name the
// pool in the frame they publish for it. Reading that pool again, whole, to
// learn a name the pools in hand already hold was a fifth of a poller sweep.

func TestAJobIsViewedWithItsPoolsNameFromThePoolsAlreadyRead(t *testing.T) {
	h := newHarness(t)
	p := h.pool(h.installation(), "gpu", "self-hosted", "gpu")
	pools, err := h.st.ListPools(h.ctx)
	if err != nil {
		t.Fatalf("ListPools: %v", err)
	}
	// With its row gone, a name can only have come from the pools in hand.
	if _, _, err := h.st.DeletePool(h.ctx, p.ID); err != nil {
		t.Fatalf("DeletePool: %v", err)
	}

	job := &store.Job{ID: "job_1", PoolID: p.ID, Matched: true}
	if got := h.c.jobViewIn(h.ctx, job, pools).PoolName; got != p.Name {
		t.Errorf("PoolName = %q, want %q from the pools in hand", got, p.Name)
	}
}

func TestAJobWhosePoolIsNotAmongThePoolsInHandIsStillNamed(t *testing.T) {
	h := newHarness(t)
	p := h.pool(h.installation(), "gpu", "self-hosted", "gpu")
	job := &store.Job{ID: "job_1", PoolID: p.ID, Matched: true}

	for name, pools := range map[string][]*store.Pool{
		"no pools read":             nil,
		"only another pool is read": {{ID: "pool_other", Name: "other"}},
	} {
		if got := h.c.jobViewIn(h.ctx, job, pools).PoolName; got != p.Name {
			t.Errorf("%s: PoolName = %q, want %q read from the store", name, got, p.Name)
		}
	}
}

// The frame is the same one: only where the name comes from differs.
func TestAJobViewFromThePoolsInHandIsTheViewAPlainReadGives(t *testing.T) {
	h := newHarness(t)
	p := h.pool(h.installation(), "gpu", "self-hosted", "gpu")
	pools, err := h.st.ListPools(h.ctx)
	if err != nil {
		t.Fatalf("ListPools: %v", err)
	}

	for name, job := range map[string]*store.Job{
		"a claimed job":    {ID: "job_1", PoolID: p.ID, Matched: true, Labels: []string{"self-hosted", "gpu"}},
		"an unclaimed one": {ID: "job_2", Labels: []string{"self-hosted", "tpu"}},
	} {
		if got, want := h.c.jobViewIn(h.ctx, job, pools), h.c.jobView(h.ctx, job); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the view from the pools in hand is\n%+v\nwant what a plain read gives\n%+v", name, got, want)
		}
	}
}
