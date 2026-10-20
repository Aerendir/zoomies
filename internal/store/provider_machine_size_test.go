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
		diskMB    int64
		from      string
		at        time.Time
		wantWrote bool
	}{
		{"a machine measures itself", 2, 3915, 20480, MachineSizeFromMachine, built, true},
		{"the same machine says the same again", 2, 3915, 20480, MachineSizeFromMachine, built, false},
		{"a check reads an enlarged template", 4, 8192, 32768, MachineSizeFromTemplate, checked, true},
		{"the older machine is read again", 2, 3915, 20480, MachineSizeFromMachine, built, false},
	} {
		wrote, err := s.SetProviderMachineSizeSeen(ctx, p.ID, step.cpus, step.memoryMB, step.diskMB, step.from, step.at)
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
	if got.SeenCPUs != 4 || got.SeenMemoryMB != 8192 || got.SeenDiskMB != 32768 || got.SeenFrom != MachineSizeFromTemplate ||
		got.SeenAt == nil || !got.SeenAt.Equal(checked) {
		t.Fatalf("seen = %d CPUs, %d MB memory, %d MB disk from %q at %v; want the check's 4 CPUs, 8 GB and 32 GB disk", got.SeenCPUs, got.SeenMemoryMB, got.SeenDiskMB, got.SeenFrom, got.SeenAt)
	}
	if !got.UpdatedAt.Equal(created.UpdatedAt) {
		t.Errorf("updated_at moved from %v to %v for an observation", created.UpdatedAt, got.UpdatedAt)
	}
}

// The seen size is evidence about the machines the provider built at the shape
// it had then. An edit that changes the shape (the size typed on the form, or
// the settings that pick the template) makes it evidence about some other
// machine, so it is forgotten; an edit to anything else keeps it.
func TestAProvidersSeenSizeIsForgottenWhenItsMachineShapeChanges(t *testing.T) {
	for _, tc := range []struct {
		name       string
		edit       func(*Provider)
		wantForgot bool
	}{
		{"its maximum machines", func(p *Provider) { p.MaxMachines = 9 }, false},
		{"its typed size cleared for the template", func(p *Provider) { p.MachineCPUs, p.MachineMemoryMB = 0, 0 }, true},
		{"its template", func(p *Provider) { p.Settings = StringMap{"template_id": "9200"} }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			p := &Provider{Kind: ProviderFake, Name: "lab", MachineCPUs: 8, MachineMemoryMB: 16384,
				Settings: StringMap{"template_id": "9100"}, MaxMachines: 2}
			if err := s.CreateProvider(ctx, p); err != nil {
				t.Fatalf("CreateProvider: %v", err)
			}
			if _, err := s.SetProviderMachineSizeSeen(ctx, p.ID, 8, 15800, 40960, MachineSizeFromMachine, time.Now()); err != nil {
				t.Fatalf("SetProviderMachineSizeSeen: %v", err)
			}
			row, err := s.GetProvider(ctx, p.ID)
			if err != nil {
				t.Fatalf("GetProvider: %v", err)
			}
			tc.edit(row)
			if err := s.UpdateProvider(ctx, row); err != nil {
				t.Fatalf("UpdateProvider: %v", err)
			}
			got, err := s.GetProvider(ctx, p.ID)
			if err != nil {
				t.Fatalf("GetProvider: %v", err)
			}
			forgot := got.SeenCPUs == 0 && got.SeenMemoryMB == 0 && got.SeenDiskMB == 0 && got.SeenFrom == "" && got.SeenAt == nil
			kept := got.SeenCPUs == 8 && got.SeenMemoryMB == 15800 && got.SeenDiskMB == 40960 && got.SeenAt != nil
			if tc.wantForgot && !forgot || !tc.wantForgot && !kept {
				t.Fatalf("seen = %d CPUs, %d MB memory, %d MB disk from %q at %v; want forgotten = %v", got.SeenCPUs, got.SeenMemoryMB, got.SeenDiskMB, got.SeenFrom, got.SeenAt, tc.wantForgot)
			}
		})
	}
}
