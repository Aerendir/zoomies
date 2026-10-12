package controller

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/scheduler"
	"github.com/eyupio/zoomies/internal/store"
)

// theVerdict is the one verdict a snapshot of one pool should produce, so a
// test says what it expects rather than indexing a slice.
func theVerdict(t *testing.T, plan MachinePlan) PoolRenting {
	t.Helper()
	if len(plan.Pools) != 1 {
		t.Fatalf("verdicts = %+v, want exactly one", plan.Pools)
	}
	return plan.Pools[0]
}

// The fleet this was reported from: three Linux hosts with every slot busy, a
// fourth throttled to its last rung, and a Windows host that only offers the
// process backend. The Windows host can never run the pool. While it kept the
// fleet from reading as full, the queued job waited for a slot only a rented
// machine could give, and none was ever rented. With it or without it, the
// answer is one machine, and the verdict says so.
func TestAMixedFleetBuysAMachineForABusyPool(t *testing.T) {
	for _, tc := range []struct {
		name    string
		windows bool
	}{
		{name: "beside a Windows host", windows: true},
		{name: "with its Linux hosts alone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := demandPool("zoomies-linux-x64", "zoomies", "zoomies-linux-x64")
			pool.MinRunners, pool.MaxRunners = 2, 15

			throttled := demandHost("linux_4", 4, 2)
			throttled.Throttle = store.HostThrottle{Level: store.MaxThrottleLevel}
			hosts := []*store.Host{demandHost("linux_1", 2, 2), demandHost("linux_2", 1, 1), demandHost("linux_3", 2, 2), throttled}
			var runners []*store.Runner
			for _, h := range hosts {
				for i := range h.ActiveRunners {
					id := fmt.Sprintf("run_%s_%d", h.ID, i)
					runners = append(runners, &store.Runner{
						ID: id, Name: id, PoolID: pool.ID, HostID: h.ID, State: store.RunnerBusy,
						CreatedAt: machineNow.Add(-10 * time.Minute),
					})
				}
			}
			if tc.windows {
				windows := demandHost("windows_1", 4, 0)
				windows.Backends = store.StringSlice{string(store.BackendProcess)}
				windows.OS, windows.Distro = "windows", "windows"
				hosts = append(hosts, windows)
			}
			jobs := []*store.Job{{
				ID: "job_1", Repo: "acme/widgets", State: store.JobQueued, InstallationID: "inst_1",
				Labels: store.StringSlice{"zoomies-linux-x64"}, QueuedAt: machineNow.Add(-3 * time.Minute),
			}}
			fleet := scheduler.Snapshot{
				Now: machineNow, Pools: []*store.Pool{pool}, Hosts: hosts, Jobs: jobs,
				Runners: map[string][]*store.Runner{pool.ID: runners},
				Policy: scheduler.Policy{
					MaxRunnerLifetime: 6 * time.Hour, ProvisionTimeout: 5 * time.Minute,
					DrainTimeout: 15 * time.Minute, MaxCreatesPerTick: 10,
				},
			}
			placed := scheduler.Decide(fleet)
			if len(placed.Pools) != 1 || !placed.Pools[0].BlockedAtCapacity {
				t.Fatalf("pool plans = %+v, want the one pool blocked at capacity", placed.Pools)
			}

			lab := demandProvider("proxmox-lab")
			lab.MaxMachines, lab.MaxCreatesInFlight = 2, 1
			limits := demandLimits()
			limits.MaxMachines, limits.MaxCreatesInFlight = 2, 2
			plan := DecideMachines(MachineSnapshot{
				Now: machineNow, Plan: placed, Pools: fleet.Pools, Hosts: hosts, Runners: fleet.Runners,
				Jobs: jobs, Providers: []*store.Provider{lab}, Limits: limits,
			})

			got := planFor(t, plan, lab.ID)
			if got.Create != 1 {
				t.Fatalf("create = %d, want 1: the pool is one runner short and every host that could run it is full", got.Create)
			}
			want := "buy 1 machine on proxmox-lab: pool zoomies-linux-x64 is 1 runner slot short with 1 job queued, and every host that can run it is full"
			if got.Reason != want {
				t.Fatalf("reason = %q,\n want %q", got.Reason, want)
			}
			v := theVerdict(t, plan)
			if v.State != RentingBuying {
				t.Fatalf("verdict = %+v, want %s", v, RentingBuying)
			}
			summary := "Renting 1 machine from proxmox-lab, which covers this pool: 1 runner slot short with 1 job queued, and every host that can run it is full."
			if v.Summary != summary {
				t.Fatalf("summary = %q,\n want %q", v.Summary, summary)
			}
		})
	}
}

