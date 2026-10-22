package store

import (
	"context"
	"testing"
)

// BenchmarkFindInstallationByTarget is the question asked twice of every webhook
// delivery and once for each repository of a poller sweep, with an organisation's
// installation, two for single repositories, and a repository the organisation's
// covers. The driver parses the statement every time it is run unless the store
// has prepared it.
func BenchmarkFindInstallationByTarget(b *testing.B) {
	ctx := context.Background()
	s := newTestStore(b)
	var org *Installation
	for _, in := range []Installation{
		{AppID: 1, InstallationID: 2, Target: "acme", TargetType: TargetOrg},
		{AppID: 1, InstallationID: 3, Target: "acme/special", TargetType: TargetRepo},
		{AppID: 1, InstallationID: 4, Target: "other/lib", TargetType: TargetRepo},
	} {
		if err := s.CreateInstallation(ctx, &in); err != nil {
			b.Fatalf("CreateInstallation: %v", err)
		}
		if in.Target == "acme" {
			org = &in
		}
	}
	if got, err := s.FindInstallationByTarget(ctx, "acme/widgets"); err != nil || got.ID != org.ID {
		b.Fatalf("acme/widgets resolved to %+v, %v; want the organisation's installation", got, err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.FindInstallationByTarget(ctx, "acme/widgets"); err != nil {
			b.Fatal(err)
		}
	}
}
