package controller

import (
	"slices"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/events"
	"github.com/eyupio/zoomies/internal/store"
)

// rentingOffFleet is a fleet that would rent a machine the moment renting were
// switched on: a fleet ceiling, a provider that can serve the pool, one queued
// job and no host, with provider.enabled as it ships. One pass has run.
func (h *harness) rentingOffFleet(t *testing.T) (*store.Pool, *store.Provider) {
	t.Helper()
	h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 10 })
	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	row := h.providerRow(t, "lab")
	h.queueWork(t)
	h.machinePass(t)
	return pool, row
}

// renting reads the fleet's renting view, failing the test on an error.
func (h *harness) renting(t *testing.T) RentingView {
	t.Helper()
	view, err := h.c.Renting(h.ctx)
	if err != nil {
		t.Fatalf("Renting: %v", err)
	}
	return view
}

// A pool with work waiting and nothing rented for it looks the same whether
// renting is switched off or nothing is needed, and only the planner's verdict
// tells the two apart, so a pass decides whether or not it may act. It acts on
// nothing: no machine is planned, and the provider is asked for nothing at all,
// not even a listing.
func TestAPassWhileRentingIsOffDecidesAndStoresButBuysNothing(t *testing.T) {
	h := newHarness(t)
	pool, row := h.rentingOffFleet(t)

	if got := len(h.machines()); got != 0 {
		t.Fatalf("renting is off and %d machines were planned", got)
	}
	if got := h.callsTo("allocate"); got != 0 {
		t.Fatalf("renting is off and the provider was asked to allocate %d times", got)
	}
	if calls := h.fake.Calls(); len(calls) != 0 {
		t.Fatalf("renting is off and the provider was asked for %v", calls)
	}

	view := h.renting(t)
	if view.State != "off" || view.Summary != wantFleetOff {
		t.Fatalf("state = %q, summary = %q; want off, %q", view.State, view.Summary, wantFleetOff)
	}
	if view.DecidedAt == nil {
		t.Fatal("the view has no decision time, so the pass's verdict was not kept")
	}
	want := PoolRenting{
		PoolID: pool.ID, PoolName: pool.Name, State: RentingOff,
		Summary: "Would rent 1 machine for this pool, but renting is switched off.",
		Fix:     wantFleetOffFix, Setting: "provider.enabled",
		ProviderID: row.ID, ProviderName: row.Name, Queued: 1, Short: 1, Machines: 1,
	}
	if len(view.Pools) != 1 || view.Pools[0] != want {
		t.Fatalf("pools:\n got %+v\nwant [%+v]", view.Pools, want)
	}
}

// A pass while renting is off still decides, so it sends what is computed from
// the fleet rather than stored, the problems list among it, as any other pass
// does. Left to a pass that may act, all of that would wait for renting to be
// switched on.
func TestAPassWhileRentingIsOffSendsWhatItWorkedOut(t *testing.T) {
	h := newHarness(t)
	h.providerRow(t, "lab")
	sub := h.listen(events.KindProblems)

	h.machinePass(t)
	nextOfKind(t, sub, events.KindProblems)
}

// The verdict is the last pass's and the switches are read with the view. An
// operator who has just switched renting on reads the page to see that it
// took, often before the loop has woken; told renting was still off, they would
// switch it again, or go looking for a restart that is no longer needed.
func TestTheVerdictFollowsTheSwitchWithoutWaitingForAPass(t *testing.T) {
	h := newHarness(t)
	pool, row := h.rentingOffFleet(t)
	decided := h.renting(t).DecidedAt
	if decided == nil {
		t.Fatal("precondition: the pass while renting was off kept no verdict")
	}

	h.c.UpdateConfig(func(c *config.Config) { c.Provider.Enabled = true })
	view := h.renting(t)
	if view.State != "on" || view.Summary != wantFleetOn {
		t.Fatalf("with renting switched on the state is %q (%q), want on", view.State, view.Summary)
	}
	want := PoolRenting{
		PoolID: pool.ID, PoolName: pool.Name, State: RentingBuying,
		Summary:    "Renting 1 machine from lab, which covers this pool: 1 runner slot short with 1 job queued, and no host in the fleet can run it.",
		ProviderID: row.ID, ProviderName: row.Name, Queued: 1, Short: 1, Machines: 1,
	}
	if len(view.Pools) != 1 || view.Pools[0] != want {
		t.Fatalf("pools:\n got %+v\nwant [%+v]", view.Pools, want)
	}
	if view.DecidedAt == nil || !view.DecidedAt.Equal(*decided) {
		t.Fatalf("decided at %v, want %v: no pass has run since, so the verdict is the same one", view.DecidedAt, *decided)
	}

	// A page explaining one pool lays the same switches over the same verdict,
	// and a pool the pass said nothing about has nothing to explain.
	if got := h.c.rentingFor(pool.ID); got == nil || *got != want {
		t.Fatalf("rentingFor = %+v, want %+v", got, want)
	}
	if got := h.c.rentingFor("pool_nosuchpool"); got != nil {
		t.Fatalf("rentingFor an unknown pool = %+v, want nothing", got)
	}

	// And back, still without a pass.
	h.c.UpdateConfig(func(c *config.Config) { c.Provider.Enabled = false })
	view = h.renting(t)
	if view.State != "off" || len(view.Pools) != 1 || view.Pools[0].State != RentingOff {
		t.Fatalf("with renting switched off again the view is %+v, want off with the pool's verdict off", view)
	}
	if got := len(h.machines()); got != 0 {
		t.Fatalf("reading the view planned %d machines", got)
	}
}