// Each verdict is the sentence an operator reads when a pool queues and no
// machine arrives for it, so each is pinned word for word: the pool page, the
// problem and the Providers page all quote these words, and a verdict that
// named the wrong setting would send somebody to the wrong screen.
func TestEveryVerdictStateSaysItsSentence(t *testing.T) {
	// snapshot is provider lab and pool alpha and nothing else, with alpha as
	// the scheduler left it. Each row adds what makes its case.
	snapshot := func(current, desired, queued int, blocked string) MachineSnapshot {
		pool := demandPool("alpha", "linux", "x64")
		return MachineSnapshot{
			Now: machineNow, Pools: []*store.Pool{pool}, Providers: []*store.Provider{demandProvider("lab")},
			Jobs: demandJobs(queued), Limits: demandLimits(),
			Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{demandPoolPlan(pool, current, desired, queued, blocked)}},
		}
	}
	for _, tc := range []struct {
		name     string
		snapshot func() MachineSnapshot
		// want leaves out the pool, which is alpha in every row, and the
		// provider's id, which follows from its name.
		want PoolRenting
		// aside is what a page already showing the scheduler's sentence adds,
		// when that is not the whole summary.
		aside string
	}{
		{
			name:     "renting a machine",
			snapshot: func() MachineSnapshot { return snapshot(0, 1, 1, "nowhere") },
			want: PoolRenting{
				State:        RentingBuying,
				Summary:      "Renting 1 machine from lab, which covers this pool: 1 runner slot short with 1 job queued, and no host in the fleet can run it.",
				ProviderName: "lab", Queued: 1, Short: 1, Machines: 1,
			},
		},
		{
			name: "renting part of what it needs, held back by the provider's maximum",
			snapshot: func() MachineSnapshot {
				s := snapshot(0, 4, 4, "nowhere")
				s.Providers[0].MaxMachines = 1
				return s
			},
			want: PoolRenting{
				State:        RentingBuying,
				Summary:      "Renting 1 of the 2 machines it needs from lab, which covers this pool: 4 runner slots short with 4 jobs queued, and no host in the fleet can run it. The rest are held back: lab may own only 1 machine.",
				Fix:          "Raise this provider's maximum machines.",
				ProviderName: "lab", Queued: 4, Short: 4, Machines: 1,
			},
		},
		{
			name: "renting part of what it needs, held back by what the fleet may build at once",
			snapshot: func() MachineSnapshot {
				s := snapshot(0, 4, 4, "nowhere")
				s.Limits.MaxCreatesInFlight = 1
				return s
			},
			want: PoolRenting{
				State:        RentingBuying,
				Summary:      "Renting 1 of the 2 machines it needs from lab, which covers this pool: 4 runner slots short with 4 jobs queued, and no host in the fleet can run it. The rest are held back: provider.max_creates_in_flight allows only 1 machine at a time.",
				Fix:          "Raise provider.max_creates_in_flight.",
				Setting:      "provider.max_creates_in_flight",
				ProviderName: "lab", Queued: 4, Short: 4, Machines: 1,
			},
		},
		{
			// A provider's own limit is on its own form rather than in
			// Settings, so no setting is named.
			name: "renting part of what it needs, held back by what the provider may build at once",
			snapshot: func() MachineSnapshot {
				s := snapshot(0, 4, 4, "nowhere")
				s.Providers[0].MaxCreatesInFlight = 1
				return s
			},
			want: PoolRenting{
				State:        RentingBuying,
				Summary:      "Renting 1 of the 2 machines it needs from lab, which covers this pool: 4 runner slots short with 4 jobs queued, and no host in the fleet can run it. The rest are held back: lab may build only 1 machine at a time.",
				Fix:          "Raise this provider's maximum creates in flight.",
				ProviderName: "lab", Queued: 4, Short: 4, Machines: 1,
			},
		},
		{
			name: "one machine on its way",
			snapshot: func() MachineSnapshot {
				s := snapshot(0, 2, 2, "nowhere")
				m := demandMachine("mach_1", s.Providers[0], store.MachineCreating, 3*time.Minute)
				m.Capacity = 2
				s.Machines = []*store.Machine{m}
				return s
			},
			want: PoolRenting{
				State:        RentingOnItsWay,
				Summary:      "1 machine from lab is on the way for this pool, so the next runners start there.",
				ProviderName: "lab", Queued: 2, Short: 2, Machines: 1,
				Since: ptrTime(machineNow.Add(-3 * time.Minute)),
			},
		},
		{
			// Handed over newest first, so the time given is the oldest
			// machine's because it is the oldest and not because it came first.
			name: "two machines on their way",
			snapshot: func() MachineSnapshot {
				s := snapshot(0, 4, 3, "nowhere")
				s.Machines = []*store.Machine{
					demandMachine("mach_newer", s.Providers[0], store.MachinePlanned, time.Minute),
					demandMachine("mach_older", s.Providers[0], store.MachineStarting, 6*time.Minute),
				}
				return s
			},
			want: PoolRenting{
				State:        RentingOnItsWay,
				Summary:      "2 machines from lab are on the way for this pool, so the next runners start there.",
				ProviderName: "lab", Queued: 3, Short: 4, Machines: 2,
				Since: ptrTime(machineNow.Add(-6 * time.Minute)),
			},
		},
		{
			// Two providers' machines cover the pool between them, and the
			// sentence names one provider, so it counts only that provider's:
			// "2 machines from alpha-lab" would send an operator looking for a
			// machine alpha-lab is not building.
			name: "machines from two providers on their way",
			snapshot: func() MachineSnapshot {
				s := snapshot(0, 4, 4, "nowhere")
				alpha, beta := demandProvider("alpha-lab"), demandProvider("beta-lab")
				s.Providers = []*store.Provider{beta, alpha}
				s.Machines = []*store.Machine{
					demandMachine("mach_beta", beta, store.MachinePlanned, time.Minute),
					demandMachine("mach_alpha", alpha, store.MachineCreating, 5*time.Minute),
				}
				return s
			},
			want: PoolRenting{
				State:        RentingOnItsWay,
				Summary:      "1 machine from alpha-lab is on the way for this pool, so the next runners start there.",
				ProviderName: "alpha-lab", Queued: 4, Short: 4, Machines: 1,
				Since: ptrTime(machineNow.Add(-5 * time.Minute)),
			},
		},
		{
			name: "waiting for hosts that could run it",
			snapshot: func() MachineSnapshot {
				s := snapshot(1, 2, 1, "")
				s.Plan.Pools[0].Blocked = "no host can take a new docker runner (1 cordoned, 3 at capacity)"
				return s
			},
			want: PoolRenting{
				State:   RentingWaiting,
				Summary: "Not renting yet: no host can take a new docker runner (1 cordoned, 3 at capacity). A machine is rented only once every host that could run this pool is full.",
				Fix:     "Wait for those hosts to clear, or uncordon, repair or remove the ones that are unavailable.",
				Queued:  1, Short: 1,
			},
			aside: "A machine is rented only once every host that could run this pool is full.",
		},
		{
			name: "held back by the fleet's maximum",
			snapshot: func() MachineSnapshot {
				s := snapshot(0, 1, 1, "nowhere")
				s.Limits.MaxMachines = 0
				return s
			},
			want: PoolRenting{
				State:        RentingCeiling,
				Summary:      "Not renting: provider.max_machines is 0, so this fleet rents nothing.",
				Fix:          "Set provider.max_machines to the number of machines this fleet may pay for.",
				Setting:      "provider.max_machines",
				Because:      "provider.max_machines is 0, so this fleet rents nothing",
				ProviderName: "lab", Queued: 1, Short: 1,
			},
		},
		{
			// lab's one machine is a host already, and busy, so the pool is
			// still short and lab may not own another.
			name: "held back by the provider's maximum",
			snapshot: func() MachineSnapshot {
				s := snapshot(2, 3, 1, "capacity")
				p := s.Providers[0]
				p.MaxMachines = 1
				host := demandHost("host_1", 2, 2)
				ready := demandMachine("mach_1", p, store.MachineReady, time.Hour)
				ready.HostID = host.ID
				s.Hosts, s.Machines = []*store.Host{host}, []*store.Machine{ready}
				return s
			},
			want: PoolRenting{
				State:        RentingCeiling,
				Summary:      "Not renting: lab owns 1 machine and its maximum is 1.",
				Fix:          "Raise this provider's maximum machines, or wait for one of its machines to be released.",
				Because:      "lab owns 1 machine and its maximum is 1",
				ProviderName: "lab", Queued: 1, Short: 1,
			},
		},
		{
			name: "refused by a provider's pool selector",
			snapshot: func() MachineSnapshot {
				s := snapshot(0, 1, 1, "nowhere")
				s.Providers[0].PoolSelector = store.StringMap{"tier": "large"}
				return s
			},
			want: PoolRenting{
				State:   RentingUnserved,
				Summary: "No provider will rent for this pool. Provider lab only rents for pools matching tier=large, and pool alpha does not have tier=large.",
				Fix:     "Edit the provider's pool selector, or give the pool the label tier=large.",
				Queued:  1, Short: 1,
			},
		},
		{
			name: "no provider builds a machine that suits it",
			snapshot: func() MachineSnapshot {
				s := snapshot(0, 1, 1, "nowhere")
				s.Pools[0].Backend = store.BackendProcess
				return s
			},
			want: PoolRenting{
				State:   RentingUnserved,
				Summary: "No provider builds a machine this pool could use. lab: The machines it builds run docker, and this pool needs process.",
				Fix:     "Give a provider machines that suit this pool, or change the pool.",
				Queued:  1, Short: 1,
			},
		},
		{
			// Handed over out of order, so the provider named is the first by
			// name and not the first in the slice.
			name: "no provider of two builds a machine that suits it",
			snapshot: func() MachineSnapshot {
				s := snapshot(0, 1, 1, "nowhere")
				s.Pools[0].Backend = store.BackendProcess
				s.Providers = []*store.Provider{demandProvider("beta-lab"), demandProvider("alpha-lab")}
				return s
			},
			want: PoolRenting{
				State:   RentingUnserved,
				Summary: "No provider builds a machine this pool could use. alpha-lab: The machines it builds run docker, and this pool needs process.",
				Fix:     "Give a provider machines that suit this pool, or change the pool.",
				Queued:  1, Short: 1,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.snapshot()
			got := theVerdict(t, DecideMachines(s))

			want := tc.want
			want.PoolID, want.PoolName = "pool_alpha", "alpha"
			if want.ProviderName != "" {
				want.ProviderID = "prv_" + want.ProviderName
			}
			// Since is compared as an instant below; everything else must
			// match exactly.
			gotRest, wantRest := got, want
			gotRest.Since, wantRest.Since = nil, nil
			if gotRest != wantRest {
				t.Errorf("verdict:\n got %+v\nwant %+v", gotRest, wantRest)
			}
			switch {
			case (got.Since == nil) != (want.Since == nil):
				t.Errorf("since = %v, want %v", got.Since, want.Since)
			case got.Since != nil && !got.Since.Equal(*want.Since):
				t.Errorf("since = %v, want %v", *got.Since, *want.Since)
			}

			aside := tc.aside
			if aside == "" {
				aside = want.Summary
			}
			if got.Aside() != aside {
				t.Errorf("aside = %q,\n want %q", got.Aside(), aside)
			}

			// The verdict outlives the pass and the machine rows are the
			// controller's own, so a since that pointed into a row would let
			// whatever holds the verdict rewrite when the machine was made.
			if got.Since != nil {
				*got.Since = time.Time{}
				for _, m := range s.Machines {
					if m.CreatedAt.IsZero() {
						t.Errorf("writing the verdict's since rewrote machine %s's created_at", m.ID)
					}
				}
			}
		})
	}
}

