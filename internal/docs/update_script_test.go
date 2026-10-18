package docs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// deploy/instapods/update.sh brings a pod up to date: it downloads the release,
// and restarts the app onto it. On the worker a restart fails the job that is
// running, so the script looks for one first. A download takes minutes, and a
// job can start in them; these tests run the real script against stand-ins for
// the download, for systemctl and for the process table, and hold it to
// restarting only when no job is running at the moment of the restart.

// updateRig is a pod's checkout and home directory, with a fetch.sh that
// "downloads" without a network, a systemctl that records its restarts, and a
// directory standing in for /proc whose entries are the jobs running.
type updateRig struct {
	root string
	proc string // a stand-in for /proc: one directory with a cmdline file per job
}

func newUpdateRig(t *testing.T) *updateRig {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("update.sh finds a running job in /proc, which only Linux has")
	}
	root := t.TempDir()
	r := &updateRig{root: root, proc: filepath.Join(root, "proc")}

	// The script as it ships, except that it reads the stand-in for /proc. A CI
	// job itself runs under a Runner.Worker, so the real /proc would always show
	// a job running and the idle cases could not be told from the busy ones.
	script := scriptText(t, "deploy/instapods/update.sh")
	const probe = "/proc/[0-9]*/cmdline"
	if strings.Count(script, probe) != 1 {
		t.Fatalf("update.sh no longer looks for a job in %s exactly once, so this test cannot point it at its own process table", probe)
	}
	script = strings.Replace(script, probe, `"$FAKE_PROC"/[0-9]*/cmdline`, 1)

	for path, body := range map[string]string{
		"app/deploy/instapods/update.sh": script,
		"app/deploy/instapods/lib.sh":    scriptText(t, "deploy/instapods/lib.sh"),
		// The download: a new binary on request, and a job that starts while it
		// is in flight on request.
		"app/deploy/instapods/fetch.sh": `#!/bin/sh
set -eu
if [ -n "${NEW_BINARY:-}" ]; then
    mkdir -p "$(dirname "$ZOOMIES_BIN")"
    n=$(cat "$STATE/n" 2>/dev/null || echo 0)
    n=$((n + 1))
    echo "$n" > "$STATE/n"
    printf '#!/bin/sh\necho "zoomies fake-%s"\n' "$n" > "$ZOOMIES_BIN"
    chmod 0755 "$ZOOMIES_BIN"
fi
if [ -n "${START_JOB_DURING_DOWNLOAD:-}" ]; then
    mkdir -p "$FAKE_PROC/4242"
    echo "/opt/runner/bin/Runner.Worker spawnclient" > "$FAKE_PROC/4242/cmdline"
fi
`,
		"shim/systemctl": `#!/bin/sh
case "$1" in
    cat) [ -z "${NO_SERVICE:-}" ] ;;
    restart) echo "restart $2" >> "$SYSTEMCTL_LOG" ;;
esac
`,
		"shim/sudo":  "#!/bin/sh\n[ \"$1\" != -n ] || shift\nexec \"$@\"\n",
		"shim/sleep": "#!/bin/sh\nexit 0\n",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{r.proc, filepath.Join(root, "state"), filepath.Join(root, "home")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

type updateRun struct {
	output   string
	exit     int
	restarts int // how many times the app was restarted
}

// run is `sh deploy/instapods/update.sh args`, with the rest of the pod's
// environment in env.
func (r *updateRig) run(t *testing.T, args []string, env ...string) updateRun {
	t.Helper()
	logPath := filepath.Join(r.root, "systemctl.log")
	before := countLines(logPath, "restart")
	cmd := exec.Command("sh", append([]string{filepath.Join(r.root, "app/deploy/instapods/update.sh")}, args...)...)
	cmd.Env = append([]string{
		"PATH=" + filepath.Join(r.root, "shim") + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + filepath.Join(r.root, "home"),
		"ZOOMIES_BIN=" + filepath.Join(r.root, "home/zoomies/bin/zoomies"),
		"STATE=" + filepath.Join(r.root, "state"), "FAKE_PROC=" + r.proc, "SYSTEMCTL_LOG=" + logPath,
	}, env...)
	out, err := cmd.CombinedOutput()
	got := updateRun{output: string(out)}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		got.exit = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running sh: %v", err)
	}
	got.restarts = countLines(logPath, "restart") - before
	return got
}

func countLines(path, contains string) int {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(body), "\n") {
		if strings.Contains(line, contains) {
			n++
		}
	}
	return n
}

