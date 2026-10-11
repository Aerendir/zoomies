package controller

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// The fleet's sentences, written out here rather than read from the code so
// that a change to the wording is a change to this file as well: the Providers
// page, the renting problem and the MCP tools all quote them.
const (
	wantFleetNone         = "No provider is configured, so nothing is rented."
	wantFleetOff          = "Renting is switched off: this controller is not allowed to rent machines, so work with nowhere to run waits for the hosts it has."
	wantFleetOffFix       = "Turn on Rent machines in Settings, and set Machines the fleet may rent to the most you are willing to pay for."
	wantFleetNoCeiling    = "Renting is on, but the fleet may rent no machines: a maximum of none is none."
	wantFleetNoCeilingFix = "Set Machines the fleet may rent to the most machines you are willing to pay for at once."
	wantFleetOn           = "Renting is on."
)

// rentingProvider is a provider with a ceiling of its own, for the tests that
// add ceilings up.
func rentingProvider(name string, ceiling int) *store.Provider {
	p := demandProvider(name)
	p.MaxMachines = ceiling
	return p
}

// lastPass is the report of a pass that reached these verdicts.
func lastPass(verdicts ...PoolRenting) *rentingReport {
	return &rentingReport{At: machineNow, Pools: verdicts}
}

// The fleet's state is the first thing an operator reads about renting, and the
// four call for different things: add a provider, switch renting on, give the
// fleet a ceiling, or nothing at all. Each is pinned with its words and the one
// setting its fix changes, because the Providers page offers to change exactly
// that setting, and with the ceiling it offers, because that is what the one
// click sets.
func TestTheRentingViewSaysWhichSwitchIsHoldingRentingBack(t *testing.T) {
	disabled := rentingProvider("lab", 2)
	disabled.Enabled = false
	demo := rentingProvider("demo-pve", 2)
	demo.ID = "prv_demofixture"
	verdict := PoolRenting{
		PoolID: "pool_alpha", PoolName: "alpha", State: RentingBuying,
		Summary:    "Renting 1 machine from lab, which covers this pool: 1 runner slot short, and no host in the fleet can run it.",
		ProviderID: "prv_lab", ProviderName: "lab", Short: 1, Machines: 1,
	}

	for _, tc := range []struct {
		name                         string
		providers                    []*store.Provider
		enabled                      bool
		maxMachines                  int
		state, summary, fix, setting string
		suggested                    int
		// pools is how many of the pass's verdicts the view carries: every one
		// while some provider may rent, and none when nothing can be.
		pools int
	}{
		{
			name:      "renting switched off",
			providers: []*store.Provider{rentingProvider("lab", 2)},
			state:     "off", summary: wantFleetOff, fix: wantFleetOffFix, setting: "provider.enabled",
			suggested: 2, pools: 1,
		},
		{
			name:      "renting on with no ceiling",
			providers: []*store.Provider{rentingProvider("lab", 2)},
			enabled:   true,
			state:     "no_ceiling", summary: wantFleetNoCeiling, fix: wantFleetNoCeilingFix, setting: "provider.max_machines",
			suggested: 2, pools: 1,
		},
		{
			name:      "renting on with a ceiling",
			providers: []*store.Provider{rentingProvider("lab", 2)},
			enabled:   true, maxMachines: 2,
			state: "on", summary: wantFleetOn,
			suggested: 2, pools: 1,
		},
		{
			name:    "no provider at all",
			enabled: true, maxMachines: 2,
			state: "none", summary: wantFleetNone,
			suggested: 1,
		},
		{
			// Review Focus 1. With renting off, provisioningHeld answers with
			// the switch for this row as for any other, so a state read from
			// it would be off, and the page would offer to switch renting on
			// for a provider that would still rent nothing.
			name:      "a provider whose own row is disabled",
			providers: []*store.Provider{disabled},
			state:     "none", summary: wantFleetNone,
			suggested: 1,
		},
		{
			name:      "the demo fixture alone",
			providers: []*store.Provider{demo},
			state:     "none", summary: wantFleetNone,
			suggested: 1,
		},
		{
			name:      "two providers suggest their ceilings added up",
			providers: []*store.Provider{rentingProvider("lab", 2), rentingProvider("rack", 3)},
			state:     "off", summary: wantFleetOff, fix: wantFleetOffFix, setting: "provider.enabled",
			suggested: 5, pools: 1,
		},
		{
			// A click that set the fleet's ceiling to the sum of none would
			// switch renting on and leave it renting nothing.
			name:      "a provider with no ceiling of its own still suggests one machine",
			providers: []*store.Provider{rentingProvider("lab", 0)},
			state:     "off", summary: wantFleetOff, fix: wantFleetOffFix, setting: "provider.enabled",
			suggested: 1, pools: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := buildRentingView(rentingInput{
				Report: lastPass(verdict), Usable: usableRows(tc.providers),
				Enabled: tc.enabled, MaxMachines: tc.maxMachines,
			})
			if got.State != tc.state {
				t.Errorf("state = %q, want %q", got.State, tc.state)
			}
			if got.Summary != tc.summary {
				t.Errorf("summary = %q,\n want %q", got.Summary, tc.summary)
			}
			if got.Fix != tc.fix {
				t.Errorf("fix = %q,\n want %q", got.Fix, tc.fix)
			}
			if got.Setting != tc.setting {
				t.Errorf("setting = %q, want %q", got.Setting, tc.setting)
			}
			if got.SuggestedMaxMachines != tc.suggested {
				t.Errorf("suggested max machines = %d, want %d", got.SuggestedMaxMachines, tc.suggested)
			}
			if got.Enabled != tc.enabled || got.MaxMachines != tc.maxMachines {
				t.Errorf("switches = enabled %v, max %d; want them as they stand, %v and %d",
					got.Enabled, got.MaxMachines, tc.enabled, tc.maxMachines)
			}
			if len(got.Pools) != tc.pools {
				t.Errorf("pools = %+v, want %d of them", got.Pools, tc.pools)
			}
		})
	}
}

