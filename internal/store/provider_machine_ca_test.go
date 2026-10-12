package store

import (
	"context"
	"database/sql"
	"testing"
)

// What this controller trusts when it reaches a hypervisor, and what a machine
// built there trusts when it reaches this controller, are two different
// certificates in general. One field answered both, so a cluster's own
// authority became the only thing every machine would accept from the
// controller, and a controller on a public certificate was refused by every
// machine it rented. They are stored apart, and changing one leaves the other.
func TestAProviderKeepsTheCertificateItsMachinesTrustApartFromItsOwn(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := &Provider{Kind: ProviderFake, Name: "lab", CAPEM: "the-clusters-authority", MachineCAPEM: "the-controllers-authority"}
	if err := s.CreateProvider(ctx, p); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	got, err := s.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.CAPEM != "the-clusters-authority" || got.MachineCAPEM != "the-controllers-authority" {
		t.Fatalf("the provider came back with ca_pem %q and machine_ca_pem %q", got.CAPEM, got.MachineCAPEM)
	}

	got.MachineCAPEM = ""
	if err := s.UpdateProvider(ctx, got); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	again, err := s.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if again.MachineCAPEM != "" {
		t.Errorf("clearing the machines' certificate left %q", again.MachineCAPEM)
	}
	if again.CAPEM != "the-clusters-authority" {
		t.Errorf("clearing the machines' certificate changed the provider's own to %q", again.CAPEM)
	}

	again.MachineCAPEM = "a-new-controller-authority"
	again.CAPEM = "a-new-cluster-authority"
	if err := s.UpdateProvider(ctx, again); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	last, err := s.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if last.CAPEM != "a-new-cluster-authority" || last.MachineCAPEM != "a-new-controller-authority" {
		t.Errorf("an edit of both came back as ca_pem %q and machine_ca_pem %q", last.CAPEM, last.MachineCAPEM)
	}
}

// Until now a provider's certificate was also the one its machines trusted, so
// a provider already set up that way has to go on working after the upgrade: the
// machines it builds next must be given what the machines before them were. The
// copy preserves that, and a provider with no certificate gets none, so a
// machine of its own goes on using the system's trust.
func TestAnUpgradeGivesAnExistingProviderItsCertificateAsTheOneItsMachinesTrust(t *testing.T) {
	ctx := context.Background()
	path := atSchemaBefore(t, "0101_provider_machine_ca.sql")

	const authority = "-----BEGIN CERTIFICATE-----\nthe-clusters-authority\n-----END CERTIFICATE-----"
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	for _, row := range []struct{ id, name, ca string }{
		{"prv_with", "with-authority", authority},
		{"prv_without", "without-authority", ""},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO providers (id, kind, name, endpoint, ca_pem, created_at, updated_at)
			VALUES (?, 'fake', ?, 'https://pve.example:8006', ?, 0, 0)`, row.id, row.name, row.ca); err != nil {
			t.Fatalf("writing %s as an older release did: %v", row.id, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the fixture: %v", err)
	}

	s, err := Open(ctx, Options{Path: path})
	if err != nil {
		t.Fatalf("opening the older database: %v", err)
	}
	defer s.Close()

	with, err := s.GetProvider(ctx, "prv_with")
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if with.MachineCAPEM != authority {
		t.Errorf("a provider with a certificate came out of the upgrade with machine_ca_pem %q", with.MachineCAPEM)
	}
	if with.CAPEM != authority {
		t.Errorf("the upgrade changed the provider's own certificate to %q", with.CAPEM)
	}

	without, err := s.GetProvider(ctx, "prv_without")
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if without.MachineCAPEM != "" {
		t.Errorf("a provider with no certificate came out of the upgrade with machine_ca_pem %q", without.MachineCAPEM)
	}
}