// The demo fixture's provider has no hypervisor behind it and its pools exist
// for screenshots, so neither is counted. A demo instance told that renting is
// switched off would be invited to switch it on and rent from a cluster that
// does not exist, and a real fleet would be told about pools it never made.
func TestTheDemoFixtureIsNotInTheRentingView(t *testing.T) {
	t.Run("the demo provider alone", func(t *testing.T) {
		h := newHarness(t)
		inst := h.installation()
		h.pool(inst, "linux-x64")
		h.demoProviderFixture(t)
		h.queueWork(t)

		// Before the first pass the view reads the providers for itself.
		if view := h.renting(t); view.State != "none" {
			t.Fatalf("before the first pass the state is %q (%q), want none", view.State, view.Summary)
		}
		h.machinePass(t)
		h.enableProviders(t)
		h.machinePass(t)
		view := h.renting(t)
		if view.State != "none" || view.Summary != wantFleetNone {
			t.Fatalf("after a pass the state is %q (%q), want none", view.State, view.Summary)
		}
		if len(view.Pools) != 0 {
			t.Fatalf("pools = %+v, want none: there is nothing to rent from", view.Pools)
		}
	})

	t.Run("a demo pool beside a real one", func(t *testing.T) {
		h := newHarness(t)
		h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 10 })
		inst := h.installation()
		real := h.pool(inst, "linux-x64")
		demo := &store.Pool{
			ID: "pool_demolab", Name: "demo-lab", InstallationID: inst.ID,
			Labels:  []string{"self-hosted", "linux", "x64", "fixture"},
			Backend: store.BackendDocker, Image: "ghcr.io/eyupio/zoomies-runner:test",
			MaxRunners: 4, IdleTimeout: store.Duration(5 * time.Minute),
			Ephemeral: true, DockerMode: store.DockerNone, Enabled: true,
		}
		if err := h.st.CreatePool(h.ctx, demo); err != nil {
			t.Fatalf("CreatePool: %v", err)
		}
		h.providerRow(t, "lab")
		h.queueWork(t)
		h.deliverJob(jobEvent{Action: "queued", JobID: 7001, Labels: demo.Labels})

		// A precondition, so that the test cannot pass by the planner having
		// nothing to say about the fixture: it decides for every pool it is
		// handed, and leaving the fixture out is the report's to do.
		rows, err := h.st.ListProviders(h.ctx)
		if err != nil {
			t.Fatalf("ListProviders: %v", err)
		}
		env, err := h.c.machineSnapshot(h.ctx, rows)
		if err != nil {
			t.Fatalf("machineSnapshot: %v", err)
		}
		if !slices.ContainsFunc(DecideMachines(env.demand()).Pools, func(v PoolRenting) bool { return v.PoolID == demo.ID }) {
			t.Fatal("precondition: the planner gave the demo pool no verdict, so there is nothing for the view to leave out")
		}

		h.machinePass(t)
		var pools []string
		for _, v := range h.renting(t).Pools {
			pools = append(pools, v.PoolID)
		}
		if want := []string{real.ID}; !slices.Equal(pools, want) {
			t.Fatalf("the view has verdicts for %v, want only the real pool's %v", pools, want)
		}
		if got := h.c.rentingFor(demo.ID); got != nil {
			t.Fatalf("rentingFor the demo pool = %+v, want nothing", got)
		}
	})
}

