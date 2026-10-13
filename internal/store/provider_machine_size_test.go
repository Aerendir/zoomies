package store

import (
	"context"
	"testing"
	"time"
)

// The size a provider's machines were seen to be is evidence about the machines
// it builds now, so an older observation never overrules a newer one: a machine
// built before the template was enlarged would otherwise put the old size back
// on every pass, and the fleet would refuse the larger machines it could now
// rent. Writing it is an observation, so the provider's configuration time
// does not move, and the problems list does not read "settings changed".
func TestAProvidersSeenMachineSizeIsOnlyEverReplacedByNewerEvidence(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := &Provider{Kind: ProviderFake, Name: "lab"}
	if err := s.CreateProvider(ctx, p); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	created, err := s.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}

	built := time.Date(2026, 10, 12, 16, 0, 0, 0, time.UTC)
	checked := built.Add(time.Hour)
	for _, step := range []struct {
		name      string
		cpus      int
		memoryMB  int64
		from      string
		at        time.Time
		wantWrote bool
	}{
		{"a machine measures itself", 2, 3915, MachineSizeFromMachine, built, true},
		{"the same machine says the same again", 2, 3915, MachineSizeFromMachine, built, false},
		{"a check reads an enlarged template", 4, 8192, MachineSizeFromTemplate, checked, true},
		{"the older machine is read again", 2, 3915, MachineSizeFromMachine, built, false},
	} {
		wrote, err := s.SetProviderMachineSizeSeen(ctx, p.ID, step.cpus, step.memoryMB, step.from, step.at)
		if err != nil {
			t.Fatalf("%s: SetProviderMachineSizeSeen: %v", step.name, err)
		}
		if wrote != step.wantWrote {
			t.Fatalf("%s: wrote = %v, want %v", step.name, wrote, step.wantWrote)
		}
	}

	got, err := s.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.SeenCPUs != 4 || got.SeenMemoryMB != 8192 || got.SeenFrom != MachineSizeFromTemplate ||
		got.SeenAt == nil || !got.SeenAt.Equal(checked) {
		t.Fatalf("seen = %d CPUs, %d MB from %q at %v; want the check's 4 CPUs and 8 GB", got.SeenCPUs, got.SeenMemoryMB, got.SeenFrom, got.SeenAt)
	}
	if !got.UpdatedAt.Equal(created.UpdatedAt) {
		t.Errorf("updated_at moved from %v to %v for an observation", created.UpdatedAt, got.UpdatedAt)
	}
}
