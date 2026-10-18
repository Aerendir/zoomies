package store

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// A claim asks which pool takes a job, and answers from a handful of a pool's
// fields. Reading the other thirty of every row, and decoding ten JSON
// documents in each, was most of what a webhook delivery cost at a hundred
// pools, so a claim reads the handful, and the pools it gets carry only those.
// ClaimView says which they are, so that what a read returns and what a test
// strips a whole pool to cannot drift apart.

func TestListPoolsForClaimReturnsWhatAClaimReadsOfEveryPool(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	one := &Installation{AppID: 1, InstallationID: 2, Target: "acme", TargetType: TargetOrg}
	two := &Installation{AppID: 1, InstallationID: 3, Target: "widgets", TargetType: TargetOrg}
	for _, inst := range []*Installation{one, two} {
		if err := s.CreateInstallation(ctx, inst); err != nil {
			t.Fatalf("CreateInstallation: %v", err)
		}
	}

	// Pools that differ in the fields a claim reads and, as they do in a real
	// fleet, in a good many it does not.
	pools := []*Pool{
		{Name: "linux-x64", InstallationID: one.ID, Labels: StringSlice{"linux", "docker"},
			Backend: BackendDocker, Platform: Platform{OS: "ubuntu", OSVersion: "24.04", Arch: "amd64"},
			MaxRunners: 4, Enabled: true, Ephemeral: true, Image: "ghcr.io/example/runner:1", DockerMode: DockerNone},
		{Name: "arm", InstallationID: one.ID, Labels: StringSlice{"arm64"}, NoDefaultLabels: true,
			Backend: BackendProcess, Platform: Platform{Arch: "arm64"}, MaxRunners: 2, Enabled: true, DockerMode: DockerNone,
			Env: StringMap{"A": "b"}, HostSelector: StringMap{"zone": "a"}},
		{Name: "kept", InstallationID: two.ID, Labels: StringSlice{"gpu"}, AutoKey: "arm64/large",
			Backend: BackendDocker, Platform: Platform{Arch: "arm64"}, MaxRunners: 8, Enabled: false, DockerMode: DockerNone,
			Resources: Resources{CPUs: 8, MemoryMB: 16384}, ProviderSelector: StringMap{"kind": "proxmox"}},
		{Name: "bare", InstallationID: two.ID, Backend: BackendDocker, MaxRunners: 1, Enabled: true, DockerMode: DockerNone},
	}
	for _, p := range pools {
		if err := s.CreatePool(ctx, p); err != nil {
			t.Fatalf("CreatePool %s: %v", p.Name, err)
		}
	}

	whole, err := s.ListPools(ctx)
	if err != nil {
		t.Fatalf("ListPools: %v", err)
	}
	got, err := s.ListPoolsForClaim(ctx)
	if err != nil {
		t.Fatalf("ListPoolsForClaim: %v", err)
	}
	if len(got) != len(whole) || len(whole) != len(pools) {
		t.Fatalf("ListPoolsForClaim returned %d pools, ListPools %d, and %d were made", len(got), len(whole), len(pools))
	}
	for i, p := range whole {
		if want := ClaimView(p); !reflect.DeepEqual(got[i], want) {
			t.Errorf("pool %s as read for a claim is\n%+v\nwant the claim view of the whole pool\n%+v", p.Name, got[i], want)
		}
	}
}

func TestAClaimViewKeepsWhatAClaimReadsAndNothingElse(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	p := &Pool{
		ID: "pool_1", Name: "gpu", InstallationID: "inst_1", Labels: StringSlice{"gpu", "linux"},
		Backend: BackendDocker, Platform: Platform{OS: "ubuntu", OSVersion: "24.04", Arch: "arm64"},
		AutoKey: "arm64/large", Enabled: true, NoDefaultLabels: true,
		MaxRunners: 8, Priority: 3, Image: "img", Env: StringMap{"A": "b"}, CreatedAt: now, UpdatedAt: now,
	}
	want := &Pool{
		ID: "pool_1", Name: "gpu", InstallationID: "inst_1", Labels: StringSlice{"gpu", "linux"},
		Backend: BackendDocker, Platform: Platform{Arch: "arm64"},
		AutoKey: "arm64/large", Enabled: true, NoDefaultLabels: true,
	}
	if got := ClaimView(p); !reflect.DeepEqual(got, want) {
		t.Errorf("ClaimView = %+v, want %+v", got, want)
	}
	if p.MaxRunners != 8 || p.Image != "img" || p.Platform.OS != "ubuntu" {
		t.Errorf("ClaimView changed the pool it was given: %+v", p)
	}
}
