package analysis

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

var waitBase = time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)

func at(d time.Duration) *time.Time { v := waitBase.Add(d); return &v }

func waitSnapshot(jobs []WaitJob, pools ...PoolConditions) QueueWaitSnapshot {
	return QueueWaitSnapshot{Since: waitBase.Add(-time.Hour), Until: waitBase.Add(2 * time.Hour), Jobs: jobs, Pools: pools}
}

// linuxJobs makes n jobs queued and eligible at waitBase on pool linux, each
// then shaped by f.
func linuxJobs(n int, f func(*WaitJob)) []WaitJob {
	out := make([]WaitJob, n)
	for i := range out {
		j := WaitJob{ID: fmt.Sprintf("job-%02d", i), PoolID: "pool_linux", Repo: "acme/api",
			Workflow: "ci", JobName: "test", QueuedAt: waitBase, EligibleAt: at(0)}
		f(&j)
		out[i] = j
	}
	return out
}

// ran stamps a runner requested, registered and the job started, as offsets
// from waitBase.
func ran(requested, registered, started time.Duration) func(*WaitJob) {
	return func(j *WaitJob) {
		j.RunnerRequestedAt, j.RunnerRegisteredAt, j.StartedAt = at(requested), at(registered), at(started)
	}
}

func minutes(from, n int) []time.Time {
	out := make([]time.Time, n)
	for i := range out {
		out[i] = waitBase.Add(time.Duration(from+i) * time.Minute)
	}
	return out
}

func linuxPool() PoolConditions { return PoolConditions{PoolID: "pool_linux", PoolName: "linux"} }

type waitFixture struct {
	cause   WaitCause
	snap    QueueWaitSnapshot
	summary string
	setting string
	code    string
	saved   string
}

// waitFixtures seeds each cause on ten jobs of pool linux so that it takes
// most of the wait. Every job waits 320s (260s for runner startup, the
// pinned example) of which 300s (240s) is the cause.
func waitFixtures() []waitFixture {
	sentence := func(phrase string) string {
		return "Jobs waited 53m 20s in all; 94% of it was " + phrase + ", most of it on pool linux."
	}
	ceiling := linuxPool()
	ceiling.AtCeiling = []Interval{{From: waitBase.Add(-time.Minute), To: waitBase.Add(5 * time.Minute)}}
	throttled := linuxPool()
	throttled.NoRoom = minutes(0, 5)
	throttled.Throttled = []Interval{{From: waitBase.Add(-10 * time.Minute), To: waitBase.Add(10 * time.Minute)}}
	noRoom := linuxPool()
	noRoom.NoRoom = minutes(0, 5)
	scheduled := ran(5*time.Minute, 5*time.Minute+10*time.Second, 5*time.Minute+20*time.Second)
	return []waitFixture{
		{CauseHeldForReview, waitSnapshot(linuxJobs(10, func(j *WaitJob) {
			j.HeldAt, j.ApprovedAt, j.EligibleAt = at(0), at(5*time.Minute), at(5*time.Minute)
			scheduled(j)
		}), linuxPool()), sentence("held for a deployment review"), "", "", "50m"},
		{CauseUnmatched, waitSnapshot(linuxJobs(10, func(j *WaitJob) {
			j.EligibleAt = at(5 * time.Minute)
			scheduled(j)
		}), linuxPool()), sentence("waiting for a pool to claim them"), "pool.labels", "jobs.unmatched", "50m"},
		{CausePoolCeiling, waitSnapshot(linuxJobs(10, scheduled), ceiling),
			sentence("a pool at its maximum runners"), "pool.max_runners", "", "50m"},
		{CauseThrottled, waitSnapshot(linuxJobs(10, scheduled), throttled),
			sentence("hosts throttled after sustained pressure"), "host.capacity", "host.throttled", "50m"},
		{CauseNoRoom, waitSnapshot(linuxJobs(10, scheduled), noRoom),
			sentence("no host with room"), "host.capacity", "pool.no_capacity", "50m"},
		{CauseRunnerStartup, waitSnapshot(linuxJobs(10, ran(10*time.Second, 4*time.Minute+10*time.Second, 4*time.Minute+20*time.Second)), linuxPool()),
			"Jobs waited 43m 20s in all; 92% of it was runners starting, most of it on pool linux.",
			"pool.min_runners", "pool.queue_wait_high", "40m"},
		{CauseGitHubDelay, waitSnapshot(linuxJobs(10, ran(10*time.Second, 20*time.Second, 5*time.Minute+20*time.Second)), linuxPool()),
			sentence("GitHub handing jobs to runners that were ready"), "", "", "50m"},
	}
}

