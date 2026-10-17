package controller

import (
	"errors"
	"strings"
	"testing"

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
