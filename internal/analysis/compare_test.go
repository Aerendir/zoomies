package analysis

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

var compareBase = time.Date(2026, 10, 11, 8, 0, 0, 0, time.UTC)

func msp(d time.Duration) *int64 { v := d.Milliseconds(); return &v }

func spread(samples int, p50, p95 time.Duration) Spread {
	return Spread{Samples: samples, P50MS: msp(p50), P95MS: msp(p95)}
}

func slots(n float64) *float64 { return &n }

// side is a release that ran jobs jobs, every one of them succeeding, with
// the same figures as every other side this builds, so a test changes only
// the measure it is about.
func side(label string, jobs int) CompareSide {
	return CompareSide{Label: label, Since: compareBase, Until: compareBase.Add(24 * time.Hour),
		Figures: JobFigures{Jobs: jobs, Succeeded: jobs,
			Duration:  spread(jobs, time.Minute, 5*time.Minute),
			QueueWait: spread(jobs, 5*time.Second, 20*time.Second),
			Startup:   spread(jobs, 8*time.Second, 15*time.Second)},
		MeanSlots: slots(10)}
}

func releases(a, b int) CompareSnapshot {
	return CompareSnapshot{By: "version", A: side("v1.3.2", a), B: side("v1.3.3", b)}
}

func periods(a, b int) CompareSnapshot {
	s := CompareSnapshot{By: "period", A: side("the 24 hours before", a), B: side("the last 24 hours", b)}
	s.A.Since, s.A.Until = compareBase.Add(-24*time.Hour), compareBase
	return s
}

func mustCompare(t *testing.T, s CompareSnapshot) CompareFinding {
	t.Helper()
	f, err := Compare(s)
	if err != nil {
		t.Fatal(err)
	}
	assertNoCommand(t, f.NextSteps)
	return f
}

func change(t *testing.T, f CompareFinding, measure string) Change {
	t.Helper()
	for _, c := range f.Evidence.Changes {
		if c.Measure == measure {
			return c
		}
	}
	t.Fatalf("no change for %s in %+v", measure, f.Evidence.Changes)
	return Change{}
}

func hasSetting(steps []NextStep, setting string) bool {
	return slices.ContainsFunc(steps, func(s NextStep) bool { return s.Setting == setting })
}

func TestCompareRefusesToRankFewerThanThirtyJobsOnEitherSide(t *testing.T) {
	s := releases(15, 20)
	// A move this large would be ranked on a bigger sample; under thirty jobs
	// it could be three slow runs, and naming it would send someone to roll
	// back a release over noise.
	s.B.Figures.Duration = spread(20, time.Minute, 20*time.Minute)
	f := mustCompare(t, s)
	if f.Confidence != Low || f.ConfidenceReason == "" || f.Dominant != "" {
		t.Errorf("confidence %q (%q), dominant %q; want low with a reason and nothing dominant", f.Confidence, f.ConfidenceReason, f.Dominant)
	}
	if want := "v1.3.2 ran 15 jobs and v1.3.3 ran 20 in this period: fewer than 30 on either side, so the two are not ranked."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	if len(f.Evidence.Changes) != 9 {
		t.Fatalf("%d changes, want one for each of the nine measures", len(f.Evidence.Changes))
	}
	for _, c := range f.Evidence.Changes {
		if c.Direction != NotEnough {
			t.Errorf("%s is %s, want not_enough", c.Measure, c.Direction)
		}
	}
	if hasSetting(f.NextSteps, "updates.mode") || len(f.Codes) != 0 {
		t.Errorf("a comparison that ranks nothing names a regression's steps: %+v, codes %v", f.NextSteps, f.Codes)
	}
	// The window is bounded by limits.job_stats_window, so a reader told to
	// widen it is told where the ceiling is.
	if len(f.NextSteps) != 1 || f.NextSteps[0].Setting != "limits.job_stats_window" {
		t.Errorf("next steps %+v, want one naming limits.job_stats_window", f.NextSteps)
	}
}

