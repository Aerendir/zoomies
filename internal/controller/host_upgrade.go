package controller

import (
	"fmt"
	"strings"

	"github.com/eyupio/zoomies/internal/store"
	"github.com/eyupio/zoomies/internal/version"
)

// hostUpgrade tells the operator how to align a remote host. An ahead agent
// needs its controller upgraded, never a command that silently downgrades it.
func hostUpgrade(host *store.Host, controllerVersion string) (command, target, note string) {
	if host.Embedded {
		return "", "", ""
	}
	skew := version.CompareBuilds(host.Version, controllerVersion)
	if skew == version.SkewNone && !host.Incompatible {
		return "", "", ""
	}
	if skew == version.SkewAhead {
		return "", "", "This agent is newer than the controller. Upgrade the controller first."
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
