package controller

import (
	"reflect"
	"strings"
	"testing"
	"time"

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

// The detail is built from three independent parts (the providers, the pools
// that would rent, the machines still owned), and each has a corner the
// single-case tests above do not reach: the wording for several providers, the
// cap of three pools so that a large fleet's sentence is not buried, and the
// plural of the machines still paid for.
func TestTheRentingOffDetailCountsProvidersPoolsAndMachines(t *testing.T) {
	queueFor := func(h *harness, label string, n int) {
		for i := 0; i < n; i++ {
			h.deliverJob(jobEvent{Action: "queued", JobID: time.Now().UnixNano(),
				Labels: []string{"self-hosted", label}})
		}
	}
	tests := []struct {
		name   string
		setup  func(t *testing.T, h *harness)
		want   []string
		absent []string
	}{
		{
			name: "two providers are counted rather than named",
			setup: func(t *testing.T, h *harness) {
				h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 10 })
				h.providerRow(t, "lab")
				h.providerRow(t, "attic")
			},
			want: []string{"2 providers are configured, but this controller is not allowed to rent machines, "},
		},
		{
			name: "at most three pools are listed, in name order",
			setup: func(t *testing.T, h *harness) {
				h.c.UpdateConfig(func(c *config.Config) { c.Provider.MaxMachines = 10 })
				inst := h.installation()
				h.providerRow(t, "lab")
				for _, name := range []string{"d-pool", "b-pool", "a-pool", "c-pool"} {
					h.pool(inst, name, "self-hosted", name)
					queueFor(h, name, 1)
				}
			},
			want: []string{
				" Pool zoomies-a-pool would rent 1 machine now. Pool zoomies-b-pool would rent 1 machine now. Pool zoomies-c-pool would rent 1 machine now.",
			},
			absent: []string{"d-pool"},
		},
		{
			name: "several machines still owned are counted in the plural",
			setup: func(t *testing.T, h *harness) {
				h.machineFleet(t)
				queueFor(h, "demo", 2)
				h.machinePass(t)
				h.c.UpdateConfig(func(c *config.Config) { c.Provider.Enabled = false })
			},
			want: []string{"2 machines are still rented, and nothing is draining, deleting or checking them while renting is off."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			tt.setup(t, h)
			h.machinePass(t)

			p, ok := h.rentingOffProblem(t)
			if !ok {
				t.Fatal("no provider.renting_off was raised")
			}
			for _, w := range tt.want {
				if !strings.Contains(p.Detail, w) {
					t.Errorf("detail = %q, want it to contain %q", p.Detail, w)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(p.Detail, a) {
					t.Errorf("detail = %q, want it not to mention %q", p.Detail, a)
				}
			}
		})
	}
}