// The commonest comparison is a release a few hours old against the one
// before it: hundreds of jobs on one side, a dozen on the other. Saying both
// were short would be false of the side that ran hundreds.
func TestASmallSampleNamesTheSideThatIsShort(t *testing.T) {
	for _, c := range []struct {
		snap CompareSnapshot
		want string
	}{
		{releases(400, 12), "v1.3.2 ran 400 jobs and v1.3.3 ran 12 in this period: fewer than 30 under v1.3.3, so the two are not ranked."},
		{releases(12, 400), "v1.3.2 ran 12 jobs and v1.3.3 ran 400 in this period: fewer than 30 under v1.3.2, so the two are not ranked."},
		{releases(12, 15), "v1.3.2 ran 12 jobs and v1.3.3 ran 15 in this period: fewer than 30 on either side, so the two are not ranked."},
		{periods(400, 12), "400 jobs ran in the 24 hours before and 12 in the last 24 hours: fewer than 30 in the last 24 hours, so the two are not ranked."},
		{periods(12, 400), "12 jobs ran in the 24 hours before and 400 in the last 24 hours: fewer than 30 in the 24 hours before, so the two are not ranked."},
	} {
		f := mustCompare(t, c.snap)
		if f.Summary != c.want {
			t.Errorf("summary\n got %q\nwant %q", f.Summary, c.want)
		}
		if f.Confidence != Low || f.ConfidenceReason+", so the two are not ranked." != c.want {
			t.Errorf("confidence %q, reason %q; want low, and the reason the summary gives", f.Confidence, f.ConfidenceReason)
		}
	}
}

// Each bar is met by a move of exactly its size: a quarter longer and the
// floor apart both count, and a fleet exactly a quarter larger is still
// like for like.
func TestAMoveOfExactlyItsBarCounts(t *testing.T) {
	s := releases(120, 120)
	s.A.Figures.Duration = spread(120, 2*time.Minute, 5*time.Minute)
	s.B.Figures.Duration = spread(120, 2*time.Minute+30*time.Second, 5*time.Minute) // 1.25 times, 30s apart
	s.B.MeanSlots = slots(12.5)
	f := mustCompare(t, s)
	if got := change(t, f, "duration_p50").Direction; got != Regressed {
		t.Errorf("duration p50 2m to 2m 30s is %s, want regressed", got)
	}
	if f.Confidence != High {
		t.Errorf("10 and 12.5 slots: confidence %q (%q), want high", f.Confidence, f.ConfidenceReason)
	}
}

// When two measures moved as far as each other, the measures' own order
// decides the lead, so one snapshot always reads the same way.
func TestATieLeadsWithTheMeasureListedFirst(t *testing.T) {
	s := releases(120, 120)
	s.B.Figures.Startup = spread(120, 8*time.Second, 30*time.Second)   // twice as long at p95
	s.B.Figures.QueueWait = spread(120, 5*time.Second, 40*time.Second) // twice as long at p95
	for range 5 {
		if f := mustCompare(t, s); f.Dominant != "queue_wait_p95" {
			t.Fatalf("dominant %q, want queue_wait_p95, listed before startup_p95, which doubled as well", f.Dominant)
		}
	}
}

func TestCompareNamesTheLargestRegression(t *testing.T) {
	s := releases(120, 120)
	s.B.Figures.Duration = spread(120, time.Minute, 9*time.Minute)
	f := mustCompare(t, s)
	if f.Confidence != High || f.ConfidenceReason != "" {
		t.Errorf("confidence %q (%q), want high with no reason", f.Confidence, f.ConfidenceReason)
	}
	if f.Dominant != "duration_p95" {
		t.Errorf("dominant %q, want duration_p95", f.Dominant)
	}
	if want := "v1.3.3 is worse than v1.3.2 on duration p95 (5m to 9m)."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	c := change(t, f, "duration_p95")
	if c.Direction != Regressed || c.Unit != "ms" || *c.From != 300000 || *c.To != 540000 || c.Ratio == nil || *c.Ratio != 1.8 {
		t.Errorf("duration_p95 change %+v, want regressed ms 300000 to 540000, ratio 1.8", c)
	}
	if len(f.NextSteps) == 0 || f.NextSteps[0].Code != "jobs.duration_regressed" {
		t.Fatalf("next steps %+v, want the first to name jobs.duration_regressed", f.NextSteps)
	}
	if !hasSetting(f.NextSteps, "updates.mode") {
		t.Errorf("next steps %+v name no updates.mode, though a release regressed", f.NextSteps)
	}
	if !slices.Contains(f.Codes, "jobs.duration_regressed") {
		t.Errorf("codes %v leave out jobs.duration_regressed", f.Codes)
	}
}

