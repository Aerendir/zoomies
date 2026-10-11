package controller

import (
	"fmt"
	"time"

	"github.com/eyupio/zoomies/internal/scheduler"
	"github.com/eyupio/zoomies/internal/store"
)

// A pool with jobs queued and no machine rented for it looks the same on every
// screen whatever the reason, and the reasons call for opposite things: the
// hosts that could run it are only cordoned, the fleet may rent nothing, or no
// provider builds a machine that suits it. A verdict is the machine planner
// saying which, in one sentence, for each pool the scheduler could not place,
// so an operator is not left to work it out from the provider's settings, the
// pool's and the scheduler's sentence laid side by side.
//
// DecideMachines records each verdict at the branch that decided it rather than
// working it out again afterwards, because a second reading of the snapshot
// would be a second rule, and the one that explains would drift from the one
// that buys.

// RentingState is what the planner decided about renting for one pool.
type RentingState string

const (
	// RentingBuying is a pool this pass rents machines for: all it is short
	// of, or as much as the ceilings allow.
	RentingBuying RentingState = "renting"
	// RentingOnItsWay is a pool the machines already being built cover.
	RentingOnItsWay RentingState = "on_its_way"
	// RentingWaiting is a pool the scheduler could not place for a reason a
	// machine does not answer: a host that could run it is cordoned, silent,
	// incompatible, held or still starting runners. Mending a host is cheaper
	// than renting one, so nothing is rented until every host that could run
	// the pool is full.
	RentingWaiting RentingState = "waiting"
	// RentingCeiling is a pool whose machine a limit stopped.
	RentingCeiling RentingState = "ceiling"
	// RentingUnserved is a pool no provider will rent for: a selector says no,
	// or no provider builds a machine that suits it.
	RentingUnserved RentingState = "unserved"
	// RentingHeld and RentingOff are never decided here. The controller lays
	// them over a verdict, because it knows what the pure planner does not:
	// that a provider may not buy at the moment, or that renting is switched
	// off.
	RentingHeld RentingState = "held"
	RentingOff  RentingState = "off"
)

// PoolRenting is the verdict on one pool: what the planner decided about
// renting for it, why, and what would change that.
type PoolRenting struct {
	PoolID   string       `json:"pool_id"`
	PoolName string       `json:"pool_name"`
	State    RentingState `json:"state"`
	// Summary is the one sentence an operator reads and Fix what to change,
	// both written to be shown as they are. Setting is the one setting the
	// fix changes, when there is one: a provider's own limits are on its own
	// form, not in Settings, so a fix to one of them names none.
	Summary string `json:"summary"`
	Fix     string `json:"fix,omitempty"`
	Setting string `json:"setting,omitempty"`
	// ProviderID and ProviderName are the provider the pool's shortfall was
	// charged to or, for on_its_way, the one building the oldest machine that
	// covers it. A pool that is waiting, or that no provider will rent for,
	// was charged to none.
	ProviderID   string `json:"provider_id,omitempty"`
	ProviderName string `json:"provider_name,omitempty"`
	// Queued is the queued jobs the pool claims, counted as the purchase
	// sentence counts them, so that a job two pools could run is counted for
	// one. Short is the runner slots it is missing: the scheduler's own
	// shortfall for a pool that is waiting, what the free slots on hosts that
	// could run it leave for one with machines on the way, and what those
	// machines leave as well for every other.
	Queued int `json:"queued"`
	Short  int `json:"short"`
	// Machines is, for renting, how many machines the provider rents this
	// pass, and for on_its_way, how many are coming.
	Machines int `json:"machines"`
	// Since is when the oldest machine on its way for this pool was asked for.
	// It is a field rather than a phrase in the summary because a sentence with
	// a clock in it would differ on every pass, and whatever reaches the
	// problems list is sent again each time it differs.
	Since *time.Time `json:"since,omitempty"`
	// Because is, for ceiling only, the ceiling that stopped the purchase
	// without the summary's prefix or full stop, so that a sentence laid over
	// the verdict (renting switched off as well) can say it in its own words.
	// It is not sent, because the summary already says it.
	Because string `json:"-"`
}

// Aside is what a page already showing the scheduler's sentence adds to it. A
// waiting verdict's summary quotes that sentence, so only its tail is news;
// every other verdict is news in full.
func (v PoolRenting) Aside() string {
	if v.State == RentingWaiting {
		return waitingTail
	}
	return v.Summary
}

// The sentences that take no value. The waiting tail is both the end of a
// summary and the whole of an aside, and is written once so that the two
// cannot come to say different things.
const (
	waitingTail    = "A machine is rented only once every host that could run this pool is full."
	waitingFix     = "Wait for those hosts to clear, or uncordon, repair or remove the ones that are unavailable."
	unservedFitFix = "Give a provider machines that suit this pool, or change the pool."
)