// A pass decides whether or not renting is switched on, and with it off, what it
// decided is what renting being off prevents: so many machines it would rent, a
// ceiling that would have stopped it as well, or a machine nothing is
// finishing. Those three say so and name the one setting that changes it. A
// pool waiting on its hosts, or one no provider serves, is in the same place
// whichever way the switch is, and is told the same thing. With renting on, a
// purchase the provider may not make is held rather than renting.
func TestTheOverlayTurnsWhatWouldBeBoughtIntoWhatRentingBeingOffPrevents(t *testing.T) {
	since := machineNow.Add(-4 * time.Minute)
	renting := PoolRenting{
		PoolID: "pool_alpha", PoolName: "alpha", State: RentingBuying,
		Summary:    "Renting 1 machine from lab, which covers this pool: 1 runner slot short with 1 job queued, and no host in the fleet can run it.",
		ProviderID: "prv_lab", ProviderName: "lab", Queued: 1, Short: 1, Machines: 1,
	}
	partial := PoolRenting{
		PoolID: "pool_beta", PoolName: "beta", State: RentingBuying,
		Summary:    "Renting 2 of the 3 machines it needs from lab, which covers this pool: 6 runner slots short with 6 jobs queued, and no host in the fleet can run it. The rest are held back: provider.max_creates_in_flight allows only 2 machines at a time.",
		Fix:        "Raise provider.max_creates_in_flight.",
		Setting:    "provider.max_creates_in_flight",
		ProviderID: "prv_lab", ProviderName: "lab", Queued: 6, Short: 6, Machines: 2,
	}
	ceiling := PoolRenting{
		PoolID: "pool_gamma", PoolName: "gamma", State: RentingCeiling,
		Summary:    "Not renting: provider.max_machines is 0, so this fleet rents nothing.",
		Fix:        "Set provider.max_machines to the number of machines this fleet may pay for.",
		Setting:    "provider.max_machines",
		Because:    "provider.max_machines is 0, so this fleet rents nothing",
		ProviderID: "prv_lab", ProviderName: "lab", Queued: 1, Short: 1,
	}
	coming := PoolRenting{
		PoolID: "pool_delta", PoolName: "delta", State: RentingOnItsWay,
		Summary:    "1 machine from lab is on the way for this pool, so the next runners start there.",
		ProviderID: "prv_lab", ProviderName: "lab", Queued: 2, Short: 2, Machines: 1, Since: &since,
	}
	waiting := PoolRenting{
		PoolID: "pool_epsilon", PoolName: "epsilon", State: RentingWaiting,
		Summary: "Not renting yet: no host can take a new docker runner (1 cordoned, 1 at capacity). A machine is rented only once every host that could run this pool is full.",
		Fix:     "Wait for those hosts to clear, or uncordon, repair or remove the ones that are unavailable.",
		Queued:  1, Short: 1,
	}
	unserved := PoolRenting{
		PoolID: "pool_zeta", PoolName: "zeta", State: RentingUnserved,
		Summary: "No provider will rent for this pool. Provider lab only rents for pools matching tier=large, and pool zeta does not have tier=large.",
		Fix:     "Edit the provider's pool selector, or give the pool the label tier=large.",
		Queued:  1, Short: 1,
	}
	report := lastPass(renting, partial, ceiling, coming, waiting, unserved)
	decided := slices.Clone(report.Pools)
	usable := []*store.Provider{demandProvider("lab")}

	for _, tc := range []struct {
		name    string
		enabled bool
		held    map[string]string
		want    []PoolRenting
	}{
		{
			name: "renting switched off",
			want: func() []PoolRenting {
				off := func(v PoolRenting, summary string) PoolRenting {
					v.State, v.Summary, v.Fix, v.Setting = RentingOff, summary, wantFleetOffFix, "provider.enabled"
					return v
				}
				// The machines on their way were counted as coming, and while
				// renting is off nothing brings them. They are not machines the
				// pass would rent either, so the pool is said to want none.
				stranded := off(coming, "A machine for this pool was being built, but renting is switched off, so nothing is finishing it.")
				stranded.Machines = 0
				return []PoolRenting{
					off(renting, "Would rent 1 machine for this pool, but renting is switched off."),
					off(partial, "Would rent 2 machines for this pool, but renting is switched off."),
					off(ceiling, "Renting is switched off, and provider.max_machines is 0, so this fleet rents nothing."),
					stranded,
					waiting,
					unserved,
				}
			}(),
		},
		{
			// The fix a partial purchase named goes with it: raising a ceiling
			// does nothing for a provider that is paused.
			name:    "renting on, and the provider may not buy",
			enabled: true,
			held:    map[string]string{"prv_lab": "lab is paused: over budget"},
			want: func() []PoolRenting {
				held := func(v PoolRenting) PoolRenting {
					v.State, v.Summary, v.Fix, v.Setting = RentingHeld, "Not renting: lab is paused: over budget.", "", ""
					return v
				}
				return []PoolRenting{held(renting), held(partial), ceiling, coming, waiting, unserved}
			}(),
		},
		{
			// Another provider being held holds back none of lab's purchases.
			name:    "renting on, and nothing holds the provider back",
			enabled: true,
			held:    map[string]string{"prv_rack": "rack is paused: over budget"},
			want:    decided,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := buildRentingView(rentingInput{
				Report: report, Usable: usable, Enabled: tc.enabled, MaxMachines: 10, Held: tc.held,
			}).Pools
			if !slices.Equal(got, tc.want) {
				t.Errorf("pools:\n got %+v\nwant %+v", got, tc.want)
			}
			// The report is the pass's, and every read lays the switches over it
			// afresh, so a read that wrote its overlay back would leave the
			// next one describing a switch that has since moved.
			if !slices.Equal(report.Pools, decided) {
				t.Errorf("building the view rewrote the report:\n got %+v\nwant %+v", report.Pools, decided)
			}
		})
	}
}