func TestCompareSaysWhatImproved(t *testing.T) {
	s := releases(120, 120)
	s.A.Figures.QueueWait = spread(120, 40*time.Second, time.Minute)
	s.B.Figures.QueueWait = spread(120, 10*time.Second, time.Minute)
	f := mustCompare(t, s)
	if f.Dominant != "queue_wait_p50" {
		t.Errorf("dominant %q, want queue_wait_p50", f.Dominant)
	}
	if want := "v1.3.3 is better than v1.3.2 on queue wait p50 (40s to 10s)."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	if c := change(t, f, "queue_wait_p50"); c.Direction != Improved || c.Ratio == nil || *c.Ratio != 4 {
		t.Errorf("queue_wait_p50 change %+v, want improved by a factor of 4", c)
	}
	// Holding updates over an improvement would keep a fleet on the worse
	// release.
	if hasSetting(f.NextSteps, "updates.mode") {
		t.Errorf("next steps %+v hold updates though nothing regressed", f.NextSteps)
	}
}

func TestCompareIgnoresAMoveSmallerThanItsFloor(t *testing.T) {
	for _, c := range []struct {
		name     string
		measure  string
		from, to time.Duration
	}{
		// 1.4 times as long, but four seconds is not a regression anyone
		// should chase.
		{"past the ratio, under the floor", "duration_p50", 10 * time.Second, 14 * time.Second},
		// Thirty seconds more on a five-minute job is a tenth, not a quarter.
		{"past the floor, under the ratio", "duration_p95", 5 * time.Minute, 5*time.Minute + 30*time.Second},
		{"queue wait under its ten-second floor", "queue_wait_p50", 20 * time.Second, 29 * time.Second},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := releases(120, 120)
			if c.measure == "queue_wait_p50" {
				s.A.Figures.QueueWait = spread(120, c.from, time.Minute)
				s.B.Figures.QueueWait = spread(120, c.to, time.Minute)
			} else {
				s.A.Figures.Duration = spread(120, c.from, 5*time.Minute)
				s.B.Figures.Duration = spread(120, c.from, 5*time.Minute)
				if c.measure == "duration_p50" {
					s.B.Figures.Duration.P50MS = msp(c.to)
				} else {
					s.A.Figures.Duration.P95MS, s.B.Figures.Duration.P95MS = msp(c.from), msp(c.to)
				}
			}
			f := mustCompare(t, s)
			if got := change(t, f, c.measure).Direction; got != Unchanged {
				t.Errorf("%s is %s, want unchanged", c.measure, got)
			}
			if want := "v1.3.3 and v1.3.2 ran alike: no measure moved past its threshold."; f.Summary != want {
				t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
			}
			if f.Dominant != "" || f.Confidence != High {
				t.Errorf("dominant %q, confidence %q; want none and high", f.Dominant, f.Confidence)
			}
		})
	}
}

// The store leaves cancelled and skipped jobs out of what it reads for a
// comparison (Task 6 proves that against a real store); the finding has to
// say so, and it can only say what its caller did.
func TestCompareEvidenceNamesTheConclusionsTheReadLeftOut(t *testing.T) {
	s := releases(120, 120)
	s.Excluded = []string{"cancelled", "skipped"}
	if got := mustCompare(t, s).Evidence.Excluded; !slices.Equal(got, []string{"cancelled", "skipped"}) {
		t.Errorf("excluded %v, want [cancelled skipped]", got)
	}
	if got := mustCompare(t, releases(120, 120)).Evidence.Excluded; got == nil || len(got) != 0 {
		t.Errorf("excluded %#v, want an empty list rather than null", got)
	}
}

func TestADifferentlySizedFleetMakesTheComparisonMedium(t *testing.T) {
	s := releases(120, 120)
	s.B.MeanSlots = slots(20)
	f := mustCompare(t, s)
	if f.Confidence != Medium {
		t.Errorf("confidence %q, want medium", f.Confidence)
	}
	if want := "the fleet offered 10 slots under v1.3.2 and 20 under v1.3.3, so queue waits are not like for like"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
	// A fleet that grew by a fifth is still the same fleet to compare on.
	s.B.MeanSlots = slots(12)
	if f := mustCompare(t, s); f.Confidence != High {
		t.Errorf("10 and 12 slots: confidence %q (%q), want high", f.Confidence, f.ConfidenceReason)
	}
}