// Held is computed rather than stored, so no row write announces that it has
// moved: a provider's page open when renting is switched on would go on saying
// it was off until somebody reloaded. A pass that finds a provider's sentence
// changed sends the provider again, whichever way the switch went, and a pass
// that finds it as it was says nothing.
func TestAChangeOfWhatIsHeldRepublishesTheProviders(t *testing.T) {
	h := newHarness(t)
	h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 10 })
	inst := h.installation()
	h.pool(inst, "linux-x64")
	row := h.providerRow(t, "lab")
	sub := h.listen(events.KindProviderUpdated)

	// The first pass has no earlier sentence to differ from, so it only
	// remembers this one.
	h.machinePass(t)
	nothingFor(t, sub)

	h.c.UpdateConfig(func(c *config.Config) { c.Provider.Enabled = true })
	h.machinePass(t)
	frame := nextOfKind(t, sub, events.KindProviderUpdated)
	if frame["id"] != row.ID {
		t.Fatalf("the frame is about %v, want provider %s", frame["id"], row.ID)
	}
	if held, _ := frame["held"].(string); held != "" {
		t.Fatalf("with renting switched on the provider is still held for %q, want nothing", held)
	}

	h.machinePass(t)
	nothingFor(t, sub)

	h.c.UpdateConfig(func(c *config.Config) { c.Provider.Enabled = false })
	h.machinePass(t)
	frame = nextOfKind(t, sub, events.KindProviderUpdated)
	if frame["id"] != row.ID {
		t.Fatalf("the frame is about %v, want provider %s", frame["id"], row.ID)
	}
	if held, _ := frame["held"].(string); held != heldRentingOff {
		t.Fatalf("with renting switched off again the provider is held for %q, want %q", held, heldRentingOff)
	}
}

// The planner charges a pool to whichever provider serves it first, whatever
// that provider's own row says, and the pass then buys nothing, because a
// disabled row is offered no new demand. Told it was renting, an operator would
// wait for a machine that is never coming; told it is held, they are told which
// switch it is.
func TestTheOverlayHoldsAPoolChargedToADisabledProvider(t *testing.T) {
	h := newHarness(t)
	h.enableProviders(t)
	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	alpha := h.providerRow(t, "alpha")
	alpha.Enabled = false
	if err := h.st.UpdateProvider(h.ctx, alpha); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	// beta may rent, so the fleet has something to rent from, but for no pool
	// like this one: alpha is the only provider the pool could be charged to.
	beta := h.providerRow(t, "beta")
	beta.PoolSelector = store.StringMap{"tier": "large"}
	if err := h.st.UpdateProvider(h.ctx, beta); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	h.queueWork(t)
	h.machinePass(t)

	if got := len(h.machines()); got != 0 {
		t.Fatalf("a disabled provider was charged with the pool and %d machines were planned", got)
	}
	view := h.renting(t)
	if view.State != "on" {
		t.Fatalf("state = %q (%q), want on: beta may rent", view.State, view.Summary)
	}
	want := PoolRenting{
		PoolID: pool.ID, PoolName: pool.Name, State: RentingHeld,
		Summary:    "Not renting: alpha is disabled, so it is offered no new demand.",
		ProviderID: alpha.ID, ProviderName: alpha.Name, Queued: 1, Short: 1, Machines: 1,
	}
	if len(view.Pools) != 1 || view.Pools[0] != want {
		t.Fatalf("pools:\n got %+v\nwant [%+v]", view.Pools, want)
	}
	if got := h.c.rentingFor(pool.ID); got == nil || *got != want {
		t.Fatalf("rentingFor = %+v, want %+v", got, want)
	}
}

// Review Focus 1, through a real pass. A provider whose own row is disabled
// rents nothing whatever the switches say, so a fleet with nothing else has
// nothing to rent from. With renting off its held sentence is the switch's,
// like every provider's, and a view that read held rather than the row would
// tell an operator to switch renting on for a provider that would still rent
// nothing; the verdicts the planner reached against that row would say the
// same, so there are none either.
func TestTheRentingViewOfAFleetWhoseOnlyProviderIsDisabledIsNone(t *testing.T) {
	h := newHarness(t)
	h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 10 })
	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	row := h.providerRow(t, "lab")
	row.Enabled = false
	if err := h.st.UpdateProvider(h.ctx, row); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	h.queueWork(t)

	before := h.renting(t)
	h.machinePass(t)
	for _, tc := range []struct {
		when string
		view RentingView
	}{
		{"before the first pass", before},
		{"after a pass", h.renting(t)},
	} {
		if tc.view.State != "none" || tc.view.Summary != wantFleetNone {
			t.Errorf("%s the state is %q (%q), want none", tc.when, tc.view.State, tc.view.Summary)
		}
		if len(tc.view.Pools) != 0 {
			t.Errorf("%s the pools are %+v, want none", tc.when, tc.view.Pools)
		}
	}
	if got := h.c.rentingFor(pool.ID); got != nil {
		t.Errorf("rentingFor = %+v, want nothing", got)
	}
	usable, err := h.c.usableProviders(h.ctx)
	if err != nil {
		t.Fatalf("usableProviders: %v", err)
	}
	if len(usable) != 0 {
		t.Errorf("usable providers = %v, want none", usable)
	}
}
