package controller

import (
	"fmt"
	"strings"

	"github.com/eyupio/zoomies/internal/store"
	"github.com/eyupio/zoomies/internal/version"
)

// hostUpgrade tells the operator how to align a remote host. An ahead agent
// needs its controller upgraded, never a command that silently downgrades it.
//
// rented is the machine this host is, when Zoomies rents it. A machine gets no
// command, because there is nowhere to run one: nobody logs into a clone, it
// is destroyed once it has been idle, and the machine that replaces it is
// cloned from the template its provider builds from. What an operator does
// instead is prepare that template again, which is where the machine's agent
// comes from, and the sentence says so rather than offering a command that
// would be run on a host that is about to stop existing.
func hostUpgrade(host *store.Host, controllerVersion string, rented rentedHost) (command, target, note string) {
	if host.Embedded {
		return "", "", ""
	}
	skew := version.CompareBuilds(host.Version, controllerVersion)
	if skew == version.SkewNone && !host.Incompatible {
		return "", "", ""
	}
	if skew == version.SkewAhead {
		if rented.rented() {
			return "", "", "This machine runs an agent newer than this controller, which is the direction nothing is tested in. Prepare " +
				"its provider's template from a newer release, or upgrade this controller."
		}
		return "", "", "This agent is newer than the controller. Upgrade the controller first."
	}
	if rented.rented() {
		// The tag is what a setup command would prepare the template from,
		// and it is what the operator has to type; a build that is neither a
		// release nor one from main has none, and says so.
		tag, ok := version.InstallTag(controllerVersion)
		if !ok {
			return "", "", rentedMachineAdvice(rented, "")
		}
		return "", "", rentedMachineAdvice(rented, tag)
	}
	tag, ok := version.InstallTag(controllerVersion)
	if !ok {
		return "", "", fmt.Sprintf("The controller is %s, a local or unpublished build. Install the matching agent from that build; there is no published upgrade command.", controllerVersion)
	}
	quoted := "'" + strings.ReplaceAll(tag, "'", "'\"'\"'") + "'"
	if host.OS == "windows" {
		// Windows has no sudo, and zoomies upgrade replaces a service it finds
		// under systemd or launchd and knows no Windows one, so that command
		// fails there. The installer script is what upgrades an enrolled
		// Windows machine: it stops the service, swaps the binary, keeps the
		// credentials and starts it again, and needs no token to do so.
		command = "& ([scriptblock]::Create((irm https://zoomies.sh/install.ps1))) -Mode agent -Version " + quoted
		note = "Run this in PowerShell opened as administrator on the host. zoomies upgrade knows no Windows service, so the installer does it instead: " +
			"it keeps the existing credentials, replaces zoomies.exe and restarts the agent. Running runner processes are adopted when the agent starts again."
		return command, controllerVersion, note
	}
	command = "sudo zoomies upgrade --mode agent --version " + quoted
	note = "Run this on the host. It keeps the existing configuration and credentials, refreshes stock runner images and restarts the agent."
	if tag == "dev" {
		note += " The dev channel moves with main, so it may contain a newer build than this controller."
	}
	return command, controllerVersion, note
}

// rentedMachineAdvice is what an operator does about a machine this fleet rents
// that runs an older agent than its controller: the card's sentence and the
// note under it, and the provider's problem, all say this one thing, so that
// the three cannot drift apart.
//
// It names no command, and that is the point. A machine is created by Zoomies,
// holds no shell anybody can reach, and is destroyed when it goes idle, so
// every command that updates an agent elsewhere has nobody to run it: the way
// to change what a machine runs is the image it was cloned from.
//
// build is the release an operator would prepare the template from, which is
// this controller's own tag, and empty when this controller is a build no
// release carries.
func rentedMachineAdvice(rented rentedHost, build string) string {
	what := "This is a machine " + rented.Provider() + " rents, and its agent is the one its provider's template carried when the " +
		"machine was cloned. Zoomies installs nothing of its own inside a rented machine, so nothing updates it in place: it is deleted " +
		"once it has been idle, and the machine that replaces it is cloned from that same template."
	if build == "" {
		return what + " This controller runs a build no release carries, so prepare the template from a release."
	}
	return what + " Prepare that template again from " + build + ", and the machines built afterwards carry it."
}