func TestAnUnknownFleetSizeMakesTheComparisonMedium(t *testing.T) {
	s := releases(120, 120)
	s.A.MeanSlots = nil
	f := mustCompare(t, s)
	if f.Confidence != Medium {
		t.Errorf("confidence %q, want medium", f.Confidence)
	}
	if !strings.Contains(f.ConfidenceReason, "retention.samples") || !strings.Contains(f.ConfidenceReason, "v1.3.2") {
		t.Errorf("reason %q names neither retention.samples nor the side it is about", f.ConfidenceReason)
	}
}

func TestACountsOnlySideSaysPercentilesAreUnavailable(t *testing.T) {
	s := periods(120, 120)
	// The roll-up keeps how many jobs ran and how many the fleet lost; it
	// has no failures, no OOM kills and no times, so those read zero. Read
	// as figures they would be a fall in failures that never happened.
	s.A.CountsOnly = true
	s.A.Figures = JobFigures{Jobs: 120, FleetFailed: 2}
	s.B.Figures.FleetFailed, s.B.Figures.Failed, s.B.Figures.OOMKilled = 12, 20, 5
	f := mustCompare(t, s)
	for _, c := range f.Evidence.Changes {
		want := NotEnough
		if c.Measure == "fleet_failure_rate" {
			want = Regressed
		}
		if c.Direction != want {
			t.Errorf("%s is %s, want %s", c.Measure, c.Direction, want)
		}
	}
	if !f.Evidence.PercentilesUnavailable {
		t.Error("percentiles_unavailable is false with one side read from the roll-up")
	}
	if f.Confidence != Medium {
		t.Errorf("confidence %q, want medium", f.Confidence)
	}
	for _, want := range []string{"retention.jobs", "percentiles are unavailable", "defined differently"} {
		if !strings.Contains(f.ConfidenceReason, want) {
			t.Errorf("reason %q does not say %q", f.ConfidenceReason, want)
		}
	}
	if want := "Jobs did worse in the last 24 hours than in the 24 hours before on fleet failure rate (2% to 10%)."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}

	// With nothing moved, "ran alike" would claim the eight measures it never
	// saw; the sentence says how many could not be compared.
	s.B.Figures.FleetFailed = 2
	if want := "Jobs ran alike in the 24 hours before and the last 24 hours: no measure moved past its threshold, though 8 of 9 could not be compared."; mustCompare(t, s).Summary != want {
		t.Errorf("summary\n got %q\nwant %q", mustCompare(t, s).Summary, want)
	}
}

func TestACompareWithOneSideEmptySaysWhichRanNothing(t *testing.T) {
	for _, c := range []struct {
		a, b int
		want string
	}{
		{120, 0, "v1.3.3 ran no jobs in this period, so there is nothing to compare v1.3.2 with."},
		{0, 120, "v1.3.2 ran no jobs in this period, so there is nothing to compare v1.3.3 with."},
		{0, 0, "Neither v1.3.2 nor v1.3.3 ran a job in this period, so there is nothing to compare."},
	} {
		s := releases(c.a, c.b)
		f := mustCompare(t, s)
		if f.Confidence != Low || f.ConfidenceReason == "" {
			t.Errorf("%d and %d jobs: confidence %q (%q), want low with a reason", c.a, c.b, f.Confidence, f.ConfidenceReason)
		}
		if f.Summary != c.want {
			t.Errorf("%d and %d jobs: summary\n got %q\nwant %q", c.a, c.b, f.Summary, c.want)
		}
	}
}

