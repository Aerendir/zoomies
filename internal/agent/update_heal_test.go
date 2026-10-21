package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/updates"
	"github.com/eyupio/zoomies/internal/updates/channel"
)

// fakeUnits stands in for systemctl on a host whose helper has stopped. The
// agent only ever asks it questions; the fake root below is what changes it.
type fakeUnits struct {
	mu                        sync.Mutex
	pathActive, pathEnabled   string
	pathFailed, serviceFailed bool
	serviceActive             string
	asked                     int
}

func newStoppedUnits() *fakeUnits {
	return &fakeUnits{pathActive: "inactive", pathEnabled: "disabled", pathFailed: true, serviceActive: "inactive"}
}

func (f *fakeUnits) run(_ context.Context, _ string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked++
	unit := args[len(args)-1]
	switch args[0] {
	case "is-active":
		if unit == channel.PathUnit {
			return f.pathActive, nil
		}
		return f.serviceActive, nil
	case "is-enabled":
		if unit == channel.PathUnit {
			return f.pathEnabled, nil
		}
		return "static", nil
	case "is-failed":
		if (unit == channel.PathUnit && f.pathFailed) || (unit == channel.ServiceUnit && f.serviceFailed) {
			return "failed", nil
		}
		return "active", nil
	}
	return "", nil
}

func (f *fakeUnits) heal() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pathActive, f.pathEnabled, f.pathFailed, f.serviceFailed = "active", "enabled", false, false
}

// fakeRoot is the repair unit: it watches the folder for repair.json and answers
// it. fix says what it does to the host, and healed what it reports.
type fakeRoot struct {
	dir     string
	units   *fakeUnits
	healed  bool
	mu      sync.Mutex
	repairs []string
}

func (r *fakeRoot) start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	t.Cleanup(func() { cancel(); <-done })
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			time.Sleep(2 * time.Millisecond)
			body, err := os.ReadFile(filepath.Join(r.dir, updates.RepairFile))
			if err != nil {
				continue
			}
			req, err := updates.ParseRepairRequest(body)
			if err != nil {
				continue
			}
			_ = os.Remove(filepath.Join(r.dir, updates.RepairFile))
			r.mu.Lock()
			r.repairs = append(r.repairs, req.ID)
			r.mu.Unlock()
			res := updates.RepairResult{V: updates.WireVersion, ID: req.ID, Healed: r.healed, FinishedAt: time.Now().UTC(), Verdict: updates.StuckPathDown}
			if r.healed {
				r.units.heal()
				_ = os.Remove(filepath.Join(r.dir, channel.RequestFile))
				res.Actions = []string{updates.ActionResetFailed, updates.ActionRewroteUnits, updates.ActionStartedPath, updates.ActionRemovedRequest}
			} else {
				res.Error = "the helper's units could not be put right: cannot write the unit"
			}
			b, _ := json.Marshal(res)
			_ = os.WriteFile(filepath.Join(r.dir, updates.RepairResultFile), b, 0o644)
		}
	}()
}

func (r *fakeRoot) asked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.repairs)
}

// stuckHost is a host the way the report found it: a helper installed, its path
// unit dead, and a request three days old in the folder. The agent can ask
// systemd and can write the folder, and nothing else.
type stuckHost struct {
	dir   string
	units *fakeUnits
	root  *fakeRoot
	h     *harness
}

