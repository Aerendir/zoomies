package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/backend"
	"github.com/eyupio/zoomies/internal/store"
)

// pruningBackend is a fakeBackend that keeps native logs, so it is the kind of
// backend the log loop must call: it records every limit it was handed and
// signals a channel on each call, which is how a test observes the loop's
// immediate first pass without waiting for the minute tick.
type pruningBackend struct {
	*fakeBackend

	mu       sync.Mutex
	limits   []int64
	pruneErr error
	called   chan struct{}
}

func newPruningBackend(kind store.BackendKind, pruneErr error) *pruningBackend {
	return &pruningBackend{
		fakeBackend: newFakeBackend(kind),
		pruneErr:    pruneErr,
		called:      make(chan struct{}, 8),
	}
}

func (p *pruningBackend) PruneLogs(_ context.Context, limit int64) error {
	// The limit is recorded under the fake's own lock discipline, because the
	// loop runs on its own goroutine and the assertions read it from the test's.
	p.mu.Lock()
	p.limits = append(p.limits, limit)
	p.mu.Unlock()
	select {
	case p.called <- struct{}{}:
	default:
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pruneErr
}

// pruneLimits returns every limit the backend was handed so far.
func (p *pruningBackend) pruneLimits() []int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int64(nil), p.limits...)
}

// The log loop is the only thing standing between a chatty runner and a full
// host disk: nothing else in the fleet notices it breaking. Its contract is
// to bound every backend that keeps logs, at the 10 MiB limit, without
// disturbing the backends that do not.
func TestTheNativeLogLoopBoundsEveryBackendThatKeepsLogs(t *testing.T) {
	pruner := newPruningBackend(store.BackendProcess, nil)
	plain := newFakeBackend(store.BackendDocker)
	a, _, _, _ := newAgent(t, 1, func(o *Options) {
		o.Backends = backend.NewRegistry(pruner, plain)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.nativeLogLoop(ctx) }()

	// The first pass runs before the loop's first select, so the call arrives
	// without any ticking; waiting on the channel is enough.
	select {
	case <-pruner.called:
	case <-time.After(10 * time.Second):
		t.Fatal("the log loop never pruned within 10s")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the log loop returned %v on cancel, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the log loop did not return within 10s of cancel")
	}

	limits := pruner.pruneLimits()
	if len(limits) == 0 {
		t.Fatal("the maintainer was never called")
	}
	for _, l := range limits {
		if l != 10<<20 {
			t.Fatalf("the loop pruned with limit %d, want 10 MiB", l)
		}
	}
}

// A backend whose prune fails must not stop the walk: the other backends
// still get their turn, and the loop itself keeps running and still returns
// nil on cancel. One broken backend cannot be allowed to unbound every other
// runner's logs.
func TestTheNativeLogLoopSurvivesABackendThatFailsToPrune(t *testing.T) {
	failing := newPruningBackend(store.BackendProcess, errors.New("the disk is full"))
	healthy := newPruningBackend(store.BackendPodman, nil)
	plain := newFakeBackend(store.BackendDocker)
	a, _, _, _ := newAgent(t, 1, func(o *Options) {
		o.Backends = backend.NewRegistry(failing, healthy, plain)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.nativeLogLoop(ctx) }()

	// Kinds() comes from a map, so the walk order is not fixed; both
	// maintainers must be called in the same first pass whichever way round
	// the failing one lands.
	for name, b := range map[string]*pruningBackend{"failing": failing, "healthy": healthy} {
		select {
		case <-b.called:
		case <-time.After(10 * time.Second):
			t.Fatalf("the log loop never reached the %s backend within 10s", name)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the log loop returned %v on cancel, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the log loop did not return within 10s of cancel")
	}
}
