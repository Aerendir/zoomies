package controller

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/scheduler"
	"github.com/eyupio/zoomies/internal/store"
)

// The shape a live fleet got wrong: a Proxmox provider whose row names no size,
// so its machines are its template's 2 vCPU and 4 GB (3915 MB once the guest
// measures itself), two slots each, and a docker-in-docker pool whose daemon is
// given 15% of a slot's memory. One runner of that pool needs a slot of about
// 6.7 GB, and the machine has 3.3 GB to place on after its reserve.
func smallProvider(name string) *store.Provider {
	p := demandProvider(name)
	p.MaxMachines = 2
	return p
}

func dindPool(name string) *store.Pool {
	pool := demandPool(name, "self-hosted", "linux", "x64")
	pool.DockerMode, pool.SizeFromProfile = store.DockerDinD, true
	pool.Resources = store.Resources{MinCPUs: 0.5, MinMemoryMB: 1024, DaemonMemorySharePercent: 15}
	return pool
}

// smallVMHost is a machine that provider built, as its agent reports it.
func smallVMHost(id string) *store.Host {
	h := demandHost(id, 2, 0)
	h.CPUs, h.MemoryMB = 2, 3915
	return h
}

// The fit check has to judge size the way placement does, or the fleet buys a
// machine at the moment the one it has sits idle for the same reason. Each of
// the three places a provider's size can come from is judged, and each says
// which figure to go and change. None of them suggests a larger maximum.
func TestAProviderWhoseMachinesAreTooSmallForADinDPoolDoesNotFit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		size      func(*store.Provider)
		wantWhy   string
		wantWhere string
	}{
		{
			name: "the size its machines measured",
			size: func(p *store.Provider) {
				p.SeenCPUs, p.SeenMemoryMB, p.SeenFrom = 2, 3915, store.MachineSizeFromMachine
			},
			wantWhy:   "its machines measure 2 CPUs and 3.8 GB of memory once they join",
			wantWhere: "the template it clones",
		},
		{
			name: "the template a connection check read",
			size: func(p *store.Provider) {
				p.SeenCPUs, p.SeenMemoryMB, p.SeenFrom = 2, 4096, store.MachineSizeFromTemplate
			},
			wantWhy:   "the template it clones gives 2 CPUs and 4 GB of memory",
			wantWhere: "the template it clones",
		},
		{
			name:      "a size typed on the provider",
			size:      func(p *store.Provider) { p.MachineCPUs, p.MachineMemoryMB = 2, 4096 },
			wantWhy:   "its machine size is 2 CPUs and 4 GB of memory",
			wantWhere: "raise its machine size",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, pool := smallProvider("proxmox-elitedesk-node1"), dindPool("zoomies-linux-x64")
			tc.size(p)
			misfit := machineFit(p, providerSynthHost(p, machineNow), pool)
			if !misfit.Size || misfit.Why == "" {
				t.Fatalf("the provider fits the pool, so renting would buy it: %+v", misfit)
			}
			for _, want := range []string{"too small for one of this pool's runners", tc.wantWhy, "6.7 GB"} {
				if !strings.Contains(misfit.Why, want) {
					t.Errorf("fit_why %q does not say %q", misfit.Why, want)
				}
			}
			for _, want := range []string{"at least 2 CPUs and 8 GB of memory", tc.wantWhere} {
				if !strings.Contains(misfit.Fix, want) {
					t.Errorf("fix %q does not say %q", misfit.Fix, want)
				}
			}
			if strings.Contains(misfit.Fix, "maximum") {
				t.Errorf("fix %q suggests a larger maximum, which buys more machines that cannot run a job", misfit.Fix)
			}
		})
	}
}

// A provider nothing has said the size of is judged as a host that measured
// nothing, which fits by slots alone: the first machine is how the size is
// learnt, and refusing it would leave a provider that names no size unable to
// rent anything at all.
func TestAProviderOfUnknownSizeIsJudgedAsItsUnmeasuredHostWouldBe(t *testing.T) {
	p, pool := smallProvider("lab"), dindPool("alpha")
	if misfit := machineFit(p, providerSynthHost(p, machineNow), pool); misfit.Why != "" {
		t.Fatalf("a provider of unknown size was refused: %s", misfit.Why)
	}
}

