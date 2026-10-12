package analysis

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/scheduler"
	"github.com/eyupio/zoomies/internal/store"
)

var pressBase = time.Date(2026, 10, 12, 6, 0, 0, 0, time.UTC)

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }

// pressTime is half a minute into minute m of the period, so every lookup
// has to truncate to find its sample.
func pressTime(m int) *time.Time {
	v := pressBase.Add(time.Duration(m)*time.Minute + 30*time.Second)
	return &v
}

// loadMinutes is one sample a minute for n minutes, the first busy of them at
// busyLoad a core and the rest at calmLoad, CPU and memory unmeasured.
func loadMinutes(n, busy int, busyLoad, calmLoad float64) []HostMinute {
	out := make([]HostMinute, n)
	for i := range out {
		load := calmLoad
		if i < busy {
			load = busyLoad
		}
		out[i] = HostMinute{At: pressBase.Add(time.Duration(i) * time.Minute), LoadPerCPU: f64(load)}
	}
	return out
}

func pressSnapshot(minutes []HostMinute, jobs []HostJob) HostPressureSnapshot {
	return HostPressureSnapshot{Since: pressBase, Until: pressBase.Add(2 * time.Hour), HostID: "host_demob",
		HostName: "demo-builder-2", Minutes: minutes, Jobs: jobs, Thresholds: DefaultPressure}
}

// failedIn is one failed job per minute given, completing in that minute.
func failedIn(minutes ...int) []HostJob {
	var out []HostJob
	for _, m := range minutes {
		out = append(out, HostJob{ID: fmt.Sprintf("job-%03d", m), Repo: "acme/api", Workflow: "ci", JobName: "test",
			StartedAt: pressTime(m - 3), CompletedAt: pressTime(m), Conclusion: "failure"})
	}
	return out
}

func mustPressure(t *testing.T, s HostPressureSnapshot) HostPressureFinding {
	t.Helper()
	f, err := HostPressure(s)
	if err != nil {
		t.Fatal(err)
	}
	if f.Analysis != "host_pressure" || f.Version != HostPressureVersion {
		t.Errorf("finding is %s/%s, want host_pressure/%s", f.Analysis, f.Version, HostPressureVersion)
	}
	if f.NextSteps == nil || f.Codes == nil || f.Evidence.Events == nil {
		t.Error("next steps, codes and events must be empty lists, never nil")
	}
	if (f.Confidence == High) != (f.ConfidenceReason == "") {
		t.Errorf("confidence %s with reason %q: a reason is given exactly when it is not high", f.Confidence, f.ConfidenceReason)
	}
	if strings.Contains(f.Summary, "\u2014") || strings.Contains(f.ConfidenceReason, "\u2014") {
		t.Error("an em dash in the finding's prose")
	}
	assertNoCommand(t, f.NextSteps)
	return f
}

func TestHostPressureFindsFailuresClusteredInBusyMinutes(t *testing.T) {
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), failedIn(1, 4, 7, 10, 13, 16, 60)))
	if f.Dominant != "pressure" || f.Confidence != High {
		t.Errorf("dominant %q at %s (%s), want pressure at high", f.Dominant, f.Confidence, f.ConfidenceReason)
	}
	want := PressureThreshold{Measure: "load_per_cpu", Value: 2.5, EventShare: 6.0 / 7, MinuteShare: 20.0 / 120}
	if f.Evidence.Threshold == nil || *f.Evidence.Threshold != want {
		t.Errorf("threshold %+v, want %+v", f.Evidence.Threshold, want)
	}
	if got := "demo-builder-2's failures cluster in its busy minutes: 6 of 7 came while its load per core was at or above 2.5, which was true for 17% of the period."; f.Summary != got {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, got)
	}
	if len(f.NextSteps) == 0 || f.NextSteps[0].Setting != "host.capacity" || f.NextSteps[0].Code != "host.throttled" {
		t.Errorf("first step %+v, want host.capacity and host.throttled", f.NextSteps)
	}
	if !slices.Contains(f.Codes, "host.throttled") {
		t.Errorf("codes %v leave out the first step's", f.Codes)
	}
	e := f.Evidence
	if e.HostID != "host_demob" || e.HostName != "demo-builder-2" || e.Minutes != 120 || e.ExpectedMinutes != 120 ||
		e.PressuredMinutes != 20 || e.MeasuredEvents != 7 || e.PressuredEvents != 6 || e.Unmeasured != 0 || len(e.Events) != 7 {
		t.Errorf("evidence %+v", e)
	}
	if e.Pressure != DefaultPressure {
		t.Errorf("evidence says pressure was %+v, want what the snapshot judged by, %+v", e.Pressure, DefaultPressure)
	}
}

func TestHostPressureSaysWhenFailuresDoNotCluster(t *testing.T) {
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), failedIn(5, 30, 50, 70, 90, 110)))
	if f.Dominant != "not_pressure" || f.Confidence != High {
		t.Errorf("dominant %q at %s (%s), want not_pressure at high", f.Dominant, f.Confidence, f.ConfidenceReason)
	}
	if got := "demo-builder-2's failures do not cluster in its busy minutes: 5 of 6 came while it was calm."; f.Summary != got {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, got)
	}
	if len(f.NextSteps) == 0 || f.NextSteps[0].Code != "jobs.workflow_failing" {
		t.Errorf("first step %+v, want jobs.workflow_failing", f.NextSteps)
	}
	// A threshold is the line that separates failures from calm minutes, and
	// here nothing does; one reported anyway would be read as the cause.
	if f.Evidence.Threshold != nil {
		t.Errorf("threshold %+v on failures that do not cluster", f.Evidence.Threshold)
	}
}

