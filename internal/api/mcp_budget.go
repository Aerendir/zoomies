package api

import (
	"sync"
	"time"
)

// inProcessBudget bounds how many route answers one caller may be holding
// at once. Every tool call buffers a whole answer (mcpResponseLimit is 32
// MiB), and the agent routes answer a caller that tries this with 429
// before reading a byte; the MCP and assistant paths funnelled through
// inProcessAPI were the two entry points with no such ceiling, and they run
// on the process that owns the SQLite writer. A keyed semaphore rather than
// auth.RateLimiter, which bounds how often a key may call, not how many
// calls it may hold.
//
// The map needs no sweeping: every take is paired with a give, a key's count
// returns to zero, and the entry goes with it.
type inProcessBudget struct {
	mu        sync.Mutex
	held      map[string]int
	totalHeld int
}

const (
	// inProcessPerCaller is the ceiling per identity. Two concurrent large
	// log reads (one live, one download) are a plausible legitimate pair;
	// a third held answer from the same caller is a loop, not a reader.
	inProcessPerCaller = 2
	// inProcessTotal is the fleet-wide ceiling, so many callers together
	// still pin a bounded slice of this process's memory.
	inProcessTotal = 16
)

// inProcessRetryAfter is how long a refused caller is told to wait. The
// refusal is about calls in flight rather than about a window of calls, so
// the wait is short: an answer that is being read is usually nearly ready,
// and a caller that waits longer than that has a problem the wait will not
// fix.
const inProcessRetryAfter = 2 * time.Second

// take spends one slot for key and reports how long to wait when the
// ceiling refuses. key is the caller's identity string; the empty string is
// every call without an identity, which cannot happen on a real path but
// must not bypass the budget.
func (b *inProcessBudget) take(key string) (ok bool, retryAfter time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.held == nil {
		b.held = map[string]int{}
	}
	if b.held[key] >= inProcessPerCaller || b.totalHeld >= inProcessTotal {
		return false, inProcessRetryAfter
	}
	b.held[key]++
	b.totalHeld++
	return true, 0
}

// give returns one slot for key. It is safe to call without a matching take
// (a give for a key holding nothing is a no-op), because release runs on
// every exit path of do, including the ones a panic-free rewrite of do
// might add later.
func (b *inProcessBudget) give(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.held == nil {
		return
	}
	n := b.held[key]
	if n == 0 {
		return
	}
	if n == 1 {
		delete(b.held, key)
	} else {
		b.held[key] = n - 1
	}
	b.totalHeld--
}
