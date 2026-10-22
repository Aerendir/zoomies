package controller

import (
	"context"
	"errors"
	"runtime/debug"
	"slices"
	"sync"
	"time"
)

// problemsFlight lets callers that ask for the problems list at the same moment
// share one computation of it.
//
// The list is a few dozen queries, and every browser that opens the Overview
// asks for it, so eight people arriving together used to be eight computations
// and a queue of several seconds each. There is deliberately no time-to-live: a
// caller that arrives after a computation has finished starts a new one, so the
// answer is never older than the moment it was asked for, and a test that
// changes something and asks again sees the change. The one exception is a single
// section, the slower-job check, which keeps what it found for a minute (see
// jobtrends.go).
type problemsFlight struct {
	mu     sync.Mutex
	active *problemsCall
}

type problemsCall struct {
	done  chan struct{}
	items []Problem
	err   error
}

// problemsTimeout bounds a shared computation. It is not any one caller's
// request, so it is not cut short when the caller that happened to start it
// gives up.
const problemsTimeout = 30 * time.Second

// errProblemsPanicked is what the requests waiting on a computation that
// panicked are told. The panic and its stack are in the log.
var errProblemsPanicked = errors.New("the problems list could not be worked out because the controller hit an internal error; " +
	"its log has the stack under \"" + detachedPanicMessage + "\"")

// SharedProblems is Problems for callers that may overlap: the API handler and
// the event stream. Callers that arrive while a computation is running wait for
// it rather than starting another, and each gets its own copy of the list.
func (c *Controller) SharedProblems(ctx context.Context) ([]Problem, error) {
	f := &c.problemsFlight
	f.mu.Lock()
	call := f.active
	if call == nil {
		call = &problemsCall{done: make(chan struct{})}
		f.active = call
		go func() {
			// However the computation ends, the requests waiting on it are
			// released and the next one starts afresh. A call left registered
			// would make every request after it wait for a result that is
			// never coming.
			defer func() {
				f.mu.Lock()
				f.active = nil
				f.mu.Unlock()
				close(call.done)
			}()
			// Recovered here and not by recoverDetached, because the requests
			// waiting are owed an error and not an empty list.
			defer func() {
				if r := recover(); r != nil {
					c.reportDetachedPanic("problems-list", r, debug.Stack())
					call.items, call.err = nil, errProblemsPanicked
				}
			}()
			// Detached from the caller's cancellation but not from its values.
			run, cancel := context.WithTimeout(context.WithoutCancel(ctx), problemsTimeout)
			defer cancel()
			call.items, call.err = c.Problems(run)
		}()
	}
	f.mu.Unlock()

	select {
	case <-call.done:
		return slices.Clone(call.items), call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