// The live fleet: two ready machines too small for the pool hold the
// provider's whole ceiling, and jobs are queued. Nothing is bought, and the
// verdict says the machines are too small, names the size they would have to
// be, and says outright that a higher maximum buys more of the same.
func TestRentingBuysNothingForADinDPoolItsProvidersMachinesAreTooSmallFor(t *testing.T) {
	p, pool := smallProvider("proxmox-elitedesk-node1"), dindPool("zoomies-linux-x64")
	p.SeenCPUs, p.SeenMemoryMB, p.SeenFrom = 2, 3915, store.MachineSizeFromMachine
	var hosts []*store.Host
	var machines []*store.Machine
	for _, id := range []string{"a", "b"} {
		h := smallVMHost("host_" + id)
		m := demandMachine("mach_"+id, p, store.MachineReady, time.Hour)
		m.HostID = h.ID
		hosts, machines = append(hosts, h), append(machines, m)
	}
	s := MachineSnapshot{
		Now: machineNow, Pools: []*store.Pool{pool}, Providers: []*store.Provider{p},
		Hosts: hosts, Machines: machines, Jobs: demandJobs(2), Limits: demandLimits(),
		Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{demandPoolPlan(pool, 0, 2, 2, "nowhere")}},
	}

	plan := DecideMachines(s)
	if got := planFor(t, plan, p.ID); got.Create != 0 || got.Blocked != "" {
		t.Fatalf("create = %d, blocked = %q; want nothing bought and no ceiling blamed", got.Create, got.Blocked)
	}
	if len(plan.Pools) != 1 {
		t.Fatalf("verdicts = %+v, want one for the pool", plan.Pools)
	}
	v := plan.Pools[0]
	if v.State != RentingUnserved {
		t.Fatalf("state = %q, want %q: the ceiling is not what stops this pool", v.State, RentingUnserved)
	}
	for _, want := range []string{"proxmox-elitedesk-node1", "too small", "6.7 GB", "The 2 machines it owns are the same size"} {
		if !strings.Contains(v.Summary, want) {
			t.Errorf("summary %q does not say %q", v.Summary, want)
		}
	}
	if !strings.Contains(v.Fix, "at least 2 CPUs and 8 GB of memory") {
		t.Errorf("fix %q does not name the size the machines need", v.Fix)
	}
	if strings.Contains(v.Fix, "maximum") {
		t.Errorf("fix %q suggests raising the maximum", v.Fix)
	}
}

// A machine no enabled pool's runner fits on is not waiting for a burst, so it
// goes once the cooldown has passed rather than after the idle timeout as
// well, and its drain is marked as that. One that fits a pool keeps the full
// wait: it is the burst it is being kept for.
func TestAMachineNoPoolFitsIsReleasedWithoutWaitingOutTheIdleTimeout(t *testing.T) {
	for _, tc := range []struct {
		name      string
		host      *store.Host
		idleFor   time.Duration
		wantDrain bool
		wantUnfit bool
	}{
		{name: "too small, idle for the cooldown", host: smallVMHost("host_1"), idleFor: 11 * time.Minute, wantDrain: true, wantUnfit: true},
		{name: "too small, still inside the cooldown", host: smallVMHost("host_1"), idleFor: 5 * time.Minute},
		{name: "large enough, idle for the cooldown only", host: func() *store.Host {
			h := demandHost("host_1", 2, 0)
			h.CPUs, h.MemoryMB = 16, 65536
			return h
		}(), idleFor: 11 * time.Minute},
		{name: "measured nothing yet, idle for the cooldown only", host: demandHost("host_1", 2, 0), idleFor: 11 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, pool := smallProvider("lab"), dindPool("alpha")
			m := demandMachine("mach_1", p, store.MachineReady, 4*time.Hour)
			m.HostID, m.IdleSince = tc.host.ID, ptrTime(machineNow.Add(-tc.idleFor))
			s := MachineSnapshot{
				Now: machineNow, Pools: []*store.Pool{pool}, Hosts: []*store.Host{tc.host},
				Providers: []*store.Provider{p}, Machines: []*store.Machine{m}, Limits: demandLimits(),
				Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{demandPoolPlan(pool, 0, 0, 0, "")}},
			}

			got := planFor(t, DecideMachines(s), p.ID)
			if drained := slices.Contains(got.Drain, m.ID); drained != tc.wantDrain {
				t.Fatalf("drain = %v, want drained = %v", got.Drain, tc.wantDrain)
			}
			if unfit := slices.Contains(got.Unfit, m.ID); unfit != tc.wantUnfit {
				t.Fatalf("unfit = %v, want unfit = %v", got.Unfit, tc.wantUnfit)
			}
		})
	}
}