// When the load is not why, each kind of thing that went wrong has its own
// place to look, and the sentence names every kind it counts.
func TestCalmFailuresOOMKillsAndSlowStartupsEachGetAStep(t *testing.T) {
	jobs := failedIn(5, 30, 50, 70)
	jobs[3].OOMKilled = true
	jobs = append(jobs, HostJob{ID: "job-slow", StartedAt: pressTime(90), CompletedAt: pressTime(95), Conclusion: "success", StartupSeconds: i64(300)})
	for i := range 3 {
		jobs = append(jobs, HostJob{ID: fmt.Sprintf("fast-%d", i), StartedAt: pressTime(100), CompletedAt: pressTime(105),
			Conclusion: "success", StartupSeconds: i64(10)})
	}
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), jobs))
	if want := "demo-builder-2's failures, OOM kills and slow startups do not cluster in its busy minutes: 4 of 5 came while it was calm."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	var codes []string
	for _, s := range f.NextSteps {
		codes = append(codes, s.Code)
	}
	if want := []string{"jobs.workflow_failing", "jobs.oom_killed", "pool.queue_wait_high"}; !slices.Equal(codes, want) || !slices.Equal(f.Codes, want) {
		t.Errorf("steps' codes %v, finding's %v, want %v", codes, f.Codes, want)
	}
}

// An OOM kill that came while the host was calm is the job's limit, not the
// host's memory, so a clustered finding names OOM kills first only when they
// came under pressure.
func TestACalmOOMKillIsNotBlamedOnTheHostsMemory(t *testing.T) {
	jobs := failedIn(1, 4, 7, 10, 13, 16, 60)
	jobs[6].OOMKilled = true
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), jobs))
	if f.Dominant != "pressure" || f.NextSteps[0].Code != "host.throttled" {
		t.Errorf("dominant %q, first step %+v, want pressure led by host.throttled", f.Dominant, f.NextSteps[0])
	}
}

// A host busy most of the time sees most of its failures while busy whether
// or not its load has anything to do with them, so saying "came while it was
// calm" of the few that did would hide why they do not cluster.
func TestFailuresMostlyInBusyMinutesOfAMostlyBusyHostDoNotCluster(t *testing.T) {
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 100, 2.5, 0.5), failedIn(1, 10, 20, 30, 40, 50, 60, 70, 105, 115)))
	if f.Dominant != "not_pressure" {
		t.Errorf("dominant %q, want not_pressure", f.Dominant)
	}
	if got := "demo-builder-2's failures do not cluster in its busy minutes: 8 of 10 came while it was under pressure, but it was under pressure for 83% of the period."; f.Summary != got {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, got)
	}
}

// The rule is "at least twice the rate": exactly twice clusters, a hair
// under does not.
func TestFailuresClusterFromExactlyTwiceTheCalmRate(t *testing.T) {
	// 24 busy minutes and 96 calm: 3 busy failures against 6 calm is a rate
	// of 1/8 against 1/16, exactly twice; against 7 calm it is under.
	exact := mustPressure(t, pressSnapshot(loadMinutes(120, 24, 2.5, 0.5), failedIn(2, 12, 22, 30, 45, 60, 75, 90, 110)))
	if exact.Dominant != "pressure" {
		t.Errorf("exactly twice the calm rate: dominant %q, want pressure", exact.Dominant)
	}
	// The 20th percentile of the failures' loads is a calm one, which
	// separates nothing, so the sentence names the pressure itself.
	if got := "demo-builder-2's failures cluster in its busy minutes: 3 of 9 came while it was under pressure, which was true for 20% of the period."; exact.Summary != got {
		t.Errorf("summary\n got %q\nwant %q", exact.Summary, got)
	}
	if exact.Evidence.Threshold != nil {
		t.Errorf("threshold %+v separates nothing", exact.Evidence.Threshold)
	}
	under := mustPressure(t, pressSnapshot(loadMinutes(120, 24, 2.5, 0.5), failedIn(2, 12, 22, 30, 40, 50, 60, 75, 90, 110)))
	if under.Dominant != "not_pressure" {
		t.Errorf("under twice the calm rate: dominant %q, want not_pressure", under.Dominant)
	}
}

// One or two failures in the only busy minutes of a day are a coincidence,
// not a cluster, however much denser they make those minutes: called High,
// they would tell an operator that load is the cause and capacity the fix
// while most failures had nothing to do with it. Three is the fewest, as it
// is for judging at all.
func TestFewerThanThreePressuredEventsNeverCluster(t *testing.T) {
	for _, pressured := range []int{1, 2, 3} {
		var ms []int
		for i := range pressured {
			ms = append(ms, i)
		}
		ms = append(ms, 30, 50, 70, 90)
		f := mustPressure(t, pressSnapshot(loadMinutes(120, pressured, 2.5, 0.5), failedIn(ms...)))
		want := map[bool]string{true: "pressure", false: "not_pressure"}[pressured >= 3]
		if f.Dominant != want {
			t.Errorf("%d pressured failures of %d: dominant %q (%s), want %s", pressured, len(ms), f.Dominant, f.Summary, want)
		}
	}
}