func newStuckHost(t *testing.T, requestAge time.Duration, tweaks ...func(*Options)) *stuckHost {
	t.Helper()
	runningBuild(t, "1.3.0")
	was := repairPoll
	repairPoll = 5 * time.Millisecond
	t.Cleanup(func() { repairPoll = was })

	dir := readyUpdateFolder(t)
	s := &stuckHost{dir: dir, units: newStoppedUnits()}
	s.root = &fakeRoot{dir: dir, units: s.units, healed: true}
	opts := []func(*Options){withUpdateFolder(dir), func(o *Options) {
		o.UnitRun = s.units.run
		o.Processes = func() [][]string { return nil }
		o.RepairWait = 2 * time.Second
	}}
	s.h = newHarness(t, 1, append(opts, tweaks...)...)
	if requestAge > 0 {
		old := []byte(`{"v":1,"id":"upd_stale","tag":"v1.3.4","requested_by":"the controller","requested_at":"2026-01-01T09:00:00Z"}`)
		path := filepath.Join(dir, channel.RequestFile)
		if err := os.WriteFile(path, old, 0o640); err != nil {
			t.Fatal(err)
		}
		at := s.h.clock.Now().Add(-requestAge)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func (s *stuckHost) request(t *testing.T) updates.Request {
	t.Helper()
	req, found, err := channel.PendingRequest(s.dir)
	if err != nil || !found {
		t.Fatalf("no readable request in the folder: found %v, err %v", found, err)
	}
	return req
}

// The report: every attempt failed for days with "an update request is already
// waiting for the helper", and nothing was written or started. The agent now
// sees that the path unit is dead, asks root to repair, and writes its request.
func TestAnUpdateTaskAsksForARepairWhenThePathUnitIsDeadAndThenWritesItsRequest(t *testing.T) {
	s := newStuckHost(t, 72*time.Hour)
	s.root.start(t)

	s.h.tr.tasks <- []Task{updateTask("task-1", "upd_abc123", "v1.3.5")}
	res := s.h.nextResult()

	if !res.OK || res.Stuck == nil || !res.Stuck.Healed || res.Stuck.Kind != updates.StuckPathDown {
		t.Fatalf("the update task reported %+v (stuck %+v), want OK with a healed path_down", res, res.Stuck)
	}
	if !slices.Contains(res.Stuck.Repaired, updates.ActionStartedPath) || res.Stuck.Path != "inactive, disabled, failed" {
		t.Errorf("stuck = %+v, want the repair actions and the unit state the agent found", res.Stuck)
	}
	if res.Stuck.RequestAgeSeconds < int64((72*time.Hour)/time.Second) {
		t.Errorf("request age = %ds, want about three days", res.Stuck.RequestAgeSeconds)
	}
	if got := s.request(t); got.ID != "upd_abc123" || got.Tag != "v1.3.5" {
		t.Errorf("the request in the folder is %+v, want this attempt's", got)
	}
	if asked := s.root.asked(); !slices.Equal(asked, []string{"upd_abc123"}) {
		t.Errorf("root was asked to repair for %v, want exactly this attempt", asked)
	}
}

func TestAnUpdateTaskAsksForARepairWhenTheRequestIsStaleUnderHealthyLookingUnits(t *testing.T) {
	s := newStuckHost(t, 3*time.Hour)
	s.units.heal()
	s.root.start(t)

	s.h.tr.tasks <- []Task{updateTask("task-1", "upd_abc123", "v1.3.5")}
	res := s.h.nextResult()

	if !res.OK || res.Stuck == nil || res.Stuck.Kind != updates.StuckStaleRequest || !res.Stuck.Healed {
		t.Fatalf("the update task reported %+v (stuck %+v), want OK after repairing a stale request", res, res.Stuck)
	}
}

// A helper that is mid-upgrade may still be reading the request, and the engine
// owns the lock. The agent leaves everything alone, asks for nothing, and says so.
func TestAnUpdateTaskNeverAsksForARepairWhileTheHelperIsMidUpgrade(t *testing.T) {
	for name, tweak := range map[string]func(*stuckHost){
		"the helper's service is running": func(s *stuckHost) { s.units.serviceActive = "active" },
		"the engine is alive": func(s *stuckHost) {
			s.h.agent.opts.Processes = func() [][]string { return [][]string{{"/usr/local/bin/zoomies", "upgrade", "--version", "v1.3.5"}} }
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newStuckHost(t, 72*time.Hour)
			s.root.start(t)
			tweak(s)

			s.h.tr.tasks <- []Task{updateTask("task-1", "upd_abc123", "v1.3.5")}
			res := s.h.nextResult()

			if res.OK || res.Stuck == nil || res.Stuck.Kind != updates.StuckBusy || res.Stuck.Asked {
				t.Fatalf("the update task reported %+v (stuck %+v), want a refusal because the helper is busy", res, res.Stuck)
			}
			if asked := s.root.asked(); len(asked) != 0 {
				t.Errorf("root was asked to repair a helper that is mid-upgrade: %v", asked)
			}
			if got := s.request(t); got.ID != "upd_stale" {
				t.Errorf("the request in the folder is %+v; a live helper's request must be left alone", got)
			}
			if _, err := os.Lstat(filepath.Join(s.dir, updates.RepairFile)); err == nil {
				t.Error("a repair request was left in the folder")
			}
		})
	}
}

// Repair units that are not installed answer nothing. The task says so, in words
// that name the fix, and takes its request back so that nothing acts on it later.
func TestAnUpdateTaskReportsThatNothingAnsweredTheRepairRequest(t *testing.T) {
	s := newStuckHost(t, 72*time.Hour, func(o *Options) { o.RepairWait = 60 * time.Millisecond })

	s.h.tr.tasks <- []Task{updateTask("task-1", "upd_abc123", "v1.3.5")}
	res := s.h.nextResult()

	if res.OK || res.Stuck == nil || res.Stuck.Note != "no_answer" || !res.Stuck.Asked {
		t.Fatalf("the update task reported %+v (stuck %+v), want a failure saying nobody answered", res, res.Stuck)
	}
	if !strings.Contains(res.Error, "sudo zoomies updates helper install") || strings.Contains(res.Error, s.dir) {
		t.Errorf("error = %q, want the fix and no path on the host", res.Error)
	}
	if _, err := os.Lstat(filepath.Join(s.dir, updates.RepairFile)); err == nil {
		t.Error("the repair request was left for a unit that might act on it later")
	}
	if got := s.request(t); got.ID != "upd_stale" {
		t.Errorf("the stale request changed to %+v; the agent must never remove it", got)
	}
}

// Never in a loop: a repair that did not put the helper right is not asked for
// again by the next attempt a minute later.
func TestAnUpdateTaskDoesNotAskForASecondRepairSoSoon(t *testing.T) {
	s := newStuckHost(t, 72*time.Hour)
	s.root.healed = false
	s.root.start(t)

	s.h.tr.tasks <- []Task{updateTask("task-1", "upd_one", "v1.3.5")}
	first := s.h.nextResult()
	if first.OK || first.Stuck == nil || first.Stuck.Note != "refused" || len(s.root.asked()) != 1 {
		t.Fatalf("first = %+v (stuck %+v), repairs asked %v; want one repair that did not heal", first, first.Stuck, s.root.asked())
	}

	s.h.clock.advance(time.Minute)
	s.h.tr.tasks <- []Task{updateTask("task-2", "upd_two", "v1.3.5")}
	second := s.h.nextResult()

	if second.OK || second.Stuck == nil || second.Stuck.Note != "held_back" || second.Stuck.Asked {
		t.Fatalf("second = %+v (stuck %+v), want it held back by the budget", second, second.Stuck)
	}
	if asked := s.root.asked(); len(asked) != 1 {
		t.Errorf("root was asked to repair %d times, want 1: %v", len(asked), asked)
	}
}

// Two attempts arriving together are one repair and one request. The update lock
// serialises them, so the second finds the first's repaired helper and its
// request, and refuses in the ordinary way instead of repairing on top of it.
func TestTwoUpdateTasksRacingAreOneRepairAndOneRequest(t *testing.T) {
	s := newStuckHost(t, 72*time.Hour)
	s.root.start(t)

	s.h.tr.tasks <- []Task{updateTask("task-1", "upd_aaaaaa", "v1.3.5"), updateTask("task-2", "upd_bbbbbb", "v1.3.5")}
	a, b := s.h.nextResult(), s.h.nextResult()

	if asked := s.root.asked(); len(asked) != 1 {
		t.Fatalf("root was asked to repair %d times, want exactly 1: %v", len(asked), asked)
	}
	winners := 0
	for _, res := range []TaskResult{a, b} {
		if res.OK {
			winners++
			if res.Stuck == nil || !res.Stuck.Healed {
				t.Errorf("the task that won reported %+v, want the repair recorded on it", res)
			}
		} else if res.Stuck != nil && res.Stuck.Asked {
			t.Errorf("the task that lost asked for a repair of its own: %+v", res.Stuck)
		}
	}
	if winners != 1 {
		t.Fatalf("%d tasks wrote a request (%+v, %+v), want exactly 1", winners, a, b)
	}
	if got := s.request(t); got.ID != s.root.asked()[0] {
		t.Errorf("the request in the folder is %s, want the one whose attempt asked for the repair (%s)", got.ID, s.root.asked()[0])
	}
}

// A helper that is fine, with a request that is merely recent, is the ordinary
// refusal and nothing more: no repair is asked for.
func TestAFreshWaitingRequestUnderAHealthyHelperIsNotRepaired(t *testing.T) {
	s := newStuckHost(t, time.Minute)
	s.units.heal()
	s.root.start(t)

	s.h.tr.tasks <- []Task{updateTask("task-1", "upd_abc123", "v1.3.5")}
	res := s.h.nextResult()

	if res.OK || res.Stuck != nil || !strings.Contains(res.Error, "zoomies updates helper status") {
		t.Fatalf("the update task reported %+v (stuck %+v), want the ordinary refusal", res, res.Stuck)
	}
	if len(s.root.asked()) != 0 {
		t.Errorf("a repair was asked for under a healthy helper: %v", s.root.asked())
	}
}