func TestQueueWaitNamesTheDominantCause(t *testing.T) {
	codes := CatalogCodes()
	for _, c := range waitFixtures() {
		t.Run(string(c.cause), func(t *testing.T) {
			f, err := QueueWait(c.snap)
			if err != nil {
				t.Fatal(err)
			}
			if f.Analysis != "queue_wait" || f.Version != "queue-wait-v1" || !f.Since.Equal(c.snap.Since) || !f.Until.Equal(c.snap.Until) {
				t.Errorf("finding is labelled %s %s over %v to %v", f.Analysis, f.Version, f.Since, f.Until)
			}
			if f.Dominant != string(c.cause) || f.Confidence != High || f.ConfidenceReason != "" {
				t.Errorf("dominant %q at %s (%q), want %s at high", f.Dominant, f.Confidence, f.ConfidenceReason, c.cause)
			}
			if f.Summary != c.summary {
				t.Errorf("summary\n got %q\nwant %q", f.Summary, c.summary)
			}
			assertNoCommand(t, f.NextSteps)
			if len(f.NextSteps) == 0 {
				t.Fatal("no next step")
			}
			first := f.NextSteps[0]
			if first.Setting != c.setting || first.Code != c.code {
				t.Errorf("first step names setting %q and code %q, want %q and %q", first.Setting, first.Code, c.setting, c.code)
			}
			if !strings.Contains(first.Text, "pool linux") || !strings.Contains(first.Text, "would have saved up to "+c.saved) {
				t.Errorf("first step %q does not name pool linux and what the cause cost there (%s)", first.Text, c.saved)
			}
			for _, s := range f.NextSteps {
				if s.Code != "" && !slices.Contains(codes, s.Code) {
					t.Errorf("step names %q, which CatalogCodes does not list", s.Code)
				}
				if s.Code != "" && !slices.Contains(f.Codes, s.Code) {
					t.Errorf("step names %q, which the finding's codes leave out", s.Code)
				}
			}
			if f.Evidence.Jobs != 10 || f.Evidence.Fleet[0].Cause != c.cause || len(f.Evidence.Pools) != 1 ||
				f.Evidence.Pools[0].Dominant != c.cause || f.Evidence.Pools[0].PoolName != "linux" {
				t.Errorf("evidence does not lead with the cause on pool linux: %+v", f.Evidence)
			}
		})
	}
}

func TestQueueWaitPinsTheRunnerStartupFigures(t *testing.T) {
	c := waitFixtures()[5]
	f, err := QueueWait(c.snap)
	if err != nil {
		t.Fatal(err)
	}
	e := f.Evidence
	want := []CauseShare{{CauseRunnerStartup, 2400, 0.92}, {CauseGitHubDelay, 100, 0.04}, {CauseUnattributed, 100, 0.04}}
	if e.WaitSeconds != 2600 || !slices.Equal(e.Fleet, want) || !slices.Equal(e.Pools[0].Causes, want) || len(e.Pools[0].Longest) != 5 {
		t.Fatalf("got %+v", e)
	}
	if l := e.Pools[0].Longest[0]; l.JobID != "job-00" || l.WaitSeconds != 260 || l.Cause != CauseRunnerStartup || l.JobName != "test" {
		t.Fatalf("longest wait %+v", l)
	}
}

// A failed start leaves the job waiting for another runner; that wait is the
// failure's, and the step should send the reader to the failures rather than
// to warm runners alone.
func TestWaitAfterAFailedStartIsRunnerStartupAndPointsAtTheFailures(t *testing.T) {
	jobs := linuxJobs(10, func(j *WaitJob) {
		j.FirstStartFailureAt = at(time.Minute)
		ran(5*time.Minute, 5*time.Minute+10*time.Second, 5*time.Minute+20*time.Second)(j)
	})
	f, err := QueueWait(waitSnapshot(jobs, linuxPool()))
	if err != nil {
		t.Fatal(err)
	}
	want := []CauseShare{{CauseRunnerStartup, 2500, 0.78}, {CauseUnattributed, 600, 0.19}, {CauseGitHubDelay, 100, 0.03}}
	if !slices.Equal(f.Evidence.Fleet, want) {
		t.Fatalf("fleet %+v, want %+v", f.Evidence.Fleet, want)
	}
	// More warm runners on a pool whose starts fail is more failing starts,
	// so the step names no setting to raise.
	if s := f.NextSteps[0]; s.Setting != "" || s.Code != "pool.runners_failing" || !strings.Contains(s.Text, "failed to start") {
		t.Fatalf("first step %+v", s)
	}
}