// A cluster most events stand outside of is real but not the whole story:
// it is never High, and the calm events get their own place to look, not
// only the host's capacity.
func TestACalmMajorityCapsAClusterAtMedium(t *testing.T) {
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 3, 2.5, 0.5), failedIn(0, 1, 2, 30, 50, 70, 90)))
	if f.Dominant != "pressure" || f.Confidence != Medium {
		t.Fatalf("dominant %q at %s, want pressure at medium", f.Dominant, f.Confidence)
	}
	if want := "only 3 of 7 came while it was under pressure, and the other 4 came while it was calm"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
	var codes []string
	for _, s := range f.NextSteps {
		codes = append(codes, s.Code)
	}
	if want := []string{"host.throttled", "jobs.workflow_failing"}; !slices.Equal(codes, want) {
		t.Errorf("steps' codes %v, want %v", codes, want)
	}
	// The capacity step is about the pressured ones and says how many
	// they were; "its failures came in the minutes it was under pressure"
	// would be false of the other 4.
	if !strings.Contains(f.NextSteps[0].Text, ": 3 of its 7 failures came in the minutes it was under pressure,") {
		t.Errorf("capacity step %q does not say how many came under pressure", f.NextSteps[0].Text)
	}
	if !strings.Contains(f.NextSteps[1].Text, "while it was calm") {
		t.Errorf("calm step %q does not say which failures it is about", f.NextSteps[1].Text)
	}
	// Half and half is not a calm majority.
	even := mustPressure(t, pressSnapshot(loadMinutes(120, 3, 2.5, 0.5), failedIn(0, 1, 2, 30, 50, 70)))
	if even.Confidence != High || len(even.NextSteps) != 1 {
		t.Errorf("3 pressured and 3 calm: %s (%s), steps %+v, want high with the capacity step alone", even.Confidence, even.ConfidenceReason, even.NextSteps)
	}
	if !strings.Contains(even.NextSteps[0].Text, ": 3 of its 6 failures came in the minutes it was under pressure,") {
		t.Errorf("capacity step %q does not say how many came under pressure", even.NextSteps[0].Text)
	}
	// Where every one came under pressure, the step can say so plainly.
	all := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), failedIn(1, 4, 7, 10, 13)))
	if !strings.Contains(all.NextSteps[0].Text, ": its failures came in the minutes it was under pressure,") {
		t.Errorf("capacity step %q, want every failure said to have come under pressure", all.NextSteps[0].Text)
	}
}

// A line with most minutes above it says nothing about when things went
// wrong, even with a few calm minutes below it, so the finding falls back to
// naming the pressure rather than a line that separates nothing.
func TestALineUnderMostMinutesSeparatesNothing(t *testing.T) {
	minutes := loadMinutes(120, 20, 2.5, 0.5)
	for i := 110; i < 120; i++ {
		minutes[i].LoadPerCPU = f64(0.1)
	}
	f := mustPressure(t, pressSnapshot(minutes, failedIn(2, 5, 8, 30, 50, 70, 90, 115)))
	if f.Dominant != "pressure" || f.Evidence.Threshold != nil {
		t.Errorf("dominant %q, threshold %+v; a line at 0.5 a core is under 110 of 120 minutes", f.Dominant, f.Evidence.Threshold)
	}
	if got := "demo-builder-2's failures cluster in its busy minutes: 3 of 8 came while it was under pressure, which was true for 17% of the period."; f.Summary != got {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, got)
	}
}

// F9: pressure is the throttle's own "overwhelmed" test. A host whose
// runners are all inside their CPU quotas sits near 100% CPU all day and is
// healthy; reading that as pressure would make every busy host's failures
// "cluster in its busy minutes".
func TestCPUAloneNeverMakesAMinutePressured(t *testing.T) {
	minutes := loadMinutes(120, 0, 0, 1.0)
	for i := range minutes {
		minutes[i].CPUPercent = f64(99)
	}
	f := mustPressure(t, pressSnapshot(minutes, failedIn(1, 4, 7, 10, 13, 16)))
	if f.Evidence.PressuredMinutes != 0 || f.Dominant != "not_pressure" {
		t.Errorf("%d pressured minutes, dominant %q; CPU at 99%% is not pressure", f.Evidence.PressuredMinutes, f.Dominant)
	}
	if got := "demo-builder-2 was not under pressure in any minute it was measured, so none of its 6 failures came under pressure."; f.Summary != got {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, got)
	}
}

// The line is the throttle's: load at exactly twice the cores is
// overwhelmed, a hundredth under is not.
func TestLoadAtTwiceTheCoresIsPressure(t *testing.T) {
	for _, c := range []struct {
		load float64
		want int
	}{{2.0, 20}, {1.99, 0}} {
		f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, c.load, 0.5), nil))
		if f.Evidence.PressuredMinutes != c.want {
			t.Errorf("load %v a core: %d pressured minutes, want %d", c.load, f.Evidence.PressuredMinutes, c.want)
		}
	}
}

// The scheduler throttles a host whose available memory is at or below its
// reserve; a minute there is pressured, a megabyte above it is not.
func TestMemoryAtTheReserveIsPressure(t *testing.T) {
	for _, c := range []struct {
		available int64
		want      int
	}{{1024, 20}, {1025, 0}} {
		minutes := loadMinutes(120, 0, 0, 0.5)
		for i := range minutes[:20] {
			minutes[i].MemoryAvailableMB, minutes[i].MemoryMB = i64(c.available), 16384
		}
		s := pressSnapshot(minutes, nil)
		s.Thresholds.MemoryReserveMB = 1024
		f := mustPressure(t, s)
		if f.Evidence.PressuredMinutes != c.want {
			t.Errorf("%d MB available against a 1024 MB reserve: %d pressured minutes, want %d", c.available, f.Evidence.PressuredMinutes, c.want)
		}
	}
}

func TestOneFailureIsCountedInTheSingular(t *testing.T) {
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), failedIn(1)))
	if want := "only 1 failure, OOM kill or slow startup: too few to say whether they follow its load"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
}

func TestHostPressureWithTooFewFailuresIsLow(t *testing.T) {
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), failedIn(1, 4)))
	if f.Confidence != Low || f.Dominant != "" {
		t.Errorf("dominant %q at %s, want none at low", f.Dominant, f.Confidence)
	}
	if want := "only 2 failures, OOM kills and slow startups: too few to say whether they follow its load"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
	if want := "demo-builder-2 had only 2 failures, OOM kills and slow startups in this period: too few to say whether they follow its load."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
}

