package controller

import (
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

func TestHostUpgradeCommandsMatchTheControllerWithoutDowngradingAheadAgents(t *testing.T) {
	for _, tt := range []struct {
		name, agent, controller, want string
		embedded, incompatible        bool
	}{
		{"older release", "1.0", "1.2.3", "--version 'v1.2.3'", false, false},
		{"development", "main-sha-1234567", "main-sha-abcdef0", "--version 'dev'", false, false},
		{"matching", "1.2.3", "1.2.3", "", false, false},
		{"newer agent", "2.0", "1.2.3", "", false, false},
		{"local controller", "1.0", "local-dirty", "", false, false},
		{"embedded", "1.0", "1.2.3", "", true, false},
		{"protocol mismatch", "1.2.3", "1.2.3", "--version 'v1.2.3'", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, note := hostUpgrade(&store.Host{Version: tt.agent, Embedded: tt.embedded, Incompatible: tt.incompatible}, tt.controller, rentedHost{})
			if tt.want == "" {
				if cmd != "" {
					t.Fatalf("unexpected command %q", cmd)
				}
			} else if !strings.Contains(cmd, tt.want) || !strings.Contains(cmd, "zoomies upgrade --mode agent") {
				t.Fatalf("command %q", cmd)
			}
			if strings.Contains(cmd, "join-token") {
				t.Fatal("upgrade must not enrol again")
			}
			if tt.name == "local controller" && !strings.Contains(note, "unpublished") {
				t.Fatalf("missing explanation: %s", note)
			}
		})
	}
}

// A machine this fleet rents is given no command at all, and a sentence that
// says where its agent actually comes from. Nobody logs into a clone, it is
// destroyed when it goes idle, and every command that updates an agent
// elsewhere would be run on a host that is about to stop existing.
func TestHostUpgradeOnARentedMachineOffersNoCommandAndNamesTheTemplate(t *testing.T) {
	rented := rentedHost{MachineID: "mach_1", ProviderID: "prv_1", ProviderName: "proxmox-pve"}
	cmd, _, note := hostUpgrade(&store.Host{Version: "1.3.1"}, "1.4.0", rented)
	if cmd != "" {
		t.Fatalf("a rented machine was given a command to run somewhere: %q", cmd)
	}
	for _, want := range []string{"proxmox-pve", "template", "cloned", "v1.4.0", "idle", "replaces"} {
		if !strings.Contains(note, want) {
			t.Errorf("the note does not say %q: %s", want, note)
		}
	}
	if strings.Contains(note, "sudo") || strings.Contains(note, "zoomies upgrade") {
		t.Fatalf("the note offers a command nobody can run: %s", note)
	}
	// The same advice, in the same words, is what the update block and the
	// provider's problem give: one fact, three surfaces, one spelling.
	if why := rentedMachineAdvice(rented, "v1.4.0"); !strings.Contains(why, "proxmox-pve") {
		t.Fatalf("the shared sentence does not name the provider: %s", why)
	}
}

// A Windows agent is not offered sudo or zoomies upgrade, which knows no
// Windows service. It is offered the installer script, which upgrades an
// enrolled machine without a token, so the command never enrols it twice.
func TestHostUpgradeOnWindowsIsTheInstallerScript(t *testing.T) {
	cmd, version, note := hostUpgrade(&store.Host{OS: "windows", Version: "1.0"}, "1.2.3", rentedHost{})
	want := "& ([scriptblock]::Create((irm https://zoomies.sh/install.ps1))) -Mode agent -Version 'v1.2.3'"
	if cmd != want {
		t.Fatalf("command = %q, want %q", cmd, want)
	}
	if version != "1.2.3" || !strings.Contains(note, "administrator") {
		t.Fatalf("version %q, note %q", version, note)
	}
}
