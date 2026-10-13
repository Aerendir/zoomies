package store

import (
	"context"
	"testing"
)

// A provider's minimum is its own setting: it is kept through a save, an edit
// changes it and nothing else, and a provider that never had one keeps none.
func TestAProviderKeepsItsMinimumMachines(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := &Provider{Kind: ProviderFake, Name: "lab", MaxMachines: 5, MinMachines: 2}
	if err := s.CreateProvider(ctx, p); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	got, err := s.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.MinMachines != 2 || got.MaxMachines != 5 {
		t.Fatalf("the provider came back with min %d and max %d, want 2 and 5", got.MinMachines, got.MaxMachines)
	}

	got.MinMachines = 0
	if err := s.UpdateProvider(ctx, got); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	again, err := s.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if again.MinMachines != 0 || again.MaxMachines != 5 {
		t.Errorf("clearing the minimum left min %d and max %d, want 0 and 5", again.MinMachines, again.MaxMachines)
	}

	none := &Provider{Kind: ProviderFake, Name: "plain"}
	if err := s.CreateProvider(ctx, none); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if got, _ := s.GetProvider(ctx, none.ID); got == nil || got.MinMachines != 0 {
		t.Errorf("a provider that never set one has %+v, want none", got)
	}
}