func TestQueueWaitWithTooLittleWaitingIsLow(t *testing.T) {
	f, err := QueueWait(waitSnapshot(linuxJobs(3, ran(time.Second, 3*time.Second, 5*time.Second)), linuxPool()))
	if err != nil {
		t.Fatal(err)
	}
	if f.Confidence != Low || f.ConfidenceReason != "only 3 jobs waited, 15s in all: too little to break down" || f.Dominant != "" {
		t.Errorf("got %s %q dominant %q", f.Confidence, f.ConfidenceReason, f.Dominant)
	}
	if f.Summary != "Only 3 jobs waited, 15s in all: too little to break down." {
		t.Errorf("summary %q", f.Summary)
	}
	if f.NextSteps == nil || f.Codes == nil || len(f.NextSteps) != 0 {
		t.Errorf("steps %#v codes %#v, want empty and not nil", f.NextSteps, f.Codes)
	}
	if f.Evidence.WaitSeconds != 15 || f.Evidence.Fleet == nil || f.Evidence.Pools == nil {
		t.Errorf("evidence %+v, want it kept", f.Evidence)
	}

	f, err = QueueWait(waitSnapshot(nil))
	if err != nil {
		t.Fatal(err)
	}
	if f.Confidence != Low || f.ConfidenceReason != "no jobs were queued in this period" ||
		f.Summary != "No jobs were queued in this period." || f.Dominant != "" || f.Evidence.Fleet == nil || f.Evidence.Pools == nil {
		t.Errorf("an empty period reads %s %q %q", f.Confidence, f.ConfidenceReason, f.Summary)
	}
}

// A pool with no eligible host is recorded only as a current problem, never
// minute by minute, so its time falls here; guessing would name the wrong fix.
func TestQueueWaitMostlyUnattributedIsLow(t *testing.T) {
	jobs := linuxJobs(10, ran(5*time.Minute, 5*time.Minute+10*time.Second, 5*time.Minute+20*time.Second))
	f, err := QueueWait(waitSnapshot(jobs, linuxPool()))
	if err != nil {
		t.Fatal(err)
	}
	if f.Dominant != "unattributed" || f.Confidence != Low ||
		!strings.Contains(f.ConfidenceReason, "a pool with no eligible host") || !strings.Contains(f.ConfidenceReason, "pool.no_eligible_host") {
		t.Errorf("got %q at %s (%q)", f.Dominant, f.Confidence, f.ConfidenceReason)
	}
	if want := "Jobs waited 53m 20s in all; 94% of it was time the fleet records no cause for, most of it on pool linux."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	if f.NextSteps[0].Code != "pool.no_eligible_host" || !slices.Contains(f.Codes, "pool.no_eligible_host") {
		t.Errorf("steps %+v codes %v", f.NextSteps, f.Codes)
	}
	assertNoCommand(t, f.NextSteps)
}

func TestQueueWaitSaysWhyItIsOnlyMedium(t *testing.T) {
	startup := ran(10*time.Second, 4*time.Minute+10*time.Second, 4*time.Minute+20*time.Second)
	split := linuxJobs(10, func(j *WaitJob) {
		if j.ID < "job-05" {
			startup(j)
		} else {
			ran(10*time.Second, 20*time.Second, 4*time.Minute+10*time.Second)(j)
		}
	})
	gone := linuxJobs(10, func(j *WaitJob) {
		startup(j)
		if j.ID < "job-02" {
			j.RunnerRequestedAt, j.RunnerRegisteredAt = nil, nil
		}
	})
	for _, c := range []struct {
		name   string
		jobs   []WaitJob
		reason string
	}{
		{"few jobs", linuxJobs(7, startup), "only 7 jobs waited: too few to call one cause dominant"},
		{"no cause took half", split, "the largest cause, runners starting, took 49% of the wait, not half"},
		{"runner records gone", gone, "2 of 10 jobs that waited have no runner record, so their wait after eligibility could not be split into runner startup and GitHub's delay"},
		// The truncation reason has a semicolon of its own, so a bare one
		// between reasons would hide where the second begins.
		{"truncated and few", linuxJobs(7, startup), "the period held more than 5000 jobs; the newest 5000 were read, so narrow since; also, only 7 jobs waited: too few to call one cause dominant"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := waitSnapshot(c.jobs, linuxPool())
			s.JobsTruncated = c.name == "truncated and few"
			f, err := QueueWait(s)
			if err != nil {
				t.Fatal(err)
			}
			if f.Confidence != Medium || f.ConfidenceReason != c.reason || f.Dominant != string(CauseRunnerStartup) {
				t.Errorf("got %q at %s (%q), want runner_startup at medium (%q)", f.Dominant, f.Confidence, f.ConfidenceReason, c.reason)
			}
		})
	}
}