// Three is the fewest measured events judged: two is too few, three is not.
func TestThreeMeasuredEventsAreEnoughToJudge(t *testing.T) {
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), failedIn(1, 4, 7)))
	if f.Dominant != "pressure" || f.Confidence != Medium {
		t.Errorf("dominant %q at %s, want pressure at medium", f.Dominant, f.Confidence)
	}
	if want := "only 3 failures came in a measured minute, too few to be sure of the pattern"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
}

// Five measured events are the fewest that make a finding high.
func TestFiveMeasuredEventsAreEnoughForHighConfidence(t *testing.T) {
	for _, c := range []struct {
		jobs []HostJob
		want Confidence
	}{{failedIn(1, 4, 7, 10), Medium}, {failedIn(1, 4, 7, 10, 13), High}} {
		f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), c.jobs))
		if f.Confidence != c.want {
			t.Errorf("%d measured events: %s (%s), want %s", len(c.jobs), f.Confidence, f.ConfidenceReason, c.want)
		}
	}
}

func TestFailuresOutsideAnySampleAreTooFewWhenMeasuredOnesAre(t *testing.T) {
	// Samples stop after an hour; three of the five failures come later.
	f := mustPressure(t, pressSnapshot(loadMinutes(60, 20, 2.5, 0.5), failedIn(1, 4, 80, 90, 100)))
	if f.Confidence != Low || f.Dominant != "" || f.Evidence.Unmeasured != 3 {
		t.Errorf("dominant %q at %s, %d unmeasured", f.Dominant, f.Confidence, f.Evidence.Unmeasured)
	}
	if want := "only 2 of its 5 failures, OOM kills and slow startups came in a minute with a sample: too few to say whether they follow its load"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
}

// A burst of failures while the agent sent nothing is what an operator most
// wants to hear about, so it is counted in the sentence and, once it is more
// than a tenth of them, lowers the confidence.
func TestFailuresWhereTheHostSentNoSamplesAreCountedAndSaid(t *testing.T) {
	minutes := loadMinutes(120, 20, 2.5, 0.5)
	minutes = append(minutes[:60:60], minutes[72:]...) // 108 of 120 minutes: 90%, enough on its own
	f := mustPressure(t, pressSnapshot(minutes, failedIn(1, 4, 7, 10, 13, 62, 63, 64, 65, 66, 67, 68, 69, 70, 90)))
	if f.Evidence.Unmeasured != 9 || f.Confidence != Medium {
		t.Fatalf("%d unmeasured at %s (%s), want 9 at medium", f.Evidence.Unmeasured, f.Confidence, f.ConfidenceReason)
	}
	if want := "demo-builder-2's failures cluster in its busy minutes: 5 of 6 came while its load per core was at or above 2.5, which was true for 19% of the minutes it was measured; 9 of 15 fell where the host sent no samples."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	if want := "9 of its 15 failures came in minutes with no sample"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
	// A tenth unmeasured is said but does not lower the confidence; more does.
	for _, c := range []struct {
		jobs []HostJob
		want Confidence
	}{
		{failedIn(1, 4, 7, 10, 13, 16, 19, 30, 50, 115), High},
		{failedIn(1, 4, 7, 10, 13, 16, 19, 30, 115, 118), Medium},
	} {
		// Samples for the first 108 minutes, 90%; the last events fall
		// well past them.
		f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5)[:108], c.jobs))
		if f.Confidence != c.want {
			t.Errorf("%d of %d unmeasured: %s (%s), want %s", f.Evidence.Unmeasured, len(c.jobs), f.Confidence, f.ConfidenceReason, c.want)
		}
		if !strings.Contains(f.Summary, "fell where the host sent no samples") {
			t.Errorf("summary %q does not count the unmeasured", f.Summary)
		}
	}
}

// gapMinutes is the busy-then-calm load with no samples for minutes 60 to
// 71, so events from minute 62 to 71 have none near them.
func gapMinutes(busy int) []HostMinute {
	m := loadMinutes(120, busy, 2.5, 0.5)
	return append(m[:60:60], m[72:]...)
}

func oomIn(minutes ...int) []HostJob {
	jobs := failedIn(minutes...)
	for i := range jobs {
		jobs[i].ID, jobs[i].OOMKilled = fmt.Sprintf("oom-%03d", minutes[i]), true
	}
	return jobs
}

// A count of measured events names only the kinds among them: OOM kills
// that all fell in a gap are named in the gap's clause and nowhere else, or
// "5 of 6" would read as OOM kills pointing at the host's load.
func TestAKindOnlyInTheGapIsNotNamedInACountOfMeasuredEvents(t *testing.T) {
	clustered := mustPressure(t, pressSnapshot(gapMinutes(20), append(failedIn(1, 4, 7, 10, 13, 90), oomIn(62, 63, 64, 65, 66, 67, 68)...)))
	if want := "demo-builder-2's failures cluster in its busy minutes: 5 of 6 came while its load per core was at or above 2.5, which was true for 19% of the minutes it was measured; 7 of 13 fell where the host sent no samples."; clustered.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", clustered.Summary, want)
	}
	for _, step := range clustered.NextSteps {
		if strings.Contains(step.Text, "OOM") || step.Code == "jobs.oom_killed" {
			t.Errorf("step %+v names OOM kills, none of which came in a measured minute", step)
		}
	}
	if want := "7 of its 13 failures and OOM kills came in minutes with no sample"; !strings.Contains(clustered.ConfidenceReason, want) {
		t.Errorf("reason %q does not count every kind in the gap: %q", clustered.ConfidenceReason, want)
	}

	calm := mustPressure(t, pressSnapshot(gapMinutes(0), append(failedIn(10, 20, 30, 40), oomIn(62, 63, 64, 65, 66, 67, 68)...)))
	if want := "demo-builder-2 was not under pressure in any minute it was measured, so none of the 4 failures in measured minutes came under pressure; 7 of 11 fell where the host sent no samples."; calm.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", calm.Summary, want)
	}
}