func TestARateMovesOnlyByFivePointsAndThreeEvents(t *testing.T) {
	for _, c := range []struct {
		name       string
		jobs, a, b int
		want       string
	}{
		{"two and a half points", 120, 0, 3, Unchanged},
		{"five points, six failures", 120, 0, 6, Regressed},
		{"five points, but two failures", 40, 0, 2, Unchanged},
		{"down five points", 120, 10, 4, Improved},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := releases(c.jobs, c.jobs)
			s.A.Figures.Failed, s.B.Figures.Failed = c.a, c.b
			f := mustCompare(t, s)
			if got := change(t, f, "failure_rate").Direction; got != c.want {
				t.Errorf("failure_rate is %s, want %s", got, c.want)
			}
		})
	}
	s := releases(120, 120)
	s.B.Figures.Failed = 6
	f := mustCompare(t, s)
	if want := "v1.3.3 is worse than v1.3.2 on failure rate (0% to 5%)."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	if c := change(t, f, "failure_rate"); c.Unit != "rate" || c.Ratio == nil || *c.Ratio != 1 {
		t.Errorf("failure_rate change %+v, want unit rate and a ratio of 1 (five points over five)", c)
	}
	if len(f.NextSteps) == 0 || f.NextSteps[0].Code != "jobs.workflow_failing" {
		t.Fatalf("next steps %+v, want the first to name jobs.workflow_failing", f.NextSteps)
	}
	if want := "More jobs failed under v1.3.3 (failure rate 0% to 5%); a failure the fleet did not cause is the workflow's own, and the step each job failed at says what to fix."; f.NextSteps[0].Text != want {
		t.Errorf("step\n got %q\nwant %q", f.NextSteps[0].Text, want)
	}
}

// The catalog's own fix for an OOM kill is the pool's memory, so the step
// names that field rather than leaving the reader to find it.
func TestAnOOMRegressionNamesThePoolsMemory(t *testing.T) {
	s := releases(120, 120)
	s.B.Figures.OOMKilled = 12
	f := mustCompare(t, s)
	if len(f.NextSteps) == 0 || f.NextSteps[0].Code != "jobs.oom_killed" || f.NextSteps[0].Setting != "pool.resources.memory_mb" {
		t.Errorf("next steps %+v, want the first to name jobs.oom_killed and pool.resources.memory_mb", f.NextSteps)
	}
}

func TestTheLeadIsTheRegressionThatMovedFurthest(t *testing.T) {
	s := releases(120, 120)
	s.B.Figures.Duration = spread(120, time.Minute, 9*time.Minute) // 1.8 times
	s.B.Figures.FleetFailed, s.B.Figures.Failed = 12, 12           // ten points: twice the five-point bar
	f := mustCompare(t, s)
	if f.Dominant != "fleet_failure_rate" {
		t.Errorf("dominant %q, want fleet_failure_rate, which moved twice its bar against duration's 1.8", f.Dominant)
	}
	var codes []string
	for _, st := range f.NextSteps {
		codes = append(codes, st.Code)
	}
	if want := []string{"jobs.runner_lost", "jobs.workflow_failing", "jobs.duration_regressed", ""}; !slices.Equal(codes, want) {
		t.Errorf("step codes %q, want %q: one step per regressed measure family, largest first, then hold updates", codes, want)
	}

	// A wait that went from nothing to twenty seconds has no finite ratio,
	// and is the furthest any measure can move.
	s = releases(120, 120)
	s.B.Figures.Duration = spread(120, time.Minute, 9*time.Minute)
	s.A.Figures.QueueWait = spread(120, 0, 20*time.Second)
	s.B.Figures.QueueWait = spread(120, 20*time.Second, 20*time.Second)
	f = mustCompare(t, s)
	if c := change(t, f, "queue_wait_p50"); c.Direction != Regressed || c.Ratio != nil {
		t.Errorf("queue_wait_p50 change %+v, want regressed with no ratio", c)
	}
	if f.Dominant != "queue_wait_p50" {
		t.Errorf("dominant %q, want queue_wait_p50", f.Dominant)
	}
}

func TestCompareSaysHowManyMoreMovedTheSameWay(t *testing.T) {
	s := releases(120, 120)
	s.B.Figures.Duration = spread(120, 2*time.Minute, 9*time.Minute)
	s.B.Figures.QueueWait = spread(120, 5*time.Second, 30*time.Second)
	// An improvement is not counted among the regressions.
	s.A.Figures.Startup = spread(120, 40*time.Second, 50*time.Second)
	// Duration p50 moved 2 times, p95 1.8 times, queue wait p95 1.5 times.
	f := mustCompare(t, s)
	if want := "v1.3.3 is worse than v1.3.2 on duration p50 (1m to 2m), and on 2 more."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	// Two duration percentiles that both got worse are one thing to read
	// about, not two steps saying the same.
	var codes []string
	for _, st := range f.NextSteps {
		codes = append(codes, st.Code)
	}
	if want := []string{"jobs.duration_regressed", "pool.queue_wait_high", ""}; !slices.Equal(codes, want) {
		t.Errorf("step codes %q, want %q", codes, want)
	}
	if !strings.Contains(f.NextSteps[0].Text, "duration p50 1m to 2m") {
		t.Errorf("first step %q does not give the figures of the measure that led", f.NextSteps[0].Text)
	}
}