// endJob takes the Runner.Worker the download started out of the stand-in
// process table, as a job finishing does.
func (r *updateRig) endJob(t *testing.T) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(r.proc, "4242")); err != nil {
		t.Fatal(err)
	}
}

func TestTheInstaPodsUpdateRestartsAnIdleWorkerOntoTheNewVersion(t *testing.T) {
	r := newUpdateRig(t)
	got := r.run(t, []string{"agent"}, "NEW_BINARY=1")
	if got.exit != 0 || got.restarts != 1 || !strings.Contains(got.output, "now running: zoomies fake-1") {
		t.Errorf("an idle worker, new binary: exit %d, restarts %d, output:\n%s\nwant one restart and the new version said to be running", got.exit, got.restarts, got.output)
	}
}

// docs/instapods.md promises the worker is left alone while a job runs. The
// script checks before it downloads, but the download takes minutes, and a job
// that starts in them used to be killed by the restart that followed. The
// restart waits instead, and is not forgotten: the next run, with nothing new
// to download, makes it.
func TestTheInstaPodsUpdateDoesNotRestartAWorkerWhoseJobStartedDuringTheDownload(t *testing.T) {
	r := newUpdateRig(t)

	got := r.run(t, []string{"agent"}, "NEW_BINARY=1", "START_JOB_DURING_DOWNLOAD=1")
	if got.exit != 0 || got.restarts != 0 {
		t.Fatalf("a job started during the download: exit %d, restarts %d, output:\n%s\nwant the app left running so the job survives", got.exit, got.restarts, got.output)
	}
	if !strings.Contains(got.output, "job") || !strings.Contains(got.output, "not restarted") {
		t.Errorf("the output does not say a job held the restart back:\n%s", got.output)
	}

	r.endJob(t)
	got = r.run(t, []string{"agent"})
	if got.exit != 0 || got.restarts != 1 || !strings.Contains(got.output, "now running: zoomies fake-1") {
		t.Errorf("the next run, with the job finished and nothing new to download: exit %d, restarts %d, output:\n%s\nwant the restart that was held back", got.exit, got.restarts, got.output)
	}

	got = r.run(t, []string{"agent"})
	if got.restarts != 0 || !strings.Contains(got.output, "already up to date") {
		t.Errorf("the run after that: restarts %d, output:\n%s\nwant it left alone, the restart having been made", got.restarts, got.output)
	}
}

func TestTheInstaPodsUpdateRestartsAWorkerWithAJobWhenToldTo(t *testing.T) {
	r := newUpdateRig(t)
	got := r.run(t, []string{"--force", "agent"}, "NEW_BINARY=1", "START_JOB_DURING_DOWNLOAD=1")
	if got.exit != 0 || got.restarts != 1 {
		t.Errorf("--force with a job running: exit %d, restarts %d, output:\n%s\nwant the restart made", got.exit, got.restarts, got.output)
	}
}

// With no app service there is nothing to restart: the script says what to run,
// and what it printed after that was "now running", which nothing was.
func TestTheInstaPodsUpdateOnAPodWithNoServiceSaysInstalledNotRunning(t *testing.T) {
	r := newUpdateRig(t)
	got := r.run(t, []string{"agent"}, "NEW_BINARY=1", "NO_SERVICE=1")
	if got.exit != 0 || got.restarts != 0 || !strings.Contains(got.output, "systemctl restart app") {
		t.Fatalf("no app service: exit %d, restarts %d, output:\n%s\nwant the instruction to restart it by hand", got.exit, got.restarts, got.output)
	}
	if strings.Contains(got.output, "now running") || !strings.Contains(got.output, "installed: zoomies fake-1") {
		t.Errorf("no app service, output:\n%s\nwant the version said to be installed, and not to be running", got.output)
	}
}