// With neither samples nor failures there is nothing to say about failures.
func TestAHostWithNoSamplesAndNoFailuresIsNotToldAboutFailures(t *testing.T) {
	f := mustPressure(t, pressSnapshot(nil, nil))
	if want := "demo-builder-2 had no failures, OOM kills or slow startups in this period."; f.Summary != want {
		t.Errorf("summary %q, want %q", f.Summary, want)
	}
}

func TestAHostWithNoFailuresHasNothingToTieToItsLoad(t *testing.T) {
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), nil))
	if f.Confidence != Low || f.Dominant != "" || len(f.NextSteps) != 0 {
		t.Errorf("dominant %q at %s with steps %+v", f.Dominant, f.Confidence, f.NextSteps)
	}
	if want := "demo-builder-2 had no failures, OOM kills or slow startups in this period."; f.Summary != want {
		t.Errorf("summary %q, want %q", f.Summary, want)
	}
}

func TestAHostThatSentNoMeasurementsIsLow(t *testing.T) {
	// A sample row with no load, CPU or memory is what an agent too old to
	// measure them leaves, and is no measurement.
	blank := []HostMinute{{At: pressBase}, {At: pressBase.Add(time.Minute)}}
	for name, minutes := range map[string][]HostMinute{"no samples": nil, "samples with nothing measured": blank} {
		f := mustPressure(t, pressSnapshot(minutes, failedIn(1, 4, 7, 10, 13)))
		if f.Confidence != Low || f.Dominant != "" || f.Evidence.Minutes != 0 {
			t.Errorf("%s: dominant %q at %s over %d minutes", name, f.Dominant, f.Confidence, f.Evidence.Minutes)
		}
		for _, want := range []string{"agent too old", "retention.samples"} {
			if !strings.Contains(f.ConfidenceReason, want) {
				t.Errorf("%s: reason %q does not name %q", name, f.ConfidenceReason, want)
			}
		}
		if want := "demo-builder-2 sent no load, CPU or memory samples for this period, so whether its failures follow its load cannot be said."; f.Summary != want {
			t.Errorf("%s: summary %q", name, f.Summary)
		}
		if len(f.NextSteps) != 2 || f.NextSteps[0].Code != "host.version_behind" || f.NextSteps[1].Setting != "retention.samples" {
			t.Errorf("%s: steps %+v, want host.version_behind then retention.samples", name, f.NextSteps)
		}
	}
}

// A host under pressure the whole time has no calm minutes to set its
// failures against; it is a finding about capacity, not about clustering.
func TestAHostUnderPressureEveryMinuteCannotBeToldApart(t *testing.T) {
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 120, 2.5, 0.5), failedIn(1, 4, 7, 10, 13)))
	if f.Confidence != Low || f.Dominant != "" {
		t.Errorf("dominant %q at %s, want none at low", f.Dominant, f.Confidence)
	}
	if want := "demo-builder-2 was under pressure in every minute it was measured, so there are no calm minutes to set its 5 failures against."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	if len(f.NextSteps) != 1 || f.NextSteps[0].Setting != "host.capacity" || f.NextSteps[0].Code != "host.throttled" {
		t.Errorf("steps %+v, want host.capacity", f.NextSteps)
	}
}

func TestSlowStartupCountsOnlyPastTwiceTheMedianAndAMinute(t *testing.T) {
	for _, c := range []struct {
		median, startup int64
		slow            bool
	}{
		{20, 50, false}, // twice the median, but under a minute
		{20, 59, false},
		{20, 60, true},
		{20, 70, true},
		{40, 79, false}, // over a minute, but under twice the median
		{40, 80, true},
	} {
		var jobs []HostJob
		for i := range 5 {
			jobs = append(jobs, HostJob{ID: fmt.Sprintf("job-%d", i), StartedAt: pressTime(30 + i), CompletedAt: pressTime(40 + i),
				Conclusion: "success", StartupSeconds: i64(c.median)})
		}
		jobs = append(jobs, HostJob{ID: "job-slow", StartedAt: pressTime(10), CompletedAt: pressTime(20),
			Conclusion: "success", StartupSeconds: i64(c.startup)})
		f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), jobs))
		events := f.Evidence.Events
		if got := len(events) == 1; got != c.slow {
			t.Errorf("median %ds, startup %ds: events %+v, want slow %v", c.median, c.startup, events, c.slow)
			continue
		}
		if c.slow && (events[0].Kind != "slow_startup" || events[0].JobID != "job-slow" || !events[0].At.Equal(*pressTime(10))) {
			t.Errorf("event %+v, want job-slow's slow startup at its start", events[0])
		}
	}
}

// A slow start and a failure are two things that went wrong at two times, so
// one job can be both.
func TestAJobThatStartedSlowlyAndFailedIsTwoEvents(t *testing.T) {
	jobs := failedIn(10)
	jobs[0].StartupSeconds = i64(300)
	for i := range 3 {
		jobs = append(jobs, HostJob{ID: fmt.Sprintf("fast-%d", i), StartedAt: pressTime(50), CompletedAt: pressTime(55),
			Conclusion: "success", StartupSeconds: i64(10)})
	}
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), jobs))
	var kinds []string
	for _, e := range f.Evidence.Events {
		kinds = append(kinds, e.Kind)
	}
	if !slices.Equal(kinds, []string{"failure", "slow_startup"}) {
		t.Errorf("kinds %v, newest first, want the failure and the slow startup", kinds)
	}
}

