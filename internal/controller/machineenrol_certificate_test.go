package controller

import (
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/provider"
	"github.com/eyupio/zoomies/internal/store"
)

// fileAt is the file written to a path in the guest, or nil.
func fileAt(payload provider.Bootstrap, path string) *provider.File {
	for i := range payload.Files {
		if payload.Files[i].Path == path {
			return &payload.Files[i]
		}
	}
	return nil
}

func enrolFor(row *store.Provider) provider.Bootstrap {
	m := &store.Machine{ID: "mach_abc", Name: "zoomies-mach-abc", Capacity: 2}
	cfg := config.Default()
	cfg.Server.ExternalURL = "https://zoomies.test/"
	return bootstrapPayload(m, row, "zoojoin_abc_secret", cfg)
}

// A machine verifies the controller against the file it is given and nothing
// else, so what it is given has to be a certificate for the controller. The
// provider's own certificate is the hypervisor's authority; it used to be
// written here as well, which made it the only thing a machine would accept
// from a controller that the hypervisor's authority never signed.
func TestAMachineIsGivenTheCertificateForTheControllerNotTheHypervisorsOwn(t *testing.T) {
	clusters := testCertificatePEM(t, "pve-root-ca")
	controllers := testCertificatePEM(t, "controller-ca")
	row := &store.Provider{Name: "lab", CAPEM: clusters, MachineCAPEM: controllers}

	payload := enrolFor(row)
	file := fileAt(payload, machineCAPath)
	if file == nil {
		t.Fatalf("no certificate was written to %s, though the provider has one for its machines", machineCAPath)
	}
	if string(file.Content) != controllers {
		t.Errorf("the machine was given %q, want the certificate for the controller", truncateForTest(string(file.Content)))
	}
	if strings.Contains(string(file.Content), strings.TrimSpace(clusters)) {
		t.Error("the machine was given the hypervisor's authority as well")
	}
	if got := envOf(t, payload)["ZOOMIES_AGENT_CA_FILE"]; got != machineCAPath {
		t.Errorf("ZOOMIES_AGENT_CA_FILE is %q, want %q", got, machineCAPath)
	}
}

// The usual case: a controller on a public certificate, and a hypervisor with
// an authority of its own. The machine has to be left on its own trust store, or
// it refuses the controller and never enrols.
func TestAMachineIsLeftOnItsOwnTrustWhenTheProviderOnlyHasAHypervisorCertificate(t *testing.T) {
	row := &store.Provider{Name: "lab", CAPEM: testCertificatePEM(t, "pve-root-ca")}

	payload := enrolFor(row)
	if file := fileAt(payload, machineCAPath); file != nil {
		t.Errorf("a certificate was written to %s though none was set for the machines: %q", machineCAPath, truncateForTest(string(file.Content)))
	}
	if got, ok := envOf(t, payload)["ZOOMIES_AGENT_CA_FILE"]; ok {
		t.Errorf("ZOOMIES_AGENT_CA_FILE is %q, want it absent so the machine uses its own trust store", got)
	}
}

// The text is copied into every machine that is built, so what is not a
// certificate must not go: a private key pasted into the field by mistake would
// be written into each guest, readable by the jobs that run there.
func TestAMachineIsNeverGivenSomethingThatIsNotACertificate(t *testing.T) {
	key := "-----BEGIN PRIVATE KEY-----\nbm90IGEgcmVhbCBrZXk=\n-----END PRIVATE KEY-----\n"
	for name, text := range map[string]string{
		"a private key":                   key,
		"a certificate followed by a key": testCertificatePEM(t, "controller-ca") + key,
		"text that is no certificate":     "this is not a certificate",
		"a block that does not parse":     "-----BEGIN CERTIFICATE-----\nbm90IGEgY2VydGlmaWNhdGU=\n-----END CERTIFICATE-----\n",
	} {
		t.Run(name, func(t *testing.T) {
			payload := enrolFor(&store.Provider{Name: "lab", MachineCAPEM: text})
			for _, f := range payload.Files {
				if strings.Contains(string(f.Content), "PRIVATE KEY") {
					t.Fatalf("%s was written into the guest at %s", name, f.Path)
				}
			}
			if file := fileAt(payload, machineCAPath); file != nil {
				t.Errorf("%s was written to %s as the machine's certificate for the controller", name, machineCAPath)
			}
			if _, ok := envOf(t, payload)["ZOOMIES_AGENT_CA_FILE"]; ok {
				t.Errorf("the machine was told to trust a file that %s would have filled", name)
			}
		})
	}
}