// Two providers that could both rent for a pool are one purchase charged to
// one of them, and so one verdict: two would tell an operator that two
// machines were on order for work one of them answers.
func TestTwoProvidersThatServeOnePoolGiveOneVerdict(t *testing.T) {
	alpha, beta := demandProvider("alpha-lab"), demandProvider("beta-lab")
	pool := demandPool("linux-x64", "linux", "x64")
	s := MachineSnapshot{
		Now: machineNow, Pools: []*store.Pool{pool},
		// Handed over out of order, so the provider charged is the first by
		// name and not the first in the slice.
		Providers: []*store.Provider{beta, alpha},
		Jobs:      demandJobs(2), Limits: demandLimits(),
		Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{demandPoolPlan(pool, 0, 2, 2, "nowhere")}},
	}

	plan := DecideMachines(s)
	if len(plan.Pools) != 1 {
		t.Fatalf("verdicts = %+v, want one for the one pool", plan.Pools)
	}
	if got := plan.Pools[0].ProviderName; got != "alpha-lab" {
		t.Fatalf("charged to %q, want alpha-lab, the first by name", got)
	}
}

// One provider buying for several pools makes one purchase for all of them.
// When the whole purchase is made, each pool is told its own share: the
// provider's whole order would tell a pool one slot short that it needs two
// machines. When it is held back, the machines bought serve whichever pool the
// scheduler places on them first, so none of the pools is told it is covered,
// and each is told about the pools together. Either way the provider's own
// order is the group's, because it is one purchase.
func TestPoolsSharingAProviderQuoteTheirOwnShareOrSpeakForTheGroupWhenItIsHeldBack(t *testing.T) {
	// queuedFor is n queued jobs that only the pool advertising label claims.
	queuedFor := func(label string, n int) []*store.Job {
		jobs := demandJobs(n)
		for _, j := range jobs {
			j.ID += "_" + label
			j.Labels = append(j.Labels, label)
		}
		return jobs
	}
	for _, tc := range []struct {
		name   string
		limits func(*MachineLimits)
		// alpha and beta are how many runner slots each pool is short, with
		// as many jobs queued.
		alpha, beta int
		// create and wanted are the provider's own order.
		create, wanted int
		want           []PoolRenting
	}{
		{
			name:   "the fleet may build one machine at a time",
			limits: func(l *MachineLimits) { l.MaxCreatesInFlight = 1 },
			alpha:  1, beta: 3,
			create: 1, wanted: 2,
			want: []PoolRenting{
				{
					PoolID: "pool_alpha", PoolName: "alpha", State: RentingBuying,
					Summary:    "Renting 1 of the 2 machines this pool and others need from lab: 1 runner slot short with 1 job queued, and no host in the fleet can run it. The rest are held back: provider.max_creates_in_flight allows only 1 machine at a time.",
					Fix:        "Raise provider.max_creates_in_flight.",
					Setting:    "provider.max_creates_in_flight",
					ProviderID: "prv_lab", ProviderName: "lab", Queued: 1, Short: 1, Machines: 1,
				},
				{
					PoolID: "pool_beta", PoolName: "beta", State: RentingBuying,
					Summary:    "Renting 1 of the 2 machines this pool and others need from lab: 3 runner slots short with 3 jobs queued, and no host in the fleet can run it. The rest are held back: provider.max_creates_in_flight allows only 1 machine at a time.",
					Fix:        "Raise provider.max_creates_in_flight.",
					Setting:    "provider.max_creates_in_flight",
					ProviderID: "prv_lab", ProviderName: "lab", Queued: 3, Short: 3, Machines: 1,
				},
			},
		},
		{
			// Each pool alone would be covered by the one machine bought, and
			// the two together are not, so neither may say it is.
			name:   "two pools that one machine would each cover alone",
			limits: func(l *MachineLimits) { l.MaxCreatesInFlight = 1 },
			alpha:  2, beta: 2,
			create: 1, wanted: 2,
			want: []PoolRenting{
				{
					PoolID: "pool_alpha", PoolName: "alpha", State: RentingBuying,
					Summary:    "Renting 1 of the 2 machines this pool and others need from lab: 2 runner slots short with 2 jobs queued, and no host in the fleet can run it. The rest are held back: provider.max_creates_in_flight allows only 1 machine at a time.",
					Fix:        "Raise provider.max_creates_in_flight.",
					Setting:    "provider.max_creates_in_flight",
					ProviderID: "prv_lab", ProviderName: "lab", Queued: 2, Short: 2, Machines: 1,
				},
				{
					PoolID: "pool_beta", PoolName: "beta", State: RentingBuying,
					Summary:    "Renting 1 of the 2 machines this pool and others need from lab: 2 runner slots short with 2 jobs queued, and no host in the fleet can run it. The rest are held back: provider.max_creates_in_flight allows only 1 machine at a time.",
					Fix:        "Raise provider.max_creates_in_flight.",
					Setting:    "provider.max_creates_in_flight",
					ProviderID: "prv_lab", ProviderName: "lab", Queued: 2, Short: 2, Machines: 1,
				},
			},
		},
		{
			name:   "no ceiling bites",
			limits: func(*MachineLimits) {},
			alpha:  1, beta: 3,
			create: 2, wanted: 2,
			want: []PoolRenting{
				{
					PoolID: "pool_alpha", PoolName: "alpha", State: RentingBuying,
					Summary:    "Renting 1 machine from lab, which covers this pool: 1 runner slot short with 1 job queued, and no host in the fleet can run it.",
					ProviderID: "prv_lab", ProviderName: "lab", Queued: 1, Short: 1, Machines: 1,
				},
				{
					PoolID: "pool_beta", PoolName: "beta", State: RentingBuying,
					Summary:    "Renting 2 machines from lab, which covers this pool: 3 runner slots short with 3 jobs queued, and no host in the fleet can run it.",
					ProviderID: "prv_lab", ProviderName: "lab", Queued: 3, Short: 3, Machines: 2,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := demandProvider("lab")
			alpha, beta := demandPool("alpha", "linux", "x64", "alpha"), demandPool("beta", "linux", "x64", "beta")
			limits := demandLimits()
			tc.limits(&limits)
			s := MachineSnapshot{
				Now: machineNow, Pools: []*store.Pool{alpha, beta}, Providers: []*store.Provider{p},
				Jobs:   append(queuedFor("alpha", tc.alpha), queuedFor("beta", tc.beta)...),
				Limits: limits,
				Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{
					demandPoolPlan(alpha, 0, tc.alpha, tc.alpha, "nowhere"),
					demandPoolPlan(beta, 0, tc.beta, tc.beta, "nowhere"),
				}},
			}

			plan := DecideMachines(s)
			if !slices.Equal(plan.Pools, tc.want) {
				t.Errorf("verdicts:\n got %+v\nwant %+v", plan.Pools, tc.want)
			}
			if got := planFor(t, plan, p.ID); got.Create != tc.create || got.Wanted != tc.wanted {
				t.Errorf("provider create/wanted = %d/%d, want %d/%d", got.Create, got.Wanted, tc.create, tc.wanted)
			}
			if tc.create < tc.wanted {
				for _, v := range plan.Pools {
					if strings.Contains(v.Summary, "covers this pool") {
						t.Errorf("pool %s says it is covered by a purchase that was held back: %q", v.PoolName, v.Summary)
					}
				}
			}
		})
	}
}

