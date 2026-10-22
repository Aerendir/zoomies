package controller

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/store"
)

// testFleet is machineFleet without the queued job: a test machine is asked for
// by a person, so any machine that appears must be the one they asked for.
func (h *harness) testFleet(t *testing.T) *store.Provider {
	t.Helper()
	h.enableProviders(t)
	h.pool(h.installation(), "linux-x64")
	return h.providerRow(t, "lab")
}

// The point of the test is that it goes through every step a real machine does,
// so it is driven the way the other machine tests are: built, bootstrapped,
// joined by the agent inside the guest, and then released without anyone asking.
func TestATestMachineIsBuiltEnrolledAndThenReleased(t *testing.T) {
	h := newHarness(t)
	row := h.testFleet(t)

	m, err := h.c.StartTestMachine(h.ctx, row)
	if err != nil {
		t.Fatalf("StartTestMachine: %v", err)
	}
	if !m.Test {
		t.Fatal("the machine reserved for a test is not marked as one")
	}

	m = h.drive(t, m.ID, store.MachineEnrolling, 8)
	h.joinAsMachine(t, m)
	h.machinePass(t)
	m = h.drive(t, m.ID, store.MachineDeleted, 12)
	if m.ReadyAt == nil {
		t.Fatal("the test machine was released without ever reporting ready, so the test proved nothing")
	}
	if m.DeletedAt == nil {
		t.Fatal("the test machine was not confirmed deleted; a trial must not be left costing money")
	}
	h.assertOneResourcePerMachine(t)
}

// Two at once would race for the same ceiling and leave the operator unsure
// which machine an answer was about.
func TestASecondTestMachineWaitsForTheFirst(t *testing.T) {
	h := newHarness(t)
	row := h.testFleet(t)
	if _, err := h.c.StartTestMachine(h.ctx, row); err != nil {
		t.Fatalf("StartTestMachine: %v", err)
	}
	_, err := h.c.StartTestMachine(h.ctx, row)
	if !errors.Is(err, ErrTestMachineRefused) {
		t.Fatalf("a second test machine was %v, want it refused", err)
	}
	if !strings.Contains(err.Error(), "already on its way") {
		t.Fatalf("the refusal does not say why: %v", err)
	}
}

// A provider that may own no machines has no room for a test, and the sentence
// says which setting to raise rather than leaving a button that does nothing.
func TestATestMachineNeedsRoomUnderTheProvidersMaximum(t *testing.T) {
	h := newHarness(t)
	row := h.testFleet(t)
	row.MaxMachines = 0
	if err := h.st.UpdateProvider(h.ctx, row); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	_, err := h.c.StartTestMachine(h.ctx, row)
	if !errors.Is(err, ErrTestMachineRefused) || !strings.Contains(err.Error(), "maximum machines") {
		t.Fatalf("a provider with no room started a test machine: %v", err)
	}
	if got := len(h.machines()); got != 0 {
		t.Fatalf("a refused test left %d machines behind", got)
	}
}

