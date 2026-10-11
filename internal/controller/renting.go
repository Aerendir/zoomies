package controller

import (
	"context"
	"fmt"
	"slices"
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
	// the pool is full. A pool no provider could ever serve is unserved
	// instead, because waiting would promise it a machine that never comes.
	RentingWaiting RentingState = "waiting"
	// RentingCeiling is a pool whose machine a limit stopped.
	RentingCeiling RentingState = "ceiling"
	// RentingUnserved is a pool no provider will rent for: a selector says no,
	// or no provider builds a machine that suits it.
	RentingUnserved RentingState = "unserved"
	// RentingHeld and RentingOff are never decided by the planner. The
	// controller lays them over a verdict (overlayRenting), because it knows
	// what the pure planner does not: that a provider may not buy at the
	// moment, or that renting is switched off.
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
	// one. Short is the runner slots it is missing. For a pool the scheduler
	// is waiting on hosts for, that is the scheduler's own shortfall, whether
	// the verdict is waiting or, because no provider could serve it,
	// unserved. Otherwise it is what the free slots on hosts that could run
	// it leave and, for every state but on_its_way, what the machines on
	// their way leave as well.
	Queued int `json:"queued"`
	Short  int `json:"short"`
	// Machines is, for renting, the machines this pool alone needs, at most
	// what the provider rents this pass. Pools that share a machine each
	// count it, so the figures are never to be added up. For on_its_way it is
	// how many of the named provider's machines are coming. For held and off
	// it is what the pass would have rented had nothing stood in the way,
	// which is none for a pool a ceiling stopped or one the machines already
	// on their way cover.
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

// unservedVerdict is the verdict on a pool no provider will rent for, with the
// selector's refusal when that is the reason, for the caller that also lists
// it. Both places a pool can turn out unserved ask this one function, so the
// two cannot come to say different things.
//
// A selector that says no was somebody's choice, so it is quoted with its own
// fix. Otherwise no provider builds a machine the pool could use, and the first
// provider by name is the example given: every provider's machine has failed a
// test of shape to get here, so the first one's mismatch always has something
// to say. There is always a first, because DecideMachines decides nothing for
// a fleet with no provider.
func unservedVerdict(providers []*store.Provider, synth map[string]*store.Host, pool *store.Pool, now time.Time, short, queued int) (PoolRenting, *UnservedPool) {
	v := PoolRenting{PoolID: pool.ID, PoolName: pool.Name, State: RentingUnserved, Queued: queued, Short: short}
	if refused, ok := unservedBy(providers, synth, pool, now); ok {
		v.Summary = "No provider will rent for this pool. " + sentence(refused.Why)
		v.Fix = sentence(refused.Fix)
		return v, &refused
	}
	first := providers[0]
	v.Summary = "No provider builds a machine this pool could use. " + first.Name + ": " + machineFit(synth[first.ID], pool)
	v.Fix = unservedFitFix
	return v, nil
}

// chargedVerdict is a pool whose shortfall was charged to p, once every
// ceiling has had its say. create, want (what the pools charged to p need
// between them) and the ceiling's words are the provider's, decided once for
// all of those pools, and shared says there is more than one. Machines is the
// pool's own share whatever the sentence: the machines it alone needs, and as
// many of those as create covers.
//
// When the whole purchase is made, each pool quotes that share, because the
// provider's whole order would tell a pool one slot short that it needs two
// machines. When it is held back, no one of several pools can be said to be
// covered: the machines bought serve whichever pool the scheduler places on
// them first. So each is told about the pools together, and only a provider
// charged with a single pool says what "it" needs.
func chargedVerdict(p *store.Provider, d poolDemand, create, want int, shared bool, because, fix, setting string) PoolRenting {
	need := ceilDiv(d.slots, providerSlots(p))
	v := PoolRenting{
		PoolID: d.poolID, PoolName: d.pool, State: RentingBuying,
		ProviderID: p.ID, ProviderName: p.Name,
		Queued: d.queued, Short: d.slots, Machines: min(create, need),
	}
	switch {
	case create == 0:
		v.State, v.Because = RentingCeiling, because
		v.Summary = "Not renting: " + because + "."
		v.Fix, v.Setting = sentence(fix), setting
	case create >= want:
		v.Summary = fmt.Sprintf("Renting %s from %s, which covers this pool: %s.", plural(need, "machine"), p.Name, poolReason(d))
	case shared:
		v.Summary = fmt.Sprintf("Renting %d of the %d machines this pool and others need from %s: %s. The rest are held back: %s.",
			create, want, p.Name, poolReason(d), because)
		v.Fix, v.Setting = sentence(fix), setting
	default:
		v.Summary = fmt.Sprintf("Renting %d of the %d machines it needs from %s, which covers this pool: %s. The rest are held back: %s.",
			create, want, p.Name, poolReason(d), because)
		v.Fix, v.Setting = sentence(fix), setting
	}
	return v
}

// ---------------------------------------------------------------------------
// The fleet's view
// ---------------------------------------------------------------------------

// The sentences the fleet says about itself, and the one a machine on its way
// says once renting is switched off. The other sentences laid over a verdict
// take a value, and are written where they are laid over it.
const (
	fleetNone         = "No provider is configured, so nothing is rented."
	fleetOff          = "Renting is switched off: this controller is not allowed to rent machines, so work with nowhere to run waits for the hosts it has."
	fleetOffFix       = "Turn on Rent machines in Settings, and set Machines the fleet may rent to the most you are willing to pay for."
	fleetNoCeiling    = "Renting is on, but the fleet may rent no machines: a maximum of none is none."
	fleetNoCeilingFix = "Set Machines the fleet may rent to the most machines you are willing to pay for at once."
	fleetOn           = "Renting is on."
	offOnItsWay       = "A machine for this pool was being built, but renting is switched off, so nothing is finishing it."
)

// RentingView is the whole of what the fleet says about renting machines: which
// switch, if any, is holding it back, and the verdict on each pool the last
// machine pass had something to say about, with the switches laid over it.
//
// The switches are read with the view and the verdicts are as old as the last
// pass. An operator who has just switched renting on reads the page to see
// that it took, so the view cannot wait for the loop to wake before it says
// so; and the verdicts are never worked out again for a read, because the
// decision that explains has to be the one that buys.
type RentingView struct {
	// State is "none" when no provider may rent (a demo fixture, or a provider
	// whose own row is disabled, is not one that may), "off" when renting is
	// switched off, "no_ceiling" when it is on and the fleet may rent no
	// machines, and "on" otherwise.
	State string `json:"state"`
	// Summary is the state in one sentence, Fix what to change, and Setting
	// the one setting the fix changes.
	Summary string `json:"summary"`
	Fix     string `json:"fix,omitempty"`
	Setting string `json:"setting,omitempty"`
	// Enabled, Paused and MaxMachines are provider.enabled, provider.paused
	// and provider.max_machines as they stand.
	Enabled     bool `json:"enabled"`
	Paused      bool `json:"paused"`
	MaxMachines int  `json:"max_machines"`
	// SuggestedMaxMachines is the fleet ceiling a page offers to set: the
	// ceilings of the providers that may rent, added up, and never less than
	// one, so that the click that switches renting on cannot leave a fleet
	// that may rent nothing.
	SuggestedMaxMachines int `json:"suggested_max_machines,omitempty"`
	// Owned is how many machines still own a resource, and so are still being
	// paid for, as the last pass counted them: zero before the first.
	Owned int `json:"owned"`
	// DecidedAt is when the pass the verdicts come from ran, and absent before
	// the first one.
	DecidedAt *time.Time `json:"decided_at,omitempty"`
	// Pools is never nil, so that a page is sent an empty list rather than
	// null before the first pass.
	Pools []PoolRenting `json:"pools"`
}

// rentingReport is what one machine pass decided about renting, kept for the
// reads until the next pass. A pass replaces it whole and nothing changes it
// after, so a reader that has taken it from under the lock may keep it.
type rentingReport struct {
	// At is when the pass ran.
	At time.Time
	// Pools is the planner's verdicts, before anything is laid over them,
	// with the demo fixture's pools left out.
	Pools []PoolRenting
	// Providers is every provider the pass read but the demo fixture, and
	// Usable those of them whose own row is enabled: the ones that may rent.
	// A disabled row stays in Providers because the planner may still charge
	// a pool to it, and the view then has to say why that buys nothing.
	Providers []*store.Provider
	Usable    []*store.Provider
	// Owned is how many machines of those providers own a resource.
	Owned int
}

// rentingInput is everything buildRentingView reads, gathered by the caller so
// that the view is a function of it alone.
type rentingInput struct {
	// Report is the last pass's, or nil before the first one.
	Report *rentingReport
	// Usable is the providers that may rent: the report's, or the database's
	// before there is a report.
	Usable []*store.Provider
	// Enabled, Paused and MaxMachines are the switches as they stand.
	Enabled     bool
	Paused      bool
	MaxMachines int
	// Held is what provisioningHeld says of each provider the pass read, by
	// id, a row-disabled one included. It is gathered only while renting is
	// on: while it is off every provider would answer with the switch, and the
	// overlay says that in words of its own.
	Held map[string]string
}

// buildRentingView says which switch is holding renting back, and lays the
// switches over the last pass's verdicts.
func buildRentingView(in rentingInput) RentingView {
	out := RentingView{
		Enabled: in.Enabled, Paused: in.Paused, MaxMachines: in.MaxMachines,
		Pools: []PoolRenting{},
	}
	for _, p := range in.Usable {
		out.SuggestedMaxMachines += max(p.MaxMachines, 0)
	}
	out.SuggestedMaxMachines = max(out.SuggestedMaxMachines, 1)
	switch {
	case len(in.Usable) == 0:
		out.State, out.Summary = "none", fleetNone
	case !in.Enabled:
		out.State, out.Summary, out.Fix, out.Setting = "off", fleetOff, fleetOffFix, "provider.enabled"
	case in.MaxMachines <= 0:
		out.State, out.Summary, out.Fix, out.Setting = "no_ceiling", fleetNoCeiling, fleetNoCeilingFix, "provider.max_machines"
	default:
		out.State, out.Summary = "on", fleetOn
	}
	if in.Report == nil {
		return out
	}
	out.Owned = in.Report.Owned
	at := in.Report.At
	out.DecidedAt = &at
	// With no provider that may rent, nothing is rented whatever a pool needs,
	// and the fleet's own sentence is the whole answer. A verdict the planner
	// reached against a disabled row would send an operator to the renting
	// switch, or promise a machine, for a provider that rents nothing either way.
	if len(in.Usable) == 0 {
		return out
	}
	for _, v := range in.Report.Pools {
		out.Pools = append(out.Pools, overlayRenting(v, in.Enabled, in.Held))
	}
	return out
}

// overlayRenting lays over one of the planner's verdicts what the pure planner
// cannot know: that renting is switched off, or that the provider charged with
// the pool may not buy at the moment.
//
// Switched off, a purchase says how many machines it would have been, a
// ceiling that would also have stopped it is still named, and a machine on its
// way is stranded, because nothing finishes a machine while renting is off.
// Each names the one setting that changes it. A pool waiting on its hosts, or
// one no provider serves, is in the same place whichever way the switch is, and
// is told what it would have been told anyway.
//
// Switched on, a purchase the provider may not make is held rather than
// renting, and the fix a partial purchase named goes with it: raising a ceiling
// does nothing for a provider that is paused.
func overlayRenting(v PoolRenting, enabled bool, held map[string]string) PoolRenting {
	if enabled {
		if why := held[v.ProviderID]; why != "" && v.State == RentingBuying {
			v.State, v.Summary, v.Fix, v.Setting = RentingHeld, "Not renting: "+why+".", "", ""
		}
		return v
	}
	switch v.State {
	case RentingBuying:
		v.Summary = fmt.Sprintf("Would rent %s for this pool, but renting is switched off.", plural(v.Machines, "machine"))
	case RentingCeiling:
		v.Summary = "Renting is switched off, and " + v.Because + "."
	case RentingOnItsWay:
		// The machines were counted as coming, and while renting is off
		// nothing brings them. Nor are they machines the pass would rent, so
		// the pool would rent none.
		v.Summary, v.Machines = offOnItsWay, 0
	default:
		return v
	}
	v.State, v.Fix, v.Setting = RentingOff, fleetOffFix, "provider.enabled"
	return v
}

// Renting is the fleet's renting view as it stands: the switches now, laid over
// the last machine pass's verdicts. It reads the database only when there is
// no report to read, for the providers that say whether there is anything to
// rent from at all.
func (c *Controller) Renting(ctx context.Context) (RentingView, error) {
	in := c.rentingNow()
	if in.Report == nil {
		usable, err := c.usableProviders(ctx)
		if err != nil {
			return RentingView{}, err
		}
		in.Usable = usable
	}
	return buildRentingView(in), nil
}

// rentingFor is one pool's verdict with the switches laid over it, or nil when
// the last pass had nothing to say about the pool. It is the view's own entry
// for the pool, so a page explaining one pool and the page listing them all
// cannot disagree, and it reads no database, so a page explaining a pool can
// ask it for every pool it shows.
func (c *Controller) rentingFor(poolID string) *PoolRenting {
	in := c.rentingNow()
	if in.Report == nil {
		return nil
	}
	for _, v := range buildRentingView(in).Pools {
		if v.PoolID == poolID {
			return &v
		}
	}
	return nil
}

// rentingNow gathers what the view is built from: the last pass's report, and
// the switches as they stand with what holds each provider back at this moment.
func (c *Controller) rentingNow() rentingInput {
	c.mu.Lock()
	report := c.renting
	c.mu.Unlock()
	cfg := c.cfg()
	in := rentingInput{
		Report:      report,
		Enabled:     cfg.Provider.Enabled,
		Paused:      cfg.Provider.Paused,
		MaxMachines: cfg.Provider.MaxMachines,
	}
	if report == nil {
		return in
	}
	in.Usable = report.Usable
	if in.Enabled {
		now := c.Now()
		in.Held = make(map[string]string, len(report.Providers))
		for _, p := range report.Providers {
			if why := c.provisioningHeld(p, now); why != "" {
				in.Held[p.ID] = why
			}
		}
	}
	return in
}

// usableProviders is the providers that may rent, in name order. It is the
// last pass's list when there is a report, so that the fleet's state and the
// verdicts it carries agree about which providers there are, and the
// database's when there is not. The list is the caller's own to keep.
func (c *Controller) usableProviders(ctx context.Context) ([]*store.Provider, error) {
	c.mu.Lock()
	report := c.renting
	c.mu.Unlock()
	if report != nil {
		return slices.Clone(report.Usable), nil
	}
	rows, err := c.st.ListProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing providers: %w", err)
	}
	return usableRows(rows), nil
}