// A pool the scheduler is waiting on hosts for is told that a machine follows
// once they are full, which is only true if some provider could build one it
// could use. Where none could, that would promise a machine that never comes,
// so the pool is told what is really in the way. It is a verdict only:
// MachinePlan.Unserved is the pools whose work a machine would answer, and
// this pool's waits on its hosts, so that list and the purchase stay as they
// were.
func TestAPoolNoProviderCouldServeIsToldSoRatherThanToWait(t *testing.T) {
	// The plan is what the scheduler says of a pool with one host busy and one
	// cordoned: blocked, in a way an uncordon or a finished job clears, so
	// neither capacity flag is set. A fleet whose only host is cordoned is one
	// with no eligible host instead, and never takes this branch.
	waitingOn := func(pool *store.Pool) scheduler.PoolPlan {
		pp := demandPoolPlan(pool, 1, 2, 1, "")
		pp.Blocked = "no host can take a new " + string(pool.Backend) + " runner (1 cordoned, 1 at capacity)"
		return pp
	}
	for _, tc := range []struct {
		name     string
		pool     func(*store.Pool)
		provider func(*store.Provider)
		want     PoolRenting
	}{
		{
			name:     "no provider builds a machine it could use",
			pool:     func(p *store.Pool) { p.Backend = store.BackendProcess },
			provider: func(*store.Provider) {},
			want: PoolRenting{
				State:   RentingUnserved,
				Summary: "No provider builds a machine this pool could use. lab: The machines it builds run docker, and this pool needs process.",
				Fix:     "Give a provider machines that suit this pool, or change the pool.",
				Queued:  1, Short: 1,
			},
		},
		{
			name:     "a selector rules the provider out",
			pool:     func(*store.Pool) {},
			provider: func(p *store.Provider) { p.PoolSelector = store.StringMap{"tier": "large"} },
			want: PoolRenting{
				State:   RentingUnserved,
				Summary: "No provider will rent for this pool. Provider lab only rents for pools matching tier=large, and pool alpha does not have tier=large.",
				Fix:     "Edit the provider's pool selector, or give the pool the label tier=large.",
				Queued:  1, Short: 1,
			},
		},
		{
			name:     "a provider could serve it",
			pool:     func(*store.Pool) {},
			provider: func(*store.Provider) {},
			want: PoolRenting{
				State:   RentingWaiting,
				Summary: "Not renting yet: no host can take a new docker runner (1 cordoned, 1 at capacity). A machine is rented only once every host that could run this pool is full.",
				Fix:     "Wait for those hosts to clear, or uncordon, repair or remove the ones that are unavailable.",
				Queued:  1, Short: 1,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := demandProvider("lab")
			tc.provider(p)
			pool := demandPool("alpha", "linux", "x64")
			tc.pool(pool)
			s := MachineSnapshot{
				Now: machineNow, Pools: []*store.Pool{pool}, Providers: []*store.Provider{p},
				Jobs: demandJobs(1), Limits: demandLimits(),
				Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{waitingOn(pool)}},
			}

			plan := DecideMachines(s)
			want := tc.want
			want.PoolID, want.PoolName = pool.ID, pool.Name
			if got := theVerdict(t, plan); got != want {
				t.Errorf("verdict:\n got %+v\nwant %+v", got, want)
			}
			if len(plan.Unserved) != 0 {
				t.Errorf("unserved = %+v, want none: this pool's work waits on its hosts, not on a machine", plan.Unserved)
			}
			if got := planFor(t, plan, p.ID); got.Create != 0 || got.Reason != "" || got.Blocked != "" {
				t.Errorf("provider plan = %+v, want nothing bought and nothing said", got)
			}
		})
	}
}

