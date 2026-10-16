package installer

import (
	"testing"

	"github.com/eyupio/zoomies/internal/machine"
)

// A host's default name is built in two live places: Detection.HostName, which
// `zoomies agent join` uses, and machine.DefaultHostName (through Facts.Spec),
// which the configuration and the installer's environment use. They read the
// same facts through different types, and a third helper in internal/naming that
// also built one, and shortened the hostname where these two do not, was never
// called by anything. It is gone; this keeps the two that remain from drifting,
// including for a distribution the grammar has no name for.
func TestTheTwoLiveHostNameBuildersGiveTheSameName(t *testing.T) {
	for _, tc := range []struct{ distro, version string }{
		{"ubuntu", "24.04"}, {"debian", "13"}, {"fedora", "42"}, {"rocky", "9"},
		{"alpine", "3.20"}, {"macos", "15"}, {"windows", "11"},
		// Not names the grammar knows, and nothing at all.
		{"arch", ""}, {"nixos", "24.11"}, {"", ""},
	} {
		for _, host := range []string{"ci-1", "Build Box 01", "build01.fleet.example.com"} {
			detection := Detection{OS: "linux", Arch: "amd64", Distro: tc.distro, OSVersion: tc.version, CPUs: 8, MemoryMB: 16384, Hostname: host}
			facts := machine.Facts{OS: "linux", Arch: "amd64", Distro: tc.distro, OSVersion: tc.version, CPUs: 8, MemoryMB: 16384}
			if a, b := detection.HostName(), facts.Spec(host).String(); a != b {
				t.Errorf("distro %q, host %q: the installer names it %q and the machine package %q", tc.distro, host, a, b)
			}
		}
	}
}
