//go:build unix

package installer

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/updates"
	"github.com/eyupio/zoomies/internal/updates/channel"
)

// fakeSystemd stands in for systemctl: it answers the three questions the
// repair asks from fields a test sets, and does what reset-failed and enable
// --now would do to them. Every call is recorded, with whether request.json was
// still in the folder when it was made, which is how the tests hold the order.
type fakeSystemd struct {
	pathActive, pathEnabled   string
	pathFailed, serviceFailed bool
	serviceActive             string
	// onEnable runs when the path unit is started, for a helper that comes back
	// and takes the request it finds.
	onEnable func()
	calls    []string
	// requestAt is the folder's request, so a call can say whether it was there.
	requestAt     string
	requestAtCall map[string]bool
}

func (f *fakeSystemd) run(_ context.Context, name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	if f.requestAtCall == nil {
		f.requestAtCall = map[string]bool{}
	}
	_, err := os.Lstat(f.requestAt)
	f.requestAtCall[line] = err == nil
	unit := ""
	if len(args) > 0 {
		unit = args[len(args)-1]
	}
	switch args[0] {
	case "is-active":
		if unit == UpdatePathUnit {
			return f.pathActive, nil
		}
		if unit == UpdateServiceUnit {
			return f.serviceActive, nil
		}
		return "inactive", nil
	case "is-enabled":
		if unit == UpdatePathUnit {
			return f.pathEnabled, nil
		}
		return "static", nil
	case "is-failed":
		if (unit == UpdatePathUnit && f.pathFailed) || (unit == UpdateServiceUnit && f.serviceFailed) {
			return "failed", nil
		}
		return "active", nil
	case "reset-failed":
		f.pathFailed, f.serviceFailed = false, false
	case "enable":
		f.pathActive, f.pathEnabled, f.pathFailed = "active", "enabled", false
		if f.onEnable != nil {
			f.onEnable()
		}
	}
	return "", nil
}

func (f *fakeSystemd) changed() bool {
	return slices.ContainsFunc(f.calls, func(c string) bool {
		return strings.Contains(c, "reset-failed") || strings.Contains(c, " enable ") || strings.Contains(c, "daemon-reload")
	})
}

type repairHost struct {
	*helperHost
	sysd     *fakeSystemd
	unitDir  string
	lockPath string
	engines  [][]string
	now      time.Time
}