// A pool charged to a provider is decided after every pool the walk passed
// over, so a list in the order decisions were made would put beta before alpha
// here. The pages that show verdicts list pools by name, and so does this.
func TestVerdictsAreInPoolNameOrderWhicheverWayTheyWereDecided(t *testing.T) {
	p := demandProvider("lab")
	alpha, beta := demandPool("alpha", "linux", "x64"), demandPool("beta", "linux", "x64")
	waiting := demandPoolPlan(beta, 1, 2, 0, "")
	waiting.Blocked = "no host can take a new docker runner (1 cordoned, 1 at capacity)"
	s := MachineSnapshot{
		Now: machineNow, Pools: []*store.Pool{alpha, beta}, Providers: []*store.Provider{p},
		Jobs: demandJobs(1), Limits: demandLimits(),
		Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{demandPoolPlan(alpha, 0, 1, 1, "nowhere"), waiting}},
	}

	plan := DecideMachines(s)
	var got []string
	for _, v := range plan.Pools {
		got = append(got, v.PoolName+" "+string(v.State))
	}
	if want := []string{"alpha renting", "beta waiting"}; !slices.Equal(got, want) {
		t.Fatalf("verdicts = %v, want %v", got, want)
	}
}

// A pool below its min_runners with nothing queued is renting for the runners
// it keeps warm. "With 0 jobs queued" would read as a machine rented for no
// work at all.
func TestAPoolShortOfItsMinimumWithNothingQueuedDoesNotMentionAQueue(t *testing.T) {
	p := demandProvider("lab")
	pool := demandPool("alpha")
	s := MachineSnapshot{
		Now: machineNow, Pools: []*store.Pool{pool}, Providers: []*store.Provider{p}, Limits: demandLimits(),
		Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{demandPoolPlan(pool, 0, 2, 0, "nowhere")}},
	}

	want := "Renting 1 machine from lab, which covers this pool: 2 runner slots short, and no host in the fleet can run it."
	if got := theVerdict(t, DecideMachines(s)).Summary; got != want {
		t.Fatalf("summary = %q,\n want %q", got, want)
	}
}

