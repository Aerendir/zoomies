package updates

import (
	"strings"
	"testing"
	"time"
)

func healthyDiagnosis() Diagnosis {
	return Diagnosis{
		Path:    UnitState{Active: "active", Enabled: "enabled"},
		Service: UnitState{Active: "inactive", Enabled: "static"},
	}
}

// What the service must never do is repair a helper that is working, and what
// it must always do is repair one that cannot work. Every row is a state a host
// has been seen in or could reach.
func TestClassifyTellsABrokenHelperFromAWorkingOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Diagnosis)
		want   string
		repair bool
	}{
		{"a fresh request under a healthy helper is just not read yet", func(d *Diagnosis) {
			d.RequestPresent, d.RequestAge = true, 20*time.Second
		}, "", false},
		{"a disabled and dead path unit is the stall that failed updates for days", func(d *Diagnosis) {
			d.RequestPresent, d.RequestAge = true, 72*time.Hour
			d.Path = UnitState{Active: "inactive", Enabled: "disabled"}
		}, StuckPathDown, true},
		{"a path unit in the failed state, as a start limit leaves it", func(d *Diagnosis) {
			d.Path = UnitState{Active: "failed", Enabled: "enabled", Failed: true}
		}, StuckPathDown, true},
		{"a failed service unit blocks the next start", func(d *Diagnosis) {
			d.Service = UnitState{Active: "failed", Failed: true}
		}, StuckPathDown, true},
		{"a request older than a healthy helper takes", func(d *Diagnosis) {
			d.RequestPresent, d.RequestAge = true, time.Hour
		}, StuckStaleRequest, true},
		{"a helper that is mid-upgrade is left entirely alone", func(d *Diagnosis) {
			d.RequestPresent, d.RequestAge = true, 3*time.Hour
			d.Service = UnitState{Active: "activating"}
			d.Path = UnitState{Active: "inactive", Enabled: "disabled"}
		}, StuckBusy, false},
		{"an engine alive with the service idle is still mid-upgrade", func(d *Diagnosis) {
			d.EngineRunning, d.LockPresent, d.LockAge = true, true, 4*time.Hour
			d.Path = UnitState{Active: "failed", Failed: true}
		}, StuckBusy, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := healthyDiagnosis()
			tc.change(&d)
			got := Classify(d, OrphanAfter)
			if got.Kind != tc.want || got.Repair != tc.repair {
				t.Errorf("Classify = %+v, want kind %q repair %v", got, tc.want, tc.repair)
			}
		})
	}
}

func TestOnlyAbandonedFilesAreCalledOrphaned(t *testing.T) {
	old := Diagnosis{RequestPresent: true, RequestAge: time.Hour, LockPresent: true, LockAge: 2 * time.Hour}
	if r, l := old.Orphaned(); !r || !l {
		t.Errorf("an hour-old request and a two-hour-old lock with nothing running: orphaned = %v, %v", r, l)
	}
	young := old
	young.RequestAge, young.LockAge = time.Minute, time.Minute
	if r, l := young.Orphaned(); r || l {
		t.Errorf("files a minute old are not abandoned, got %v, %v", r, l)
	}
	for name, busy := range map[string]Diagnosis{
		"a service running": {Service: UnitState{Active: "active"}},
		"an engine running": {EngineRunning: true},
	} {
		d := old
		d.Service, d.EngineRunning = busy.Service, busy.EngineRunning
		if r, l := d.Orphaned(); r || l {
			t.Errorf("with %s nothing is orphaned, got %v, %v", name, r, l)
		}
	}
}

func TestEngineRunningReadsTheSubcommandNotAnyArgument(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		want bool
	}{
		{[]string{"/usr/local/bin/zoomies", "upgrade", "--version", "v1.4.0", "--non-interactive"}, true},
		{[]string{"zoomies", "--config", "/etc/zoomies/zoomies.yaml", "upgrade"}, true},
		{[]string{"/usr/local/bin/zoomies", "updates", "helper", "run"}, false},
		{[]string{"/usr/local/bin/zoomies", "controller"}, false},
		{[]string{"vim", "upgrade"}, false},
		{[]string{"zoomies"}, false},
	} {
		if got := EngineRunning([][]string{tc.argv}); got != tc.want {
			t.Errorf("EngineRunning(%q) = %v, want %v", strings.Join(tc.argv, " "), got, tc.want)
		}
	}
}

func TestTheRepairLedgerSpacesAndBoundsRepairs(t *testing.T) {
	var l RepairLedger
	t0 := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	if ok, _ := l.Admit(t0); !ok {
		t.Fatal("the first repair must be admitted")
	}
	if ok, next := l.Admit(t0.Add(time.Minute)); ok || !next.Equal(t0.Add(RepairMinInterval)) {
		t.Errorf("a second repair a minute later: ok=%v next=%v, want refused until %v", ok, next, t0.Add(RepairMinInterval))
	}
	at := t0
	for range RepairMaxPerDay - 1 {
		at = at.Add(RepairMinInterval)
		if ok, _ := l.Admit(at); !ok {
			t.Fatalf("a repair at %v is within the day's budget", at)
		}
	}
	if ok, next := l.Admit(at.Add(time.Hour)); ok || !next.Equal(t0.Add(24*time.Hour)) {
		t.Errorf("past the day's budget: ok=%v next=%v, want refused until %v", ok, next, t0.Add(24*time.Hour))
	}
	if ok, _ := l.Admit(t0.Add(24*time.Hour + time.Minute)); !ok {
		t.Error("a day later the budget is back")
	}
}

func TestRepairDocumentsAreReadStrictly(t *testing.T) {
	if _, err := ParseRepairRequest([]byte(`{"v":1,"id":"upd_abc123","requested_at":"2026-10-20T09:00:00Z"}`)); err != nil {
		t.Errorf("a good repair request was refused: %v", err)
	}
	for name, body := range map[string]string{
		"an unknown field": `{"v":1,"id":"upd_abc","requested_at":"2026-10-20T09:00:00Z","cmd":"rm -rf /"}`,
		"a path in the id": `{"v":1,"id":"../etc/passwd","requested_at":"2026-10-20T09:00:00Z"}`,
		"another version":  `{"v":9,"id":"upd_abc","requested_at":"2026-10-20T09:00:00Z"}`,
		"two documents":    `{"v":1,"id":"upd_abc","requested_at":"2026-10-20T09:00:00Z"}{}`,
		"no id":            `{"v":1,"requested_at":"2026-10-20T09:00:00Z"}`,
	} {
		if _, err := ParseRepairRequest([]byte(body)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