// newRepairHost is the host from the report: the path unit disabled and dead, a
// request three days old in the folder, and a lock beside the configuration.
func newRepairHost(t *testing.T) *repairHost {
	t.Helper()
	h := newHelperHost(t, "1.3.4")
	r := &repairHost{helperHost: h, unitDir: filepath.Join(filepath.Dir(h.dir), "units"), lockPath: h.opts.LockPath, now: t0}
	if err := os.Mkdir(r.unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r.sysd = &fakeSystemd{
		pathActive: "inactive", pathEnabled: "disabled", pathFailed: true,
		serviceActive: "inactive", requestAt: filepath.Join(h.dir, channel.RequestFile),
	}
	return r
}

func (r *repairHost) age(t *testing.T, path string, age time.Duration) {
	t.Helper()
	at := r.now.Add(-age)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// staleRequest plants request.json and the lock, both aged.
func (r *repairHost) staleRequest(t *testing.T, requestAge, lockAge time.Duration) {
	t.Helper()
	r.ask(t, `{"v":1,"id":"upd_old","tag":"v1.3.5","requested_by":"someone","requested_at":"2026-10-05T09:00:00Z"}`)
	r.age(t, r.sysd.requestAt, requestAge)
	if lockAge > 0 {
		plant(t, r.lockPath, "locked")
		r.age(t, r.lockPath, lockAge)
	}
}

func (r *repairHost) askForRepair(t *testing.T, id string) {
	t.Helper()
	plant(t, filepath.Join(r.dir, updates.RepairFile), `{"v":1,"id":"`+id+`","requested_at":"2026-10-08T09:00:00Z"}`)
}

// repair runs the repair once and returns its answer.
func (r *repairHost) repair(t *testing.T, id string) updates.RepairResult {
	t.Helper()
	r.askForRepair(t, id)
	opts := RepairOptions{HelperOptions: r.opts, unitDir: r.unitDir, run: r.sysd.run,
		procs: func() [][]string { return r.engines }, settle: time.Millisecond, sleep: func(time.Duration) {}}
	opts.Now = func() time.Time { return r.now }
	if err := RunUpdateRepair(context.Background(), opts); err != nil {
		t.Fatalf("RunUpdateRepair: %v\nlog:\n%s", err, r.log)
	}
	if _, err := os.Lstat(filepath.Join(r.dir, updates.RepairFile)); err == nil {
		t.Error("the repair request was left in the folder, and would start the repair again")
	}
	res, found, err := channel.ReadRepairResult(r.dir)
	if err != nil || !found {
		t.Fatalf("no repair result the service can read: found %v, err %v\nlog:\n%s", found, err, r.log)
	}
	return res
}

func exists_(path string) bool { _, err := os.Lstat(path); return err == nil }

// The report: path unit disabled and dead, a request three days old. The units
// are put right first, and only then is what is provably abandoned removed.
func TestARepairRestartsADisabledPathUnitBeforeItClearsAnAbandonedRequest(t *testing.T) {
	r := newRepairHost(t)
	r.staleRequest(t, 72*time.Hour, 70*time.Hour)

	res := r.repair(t, "upd_new")

	want := []string{updates.ActionResetFailed, updates.ActionRewroteUnits, updates.ActionStartedPath,
		updates.ActionRemovedRequest, updates.ActionRemovedLock}
	if !slices.Equal(res.Actions, want) || !res.Healed || res.ID != "upd_new" || res.Verdict != updates.StuckPathDown {
		t.Errorf("result = %+v, want healed with actions %v", res, want)
	}
	if exists_(r.sysd.requestAt) || exists_(r.lockPath) {
		t.Error("the abandoned request and lock were left behind")
	}
	if !r.sysd.ran("systemctl enable --now " + UpdatePathUnit) {
		t.Errorf("the path unit was not started; calls: %v", r.sysd.calls)
	}
	// The order is the point: a request is never removed from under a helper that
	// has not been given the chance to read it.
	if !r.sysd.requestAtCall["systemctl enable --now "+UpdatePathUnit] {
		t.Error("the request was already gone when the path unit was started, so the helper had no chance to read it")
	}
	for _, unit := range []string{UpdatePathUnit, UpdateServiceUnit, UpdateRepairPathUnit, UpdateRepairServiceUnit} {
		if !exists_(filepath.Join(r.unitDir, unit)) {
			t.Errorf("%s was not rewritten", unit)
		}
	}
}

func (f *fakeSystemd) ran(line string) bool { return slices.Contains(f.calls, line) }

// A helper that is running an update may be reading the request, and the engine
// owns the lock. Whatever else is wrong, none of it is touched, and nothing is
// even asked of systemd that would change a unit.
func TestARepairNeverTouchesAHelperThatIsMidUpgrade(t *testing.T) {
	for name, mid := range map[string]func(*repairHost){
		"the service is running":  func(r *repairHost) { r.sysd.serviceActive = "active" },
		"the service is starting": func(r *repairHost) { r.sysd.serviceActive = "activating" },
		"an engine process is live": func(r *repairHost) {
			r.engines = [][]string{{"/usr/local/bin/zoomies", "upgrade", "--version", "v1.3.5", "--non-interactive"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRepairHost(t)
			r.staleRequest(t, 72*time.Hour, 70*time.Hour)
			mid(r)

			res := r.repair(t, "upd_new")

			if res.Healed || res.Verdict != updates.StuckBusy || !slices.Equal(res.Actions, []string{updates.ActionRefusedBusy}) {
				t.Errorf("result = %+v, want a refusal because the helper is busy", res)
			}
			if !exists_(r.sysd.requestAt) || !exists_(r.lockPath) {
				t.Error("the request or the lock was removed from under a live helper")
			}
			if r.sysd.changed() {
				t.Errorf("systemd was changed under a live helper: %v", r.sysd.calls)
			}
		})
	}
}

// Once the units are right the helper reads what is waiting itself. A repair
// that then removed "the stale request" would be removing a live one.
func TestARepairLeavesTheRequestToAHelperThatCameBackAndTookIt(t *testing.T) {
	r := newRepairHost(t)
	r.staleRequest(t, 72*time.Hour, 0)
	r.sysd.onEnable = func() {
		_ = os.Remove(r.sysd.requestAt)
		r.sysd.serviceActive = "active"
	}

	res := r.repair(t, "upd_new")

	if !res.Healed || slices.Contains(res.Actions, updates.ActionRemovedRequest) {
		t.Errorf("result = %+v, want healed without removing a request the helper took", res)
	}
}

func TestARepairKeepsARequestThatIsTooYoungToBeCalledAbandoned(t *testing.T) {
	r := newRepairHost(t)
	r.staleRequest(t, 2*time.Minute, 2*time.Minute)

	res := r.repair(t, "upd_new")

	if slices.Contains(res.Actions, updates.ActionRemovedRequest) || slices.Contains(res.Actions, updates.ActionRemovedLock) {
		t.Errorf("actions = %v; files two minutes old are not abandoned", res.Actions)
	}
	if !exists_(r.sysd.requestAt) || !exists_(r.lockPath) {
		t.Error("a young request or lock was removed")
	}
	if !r.sysd.ran("systemctl enable --now " + UpdatePathUnit) {
		t.Error("the units still need putting right whatever the age of the request")
	}
}

// A healthy helper with nothing wrong is left exactly as it is, and the repair
// does not count against the budget for it.
func TestARepairOfAHealthyHelperChangesNothing(t *testing.T) {
	r := newRepairHost(t)
	r.sysd.pathActive, r.sysd.pathEnabled, r.sysd.pathFailed = "active", "enabled", false

	res := r.repair(t, "upd_new")

	if !res.Healed || len(res.Actions) != 0 || r.sysd.changed() {
		t.Errorf("result = %+v, calls %v; a healthy helper needs nothing", res, r.sysd.calls)
	}
}

// Never self-heal in a loop: a second repair a minute after the first is refused
// by root against its own record, whatever the service asks.
func TestRootRefusesToRepairAgainSoSoon(t *testing.T) {
	r := newRepairHost(t)
	r.staleRequest(t, 72*time.Hour, 0)
	first := r.repair(t, "upd_one")
	if !first.Healed {
		t.Fatalf("the first repair failed: %+v", first)
	}

	r.sysd.pathActive, r.sysd.pathEnabled, r.sysd.pathFailed = "inactive", "disabled", true
	r.sysd.calls = nil
	r.now = r.now.Add(time.Minute)
	second := r.repair(t, "upd_two")

	if second.Healed || !slices.Equal(second.Actions, []string{updates.ActionRefusedRateLimit}) || !strings.Contains(second.Error, "too recently") {
		t.Errorf("second result = %+v, want a refusal for repairing too soon", second)
	}
	if r.sysd.changed() {
		t.Errorf("the second repair changed systemd: %v", r.sysd.calls)
	}

	// After the interval it is allowed again, up to the day's limit.
	r.now = r.now.Add(updates.RepairMinInterval)
	if third := r.repair(t, "upd_three"); !third.Healed {
		t.Errorf("a repair after the interval was refused: %+v", third)
	}
}

func TestAGarbledRepairRequestIsRemovedAndNothingIsDoneForIt(t *testing.T) {
	r := newRepairHost(t)
	plant(t, filepath.Join(r.dir, updates.RepairFile), `{"v":1,"id":"upd_x","requested_at":"2026-10-08T09:00:00Z","run":"rm -rf /"}`)
	opts := RepairOptions{HelperOptions: r.opts, unitDir: r.unitDir, run: r.sysd.run, settle: time.Millisecond, sleep: func(time.Duration) {}}
	opts.Now = func() time.Time { return r.now }
	if err := RunUpdateRepair(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if exists_(filepath.Join(r.dir, updates.RepairFile)) {
		t.Error("a request root refused was left to be refused again")
	}
	if len(r.sysd.calls) != 0 {
		t.Errorf("systemd was asked for something on a request that could not be read: %v", r.sysd.calls)
	}
	if res, found, err := channel.ReadRepairResult(r.dir); err != nil || !found || res.Healed || res.Error == "" {
		t.Errorf("result = %+v, found %v, err %v; want a refusal that says why", res, found, err)
	}
}