// A paused provider builds nothing, and a test is a build.
func TestATestMachineIsRefusedWhileTheProviderIsPaused(t *testing.T) {
	h := newHarness(t)
	row := h.testFleet(t)
	h.pauseProvider(t, row, "the hypervisor is being patched")
	got, err := h.st.GetProvider(h.ctx, row.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if _, err := h.c.StartTestMachine(h.ctx, got); !errors.Is(err, ErrTestMachineRefused) {
		t.Fatalf("a paused provider started a test machine: %v", err)
	}
}

// A test machine is not supply. If it were counted towards a provider's
// minimum, the warm floor would be met by a machine released a minute later and
// the first real job would wait for a clone after all.
func TestATestMachineDoesNotCountTowardsTheWarmMinimum(t *testing.T) {
	h := newHarness(t)
	row := h.testFleet(t)
	row.MinMachines = 1
	if err := h.st.UpdateProvider(h.ctx, row); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if _, err := h.c.StartTestMachine(h.ctx, row); err != nil {
		t.Fatalf("StartTestMachine: %v", err)
	}
	h.machinePass(t)
	var real int
	for _, m := range h.machines() {
		if !m.Test {
			real++
		}
	}
	if real != 1 {
		t.Fatalf("with a minimum of 1 and a test machine on its way the provider bought %d real machines, want 1", real)
	}
}

// provider.max_machines is 0 until somebody sets it, and 0 means this fleet
// rents nothing: the planner buys no machine under it and the validator says
// so. A test machine is a real billed machine, so it has no more room under a
// maximum of none than any other, and the sentence names the setting to raise
// rather than leaving a button that quietly spends.
func TestATestMachineNeedsRoomUnderTheFleetsMaximumToo(t *testing.T) {
	h := newHarness(t)
	row := h.testFleet(t)
	h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 0 })

	_, err := h.c.StartTestMachine(h.ctx, row)
	if !errors.Is(err, ErrTestMachineRefused) || !strings.Contains(err.Error(), "provider.max_machines") {
		t.Fatalf("a fleet that may rent nothing started a test machine: %v", err)
	}
	if got := len(h.machines()); got != 0 {
		t.Fatalf("a refused test left %d machines behind", got)
	}
}

// The same ceiling once it is not none: a fleet that already owns as many
// machines as it may is refused, whichever provider they belong to, and is
// served again when the maximum is raised, as the sentence says to do.
func TestATestMachineIsRefusedWhenTheFleetIsAtItsMaximum(t *testing.T) {
	h := newHarness(t)
	row := h.testFleet(t)
	other := h.providerRow(t, "lab-2")
	h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 1 })

	first, err := h.c.StartTestMachine(h.ctx, row)
	if err != nil {
		t.Fatalf("StartTestMachine: %v", err)
	}
	h.machinePass(t)
	if !h.machineByID(t, first.ID).Owns() {
		t.Fatal("the first test machine does not own a resource yet, so the fleet is not at its maximum")
	}

	_, err = h.c.StartTestMachine(h.ctx, other)
	if !errors.Is(err, ErrTestMachineRefused) || !strings.Contains(err.Error(), "provider.max_machines") {
		t.Fatalf("a fleet at its maximum started a second provider's test machine: %v", err)
	}

	h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 2 })
	if _, err := h.c.StartTestMachine(h.ctx, other); err != nil {
		t.Fatalf("with room under the maximum the test machine was refused: %v", err)
	}
}

// Two operators, or one double click, asking for a test at the same moment.
// The checks read what exists and the reservation writes after, so without
// something holding them together every caller sees no test machine on the way
// and each reserves one: real machines, billed, on a provider that was to have
// one. Released together so that they overlap, and repeated because an overlap
// is a matter of luck where a lock is not.
func TestConcurrentTestMachineRequestsReserveOnlyOne(t *testing.T) {
	h := newHarness(t)
	h.testFleet(t)

	const callers = 16
	for round := range 5 {
		row := h.providerRow(t, fmt.Sprintf("lab-round-%d", round))
		var (
			wg      sync.WaitGroup
			gate    = make(chan struct{})
			started atomic.Int32
		)
		for range callers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-gate
				_, err := h.c.StartTestMachine(h.ctx, row)
				switch {
				case err == nil:
					started.Add(1)
				case !errors.Is(err, ErrTestMachineRefused):
					t.Errorf("a test machine request failed instead of being refused: %v", err)
				}
			}()
		}
		close(gate)
		wg.Wait()

		tests := 0
		for _, m := range h.machines() {
			if m.Test && m.ProviderID == row.ID {
				tests++
			}
		}
		if started.Load() != 1 || tests != 1 {
			t.Fatalf("round %d: %d of %d simultaneous requests were accepted and %d test machines were reserved, want 1 of each",
				round, started.Load(), callers, tests)
		}
	}
}