// expectedWait is what a job waited by definition: queue to start, or to the
// end of the period while it still waits, and never less than nothing.
func expectedWait(j WaitJob, until time.Time) int64 {
	end := until
	if j.StartedAt != nil && j.StartedAt.Before(until) {
		end = *j.StartedAt
	}
	return max(0, int64(end.Sub(j.QueuedAt)/time.Second))
}

func reversedJobs() []WaitJob {
	return []WaitJob{
		{ID: "eligible-before-queued", PoolID: "pool_linux", QueuedAt: waitBase, EligibleAt: at(-time.Minute),
			RunnerRequestedAt: at(10 * time.Second), RunnerRegisteredAt: at(20 * time.Second), StartedAt: at(30 * time.Second)},
		{ID: "registered-before-requested", PoolID: "pool_linux", QueuedAt: waitBase, EligibleAt: at(0),
			RunnerRequestedAt: at(time.Minute), RunnerRegisteredAt: at(30 * time.Second), StartedAt: at(2 * time.Minute)},
		{ID: "started-before-eligible", PoolID: "pool_linux", QueuedAt: waitBase, EligibleAt: at(time.Minute),
			RunnerRequestedAt: at(40 * time.Second), RunnerRegisteredAt: at(50 * time.Second), StartedAt: at(30 * time.Second)},
		{ID: "started-before-queued", PoolID: "pool_linux", QueuedAt: waitBase, EligibleAt: at(0), StartedAt: at(-time.Minute)},
	}
}

func TestQueueWaitPhasesAreDisjointAndSumToTheWait(t *testing.T) {
	var snaps []QueueWaitSnapshot
	for _, c := range waitFixtures() {
		snaps = append(snaps, c.snap)
	}
	snaps = append(snaps, waitSnapshot(reversedJobs(), linuxPool()), noRunnerSnapshot(), ceilingAndNoRoomSnapshot(),
		throttleWithoutNoRoomSnapshot(), stillWaitingSnapshot())
	for _, s := range snaps {
		for _, j := range s.Jobs {
			one := s
			one.Jobs = []WaitJob{j}
			f, err := QueueWait(one)
			if err != nil {
				t.Fatal(err)
			}
			var sum int64
			for _, c := range f.Evidence.Fleet {
				if c.Seconds < 0 {
					t.Errorf("%s: %s is %ds", j.ID, c.Cause, c.Seconds)
				}
				sum += c.Seconds
			}
			if want := expectedWait(j, s.Until); sum != want || f.Evidence.WaitSeconds != want {
				t.Errorf("%s: causes sum to %ds and the wait is %ds, want %ds", j.ID, sum, f.Evidence.WaitSeconds, want)
			}
		}
	}
}

func causeSeconds(t *testing.T, s QueueWaitSnapshot) map[WaitCause]int64 {
	t.Helper()
	f, err := QueueWait(s)
	if err != nil {
		t.Fatal(err)
	}
	out := map[WaitCause]int64{}
	for _, c := range f.Evidence.Fleet {
		out[c.Cause] = c.Seconds
	}
	return out
}

