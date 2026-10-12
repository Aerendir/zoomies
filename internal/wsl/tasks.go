package wsl

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf16"
)

// The scheduled tasks Windows runs to start the distro when nothing else has.
// Both run the same script, which checks first, so a task firing while the
// distro is already up does nothing.
const (
	taskOnStart = `Zoomies\WslOnStart`
	taskEvery5  = `Zoomies\WslEvery5Minutes`
	scriptName  = "wsl-ensure.ps1"
)

// startScript is the PowerShell the tasks run. wsl.exe prints UTF-16, which
// arrives with NULs between the letters, so they are stripped before matching.
// The command it starts holds the distro for two minutes: long enough for
// systemd to start the agent, which then holds it open for good.
func startScript(distro string) string {
	d := strings.ReplaceAll(distro, "'", "''")
	return strings.Join([]string{
		"# Written by `zoomies agent --wsl-ensure`; rewritten if it differs.",
		"$distro = '" + d + "'",
		`$wsl = Join-Path $env:SystemRoot 'System32\wsl.exe'`,
		"$running = ((& $wsl -l --running -q) -join \"`n\") -replace \"`0\", ''",
		"if ($running -notmatch ('(?m)^' + [regex]::Escape($distro) + '\\s*$')) {",
		"  & $wsl -d $distro --exec /bin/sleep 120",
		"}",
		"",
	}, "\r\n")
}

// decodeConsole turns schtasks' output into text. Piped through interop it is
// UTF-16LE with or without a byte-order mark.
func decodeConsole(b []byte) string {
	if len(b) >= 2 && (b[0] == 0xFF && b[1] == 0xFE || len(b) > 3 && b[1] == 0 && b[3] == 0) {
		if b[0] == 0xFF && b[1] == 0xFE {
			b = b[2:]
		}
		u := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
		}
		return string(utf16.Decode(u))
	}
	return strings.TrimPrefix(string(b), "\xef\xbb\xbf")
}

type taskSpec struct {
	name       string
	args       []string // the schedule part of /Create
	trigger    string   // what the exported XML must contain to be this schedule
	needsAdmin bool
}

func taskSpecs() []taskSpec {
	return []taskSpec{
		{name: taskOnStart, args: []string{"/SC", "ONSTART"}, trigger: "<BootTrigger>", needsAdmin: true},
		{name: taskEvery5, args: []string{"/SC", "MINUTE", "/MO", "5"}, trigger: "<Interval>PT5M</Interval>"},
	}
}

// ensureTask creates or repairs one task. It is current when it exists, has the
// right schedule and runs this script; otherwise it is created again with /F.
// No credentials are ever passed: without /RU and /RP a task runs as the
// current user and only while they are signed in, and an ONSTART task needs
// elevation, which is reported rather than asked for.
func ensureTask(ctx context.Context, sys System, spec taskSpec, winScript string) Step {
	step := Step{Name: "task " + spec.name}
	action := `powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -WindowStyle Hidden -File "` + winScript + `"`
	if out, err := sys.Run(ctx, SchtasksPath, "/Query", "/TN", spec.name, "/XML"); err == nil {
		xml := decodeConsole(out)
		if strings.Contains(xml, spec.trigger) && strings.Contains(xml, winScript) {
			step.Status, step.Detail = StatusOK, "already in place"
			return step
		}
	}
	args := append([]string{"/Create", "/TN", spec.name, "/TR", action}, spec.args...)
	args = append(args, "/F")
	out, err := sys.Run(ctx, SchtasksPath, args...)
	if err != nil {
		msg := strings.TrimSpace(decodeConsole([]byte(err.Error())) + " " + decodeConsole(out))
		msg = strings.ReplaceAll(msg, "\x00", "")
		if strings.Contains(strings.ToLower(msg), "access is denied") {
			step.Status = StatusWarn
			step.Detail = "Windows refused it without elevation, so it was skipped; " + spec.name + " needs an elevated session once (this never asks for a password)"
			return step
		}
		step.Status, step.Detail = StatusWarn, fmt.Sprintf("schtasks failed: %s", msg)
		return step
	}
	step.Status, step.Detail = StatusChanged, "created"
	return step
}
