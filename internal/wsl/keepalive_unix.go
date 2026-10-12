//go:build unix

package wsl

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// startDetached starts argv in its own session and does not tie it to ctx:
// cancelling the agent must leave the session open.
func startDetached(_ context.Context, argv []string) (int, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	// Reaped in the background so that an exit shows as gone, not as a zombie
	// that still answers signal 0.
	go func() { _ = cmd.Wait() }()
	return cmd.Process.Pid, nil
}

// processAlive is whether pid is a live keepalive. The command line is checked
// as well as the signal because a recorded pid outlives its process and can be
// reused by something unrelated.
func processAlive(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return false
	}
	c := string(b)
	return strings.Contains(c, "sleep") || strings.Contains(c, "wsl.exe")
}
