package wsl

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Keepalive holds a Windows-side session open on the distro for as long as the
// agent lives, because WSL2 stops a distro when its last such session closes
// and a process inside it does not count. The session is `wsl.exe -d <distro>
// --exec sleep infinity`, started through interop.
//
// The process is started in its own session and left running when the agent
// stops, so restarting the agent (an update, a crash, systemd's Restart=)
// never lets the distro go down under the jobs the agent leaves running. Its
// pid is recorded, and the next agent adopts it instead of starting another.
type Keepalive struct {
	Distro  string
	WSLExe  string // path to wsl.exe; empty falls back to a plain sleep
	PidFile string
	Log     *slog.Logger

	// Seams for tests.
	Start func(ctx context.Context, argv []string) (pid int, err error)
	Alive func(pid int) bool
	Sleep func(ctx context.Context, d time.Duration) bool
	Read  func(path string) ([]byte, error)
	Write func(path string, data []byte, perm os.FileMode) error
}

// Argv is the command that holds the session open.
func (k *Keepalive) Argv() []string {
	if k.WSLExe == "" {
		return []string{"/bin/sleep", "infinity"}
	}
	return []string{k.WSLExe, "-d", k.Distro, "--exec", "/bin/sleep", "infinity"}
}

const (
	pollEvery  = 5 * time.Second
	backoffMin = 2 * time.Second
	backoffMax = time.Minute
)

// Run keeps one session open until ctx ends. It returns only then.
func (k *Keepalive) Run(ctx context.Context) {
	k.defaults()
	backoff := backoffMin
	if k.WSLExe == "" {
		k.Log.Warn("Windows interop is unavailable, so the keepalive is a plain sleep and cannot hold this WSL2 distro up; the scheduled tasks are the only recovery",
			"expected", WSLExePath)
	}
	for ctx.Err() == nil {
		pid, adopted := k.adopt()
		if !adopted {
			var err error
			pid, err = k.Start(ctx, k.Argv())
			if err != nil {
				k.Log.Warn("could not start the WSL2 keepalive; retrying", "error", err, "in", backoff.String())
				if !k.Sleep(ctx, backoff) {
					return
				}
				backoff = min(backoff*2, backoffMax)
				continue
			}
			if err := k.Write(k.PidFile, []byte(strconv.Itoa(pid)), 0o644); err != nil {
				k.Log.Warn("could not record the keepalive's pid; a restarted agent will start another", "error", err)
			}
			k.Log.Info("holding the WSL2 distro open", "pid", pid, "distro", k.Distro)
		} else {
			k.Log.Info("adopted the WSL2 keepalive an earlier agent started", "pid", pid)
		}
		backoff = backoffMin
		for k.Alive(pid) {
			if !k.Sleep(ctx, pollEvery) {
				return
			}
		}
		if ctx.Err() == nil {
			k.Log.Warn("the WSL2 keepalive exited; starting another", "pid", pid)
		}
	}
}

func (k *Keepalive) adopt() (int, bool) {
	b, err := k.Read(k.PidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 || !k.Alive(pid) {
		return 0, false
	}
	return pid, true
}

func (k *Keepalive) defaults() {
	if k.Log == nil {
		k.Log = slog.Default()
	}
	if k.Start == nil {
		k.Start = startDetached
	}
	if k.Alive == nil {
		k.Alive = processAlive
	}
	if k.Sleep == nil {
		k.Sleep = sleepCtx
	}
	if k.Read == nil {
		k.Read = os.ReadFile
	}
	if k.Write == nil {
		k.Write = os.WriteFile
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
