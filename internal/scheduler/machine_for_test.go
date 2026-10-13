package scheduler

import (
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// dindProfilePool is the pool a live fleet rented the wrong machines for:
// docker in docker, sized by the host's profile, with a minimum of half a core
// and 1 GB a container and the daemon given 15% of a slot's memory. The thinner
// half of its memory split is the daemon's, so a slot has to be 1 GB / 0.15,
// nearly 6.7 GB, before the daemon has its gigabyte.
func dindProfilePool() *store.Pool {
	return &store.Pool{
		ID: "pool_linux", Name: "zoomies-linux-x64", Enabled: true,
		Backend: store.BackendDocker, DockerMode: store.DockerDinD, SizeFromProfile: true,
		Resources: store.Resources{MinCPUs: 0.5, MinMemoryMB: 1024, DaemonMemorySharePercent: 15},
	}
}

// smallVM is the machine that provider built: 2 vCPU and the 3915 MB a 4 GB
// guest measures itself as, cut into two slots.
func smallVM() *store.Host {
	return &store.Host{
		ID: "host_vm", Name: "vm", Capacity: 2,
		Backends: store.StringSlice{"docker"}, CPUs: 2, MemoryMB: 3915,
	}
}

// The size a refusal names has to be one placement would accept, and no
// larger than it needs to be: an operator who orders it should find the pool
// runs there, and one told to order twice what is needed stops trusting the
// number.
func TestTheMachineNamedForAPoolIsTheSmallestPlacementAccepts(t *testing.T) {
	pool, vm := dindProfilePool(), smallVM()
	if HostFits(vm, pool) {
		t.Fatal("the 3.9 GB machine fits, so there is nothing for this test to say")
	}

	cpus, memoryMB, ok := MachineFor(vm, pool)
	if !ok {
		t.Fatal("no machine was named for a pool a larger machine plainly runs")
	}
	if cpus != 2 || memoryMB != 8192 {
		t.Fatalf("named %d CPUs and %d MB, want 2 CPUs and 8 GB: two cores already cover the CPU, and 6.7 GB after the reserve rounds up to 8", cpus, memoryMB)
	}
	bigger := *vm
	bigger.CPUs, bigger.MemoryMB = cpus, memoryMB
	if !HostFits(&bigger, pool) {
		t.Fatalf("the machine named, %d CPUs and %d MB, does not fit the pool either", cpus, memoryMB)
	}
	smaller := bigger
	smaller.MemoryMB -= 1024
	if HostFits(&smaller, pool) {
		t.Fatalf("a gigabyte less than the machine named still fits, so %d MB was more than the pool needs", memoryMB)
	}
}

// A machine that already fits has no size to name, and one whose agent has
// measured nothing is placed by slots alone, so there is nothing to grow.
func TestNoMachineIsNamedWhereTheHostAlreadyFits(t *testing.T) {
	for _, tc := range []struct {
		name string
		host *store.Host
	}{
		{"a machine large enough", &store.Host{Capacity: 2, Backends: store.StringSlice{"docker"}, CPUs: 16, MemoryMB: 65536}},
		{"a machine that measured nothing", &store.Host{Capacity: 2, Backends: store.StringSlice{"docker"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := MachineFor(tc.host, dindProfilePool()); ok {
				t.Fatal("a size was named for a machine the pool already fits")
			}
		})
	}
}

// "Twice over" is right for an even split and wrong by more than three times
// for a daemon given 15%: an operator reading it beside a 1 GB minimum would
// expect a 4 GB machine to do, and it never will.
func TestAnUnevenDaemonSplitIsNotCalledTwiceOver(t *testing.T) {
	got := HostShortfall(smallVM(), dindProfilePool())
	if strings.Contains(got, "twice over") {
		t.Fatalf("the shortfall says twice over for a daemon given 15%%: %q", got)
	}
	for _, want := range []string{"3.3 GB of memory to place on", "6.7 GB", "15% of a slot"} {
		if !strings.Contains(got, want) {
			t.Errorf("the shortfall %q does not say %q", got, want)
		}
	}
}