// A pool the scheduler is placing runners for needs nothing rented and nothing
// said about renting: a verdict there would list a pool that is fine among the
// pools to look at.
func TestAPoolTheSchedulerCanPlaceItselfGetsNoVerdict(t *testing.T) {
	p := demandProvider("lab")
	pool := demandPool("alpha", "linux", "x64")
	s := MachineSnapshot{
		Now: machineNow, Pools: []*store.Pool{pool}, Providers: []*store.Provider{p},
		Jobs: demandJobs(2), Limits: demandLimits(),
		// Short, but only of this pass's create budget, which clears itself.
		Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{demandPoolPlan(pool, 0, 2, 2, "")}},
	}

	if plan := DecideMachines(s); len(plan.Pools) != 0 {
		t.Fatalf("verdicts = %+v, want none", plan.Pools)
	}
}

// A free slot on a host that could run the pool is the fleet's own answer to
// its queue. Nothing is rented against it, and a verdict there would send an
// operator looking for a fault that is not there.
func TestAPoolCoveredByFreeSlotsGetsNoVerdict(t *testing.T) {
	p := demandProvider("lab")
	pool := demandPool("alpha", "linux", "x64")
	host := demandHost("host_1", 4, 2) // two slots free
	s := MachineSnapshot{
		Now: machineNow, Pools: []*store.Pool{pool}, Hosts: []*store.Host{host}, Providers: []*store.Provider{p},
		Jobs: demandJobs(2), Limits: demandLimits(),
		Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{demandPoolPlan(pool, 2, 4, 2, "capacity")}},
	}

	plan := DecideMachines(s)
	if got := planFor(t, plan, p.ID).Create; got != 0 {
		t.Fatalf("create = %d, want 0: the free slots cover the pool", got)
	}
	if len(plan.Pools) != 0 {
		t.Fatalf("verdicts = %+v, want none", plan.Pools)
	}
}