// An OOM kill is the fleet saying why a job failed, so it is one event, an
// OOM kill, not a failure as well.
func TestAnOOMKillIsOneEventNotAFailureToo(t *testing.T) {
	jobs := failedIn(10)
	jobs[0].OOMKilled = true
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), jobs))
	if len(f.Evidence.Events) != 1 || f.Evidence.Events[0].Kind != "oom" {
		t.Errorf("events %+v, want one oom", f.Evidence.Events)
	}
}

// A failure is what the store calls one, so a job that timed out on a busy
// host counts here as it does on the Usage page.
func TestAFailureIsWhatTheStoreCountsAsOne(t *testing.T) {
	if !slices.Equal(failedConclusions, store.FailedConclusions) {
		t.Errorf("failed conclusions %v, the store's are %v", failedConclusions, store.FailedConclusions)
	}
	jobs := failedIn(1, 4, 7, 10)
	jobs[0].Conclusion = "timed_out"
	jobs[1].Conclusion = "startup_failure"
	jobs[2].Conclusion, jobs[2].FaultKind = "cancelled", "runner_lost"
	jobs[3].Conclusion = "cancelled"
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), jobs))
	if len(f.Evidence.Events) != 3 {
		t.Errorf("events %+v, want the timed-out, the startup failure and the fleet fault", f.Evidence.Events)
	}
}

// The package may not import the scheduler (TestTheAnalysisPackageIsPure),
// so the line is a constant there and pinned here, from a test file, which
// the purity rule does not cover. It also means internal/scheduler can never
// import internal/analysis; move this pin rather than the constant if it
// ever needs to.
func TestThePressureLineIsTheThrottlesOwn(t *testing.T) {
	if DefaultPressure.LoadPerCPU != scheduler.LoadPerCPUOverwhelmed {
		t.Errorf("load per core %v, the throttle's is %v", DefaultPressure.LoadPerCPU, scheduler.LoadPerCPUOverwhelmed)
	}
}

func TestAnEventTakesTheSampleOfItsMinuteOrTheTwoBefore(t *testing.T) {
	for _, c := range []struct {
		gap      int
		measured bool
	}{{0, true}, {1, true}, {2, true}, {3, false}} {
		// One sample, at minute 10; the failure lands gap minutes after it.
		minutes := []HostMinute{{At: pressBase.Add(10 * time.Minute), LoadPerCPU: f64(2.5)}}
		f := mustPressure(t, pressSnapshot(minutes, failedIn(10+c.gap)))
		e := f.Evidence.Events[0]
		if e.Measured != c.measured || e.Pressured != c.measured {
			t.Errorf("a failure %d minutes after the sample: measured %v pressured %v, want %v", c.gap, e.Measured, e.Pressured, c.measured)
		}
		if want := map[bool]int{true: 0, false: 1}[c.measured]; f.Evidence.Unmeasured != want {
			t.Errorf("a failure %d minutes after the sample: %d unmeasured, want %d", c.gap, f.Evidence.Unmeasured, want)
		}
	}
}

func TestHostPressureWithGapsInItsSamplesIsMedium(t *testing.T) {
	minutes := loadMinutes(120, 20, 2.5, 0.5)[:96]
	f := mustPressure(t, pressSnapshot(minutes, failedIn(1, 4, 7, 10, 13, 16, 60)))
	if f.Confidence != Medium || f.ConfidenceReason != "samples cover 80% of the period" {
		t.Errorf("%s (%q), want medium for samples covering 80%%", f.Confidence, f.ConfidenceReason)
	}
	// 20 of the 96 minutes sampled: the share is of those, and the sentence
	// says so rather than calling it the period's.
	if want := "demo-builder-2's failures cluster in its busy minutes: 6 of 7 came while its load per core was at or above 2.5, which was true for 21% of the minutes it was measured."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
}

// Ninety per cent coverage is high; one minute short of it is not, and the
// percentage is rounded down so the reason never says 90% of a finding it
// marked down for being under it: over a day, 1,295 minutes is 89.9%.
func TestNinetyPercentCoverageIsEnoughForHighConfidence(t *testing.T) {
	for _, c := range []struct {
		minutes int
		want    Confidence
		reason  string
	}{{1296, High, ""}, {1295, Medium, "samples cover 89% of the period"}} {
		s := pressSnapshot(loadMinutes(1440, 20, 2.5, 0.5)[:c.minutes], failedIn(1, 4, 7, 10, 13, 16, 60))
		s.Until = pressBase.Add(24 * time.Hour)
		f := mustPressure(t, s)
		if f.Confidence != c.want || f.ConfidenceReason != c.reason {
			t.Errorf("%d of 1440 minutes: %s (%q), want %s (%q)", c.minutes, f.Confidence, f.ConfidenceReason, c.want, c.reason)
		}
	}
}

// A share is a claim about the period, so it never rounds to one it cannot
// make: a minute in a day is not 0%, all but one is not 100%, and coverage
// short of a whole per cent is not none.
func TestASharePercentageNeverRoundsToAClaim(t *testing.T) {
	for _, c := range []struct {
		part, whole  int
		share, cover string
	}{
		{1, 1440, "under 1%", "under 1%"},
		{1439, 1440, "over 99%", "99%"},
		{20, 120, "17%", "16%"},
		{0, 120, "0%", "0%"},
		{120, 120, "100%", "100%"},
	} {
		if got := share(c.part, c.whole); got != c.share {
			t.Errorf("share(%d, %d) = %q, want %q", c.part, c.whole, got, c.share)
		}
		if got := coverage(c.part, c.whole); got != c.cover {
			t.Errorf("coverage(%d, %d) = %q, want %q", c.part, c.whole, got, c.cover)
		}
	}
}

