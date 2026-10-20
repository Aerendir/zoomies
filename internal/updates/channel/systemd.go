package channel

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/updates"
)

// The helper's units, as systemctl names them. The installer owns their
// contents; the names are here because the agent that diagnoses and the root
// side that repairs both have to ask about them, and neither imports the other.
const (
	PathUnit    = "zoomies-update.path"
	ServiceUnit = "zoomies-update.service"
)

// RunFunc runs a command and returns what it printed. A command that exits
// non-zero returns its output with the error, because systemctl's is-active and
// is-enabled answer in words on exactly that path.
type RunFunc func(ctx context.Context, name string, args ...string) (string, error)

// systemctlTimeout bounds one question to systemd, so a wedged bus cannot hold
// an update task for ever.
const systemctlTimeout = 10 * time.Second

// RunCommand is the RunFunc that runs the real command.
func RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, systemctlTimeout)
	defer cancel()
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// QueryUnit asks systemd what it thinks of one unit. It changes nothing and needs
// no privilege. A question that cannot be asked (no systemctl, no bus) leaves its
// field empty, which Diagnosis reads as not known and never as healthy.
func QueryUnit(ctx context.Context, run RunFunc, unit string) updates.UnitState {
	word := func(verb string) string {
		out, _ := run(ctx, "systemctl", verb, unit)
		out, _, _ = strings.Cut(strings.TrimSpace(out), "\n")
		// A word systemd would not say is an error message, and is not a state.
		if len(out) > 24 || strings.ContainsAny(out, " :/") {
			return ""
		}
		return out
	}
	return updates.UnitState{
		Active:  word("is-active"),
		Enabled: word("is-enabled"),
		Failed:  word("is-failed") == "failed",
	}
}

// Processes reads the command lines of every process this account can see. It is
// best effort: /proc may hide other accounts' processes, in which case an engine
// started by root is not seen here, and root asks again for itself before it
// acts.
func Processes() [][]string {
	dirs, err := filepath.Glob("/proc/[0-9]*/cmdline")
	if err != nil {
		return nil
	}
	var out [][]string
	for _, path := range dirs {
		body, err := os.ReadFile(path)
		if err != nil || len(body) == 0 {
			continue
		}
		out = append(out, strings.Split(strings.TrimRight(string(body), "\x00"), "\x00"))
	}
	return out
}

// Diagnose gathers everything Classify needs. lockDir is the deployment's
// configuration directory, or "" when it is not known.
func Diagnose(ctx context.Context, run RunFunc, dir, lockDir string, now time.Time, procs func() [][]string) (updates.Diagnosis, error) {
	facts, err := Look(dir, lockDir, now)
	if err != nil {
		return updates.Diagnosis{}, err
	}
	if procs == nil {
		procs = Processes
	}
	return updates.Diagnosis{
		RequestPresent: facts.RequestPresent, RequestAge: facts.RequestAge, RequestID: facts.RequestID,
		LockPresent: facts.LockPresent, LockAge: facts.LockAge,
		Path:          QueryUnit(ctx, run, PathUnit),
		Service:       QueryUnit(ctx, run, ServiceUnit),
		EngineRunning: updates.EngineRunning(procs()),
	}, nil
}
