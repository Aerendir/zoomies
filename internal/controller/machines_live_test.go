package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/store"
)

// heldRentingOff is the sentence a provider gives while the fleet may rent
// nothing. It is written out here, not read from the code, so that a change to
// the wording is a change to this file as well: the Providers page, the pool
// verdicts and the MCP tools all quote it.
const heldRentingOff = "renting is switched off for this controller (provider.enabled), " +
	"so no machine is created from any provider; nothing is drained, deleted or checked either"

// With renting off, a provider that said nothing about it (an empty Held, which
// is how a provider that may buy reads) would send an operator looking through
// the provider's own settings for a cause that is not there. The switch is the
// first thing a card says, and it says nothing once the switch is on.
func TestHeldSaysRentingIsOffWhenItIs(t *testing.T) {
	h := newHarness(t)
	row := h.providerRow(t, "lab")

	if got := h.c.provisioningHeld(row, h.c.Now()); got != heldRentingOff {
		t.Fatalf("with renting off a provider is held for %q, want %q", got, heldRentingOff)
	}

	h.enableProviders(t)
	if got := h.c.provisioningHeld(row, h.c.Now()); got != "" {
		t.Fatalf("with renting on a provider with nothing wrong is held for %q, want nothing", got)
	}
}

// When several reasons are true at once the one given is the operator's first
// suggestion of what to change, so the order is part of the behaviour. Renting
// being off is the master switch: unpausing a provider, enabling its row or
// ending a transfer cannot be the thing to reach for while the whole loop is
// stopped, so it is said before all of them. Only the recovery fence outranks
// it, because a restored database must be told apart from a fleet that is merely
// switched off, and the fence is the one thing that says so.
func TestRentingBeingOffIsSaidBeforeEveryReasonButTheFence(t *testing.T) {
	now := time.Now()
	standingDownUntil := now.Add(10 * time.Minute)
	newRow := func(edit func(*store.Provider)) *store.Provider {
		p := &store.Provider{Name: "lab", Enabled: true}
		if edit != nil {
			edit(p)
		}
		return p
	}

	for _, tc := range []struct {
		name  string
		row   *store.Provider
		setup func(*harness)
		// whenOn is a piece of the reason given once renting is on, so that the
		// same inputs are shown to have a reason of their own that the switch
		// is outranking. Empty means nothing is held.
		whenOn string
	}{
		{name: "no provider row at all", row: nil},
		{name: "a provider with nothing wrong with it", row: newRow(nil)},
		{
			name:   "a provider an operator paused",
			row:    newRow(func(p *store.Provider) { p.Paused, p.PausedReason = true, "over budget" }),
			whenOn: "lab is paused: over budget",
		},
		{
			name:   "a provider whose row is disabled",
			row:    newRow(func(p *store.Provider) { p.Enabled = false }),
			whenOn: "lab is disabled",
		},
		{
			name: "a provider standing down after failures",
			row: newRow(func(p *store.Provider) {
				p.ConsecutiveFailures, p.PausedUntil = 3, &standingDownUntil
			}),
			whenOn: "lab is standing down after 3 failures",
		},
		{
			name:   "an instance draining for a transfer",
			row:    newRow(nil),
			setup:  func(h *harness) { h.c.transferDraining.Store(true) },
			whenOn: "draining for transfer",
		},
		{
			name:   "provider.paused in the configuration",
			row:    newRow(nil),
			setup:  func(h *harness) { h.c.UpdateConfig(func(c *config.Config) { c.Provider.Paused = true }) },
			whenOn: "provider.paused is set",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			if tc.setup != nil {
				tc.setup(h)
			}
			if got := h.c.provisioningHeld(tc.row, now); got != heldRentingOff {
				t.Errorf("with renting off the reason is %q, want %q", got, heldRentingOff)
			}

			h.enableProviders(t)
			got := h.c.provisioningHeld(tc.row, now)
			switch {
			case tc.whenOn == "" && got != "":
				t.Errorf("with renting on nothing should be held, got %q", got)
			case !strings.Contains(got, tc.whenOn):
				t.Errorf("with renting on the reason is %q, want one containing %q", got, tc.whenOn)
			}
		})
	}

	t.Run("a fenced fleet says it is fenced", func(t *testing.T) {
		h := newHarness(t)
		h.fence("restored from a backup")

		fenced := h.c.mutationsHeld()
		if !strings.Contains(fenced, "fenced for recovery") {
			t.Fatalf("the fence's own sentence is %q, which does not say so", fenced)
		}
		for _, row := range []*store.Provider{nil, newRow(nil)} {
			if got := h.c.provisioningHeld(row, now); got != fenced {
				t.Errorf("with renting off and the fleet fenced the reason is %q, want the fence's %q", got, fenced)
			}
		}
	})
}

// The point of starting the machine loop whatever the setting says: an operator
// who has just been told renting is switched off, and who switches it on, should
// have a machine on its way without anybody restarting the controller. Before,
// the loop was only started when the setting was already on at startup, so the
// switch was accepted, stored, and then waited for a restart nobody knew to do.
func TestRentingSwitchedOnWhileTheControllerRunsStartsRenting(t *testing.T) {
	h := newHarness(t)
	h.cfg.Scheduler.Interval = time.Hour
	inst := h.installation()
	h.pool(inst, "linux-x64")
	row := h.providerRow(t, "lab")
	h.queueWork(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := h.c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { h.c.Stop(context.Background()) })

	// Only a precondition: the loop's first timed pass is fifteen seconds away,
	// so this cannot fail. That a running loop rents nothing while renting is
	// off is TestNothingIsRentedUntilProvidersAreSwitchedOn's to show.
	if got := len(h.machines()); got != 0 {
		t.Fatalf("renting is off and %d machines were planned", got)
	}

	h.c.UpdateConfig(func(c *config.Config) {
		c.Provider.Enabled = true
		c.Provider.MaxMachines = 10
	})

	eventually(t, 5*time.Second, "a machine is planned", func() bool {
		return len(h.machines()) > 0
	})
	if m := h.machines()[0]; m.ProviderID != row.ID {
		t.Fatalf("the machine was planned for provider %s, want %s", m.ProviderID, row.ID)
	}
}