// The jobs are read newest first up to MaxReadJobs, so on a busy host over a
// long period the oldest are not there to place; a pattern drawn from the
// rest says so rather than reading as the whole period's.
func TestATruncatedJobReadKeepsHostPressureFromHigh(t *testing.T) {
	s := pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), failedIn(1, 4, 7, 10, 13, 16, 60))
	s.JobsTruncated = true
	f := mustPressure(t, s)
	if f.Dominant != "pressure" || f.Confidence != Medium || !f.Evidence.JobsTruncated {
		t.Errorf("dominant %q at %s, truncated %v; want pressure at medium, and said", f.Dominant, f.Confidence, f.Evidence.JobsTruncated)
	}
	if want := "the period held more than 5000 jobs on this host; the newest 5000 were read, so narrow since"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
}

func TestEveryMediumReasonIsGivenJoined(t *testing.T) {
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5)[:96], failedIn(1, 4, 7)))
	if want := "only 3 failures came in a measured minute, too few to be sure of the pattern; also, samples cover 80% of the period"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
}

func TestAMemoryThresholdNamesOOMKillsFirst(t *testing.T) {
	// Load stays low throughout; for 20 minutes available memory sits at the
	// host's 1024 MB reserve, and five jobs are killed for memory then.
	minutes := loadMinutes(120, 0, 0, 0.5)
	for i := range minutes {
		minutes[i].MemoryMB, minutes[i].MemoryAvailableMB = 16384, i64(8192)
		if i < 20 {
			minutes[i].MemoryAvailableMB = i64(512)
		}
	}
	jobs := failedIn(2, 5, 8, 11, 14, 70)
	for i := range jobs[:5] {
		jobs[i].OOMKilled = true
	}
	s := pressSnapshot(minutes, jobs)
	s.Thresholds.MemoryReserveMB = 1024
	f := mustPressure(t, s)
	if f.Dominant != "pressure" || f.Evidence.Threshold == nil || f.Evidence.Threshold.Measure != "memory_used" {
		t.Fatalf("dominant %q, threshold %+v, want pressure on memory_used", f.Dominant, f.Evidence.Threshold)
	}
	if f.NextSteps[0].Code != "jobs.oom_killed" || f.NextSteps[0].Setting != "pool.resources.memory_mb" {
		t.Errorf("first step %+v, want jobs.oom_killed", f.NextSteps[0])
	}
	if want := "demo-builder-2's failures and OOM kills cluster in its busy minutes: 5 of 6 came while its memory in use was at or above 96.8%, which was true for 17% of the period."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
}

// memoryMinutes gives minutes a 16 GB machine's memory, at the 1024 MB
// reserve for the first atReserve of them and half free after.
func memoryMinutes(minutes []HostMinute, atReserve int) {
	for i := range minutes {
		minutes[i].MemoryMB, minutes[i].MemoryAvailableMB = 16384, i64(8192)
		if i < atReserve {
			minutes[i].MemoryAvailableMB = i64(1024)
		}
	}
}

// Each measure's line is the 20th percentile of its events' values; of the
// scheduler's two, the one kept leaves the fewest minutes at or above it.
func TestTheThresholdIsTheMeasureThatLeavesTheFewestMinutesAbove(t *testing.T) {
	minutes := loadMinutes(120, 30, 2.5, 0.5)
	memoryMinutes(minutes, 20)
	s := pressSnapshot(minutes, failedIn(1, 4, 7, 10, 13, 16, 60))
	s.Thresholds.MemoryReserveMB = 1024
	f := mustPressure(t, s)
	if th := f.Evidence.Threshold; th == nil || th.Measure != "memory_used" || th.MinuteShare != 20.0/120 {
		t.Errorf("threshold %+v, want memory_used over 20 minutes against load's 30", th)
	}
}

// The throttle never acts on CPU alone, so a CPU line is named only when
// neither load nor memory separates the events, even when it would leave
// fewer minutes above it.
func TestCPUIsTheLineOnlyWhenNeitherLoadNorMemorySeparates(t *testing.T) {
	minutes := loadMinutes(120, 30, 2.5, 0.5)
	for i := range minutes {
		cpu := 40.0
		if i < 20 {
			cpu = 97.25
		}
		minutes[i].CPUPercent = f64(cpu)
	}
	f := mustPressure(t, pressSnapshot(minutes, failedIn(1, 4, 7, 10, 13, 16, 60)))
	if th := f.Evidence.Threshold; th == nil || th.Measure != "load_per_cpu" {
		t.Errorf("threshold %+v, want load_per_cpu over a narrower cpu_percent", th)
	}
	// Load moved out of the way: three pressured minutes with a failure
	// each keep the cluster, but load's line is a calm one now, and CPU is
	// the line, shown as a percentage.
	for i := range minutes {
		load := 0.5
		if i < 3 {
			load = 3
		}
		minutes[i].LoadPerCPU = f64(load)
		minutes[i].CPUPercent = f64(40)
		if i < 30 {
			minutes[i].CPUPercent = f64(97.25)
		}
	}
	f = mustPressure(t, pressSnapshot(minutes, failedIn(0, 1, 2, 4, 7, 10, 13, 16, 60)))
	if th := f.Evidence.Threshold; f.Dominant != "pressure" || th == nil || th.Measure != "cpu_percent" || th.Value != 97.2 {
		t.Errorf("dominant %q, threshold %+v, want cpu_percent at 97.2", f.Dominant, th)
	}
}

