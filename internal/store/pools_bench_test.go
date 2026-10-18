package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Every reconcile pass lists the pools whole, and the pages that show them read
// them again, so what a pool row costs to read is paid continuously. A row is
// thirty-nine columns and ten JSON documents, and most of the documents are what
// a column holds until somebody sets it. These time the two reads at three fleet
// sizes, for pools as they are made when nobody has configured anything and for
// pools that carry a good deal of configuration, so that a change to how a row
// is decoded is shown to help the first without costing the second.

// benchPoolShape is how much of a pool an operator has set.
type benchPoolShape struct {
	name string
	make func(inst *Installation, i int) *Pool
}

// defaultBenchPool is a pool as it is made when nobody has configured anything.
func defaultBenchPool(inst *Installation, i int) *Pool {
	return &Pool{
		Name: fmt.Sprintf("pool%03d", i), InstallationID: inst.ID,
		Labels:  StringSlice{"self-hosted", "linux", "x64", fmt.Sprintf("pool%03d", i)},
		Backend: BackendDocker, Image: "ghcr.io/example/runner:1", MaxRunners: 4,
		IdleTimeout: Duration(5 * time.Minute), Ephemeral: true, DockerMode: DockerNone, Enabled: true,
	}
}

// configuredBenchPool carries a setting in every document a pool keeps.
func configuredBenchPool(inst *Installation, i int) *Pool {
	p := defaultBenchPool(inst, i)
	p.Resources = Resources{CPUs: 4, MemoryMB: 8192}
	p.Cache = CacheConfig{Enabled: true, Scope: CacheScopeRepository}
	p.Tmpfs = TmpfsConfig{Work: TmpfsMount{Enabled: true}, Tmp: TmpfsMount{Enabled: true}}
	p.Env = StringMap{"CI": "true", "REGION": "eu"}
	p.HostSelector = StringMap{"zone": "a"}
	p.ProviderSelector = StringMap{"kind": "proxmox"}
	p.CPUBurst = CPUBurstPolicy{Mode: CPUBurstObserve}
	return p
}

var benchPoolShapes = []benchPoolShape{
	{name: "default", make: defaultBenchPool},
	{name: "configured", make: configuredBenchPool},
}

// benchPoolStore is a store holding n pools of a shape, and checks it holds them.
func benchPoolStore(b *testing.B, shape benchPoolShape, n int) (*Store, string) {
	b.Helper()
	ctx := context.Background()
	s := newTestStore(b)
	inst := &Installation{AppID: 1, InstallationID: 2, Target: "acme", TargetType: TargetOrg}
	if err := s.CreateInstallation(ctx, inst); err != nil {
		b.Fatalf("CreateInstallation: %v", err)
	}
	var id string
	for i := range n {
		p := shape.make(inst, i)
		if err := s.CreatePool(ctx, p); err != nil {
			b.Fatalf("CreatePool: %v", err)
		}
		id = p.ID
	}
	if got, err := s.ListPools(ctx); err != nil || len(got) != n {
		b.Fatalf("the store holds %d pools, want %d: %v", len(got), n, err)
	}
	return s, id
}

func BenchmarkListPools(b *testing.B) {
	ctx := context.Background()
	for _, shape := range benchPoolShapes {
		b.Run(shape.name, func(b *testing.B) {
			for _, n := range []int{4, 20, 100} {
				b.Run(fmt.Sprintf("pools=%d", n), func(b *testing.B) {
					s, _ := benchPoolStore(b, shape, n)
					b.ReportAllocs()
					for b.Loop() {
						if _, err := s.ListPools(ctx); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

func BenchmarkGetPool(b *testing.B) {
	ctx := context.Background()
	for _, shape := range benchPoolShapes {
		b.Run(shape.name, func(b *testing.B) {
			s, id := benchPoolStore(b, shape, 1)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := s.GetPool(ctx, id); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
