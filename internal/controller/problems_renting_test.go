package controller

import (
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/config"
)

const wantRentingOffTitle = "renting is switched off, so no provider can add a machine"

// rentingOffProblem is the one provider.renting_off in the list, failing the
// test when there is not exactly one or none is wanted.
func (h *harness) rentingOffProblem(t *testing.T) (Problem, bool) {
	t.Helper()
	got := h.problemsOf(t, "provider.renting_off")
	if len(got) > 1 {
		t.Fatalf("%d provider.renting_off problems, want at most one: %+v", len(got), got)
	}
	if len(got) == 0 {
		return Problem{}, false
	}
	return got[0], true
}

// Renting is off by default, and a deployment that has just added a provider
// is in exactly that state, so the first thing it needs to be told is why
// nothing will be rented. It is only a warning once something is waiting on
// it: a fleet with no work is not harmed by the switch.
func TestAConfiguredProviderWithRentingOffRaisesAnInfoUntilWorkIsWaiting(t *testing.T) {
	h := newHarness(t)
	h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 10 })
	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	row := h.providerRow(t, "lab")
	h.machinePass(t)

	p, ok := h.rentingOffProblem(t)
	if !ok {
		t.Fatal("a configured provider with renting off raised no provider.renting_off")
	}
	if p.Severity != config.SeverityInfo {
		t.Errorf("severity = %q with nothing queued, want info", p.Severity)
	}
	if p.Title != wantRentingOffTitle {
		t.Errorf("title = %q, want %q", p.Title, wantRentingOffTitle)
	}
	if want := "lab is configured, but this controller is not allowed to rent machines, so a pool that runs out of room waits for the hosts it has."; p.Detail != want {
		t.Errorf("detail = %q, want %q", p.Detail, want)
	}
	if p.Fix != wantFleetOffFix {
		t.Errorf("fix = %q, want %q", p.Fix, wantFleetOffFix)
	}
	if p.Setting != "provider.enabled" || p.TargetKind != "provider" || p.TargetID != row.ID {
		t.Errorf("setting, target = %q, %q %q; want provider.enabled, provider %s", p.Setting, p.TargetKind, p.TargetID, row.ID)
	}
	if p.Audience != AudienceFleet {
		t.Errorf("audience = %q, want fleet", p.Audience)
	}

	h.queueWork(t)
	h.machinePass(t)
	p, ok = h.rentingOffProblem(t)
	if !ok || p.Severity != config.SeverityWarning {
		t.Fatalf("with a pool waiting the problem is %+v (raised %v), want a warning", p, ok)
	}
	if want := " Pool " + pool.Name + " would rent 1 machine now."; !strings.Contains(p.Detail, want) {
		t.Errorf("detail = %q, want it to contain %q", p.Detail, want)
	}
}

// Machines still rented are still paid for, and with renting off nothing
// drains, deletes or checks them, which is the cost an operator who switched
// renting off in a hurry has not been told about.
func TestRentingOffWithMachinesStillOwnedIsAWarning(t *testing.T) {
	h := newHarness(t)
	_, row := h.machineFleet(t)
	h.readyMachine(t, row)

	h.c.UpdateConfig(func(c *config.Config) { c.Provider.Enabled = false })
	h.machinePass(t)

	p, ok := h.rentingOffProblem(t)
	if !ok {
		t.Fatal("renting is off with a machine rented and no provider.renting_off was raised")
	}
	if p.Severity != config.SeverityWarning {
		t.Errorf("severity = %q, want warning", p.Severity)
	}
	if want := "1 machine is still rented, and nothing is draining, deleting or checking it while renting is off."; !strings.Contains(p.Detail, want) {
		t.Errorf("detail = %q, want it to contain %q", p.Detail, want)
	}
}

// A provider whose own row is disabled would rent nothing with renting on, so
// telling the operator to switch renting on would send them to the wrong place.
func TestADisabledProviderRowRaisesNoRentingProblem(t *testing.T) {
	h := newHarness(t)
	h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 10 })
	inst := h.installation()
	h.pool(inst, "linux-x64")
	row := h.providerRow(t, "lab")
	row.Enabled = false
	if err := h.st.UpdateProvider(h.ctx, row); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	h.queueWork(t)

	if p, ok := h.rentingOffProblem(t); ok {
		t.Fatalf("before a pass: %+v", p)
	}
	h.machinePass(t)
	if p, ok := h.rentingOffProblem(t); ok {
		t.Fatalf("after a pass: %+v", p)
	}
}

// The demo instance has no hypervisor, so inviting it to switch renting on
// would invite it to rent from a cluster that does not exist.
func TestTheDemoFixtureRaisesNoRentingProblem(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	h.pool(inst, "linux-x64")
	h.demoProviderFixture(t)
	h.queueWork(t)
	h.machinePass(t)

	if p, ok := h.rentingOffProblem(t); ok {
		t.Fatalf("the demo fixture raised %+v", p)
	}
}

func TestNoRentingProblemWhileRentingIsOn(t *testing.T) {
	h := newHarness(t)
	h.machineFleet(t)
	h.machinePass(t)

	if p, ok := h.rentingOffProblem(t); ok {
		t.Fatalf("renting is on and %+v was raised", p)
	}
}

// Whatever reaches the problems list is sent again each time it differs, so
// two passes that decided the same thing must say it in the same bytes.
func TestTheRentingOffProblemIsTheSameOnEveryPass(t *testing.T) {
	h := newHarness(t)
	h.rentingOffFleet(t)
	first, _ := h.rentingOffProblem(t)
	h.machinePass(t)
	second, ok := h.rentingOffProblem(t)
	if !ok || !reflect.DeepEqual(first, second) {
		t.Fatalf("the problem changed between identical passes:\n%+v\n%+v", first, second)
	}
}