// Before the first machine pass there is no verdict yet, but the switches are
// known, and a page that asks is told them rather than left waiting for the
// loop. The pools are an empty list and never null (Review Focus 3), so a page
// that lists them needs no case of its own for the first moments after a start.
func TestTheViewBeforeTheFirstPassHasTheSwitchesAndNoPools(t *testing.T) {
	got := buildRentingView(rentingInput{
		Usable: []*store.Provider{rentingProvider("lab", 3)}, Enabled: true, Paused: true, MaxMachines: 2,
	})
	if got.State != "on" || !got.Enabled || !got.Paused || got.MaxMachines != 2 || got.SuggestedMaxMachines != 3 {
		t.Errorf("view = %+v, want the switches as they stand: on, paused, a ceiling of 2, and 3 suggested", got)
	}
	if got.Pools == nil || len(got.Pools) != 0 {
		t.Errorf("pools = %#v, want an empty list", got.Pools)
	}
	if got.DecidedAt != nil || got.Owned != 0 {
		t.Errorf("decided at %v with %d owned, want neither before a pass has run", got.DecidedAt, got.Owned)
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshalling the view: %v", err)
	}
	if !strings.Contains(string(raw), `"pools":[]`) {
		t.Errorf("the view renders as %s, want pools as an empty list", raw)
	}
	if strings.Contains(string(raw), `"decided_at"`) {
		t.Errorf("the view renders as %s, want no decision time before there is a decision", raw)
	}
}
