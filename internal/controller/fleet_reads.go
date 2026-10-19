package controller

import (
	"context"
	"slices"
	"sync"

	"github.com/eyupio/zoomies/internal/store"
)

// The figures that follow a pass are worked out one after another, and each
// starts by reading the fleet. The problems list alone reads the pools in thirteen
// of its sections and the hosts in eleven, each section on its own, and the section
// that wants a host's runners asks for them one host at a time. With a page open
// that was 65% of a pass at a hundred pools, nearly all of it the same rows read
// again.
//
// fleetReads is what one set of figures reads of the fleet, read once. It is
// carried in the context the set is worked out in, so that a section needs no
// change of signature to share it, and a section run on its own, from the API or
// a test, finds none and reads from the store as it always did. It lasts as long
// as the set and no longer: nothing is kept from one pass to the next, so a row
// is never older than the figures it is read for.
//
// The rows are shared, and so a section that reads them leaves them as it found
// them. The slice a section is given is its own.
type fleetReads struct {
	mu      sync.Mutex
	pools   fleetRead[[]*store.Pool]
	hosts   fleetRead[[]*store.Host]
	runners fleetRead[map[string][]*store.Runner]
}

// fleetRead is one read, made when it is first asked for.
type fleetRead[T any] struct {
	done  bool
	val   T
	reads int
}

// get returns the read, making it if it has not been made. A read that failed
// is not kept: the next section asks again, as it would have if nothing were
// shared, so one failure costs the section it hit and not every section after.
func (r *fleetRead[T]) get(read func() (T, error)) (T, error) {
	if r.done {
		return r.val, nil
	}
	r.reads++
	v, err := read()
	if err != nil {
		var none T
		return none, err
	}
	r.val, r.done = v, true
	return v, nil
}

type fleetReadsKey struct{}

// withFleetReads is ctx with a set of reads for the figures worked out in it to
// share, unless it carries one already, or has been told to carry none.
func withFleetReads(ctx context.Context) context.Context {
	if ctx.Value(fleetReadsKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, fleetReadsKey{}, &fleetReads{})
}

// readsIn is the set of reads ctx carries, or nil if it carries none.
func readsIn(ctx context.Context) *fleetReads {
	r, _ := ctx.Value(fleetReadsKey{}).(*fleetReads)
	return r
}

// fleetPools is ListPools through the reads ctx carries, if it carries any.
func (c *Controller) fleetPools(ctx context.Context) ([]*store.Pool, error) {
	r := readsIn(ctx)
	if r == nil {
		return c.st.ListPools(ctx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	pools, err := r.pools.get(func() ([]*store.Pool, error) { return c.st.ListPools(ctx) })
	return slices.Clone(pools), err
}

// fleetHosts is ListHosts through the reads ctx carries, if it carries any.
func (c *Controller) fleetHosts(ctx context.Context) ([]*store.Host, error) {
	r := readsIn(ctx)
	if r == nil {
		return c.st.ListHosts(ctx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	hosts, err := r.hosts.get(func() ([]*store.Host, error) { return c.st.ListHosts(ctx) })
	return slices.Clone(hosts), err
}

// fleetRunnersUpOn is the runners on a host that are idle or busy, which is
// ListRunnersForHost with those two states, through the reads ctx carries, if it
// carries any. The runners of every host are read in one query, the first time any
// host's are asked for.
func (c *Controller) fleetRunnersUpOn(ctx context.Context, hostID string) ([]*store.Runner, error) {
	upStates := []store.RunnerState{store.RunnerIdle, store.RunnerBusy}
	r := readsIn(ctx)
	if r == nil {
		return c.st.ListRunnersForHost(ctx, hostID, upStates...)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	byHost, err := r.runners.get(func() (map[string][]*store.Runner, error) { return c.st.ListRunnersByHost(ctx, upStates...) })
	return slices.Clone(byHost[hostID]), err
}