// usableRows is the providers among rows that may rent: neither a demo fixture
// nor disabled on its own row.
//
// The row is asked, never provisioningHeld (Review Focus 1). With renting off,
// that answers with the switch for a disabled row as for every other, and a
// fleet whose only provider is disabled would read as one waiting for renting
// to be switched on, when switching it on would rent nothing.
func usableRows(rows []*store.Provider) []*store.Provider {
	var out []*store.Provider
	for _, p := range rows {
		if p != nil && p.Enabled && !IsDemoID(p.ID) {
			out = append(out, p)
		}
	}
	return out
}

// noteRenting keeps what this pass decided about renting for the reads until
// the next, and sends again each provider whose held sentence the pass finds
// changed. It is called with machines.mu held.
//
// The report is replaced under c.mu rather than machines.mu, which a pass holds
// for as long as it takes: a page asking why nothing is rented must not wait
// on a hypervisor.
//
// Held is computed rather than stored, so no row write announces that it has
// moved: a provider's page open when renting was switched on would otherwise go
// on saying it was off until somebody reloaded it. A provider with no sentence
// from an earlier pass has nothing to differ from, and is only remembered.
func (c *Controller) noteRenting(ctx context.Context, env *machineEnv, plan MachinePlan) {
	report := &rentingReport{At: env.now}
	for _, p := range env.rows {
		if !IsDemoID(p.ID) {
			report.Providers = append(report.Providers, p)
		}
	}
	report.Usable = usableRows(report.Providers)
	for _, v := range plan.Pools {
		if !IsDemoID(v.PoolID) {
			report.Pools = append(report.Pools, v)
		}
	}
	for _, m := range env.list {
		if m.Owns() {
			report.Owned++
		}
	}
	c.mu.Lock()
	c.renting = report
	c.mu.Unlock()

	held := make(map[string]string, len(report.Providers))
	for _, p := range report.Providers {
		held[p.ID] = c.provisioningHeld(p, env.now)
		if was, ok := c.machines.lastHeld[p.ID]; ok && was != held[p.ID] {
			c.publishProvider(ctx, p)
		}
	}
	c.machines.lastHeld = held
}

// forgetRenting drops the report once the fleet has no provider other than the
// demo fixture, so that the view reads the providers for itself rather than
// describing ones that have gone. It is called with machines.mu held.
func (c *Controller) forgetRenting() {
	c.mu.Lock()
	c.renting = nil
	c.mu.Unlock()
	c.machines.lastHeld = nil
}