// A line is kept from exactly twice the share of events as of minutes above
// it; with one minute more above it, it separates nothing.
func TestALineSeparatesFromTwiceTheMinutesShare(t *testing.T) {
	for _, c := range []struct {
		busy int
		kept bool
	}{{30, true}, {31, false}} {
		// 70 minutes: 6 of 7 failures above the line against 30 of 70
		// minutes is 6/7 against 3/7, exactly twice.
		s := pressSnapshot(loadMinutes(70, c.busy, 2.5, 0.5), failedIn(1, 4, 7, 10, 13, 16, 50))
		s.Until = pressBase.Add(70 * time.Minute)
		f := mustPressure(t, s)
		if got := f.Evidence.Threshold != nil; got != c.kept || f.Dominant != "pressure" {
			t.Errorf("%d of 70 minutes busy: dominant %q, threshold %+v, want kept %v", c.busy, f.Dominant, f.Evidence.Threshold, c.kept)
		}
	}
}

// With every event above it, as three events always are, a line separates
// only if few minutes are above it too; one under 92% of the period is no
// cluster, and a CPU line the throttle never reads least of all.
func TestALineUnderEveryEventAndMostMinutesIsNotReported(t *testing.T) {
	minutes := loadMinutes(120, 20, 2.5, 0.5)
	for i := range minutes {
		cpu := 97.0
		switch {
		case i >= 110:
			cpu = 10
		case i >= 20:
			cpu = 40
		}
		minutes[i].CPUPercent = f64(cpu)
	}
	f := mustPressure(t, pressSnapshot(minutes, failedIn(1, 4, 7, 60, 61)))
	if f.Dominant != "pressure" || f.Evidence.Threshold != nil {
		t.Errorf("dominant %q, threshold %+v; a CPU line at 40%% is under 110 of 120 minutes", f.Dominant, f.Evidence.Threshold)
	}
	if got := "demo-builder-2's failures cluster in its busy minutes: 3 of 5 came while it was under pressure, which was true for 17% of the period."; f.Summary != got {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, got)
	}
}

// Where load and memory leave as few minutes above their lines, load is
// named, the order the throttle tests them in.
func TestATiedThresholdNamesLoadBeforeMemory(t *testing.T) {
	minutes := loadMinutes(120, 20, 2.5, 0.5)
	memoryMinutes(minutes, 20)
	s := pressSnapshot(minutes, failedIn(1, 4, 7, 10, 13, 16, 60))
	s.Thresholds.MemoryReserveMB = 1024
	f := mustPressure(t, s)
	if th := f.Evidence.Threshold; th == nil || th.Measure != "load_per_cpu" {
		t.Errorf("threshold %+v, want load_per_cpu over an equal memory_used", th)
	}
}

func TestPressureEventsAreCutToTheNewest(t *testing.T) {
	var ms []int
	for i := range MaxPressureEvents + 5 {
		ms = append(ms, i*4)
	}
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), failedIn(ms...)))
	e := f.Evidence
	if len(e.Events) != MaxPressureEvents || e.EventsCut != 5 || e.MeasuredEvents != MaxPressureEvents+5 {
		t.Errorf("%d events shown, %d cut, %d measured", len(e.Events), e.EventsCut, e.MeasuredEvents)
	}
	if !e.Events[0].At.Equal(*pressTime(ms[len(ms)-1])) {
		t.Errorf("first event at %v, want the newest", e.Events[0].At)
	}
	ev := e.Events[0]
	if ev.Repo != "acme/api" || ev.Workflow != "ci" || ev.JobName != "test" || ev.JobID == "" {
		t.Errorf("event %+v does not say which job it was", ev)
	}
}

func TestTwentyPressureEventsAreAllShown(t *testing.T) {
	var ms []int
	for i := range MaxPressureEvents {
		ms = append(ms, i*5)
	}
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), failedIn(ms...)))
	if len(f.Evidence.Events) != MaxPressureEvents || f.Evidence.EventsCut != 0 {
		t.Errorf("%d events shown, %d cut, want all %d shown", len(f.Evidence.Events), f.Evidence.EventsCut, MaxPressureEvents)
	}
}

func TestEventsOutsideThePeriodAreLeftOut(t *testing.T) {
	jobs := failedIn(1, 4, 7, 10, 13)
	jobs[0].CompletedAt = pressTime(-5)
	jobs[1].CompletedAt = pressTime(125)
	f := mustPressure(t, pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), jobs))
	if f.Evidence.MeasuredEvents != 3 || len(f.Evidence.Events) != 3 {
		t.Errorf("%d events, want the 3 inside the period", len(f.Evidence.Events))
	}
}

func TestHostPressureRefusesASnapshotItCannotRead(t *testing.T) {
	good := pressSnapshot(nil, nil)
	for name, s := range map[string]HostPressureSnapshot{
		"no period":   func() HostPressureSnapshot { s := good; s.Since = time.Time{}; return s }(),
		"reversed":    func() HostPressureSnapshot { s := good; s.Since, s.Until = s.Until, s.Since; return s }(),
		"no host":     func() HostPressureSnapshot { s := good; s.HostID, s.HostName = "", ""; return s }(),
		"no pressure": func() HostPressureSnapshot { s := good; s.Thresholds = PressureThresholds{}; return s }(),
	} {
		if _, err := HostPressure(s); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestAHostWithNoNameIsNamedByItsID(t *testing.T) {
	s := pressSnapshot(loadMinutes(120, 20, 2.5, 0.5), failedIn(1, 4, 7, 10, 13, 16, 60))
	s.HostName = ""
	f := mustPressure(t, s)
	if !strings.HasPrefix(f.Summary, "host_demob's failures") {
		t.Errorf("summary %q, want it named by its id", f.Summary)
	}
}