// A fenced fleet creates and deletes nothing at any provider, so a machine that
// was on its way is not being finished. Told it was on the way, an operator
// would wait for runners that cannot start until the fence is lifted.
func TestAFencedFleetDoesNotSayAMachineIsOnItsWay(t *testing.T) {
	onItsWay := PoolRenting{
		PoolID: "pool_alpha", PoolName: "alpha", State: RentingOnItsWay, ProviderID: "prv_lab",
		ProviderName: "lab", Summary: "1 machine from lab is on the way for this pool, so the next runners start there.",
		Queued: 2, Short: 2, Machines: 1,
	}
	view := buildRentingView(rentingInput{
		Enabled: true, MaxMachines: 10, Fenced: true,
		Usable: []*store.Provider{demandProvider("lab")},
		Report: &rentingReport{Pools: []PoolRenting{onItsWay}},
	})
	if len(view.Pools) != 1 {
		t.Fatalf("pools = %+v, want one", view.Pools)
	}
	got := view.Pools[0]
	if got.State != RentingHeld {
		t.Fatalf("state = %q (%q), want held: nothing finishes a machine while the fleet is fenced", got.State, got.Summary)
	}
	if !strings.Contains(got.Summary, "fenced") || strings.Contains(got.Summary, "on the way") {
		t.Fatalf("summary = %q, want it to say the fleet is fenced and not that a machine is on the way", got.Summary)
	}
}
