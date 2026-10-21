package scheduler

import (
	"slices"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// These are web/unit/recommend.test.ts's cases, so the dialog's "Set to
// recommendations" and the controller's automatic adjustment cannot drift into
// recommending different things for the same machine.

func pool(name string, cpus float64, mem int64, mode store.DockerMode) *store.Pool {
	return &store.Pool{Name: name, Enabled: true, DockerMode: mode, Resources: store.Resources{CPUs: cpus, MemoryMB: mem}}
}

func TestTheRunnerAskIsTheLargestEnabledPoolOrTheDefault(t *testing.T) {
	if got := AskOf(nil, 0, 0); got != DefaultRunnerAsk {
		t.Fatalf("default: %+v", got)
	}
	disabled := pool("big", 16, 65536, "")
	disabled.Enabled = false
	got := AskOf([]*store.Pool{pool("a", 2, 4096, ""), pool("b", 4, 2048, ""), disabled}, 0, 0)
	if got.CPUs != 4 || got.MemoryMB != 4096 || got.Pair {
		t.Fatalf("largest of each: %+v", got)
	}
	// The fleet's own default stands in for a figure no pool sets.
	if got := AskOf(nil, 3, 6144); got.CPUs != 3 || got.MemoryMB != 6144 {
		t.Fatalf("fleet default: %+v", got)
	}
}

func TestADockerInDockerPoolAsksForBothContainersOnlyWhenItSizedThem(t *testing.T) {
	if got := AskOf([]*store.Pool{pool("d", 0, 0, store.DockerDinD)}, 0, 0); got.CPUs != 2 || got.MemoryMB != 4096 || got.Pair {
		t.Fatalf("a pool that leaves its size to the host asks for one runner: %+v", got)
	}
	got := AskOf([]*store.Pool{pool("d", 2, 4096, store.DockerDinD)}, 0, 0)
	if got.CPUs != 4 || got.MemoryMB != 8192 || !got.Pair {
		t.Fatalf("a sized pair: %+v", got)
	}
	host := &store.Host{CPUs: 12, MemoryMB: 32768}
	if r, _ := RecommendHost(host, got); r.ReserveCPUs != 1 || r.Capacity != 2 {
		t.Fatalf("twelve cores less a core hold two pairs: %+v", r)
	}
	if r, _ := RecommendHost(host, AskOf([]*store.Pool{pool("d", 0, 0, store.DockerDinD)}, 0, 0)); r.Capacity != 5 {
		t.Fatalf("the same pool sized by its host holds five: %+v", r)
	}
}

func TestTheReserveKeepsACoreAndATenthAndNeverTheWholeMachine(t *testing.T) {
	for cpus, want := range map[int]int{1: 0, 2: 1, 16: 1, 48: 3} {
		if got := ReserveCPUsFor(cpus); got != want {
			t.Errorf("ReserveCPUsFor(%d) = %d, want %d", cpus, got, want)
		}
	}
	for mem, want := range map[int64]int64{1024: 0, 1536: 1024, 32768: 3072, 16384: 2048, 262144: 8192} {
		if got := ReserveMemoryMBFor(mem); got != want {
			t.Errorf("ReserveMemoryMBFor(%d) = %d, want %d", mem, got, want)
		}
	}
	for disk, want := range map[int64]int64{4096: 0, 102_400: 8192, 1_048_576: 65_536} {
		if got := ReserveDiskMBFor(disk); got != want {
			t.Errorf("ReserveDiskMBFor(%d) = %d, want %d", disk, got, want)
		}
	}
	if got := diskNotches(65_536); !slices.Equal(got, []int64{0, 2048, 4096, 8192, 16_384, 32_768}) {
		t.Errorf("disk notches: %v", got)
	}
}

func TestCapacityIsWhatFitsOnTheTightestOfCPUMemoryAndDisk(t *testing.T) {
	big := &store.Host{CPUs: 16, MemoryMB: 32768}
	if r, ok := RecommendHost(big, DefaultRunnerAsk); !ok || r.Capacity != 7 || r.LimitedBy != "cpu" && r.LimitedBy != "memory" {
		t.Fatalf("15 cores / 2 and 29.7 GB / 4 GB are both 7: %+v", r)
	}
	if r, _ := RecommendHost(big, RunnerAsk{CPUs: 1, MemoryMB: 8192}); r.Capacity != 3 || r.LimitedBy != "memory" {
		t.Fatalf("memory is the tighter one here: %+v", r)
	}
	if _, ok := RecommendHost(&store.Host{}, DefaultRunnerAsk); ok {
		t.Fatal("a host that has said nothing about itself gets no recommendation")
	}
	if r, _ := RecommendHost(&store.Host{CPUs: 2, MemoryMB: 2048}, DefaultRunnerAsk); r.Capacity != 1 {
		t.Fatalf("never zero on a machine, however small: %+v", r)
	}
}

// The failure this exists for: a rented machine with plenty of cores and
// memory and a small disk. Cores and memory alone say sixteen runners; the disk
// holds three.
func TestASmallDiskLimitsHowManyRunnersAHostIsToldToTake(t *testing.T) {
	h := &store.Host{CPUs: 36, MemoryMB: 131072, DiskTotalMB: 40 * 1024}
	r, ok := RecommendHost(h, DefaultRunnerAsk)
	if !ok || r.LimitedBy != "disk" {
		t.Fatalf("the disk should set the capacity: %+v", r)
	}
	if want := int((h.DiskTotalMB - r.ReserveDiskMB) / RunnerDiskMB); r.Capacity != want || r.Capacity >= 16 {
		t.Fatalf("capacity %d, want %d (disk-bound)", r.Capacity, want)
	}
	if r.Reason() == "" {
		t.Fatal("a recommendation must say why")
	}
}

// The recommender and placement read one constant, so a host told it can
// hold N runners is a host the pass will place N runners on.
func TestPlacementAndTheRecommenderAgreeOnAWorkingRoom(t *testing.T) {
	h := &store.Host{Capacity: 8, Backends: store.StringSlice{"docker"}, CPUs: 36, MemoryMB: 131072,
		DiskTotalMB: 40 * 1024, DiskFreeMB: 40 * 1024}
	r, ok := RecommendHost(h, DefaultRunnerAsk)
	if !ok || r.LimitedBy != "disk" {
		t.Fatalf("the disk should set the capacity: %+v", r)
	}
	if got := Reserve(&store.Pool{Enabled: true, Backend: store.BackendDocker}, h).DiskMB; got != RunnerDiskMB {
		t.Fatalf("placement charges %d MB, the recommender assumed %d", got, RunnerDiskMB)
	}
}