// The whole of it on a running controller: the first machine is bought because
// nothing has said how big the provider's machines are, it joins and measures
// itself too small for the pool, and from then on the fleet buys nothing more
// for that pool, explains why on every page that asks, and releases the
// machine once the cooldown has passed instead of holding the ceiling with it.
func TestAMachineThatMeasuresTooSmallIsNotBoughtAgainAndIsReleased(t *testing.T) {
	h := newHarness(t)
	pool, row := h.machineFleet(t)
	pool.DockerMode, pool.SizeFromProfile = store.DockerDinD, true
	pool.Resources = store.Resources{MinCPUs: 0.5, MinMemoryMB: 1024, DaemonMemorySharePercent: 15}
	if err := h.st.UpdatePool(h.ctx, pool); err != nil {
		t.Fatalf("UpdatePool: %v", err)
	}

	h.machinePass(t)
	if got := len(h.machines()); got != 1 {
		t.Fatalf("%d machines were bought for one queued job on a provider of unknown size, want 1", got)
	}
	m := h.drive(t, h.onlyMachine(t).ID, store.MachineEnrolling, 8)
	req := joinRequest(m.Name, h.tokenFromBootstrap(t, m))
	req.Capacity, req.CPUs, req.MemoryMB = 2, 2, 3915
	if _, err := h.c.Join(h.ctx, req, "203.0.113.9"); err != nil {
		t.Fatalf("joining as machine %s: %v", m.Name, err)
	}
	h.machinePass(t)
	m = h.drive(t, m.ID, store.MachineReady, 3)

	fresh, err := h.st.GetProvider(h.ctx, row.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if fresh.SeenCPUs != 2 || fresh.SeenMemoryMB != 3915 || fresh.SeenFrom != store.MachineSizeFromMachine {
		t.Fatalf("seen size = %d CPUs, %d MB from %q; want what the machine measured", fresh.SeenCPUs, fresh.SeenMemoryMB, fresh.SeenFrom)
	}

	h.queueWork(t)
	h.queueWork(t)
	h.machinePass(t)
	if got := len(h.machines()); got != 1 {
		t.Fatalf("%d machines after more work queued, want the 1 already bought: its provider's machines cannot run this pool", got)
	}

	pairings, err := h.c.ProviderPairings(h.ctx)
	if err != nil {
		t.Fatalf("ProviderPairings: %v", err)
	}
	if len(pairings) != 1 || pairings[0].Fits || pairings[0].Serves || !strings.Contains(pairings[0].FitWhy, "too small") {
		t.Fatalf("pairings = %+v, want the provider not to fit, for its size", pairings)
	}

	view := h.renting(t)
	if len(view.Pools) != 1 || view.Pools[0].State != RentingUnserved ||
		!strings.Contains(view.Pools[0].Summary, "The machine it owns is the same size") {
		t.Fatalf("renting pools = %+v, want the pool unserved because the machine is too small", view.Pools)
	}

	// pool.no_capacity is the scheduling pass's, with the renting verdict laid
	// over it.
	if err := h.c.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var found bool
	for _, p := range h.problemsOf(t, "pool.no_capacity") {
		if p.TargetID != pool.ID {
			continue
		}
		found = true
		if !strings.Contains(p.Fix, "at least 2 CPUs and 8 GB of memory") || strings.Contains(p.Fix, "maximum machines") {
			t.Errorf("pool.no_capacity fix %q, want the size the machines need and no larger maximum", p.Fix)
		}
	}
	if !found {
		t.Fatal("no pool.no_capacity for a pool with jobs queued and nowhere to run them")
	}
	// The provider's own entry says what is wrong with its machines in the
	// same terms, rather than sending the operator to its labels.
	unservable := h.problemsOf(t, "provider.unservable")
	if len(unservable) != 1 || !strings.Contains(unservable[0].Detail, "too small for pool "+pool.Name) ||
		!strings.Contains(unservable[0].Fix, "at least 2 CPUs and 8 GB of memory") {
		t.Fatalf("provider.unservable = %+v, want it to name the size its machines need", unservable)
	}

	// Past the cooldown, with the host still heartbeating, and well inside the
	// idle timeout plus the cooldown that a machine a pool could use waits for.
	h.advance(h.cfg.Provider.ScaleDownCooldown + time.Minute)
	host := m.HostID
	if err := h.st.Heartbeat(h.ctx, host, h.c.Now()); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	h.machinePass(t)
	got := h.machineByID(t, m.ID)
	if got.State != store.MachineDraining && got.State != store.MachineDeleting && got.State != store.MachineDeleted {
		t.Fatalf("machine is %s after the cooldown, want it released", got.State)
	}
}

// A connection check reads the template, and from then on the provider's next
// machine is judged at the template's size: a template too small for a pool
// does not fit it before a single machine has been bought.
func TestAConnectionCheckRecordsTheSizeTheProviderBuildsAndThePairingJudgesIt(t *testing.T) {
	h := newHarness(t)
	h.enableProviders(t)
	pool := h.pool(h.installation(), "zoomies-linux-x64")
	pool.DockerMode, pool.SizeFromProfile = store.DockerDinD, true
	pool.Resources = store.Resources{MinCPUs: 0.5, MinMemoryMB: 1024, DaemonMemorySharePercent: 15}
	if err := h.st.UpdatePool(h.ctx, pool); err != nil {
		t.Fatalf("UpdatePool: %v", err)
	}
	row := h.providerRow(t, "lab")
	h.fake.SetBuilds(2, 4096)

	if _, err := h.c.CheckProvider(h.ctx, row); err != nil {
		t.Fatalf("CheckProvider: %v", err)
	}
	fresh, err := h.st.GetProvider(h.ctx, row.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if fresh.SeenCPUs != 2 || fresh.SeenMemoryMB != 4096 || fresh.SeenFrom != store.MachineSizeFromTemplate {
		t.Fatalf("seen = %d CPUs, %d MB from %q; want the template's", fresh.SeenCPUs, fresh.SeenMemoryMB, fresh.SeenFrom)
	}

	pairings, err := h.c.ProviderPairings(h.ctx)
	if err != nil {
		t.Fatalf("ProviderPairings: %v", err)
	}
	if len(pairings) != 1 || pairings[0].Fits {
		t.Fatalf("pairings = %+v, want the template too small for the pool", pairings)
	}
	if want := "the template it clones gives 2 CPUs and 4 GB of memory"; !strings.Contains(pairings[0].FitWhy, want) {
		t.Errorf("fit_why %q does not say %q", pairings[0].FitWhy, want)
	}
	if !strings.Contains(pairings[0].Fix, "at least 2 CPUs and 8 GB of memory") {
		t.Errorf("fix %q does not name the size the template needs", pairings[0].Fix)
	}

	h.queueWork(t)
	h.machinePass(t)
	if got := len(h.machines()); got != 0 {
		t.Fatalf("%d machines were bought from a template too small for the pool", got)
	}
}