func sameCauses(a, b map[WaitCause]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestReversedOrMissingStampsNeverMakeANegativeWait(t *testing.T) {
	want := map[string]map[WaitCause]int64{
		// A runner asked for before the job could be scheduled was already
		// there: the scheduling phase is empty, not negative.
		"eligible-before-queued": {CauseUnattributed: 10, CauseRunnerStartup: 10, CauseGitHubDelay: 10},
		// Two clocks put the registration first: startup is empty and the rest
		// is GitHub's, so the phases still meet end to end.
		"registered-before-requested": {CauseUnattributed: 60, CauseGitHubDelay: 60},
		"started-before-eligible":     {CauseUnmatched: 30},
		"started-before-queued":       {},
	}
	for _, j := range reversedJobs() {
		got := causeSeconds(t, waitSnapshot([]WaitJob{j}, linuxPool()))
		if !sameCauses(got, want[j.ID]) {
			t.Errorf("%s: got %v, want %v", j.ID, got, want[j.ID])
		}
	}
}

func noRunnerSnapshot() QueueWaitSnapshot {
	pool := linuxPool()
	pool.AtCeiling = []Interval{{From: waitBase.Add(2 * time.Minute), To: waitBase.Add(4 * time.Minute)}}
	pool.NoRoom = minutes(6, 2)
	return waitSnapshot([]WaitJob{
		{ID: "runner-gone", PoolID: "pool_linux", QueuedAt: waitBase, EligibleAt: at(0), StartedAt: at(10 * time.Minute)},
		// Still waiting with nothing requested is ordinary scheduling, not a
		// lost record, and neither is a job that never waited.
		{ID: "still-unscheduled", PoolID: "pool_linux", QueuedAt: waitBase.Add(time.Hour), EligibleAt: at(time.Hour)},
		{ID: "never-waited", PoolID: "pool_linux", QueuedAt: waitBase, EligibleAt: at(0), StartedAt: at(0)},
	}, pool)
}

func TestAJobWithNoRunnerRecordIsNotSplitIntoStartupAndGitHubDelay(t *testing.T) {
	s := noRunnerSnapshot()
	f, err := QueueWait(s)
	if err != nil {
		t.Fatal(err)
	}
	if f.Evidence.NoRunnerRecord != 1 {
		t.Errorf("NoRunnerRecord %d, want 1: only a job that started after waiting lost its record", f.Evidence.NoRunnerRecord)
	}
	s.Jobs = s.Jobs[:1]
	got := causeSeconds(t, s)
	if want := map[WaitCause]int64{CausePoolCeiling: 120, CauseNoRoom: 120, CauseUnattributed: 360}; !sameCauses(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A pool that keeps its runners hands a job to one that was there before it,
// busy or idle, so its wait after eligibility cannot be split either; but its
// runner is on record, and saying the record is missing would send the reader
// looking for a pruned runner that is still running.
func TestAJobOnARunnerItsPoolKeepsIsNotALostRecord(t *testing.T) {
	pool := linuxPool()
	pool.AtCeiling = []Interval{{From: waitBase.Add(-time.Minute), To: waitBase.Add(5 * time.Minute)}}
	s := waitSnapshot(linuxJobs(10, func(j *WaitJob) { j.RunnerKept, j.StartedAt = true, at(5*time.Minute) }), pool)
	f, err := QueueWait(s)
	if err != nil {
		t.Fatal(err)
	}
	if f.Evidence.NoRunnerRecord != 0 || f.Evidence.RunnerKept != 10 {
		t.Errorf("no runner record %d, runner kept %d; want 0 and 10", f.Evidence.NoRunnerRecord, f.Evidence.RunnerKept)
	}
	if f.Dominant != string(CausePoolCeiling) || f.Confidence != Medium {
		t.Errorf("dominant %q at %s, want pool_ceiling at medium", f.Dominant, f.Confidence)
	}
	if want := "10 of 10 jobs that waited ran on a runner their pool keeps, which was there before them and counts against its maximum runners whether busy or idle, so their wait after eligibility is put down to the pool's conditions rather than split into runner startup and GitHub's delay"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
}

func TestATruncatedReadIsNeverHighConfidence(t *testing.T) {
	for _, c := range []struct {
		name   string
		set    func(*QueueWaitSnapshot)
		reason string
	}{
		{"jobs", func(s *QueueWaitSnapshot) { s.JobsTruncated = true },
			"the period held more than 5000 jobs; the newest 5000 were read, so narrow since"},
		{"runners", func(s *QueueWaitSnapshot) { s.RunnersTruncated = true },
			"the period held more than 5000 runners; the newest 5000 were read, so narrow since"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := waitFixtures()[5].snap
			c.set(&s)
			f, err := QueueWait(s)
			if err != nil {
				t.Fatal(err)
			}
			if f.Confidence != Medium || f.ConfidenceReason != c.reason || f.Dominant != string(CauseRunnerStartup) {
				t.Errorf("got %q at %s (%q)", f.Dominant, f.Confidence, f.ConfidenceReason)
			}
			if f.Evidence.JobsTruncated != s.JobsTruncated || f.Evidence.RunnersTruncated != s.RunnersTruncated {
				t.Errorf("evidence does not say the read was cut: %+v", f.Evidence)
			}
		})
	}
}

func ceilingAndNoRoomSnapshot() QueueWaitSnapshot {
	pool := linuxPool()
	pool.AtCeiling = []Interval{{From: waitBase.Add(time.Minute), To: waitBase.Add(2 * time.Minute)}}
	pool.NoRoom = minutes(0, 3)
	return waitSnapshot(linuxJobs(1, ran(3*time.Minute, 3*time.Minute, 3*time.Minute)), pool)
}

// A pool at its ceiling would not have started a runner even with room, so
// room is not what it was waiting for.
func TestThePoolCeilingWinsOverNoRoomInTheSameMinute(t *testing.T) {
	got := causeSeconds(t, ceilingAndNoRoomSnapshot())
	if want := map[WaitCause]int64{CausePoolCeiling: 60, CauseNoRoom: 120}; !sameCauses(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func throttleWithoutNoRoomSnapshot() QueueWaitSnapshot {
	pool := linuxPool()
	pool.Throttled = []Interval{{From: waitBase, To: waitBase.Add(4 * time.Minute)}}
	pool.NoRoom = minutes(1, 1)
	return waitSnapshot(linuxJobs(1, ran(4*time.Minute, 4*time.Minute, 4*time.Minute)), pool)
}

// A throttled host with room to spare delayed nobody; blaming it would send
// the reader to a host that was not the cause.
func TestThrottledCountsOnlyWhileThePoolHadNoRoom(t *testing.T) {
	got := causeSeconds(t, throttleWithoutNoRoomSnapshot())
	if want := map[WaitCause]int64{CauseThrottled: 60, CauseUnattributed: 180}; !sameCauses(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func stillWaitingSnapshot() QueueWaitSnapshot {
	return waitSnapshot([]WaitJob{{ID: "still-waiting", PoolID: "pool_linux", QueuedAt: waitBase.Add(time.Hour),
		EligibleAt: at(time.Hour), RunnerRequestedAt: at(time.Hour + 10*time.Second)}}, linuxPool())
}

func TestAStillWaitingJobCountsToTheEndOfThePeriod(t *testing.T) {
	s := stillWaitingSnapshot()
	f, err := QueueWait(s)
	if err != nil {
		t.Fatal(err)
	}
	if f.Evidence.WaitSeconds != 3600 || f.Evidence.Pools[0].Longest[0].WaitSeconds != 3600 {
		t.Errorf("evidence %+v, want the hour to the end of the period", f.Evidence)
	}
	if got, want := causeSeconds(t, s), map[WaitCause]int64{CauseUnattributed: 10, CauseRunnerStartup: 3590}; !sameCauses(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestQueueWaitEvidenceIsBounded(t *testing.T) {
	var jobs []WaitJob
	var pools []PoolConditions
	var total int64
	for p := 0; p < 60; p++ {
		id := fmt.Sprintf("pool_%02d", p)
		pools = append(pools, PoolConditions{PoolID: id, PoolName: id})
		for j := 0; j < 7; j++ {
			wait := time.Duration((p+1)*10+j) * time.Second
			total += int64(wait / time.Second)
			jobs = append(jobs, WaitJob{ID: fmt.Sprintf("%s-job-%d", id, j), PoolID: id, QueuedAt: waitBase,
				EligibleAt: at(0), RunnerRequestedAt: at(0), RunnerRegisteredAt: at(wait), StartedAt: at(wait)})
		}
	}
	f, err := QueueWait(waitSnapshot(jobs, pools...))
	if err != nil {
		t.Fatal(err)
	}
	e := f.Evidence
	if len(e.Pools) != MaxEvidencePools || e.PoolsCut != 10 || e.Jobs != 420 || e.WaitSeconds != total {
		t.Fatalf("%d pools, %d cut, %d jobs, %ds; want 50, 10, 420, %ds", len(e.Pools), e.PoolsCut, e.Jobs, e.WaitSeconds, total)
	}
	if e.Pools[0].PoolID != "pool_59" {
		t.Errorf("first pool %s, want the longest wait, pool_59", e.Pools[0].PoolID)
	}
	for i, p := range e.Pools {
		if i > 0 && p.WaitSeconds > e.Pools[i-1].WaitSeconds {
			t.Errorf("pool %s (%ds) follows a shorter wait", p.PoolID, p.WaitSeconds)
		}
		if len(p.Longest) != MaxLongestPerPool || p.Jobs != 7 {
			t.Errorf("pool %s keeps %d long waits of %d jobs", p.PoolID, len(p.Longest), p.Jobs)
		}
		for k := 1; k < len(p.Longest); k++ {
			if p.Longest[k].WaitSeconds > p.Longest[k-1].WaitSeconds {
				t.Errorf("pool %s: long waits out of order", p.PoolID)
			}
		}
	}
}

func TestQueueWaitRefusesAPeriodItCannotRead(t *testing.T) {
	for _, s := range []QueueWaitSnapshot{
		{Until: waitBase},
		{Since: waitBase, Until: waitBase},
		{Since: waitBase, Until: waitBase.Add(-time.Hour)},
	} {
		if _, err := QueueWait(s); err == nil {
			t.Errorf("QueueWait accepted %v to %v", s.Since, s.Until)
		}
	}
}

// A job that started the moment it was queued is no example of a long wait,
// and listing it would push a real one out of the five.
func TestAJobThatNeverWaitedIsNotAmongTheLongestWaits(t *testing.T) {
	f, err := QueueWait(noRunnerSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range f.Evidence.Pools[0].Longest {
		if l.JobID == "never-waited" || l.WaitSeconds <= 0 {
			t.Errorf("longest waits list %+v", l)
		}
	}
	if f.Evidence.Pools[0].Jobs != 3 {
		t.Errorf("pool counts %d jobs, want all 3", f.Evidence.Pools[0].Jobs)
	}
}

func zeroWaitJobs(n int) []WaitJob {
	out := make([]WaitJob, n)
	for i := range out {
		out[i] = WaitJob{ID: fmt.Sprintf("instant-%03d", i), PoolID: "pool_linux", QueuedAt: waitBase,
			EligibleAt: at(0), RunnerRequestedAt: at(0), RunnerRegisteredAt: at(0), StartedAt: at(0)}
	}
	return out
}

// On a healthy fleet most jobs start at once. Counting them as jobs that
// waited would call two slow jobs a clear pattern, and tell an operator that
// "only 500 jobs waited" when 45 did.
func TestConfidenceAndItsSentencesCountOnlyJobsThatWaited(t *testing.T) {
	startup := ran(0, 30*time.Minute, 30*time.Minute)
	gone := linuxJobs(10, func(j *WaitJob) {
		ran(10*time.Second, 4*time.Minute+10*time.Second, 4*time.Minute+20*time.Second)(j)
		if j.ID < "job-02" {
			j.RunnerRequestedAt, j.RunnerRegisteredAt = nil, nil
		}
	})
	for _, c := range []struct {
		name       string
		jobs       []WaitJob
		waited     int
		confidence Confidence
		reason     string
		summary    string
	}{
		{"two slow jobs among a hundred", append(zeroWaitJobs(98), linuxJobs(2, startup)...), 2, Low,
			"only 2 jobs waited, 1h in all: too little to break down", "Only 2 jobs waited, 1h in all: too little to break down."},
		{"45 brief waits among 500", append(zeroWaitJobs(455), linuxJobs(45, ran(0, time.Second, time.Second))...), 45, Low,
			"only 45 jobs waited, 45s in all: too little to break down", "Only 45 jobs waited, 45s in all: too little to break down."},
		{"seven waits among fifty", append(zeroWaitJobs(43), linuxJobs(7, startup)...), 7, Medium,
			"only 7 jobs waited: too few to call one cause dominant", ""},
		{"lost records among the waits", append(zeroWaitJobs(90), gone...), 10, Medium,
			"2 of 10 jobs that waited have no runner record, so their wait after eligibility could not be split into runner startup and GitHub's delay", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := QueueWait(waitSnapshot(c.jobs, linuxPool()))
			if err != nil {
				t.Fatal(err)
			}
			if f.Evidence.Jobs != len(c.jobs) || f.Evidence.Waited != c.waited {
				t.Errorf("read %d and waited %d, want %d and %d", f.Evidence.Jobs, f.Evidence.Waited, len(c.jobs), c.waited)
			}
			if f.Confidence != c.confidence || f.ConfidenceReason != c.reason {
				t.Errorf("got %s (%q), want %s (%q)", f.Confidence, f.ConfidenceReason, c.confidence, c.reason)
			}
			if c.summary != "" && f.Summary != c.summary {
				t.Errorf("summary %q, want %q", f.Summary, c.summary)
			}
		})
	}
}

// A job no pool has claimed is the typo in runs-on that jobs.unmatched exists
// for; calling its wait unexplained would send the reader to the wrong fix.
func TestAJobNoPoolHasClaimedYetIsUnmatched(t *testing.T) {
	var jobs []WaitJob
	for i := 0; i < 10; i++ {
		j := WaitJob{ID: fmt.Sprintf("unclaimed-%d", i), QueuedAt: waitBase}
		if i == 0 {
			// Started after the period ended: still waiting at Until.
			j.StartedAt = at(3 * time.Hour)
		}
		jobs = append(jobs, j)
	}
	f, err := QueueWait(waitSnapshot(jobs))
	if err != nil {
		t.Fatal(err)
	}
	if f.Dominant != string(CauseUnmatched) || f.Confidence != High || f.Evidence.NoRunnerRecord != 0 {
		t.Errorf("got %q at %s (%q), %d without a record", f.Dominant, f.Confidence, f.ConfidenceReason, f.Evidence.NoRunnerRecord)
	}
	if want := "Jobs waited 20h in all; 100% of it was waiting for a pool to claim them."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	if s := f.NextSteps[0]; s.Setting != "pool.labels" || s.Code != "jobs.unmatched" || !strings.Contains(s.Text, "would have saved up to 20h") {
		t.Errorf("first step %+v", s)
	}
	assertNoCommand(t, f.NextSteps)
}

// A job that started with no eligible_at is a row from before the column was
// written, or an anomaly: something claimed it, so its wait is not
// unmatched, and with no runner on record it cannot be split either.
func TestAStartedJobWithNoEligibleStampIsUnattributed(t *testing.T) {
	s := waitSnapshot([]WaitJob{{ID: "old-row", PoolID: "pool_linux", QueuedAt: waitBase, StartedAt: at(10 * time.Minute)}}, linuxPool())
	f, err := QueueWait(s)
	if err != nil {
		t.Fatal(err)
	}
	if f.Evidence.NoRunnerRecord != 1 {
		t.Errorf("NoRunnerRecord %d, want 1", f.Evidence.NoRunnerRecord)
	}
	if got, want := causeSeconds(t, s), map[WaitCause]int64{CauseUnattributed: 600}; !sameCauses(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestQueueWaitOffersAStepForEachCauseOfATenthOrMoreUpToThree(t *testing.T) {
	ceiling := linuxPool()
	ceiling.AtCeiling = []Interval{{From: waitBase, To: waitBase.Add(time.Hour)}}
	five := linuxJobs(10, func(j *WaitJob) {
		// held 100s, unmatched 60s, at the ceiling 50s, starting 40s, GitHub 10s
		j.HeldAt, j.ApprovedAt, j.EligibleAt = at(0), at(100*time.Second), at(160*time.Second)
		ran(210*time.Second, 250*time.Second, 260*time.Second)(j)
	})
	three := linuxJobs(10, func(j *WaitJob) {
		// held 200s, starting 50s, GitHub 10s
		j.HeldAt, j.ApprovedAt, j.EligibleAt = at(0), at(200*time.Second), at(200*time.Second)
		ran(200*time.Second, 250*time.Second, 260*time.Second)(j)
	})
	for _, c := range []struct {
		name  string
		snap  QueueWaitSnapshot
		codes []string
	}{
		{"four causes over a tenth", waitSnapshot(five, ceiling), []string{"", "jobs.unmatched", ""}},
		{"two causes over a tenth", waitSnapshot(three, linuxPool()), []string{"", "pool.queue_wait_high"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := QueueWait(c.snap)
			if err != nil {
				t.Fatal(err)
			}
			var codes []string
			for _, s := range f.NextSteps {
				codes = append(codes, s.Code)
			}
			if !slices.Equal(codes, c.codes) {
				t.Errorf("steps %+v, want codes %q", f.NextSteps, c.codes)
			}
		})
	}
}

// The card shows each pool's row beside the total; rounding each pool's
// sub-second remainders separately would make them disagree.
func TestPoolRowsAddExactlyToTheFleetWait(t *testing.T) {
	var jobs []WaitJob
	var pools []PoolConditions
	for _, id := range []string{"pool_a", "pool_b", "pool_c"} {
		pools = append(pools, PoolConditions{PoolID: id, PoolName: id})
		jobs = append(jobs, WaitJob{ID: id + "-job", PoolID: id, QueuedAt: waitBase, EligibleAt: at(0),
			RunnerRequestedAt: at(0), RunnerRegisteredAt: at(0), StartedAt: at(1400 * time.Millisecond)})
	}
	f, err := QueueWait(waitSnapshot(jobs, pools...))
	if err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, p := range f.Evidence.Pools {
		sum += p.WaitSeconds
	}
	if sum != f.Evidence.WaitSeconds || f.Evidence.WaitSeconds != 3 {
		t.Errorf("pools add to %ds and the fleet waited %ds, want both 3s", sum, f.Evidence.WaitSeconds)
	}
}

// A runner was asked for, so something claimed the job even with no
// eligible_at on record: its wait is scheduling and startup, not unmatched,
// and calling it unmatched would send the reader to labels that do match.
func TestAJobWithARunnerRequestedButNoEligibleStampIsNotUnmatched(t *testing.T) {
	s := waitSnapshot([]WaitJob{{ID: "requested", PoolID: "pool_linux", QueuedAt: waitBase,
		RunnerRequestedAt: at(time.Minute)}}, linuxPool())
	if got, want := causeSeconds(t, s), map[WaitCause]int64{CauseUnattributed: 60, CauseRunnerStartup: 7140}; !sameCauses(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A job that started at or after the end of the period was still waiting
// when it ended; its runner may simply not exist yet, so it has lost no
// record and must not pull the finding's confidence down.
func TestAJobStillWaitingAtTheEndHasNotLostItsRunnerRecord(t *testing.T) {
	until := waitBase.Add(2 * time.Hour)
	for _, started := range []time.Time{until, until.Add(time.Hour)} {
		s := waitSnapshot([]WaitJob{{ID: "late", PoolID: "pool_linux", QueuedAt: waitBase, EligibleAt: at(0),
			StartedAt: &started}}, linuxPool())
		f, err := QueueWait(s)
		if err != nil {
			t.Fatal(err)
		}
		if f.Evidence.NoRunnerRecord != 0 || f.Evidence.WaitSeconds != 7200 {
			t.Errorf("started at %v: %d without a record, %ds waited; want 0 and 7200s", started, f.Evidence.NoRunnerRecord, f.Evidence.WaitSeconds)
		}
	}
}