func TestFewerThanAHundredJobsASideMakesTheComparisonMedium(t *testing.T) {
	f := mustCompare(t, releases(60, 120))
	if f.Confidence != Medium {
		t.Errorf("confidence %q, want medium", f.Confidence)
	}
	if want := "v1.3.2 ran 60 jobs and v1.3.3 ran 120 in this period: fewer than 100 on a side, so a change named here is less certain"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
}

func TestATimeMeasureWithFewerThanThirtySamplesIsNotEnough(t *testing.T) {
	s := releases(120, 120)
	// Most jobs ran too briefly for a runner's startup to be measured.
	s.B.Figures.Startup = spread(20, time.Minute, 2*time.Minute)
	f := mustCompare(t, s)
	for _, m := range []string{"startup_p50", "startup_p95"} {
		if c := change(t, f, m); c.Direction != NotEnough || c.Ratio != nil {
			t.Errorf("%s change %+v, want not_enough with no ratio", m, c)
		}
	}
	if f.Dominant != "" {
		t.Errorf("dominant %q, want none", f.Dominant)
	}
}

func TestJobsGonePastRetentionMakeTheComparisonMedium(t *testing.T) {
	s := releases(120, 120)
	gone := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	s.A.JobsGoneBefore = &gone
	f := mustCompare(t, s)
	if f.Confidence != Medium {
		t.Errorf("confidence %q, want medium", f.Confidence)
	}
	if want := "v1.3.2's jobs from before 2026-10-09 14:00 UTC are gone (retention.jobs keeps them for a set time, and raising it keeps more next time), so v1.3.2's figures cover only the jobs still kept"; f.ConfidenceReason != want {
		t.Errorf("reason\n got %q\nwant %q", f.ConfidenceReason, want)
	}
}

func TestAPeriodComparisonIsWordedForPeriods(t *testing.T) {
	s := periods(120, 120)
	s.B.Figures.Duration = spread(120, time.Minute, 9*time.Minute)
	f := mustCompare(t, s)
	if want := "Jobs did worse in the last 24 hours than in the 24 hours before on duration p95 (5m to 9m)."; f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
	// Updates are held over a release, not over a bad day.
	if hasSetting(f.NextSteps, "updates.mode") {
		t.Errorf("a period comparison holds updates: %+v", f.NextSteps)
	}
	if !f.Since.Equal(s.A.Since) || !f.Until.Equal(s.B.Until) {
		t.Errorf("finding covers %v to %v, want %v to %v", f.Since, f.Until, s.A.Since, s.B.Until)
	}
	if want := "15 jobs ran in the 24 hours before and 20 in the last 24 hours: fewer than 30 on either side, so the two are not ranked."; mustCompare(t, periods(15, 20)).Summary != want {
		t.Errorf("summary\n got %q\nwant %q", mustCompare(t, periods(15, 20)).Summary, want)
	}
	if want := "No jobs ran in the last 24 hours, so there is nothing to compare the 24 hours before with."; mustCompare(t, periods(120, 0)).Summary != want {
		t.Errorf("summary\n got %q\nwant %q", mustCompare(t, periods(120, 0)).Summary, want)
	}
}

func TestCompareRefusesASnapshotItCannotRead(t *testing.T) {
	for name, edit := range map[string]func(*CompareSnapshot){
		"an unknown by":       func(s *CompareSnapshot) { s.By = "release" },
		"a side with no name": func(s *CompareSnapshot) { s.B.Label = "" },
		"a side that ends before it starts": func(s *CompareSnapshot) {
			s.A.Until = s.A.Since.Add(-time.Hour)
		},
	} {
		s := releases(120, 120)
		edit(&s)
		if _, err := Compare(s); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// Cancelled jobs are left out of the read, so a cancelled count would always
// be 0, and a 0 in the evidence reads as a measurement of none cancelled.
// What was left out is said once, in excluded.
func TestASidesFiguresCarryNoCancelledCount(t *testing.T) {
	body, err := json.Marshal(JobFigures{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"cancelled"`) {
		t.Errorf("figures %s carry a cancelled count", body)
	}
}
