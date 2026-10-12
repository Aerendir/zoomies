package controller

import (
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// failedMachine writes a machine the fleet gave up on. With a resource it is a
// VM that may still be running and billed; without one it is a row that never
// got as far as the hypervisor.
func (h *harness) failedMachine(t *testing.T, withResource bool, fields store.Machine) *store.Machine {
	t.Helper()
	h.enableProviders(t)
	row := h.providerRow(t, "lab")
	fields.ProviderID = row.ID
	fields.State = store.MachineFailed
	fields.Labels = store.StringMap{}
	if withResource {
		fields.ResourceZone, fields.ResourceID = "zone-a", "143"
	}
	if err := h.st.CreateMachine(h.ctx, &fields); err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return &fields
}

// fixFor is the advice a problem gives for one machine.
func (h *harness) fixFor(t *testing.T, code, machineID string) string {
	t.Helper()
	for _, p := range h.problemsOf(t, code) {
		if p.TargetID == machineID {
			return p.Fix
		}
	}
	t.Fatalf("no %s problem for machine %s; the list has %v", code, machineID, h.problemCodes())
	return ""
}

// The advice used to be to release a failed row once the console had been
// checked. Release forgets a row and touches nothing, which is the escape hatch
// for a machine nobody can safely delete, and a failed machine that never got a
// resource is the opposite case: there is nothing to check and nothing to leave
// running, and the thing to do with the row is to delete it. Sending people to
// release it meant a pile of them, each opened and released by hand.
func TestAFailedMachineWithNothingBehindItIsToldToDeleteItNotToReleaseIt(t *testing.T) {
	h := newHarness(t)
	m := h.failedMachine(t, false, store.Machine{ProviderError: "no space on pve-1"})

	fix := h.fixFor(t, "provider.machine_failed", m.ID)
	for _, want := range []string{"nothing was ever created", "Delete it", "together"} {
		if !strings.Contains(fix, want) {
			t.Errorf("the advice for a failed machine with no resource does not say %q: %s", want, fix)
		}
	}
	for _, unwanted := range []string{"Release", "may still exist", "console"} {
		if strings.Contains(fix, unwanted) {
			t.Errorf("the advice for a failed machine with no resource still mentions %q: %s", unwanted, fix)
		}
	}
}

// A failed machine that does have a resource is the one that costs money, so
// the console is worth a look. What to do afterwards is still to delete the
// machine, which asks the provider again before it destroys anything. Releasing
// it would forget a VM that goes on running and billing with nothing tracking it.
func TestAFailedMachineWithAResourceIsToldToCheckTheConsoleAndThenDeleteIt(t *testing.T) {
	h := newHarness(t)
	m := h.failedMachine(t, true, store.Machine{ProviderError: "the clone timed out"})

	fix := h.fixFor(t, "provider.machine_failed", m.ID)
	for _, want := range []string{"may still be running", "console", "delete the machine", "asks the provider again"} {
		if !strings.Contains(fix, want) {
			t.Errorf("the advice for a failed machine with a resource does not say %q: %s", want, fix)
		}
	}
	if strings.Contains(fix, "Release") || strings.Contains(fix, "release") {
		t.Errorf("the advice for a failed machine with a resource still tells the operator to release it: %s", fix)
	}
}

// The agent never coming up leaves a running VM, and the advice was all about
// the template. Once that is fixed the failed machine still has to be deleted,
// or it goes on being paid for.
func TestAMachineWhoseAgentNeverCameUpIsToldToDeleteItOnceTheTemplateIsFixed(t *testing.T) {
	h := newHarness(t)
	m := h.failedMachine(t, true, store.Machine{BootstrapError: "the agent did not answer in time"})

	fix := h.fixFor(t, "provider.bootstrap_failed", m.ID)
	if !strings.Contains(fix, "agent.json") {
		t.Fatalf("the advice about the template has gone: %s", fix)
	}
	for _, want := range []string{"delete this machine", "stop paying"} {
		if !strings.Contains(fix, want) {
			t.Errorf("the advice does not say %q: %s", want, fix)
		}
	}
}