// poolReason is what one pool's verdict says it is short of: demandReason for
// a single pool without the pool's name, because the verdict is shown beside
// the pool already.
//
// The queue is mentioned only when something is queued. A pool below its
// min_runners with nothing waiting is renting for the runners it keeps warm,
// and "with 0 jobs queued" would read as a machine rented for no work at all.
func poolReason(d poolDemand) string {
	reason := plural(d.slots, "runner slot") + " short"
	if d.queued > 0 {
		reason += " with " + plural(d.queued, "job") + " queued"
	}
	if d.nowhere {
		return reason + ", and no host in the fleet can run it"
	}
	return reason + ", and every host that can run it is full"
}

// waitingVerdict is a pool the scheduler could not place for a reason a
// machine does not answer. The scheduler's own sentence says what is in the
// way, so the summary quotes it rather than describing the hosts again.
func waitingVerdict(pool *store.Pool, pp scheduler.PoolPlan, queued int) PoolRenting {
	return PoolRenting{
		PoolID: pool.ID, PoolName: pool.Name, State: RentingWaiting,
		Summary: "Not renting yet: " + pp.Blocked + ". " + waitingTail,
		Fix:     waitingFix,
		Queued:  queued, Short: pp.Desired - pp.Current,
	}
}

// onItsWayVerdict is a pool the machines already being built cover. since
// arrives by value, so the verdict points at a copy of its own and never into
// the machine row: the verdict outlives the pass, and the row is the
// controller's.
func onItsWayVerdict(pool *store.Pool, p *store.Provider, coming int, since time.Time, short, queued int) PoolRenting {
	verb := "are"
	if coming == 1 {
		verb = "is"
	}
	return PoolRenting{
		PoolID: pool.ID, PoolName: pool.Name, State: RentingOnItsWay,
		Summary: fmt.Sprintf("%s from %s %s on the way for this pool, so the next runners start there.",
			plural(coming, "machine"), p.Name, verb),
		ProviderID: p.ID, ProviderName: p.Name,
		Queued: queued, Short: short, Machines: coming, Since: &since,
	}
}

// unservedSelectorVerdict is a pool a selector keeps from a provider whose
// machines would suit it. That was somebody's choice, so the refusal is quoted
// with its own fix.
func unservedSelectorVerdict(pool *store.Pool, refused UnservedPool, short, queued int) PoolRenting {
	return PoolRenting{
		PoolID: pool.ID, PoolName: pool.Name, State: RentingUnserved,
		Summary: "No provider will rent for this pool. " + sentence(refused.Why),
		Fix:     sentence(refused.Fix),
		Queued:  queued, Short: short,
	}
}

// unservedFitVerdict is a pool for which no provider builds a machine it could
// use. The first provider by name is the example given: every provider's
// machine has failed a test of shape to get here, so the first one's mismatch
// always has something to say.
func unservedFitVerdict(pool *store.Pool, first *store.Provider, machine *store.Host, short, queued int) PoolRenting {
	return PoolRenting{
		PoolID: pool.ID, PoolName: pool.Name, State: RentingUnserved,
		Summary: "No provider builds a machine this pool could use. " + first.Name + ": " + machineFit(machine, pool),
		Fix:     unservedFitFix,
		Queued:  queued, Short: short,
	}
}

// chargedVerdict is a pool whose shortfall was charged to p, once every
// ceiling has had its say. The pools charged to one provider share its
// purchase: the machines it rents cover them together, and the ceiling that
// stopped one stopped them all, so create, want and the ceiling's words are
// the provider's rather than the pool's.
func chargedVerdict(p *store.Provider, d poolDemand, create, want int, because, fix, setting string) PoolRenting {
	v := PoolRenting{
		PoolID: d.poolID, PoolName: d.pool, ProviderID: p.ID, ProviderName: p.Name,
		Queued: d.queued, Short: d.slots,
	}
	switch {
	case create == 0:
		v.State, v.Because = RentingCeiling, because
		v.Summary = "Not renting: " + because + "."
		v.Fix, v.Setting = sentence(fix), setting
	case create < want:
		v.State, v.Machines = RentingBuying, create
		v.Summary = fmt.Sprintf("Renting %d of the %d machines it needs from %s, which covers this pool: %s. The rest are held back: %s.",
			create, want, p.Name, poolReason(d), because)
		v.Fix, v.Setting = sentence(fix), setting
	default:
		v.State, v.Machines = RentingBuying, create
		v.Summary = fmt.Sprintf("Renting %s from %s, which covers this pool: %s.", plural(create, "machine"), p.Name, poolReason(d))
	}
	return v
}
