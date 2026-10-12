package analysis

import (
	"reflect"
	"testing"
	"time"
)

func delayFixture() MergeDelaySnapshot {
	start := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	at := func(minutes int) *time.Time { v := start.Add(time.Duration(minutes) * time.Minute); return &v }
	return MergeDelaySnapshot{
		HeadSHA: "head", CurrentHeadSHA: "head", TargetBranch: "release",
		WindowStart: start, ObservedAt: *at(20),
		ProtectionCoverage: CoverageComplete, ObservationCoverage: CoverageComplete,
		Required: []RequiredCheck{{ID: "test", Mapped: true}},
		Observations: []CheckObservation{{ID: "job-1", CheckID: "test", RunID: "run-1",
			InstanceID: "matrix-linux", Attempt: 1, HeadSHA: "head", Conclusion: "success",
			QueuedAt: start, ProvisionedAt: at(8), StartedAt: at(9), CompletedAt: at(10)}},
	}
}

func TestMergeDelayKeepsQueueAndStartupApart(t *testing.T) {
	r, err := MergeDelay(delayFixture())
	if err != nil {
		t.Fatal(err)
	}
	d := r.Checks[0]
	if r.State != "checks_passed" || r.LastFinishingCheckID != "test" ||
		*d.Timing.PreStartMS != 540000 || *d.Timing.QueueMS != 480000 ||
		*d.Timing.StartupMS != 60000 || *d.Timing.ExecutionMS != 60000 || *d.CompletionDelayMS != 600000 {
		t.Fatalf("unexpected delay: %+v %+v", r, d.Timing)
	}
}

func TestMergeDelayUsesCheckReportingCompletionWithoutAddingItToExecution(t *testing.T) {
	s := delayFixture()
	completed := s.WindowStart.Add(12 * time.Minute)
	s.Observations[0].CheckCompletedAt = &completed
	r, err := MergeDelay(s)
	if err != nil || *r.Checks[0].CompletionDelayMS != 720000 || *r.Checks[0].Timing.ExecutionMS != 60000 {
		t.Fatalf("got %+v, %v", r, err)
	}
	completed = s.WindowStart.Add(9 * time.Minute)
	r, err = MergeDelay(s)
	if err != nil || r.Checks[0].State != "invalid_timing" {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestMergeDelayKeepsIncompleteEvidenceAndBlockedChecksVisible(t *testing.T) {
	cases := []struct {
		name   string
		change func(*MergeDelaySnapshot)
		state  string
		check  string
	}{
		{"protection refused", func(s *MergeDelaySnapshot) { s.ProtectionCoverage = CoverageNotPermitted }, "incomplete", "passed"},
		{"sampled observations", func(s *MergeDelaySnapshot) { s.ObservationCoverage = CoveragePartial }, "incomplete", "passed"},
		{"external check", func(s *MergeDelaySnapshot) { s.Required[0].Mapped = false }, "incomplete", "unmapped"},
		{"missing delivery", func(s *MergeDelaySnapshot) { s.Observations = nil }, "incomplete", "missing"},
		{"still running", func(s *MergeDelaySnapshot) { s.Observations[0].CompletedAt = nil }, "checks_pending", "pending"},
		{"failing tests", func(s *MergeDelaySnapshot) { s.Observations[0].Conclusion = "failure" }, "checks_failed", "failed"},
		{"unknown result", func(s *MergeDelaySnapshot) { s.Observations[0].Conclusion = "" }, "incomplete", "unknown_outcome"},
		{"future completion", func(s *MergeDelaySnapshot) { v := s.ObservedAt.Add(time.Minute); s.Observations[0].CompletedAt = &v }, "incomplete", "invalid_timing"},
		{"reversed startup", func(s *MergeDelaySnapshot) {
			v := s.WindowStart.Add(11 * time.Minute)
			s.Observations[0].ProvisionedAt = &v
		}, "incomplete", "invalid_timing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := delayFixture()
			tc.change(&s)
			r, err := MergeDelay(s)
			if err != nil {
				t.Fatal(err)
			}
			if r.State != tc.state || r.Checks[0].State != tc.check {
				t.Fatalf("got %+v", r)
			}
		})
	}
}

func TestMergeDelayDoesNotUseAnotherHeadOrGuessCurrentness(t *testing.T) {
	s := delayFixture()
	s.Observations[0].HeadSHA = "old-head"
	r, err := MergeDelay(s)
	if err != nil || r.State != "incomplete" || r.IgnoredOtherHeads != 1 || r.LastFinishingCheckID != "" {
		t.Fatalf("got %+v, %v", r, err)
	}
	for _, tc := range []struct{ head, state string }{{"new-head", "superseded"}, {"", "currentness_unknown"}} {
		s = delayFixture()
		s.CurrentHeadSHA = tc.head
		r, err = MergeDelay(s)
		if err != nil || r.State != tc.state || len(r.Checks) != 0 {
			t.Fatalf("got %+v, %v", r, err)
		}
	}
}

func TestMergeDelayUsesTheLatestAttemptWithoutCollapsingMatrixMembers(t *testing.T) {
	s := delayFixture()
	retry := s.Observations[0]
	retry.ID = "retry"
	retry.Attempt = 2
	retry.CompletedAt = nil
	s.Observations = append(s.Observations, retry)
	r, err := MergeDelay(s)
	if err != nil || r.State != "checks_pending" || r.Checks[0].ObservationID != "retry" {
		t.Fatalf("got %+v, %v", r, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*CheckObservation)
	}{
		{"matrix member", func(o *CheckObservation) { o.InstanceID = "matrix-windows" }},
		{"competing workflow", func(o *CheckObservation) { o.RunID = "run-2" }},
		{"duplicate check identity", func(o *CheckObservation) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := delayFixture()
			other := s.Observations[0]
			other.ID = "other"
			tc.change(&other)
			s.Observations = append(s.Observations, other)
			r, err := MergeDelay(s)
			if err != nil || r.State != "incomplete" || r.Checks[0].State != "ambiguous" {
				t.Fatalf("got %+v, %v", r, err)
			}
		})
	}
}

func TestMergeDelayDoesNotInventMissingTimingPhases(t *testing.T) {
	s := delayFixture()
	s.Observations[0].ProvisionedAt = nil
	r, err := MergeDelay(s)
	if err != nil {
		t.Fatal(err)
	}
	v := r.Checks[0].Timing
	if v.QueueMS != nil || v.StartupMS != nil || v.PreStartMS == nil || v.ExecutionMS == nil {
		t.Fatalf("got %+v", v)
	}
}

func TestMergeDelayKeepsASkippedCheckWithoutInventingExecutionTime(t *testing.T) {
	s := delayFixture()
	s.Observations[0].Conclusion = "skipped"
	s.Observations[0].ProvisionedAt = nil
	s.Observations[0].StartedAt = nil
	r, err := MergeDelay(s)
	if err != nil || r.State != "checks_passed" || r.Checks[0].Timing.ExecutionMS != nil {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestMergeDelayIsIndependentOfInputOrderAndDoesNotMutateItsSnapshot(t *testing.T) {
	s := delayFixture()
	s.Required = append(s.Required, RequiredCheck{ID: "build", Mapped: true})
	o := s.Observations[0]
	o.ID = "build-job"
	o.CheckID = "build"
	s.Observations = append(s.Observations, o)
	r, err := MergeDelay(s)
	if err != nil {
		t.Fatal(err)
	}
	if s.Required[0].ID != "test" || r.LastFinishingCheckID != "build" {
		t.Fatal("input changed or tie-break is unstable")
	}
	s.Required[0], s.Required[1] = s.Required[1], s.Required[0]
	s.Observations[0], s.Observations[1] = s.Observations[1], s.Observations[0]
	reversed, err := MergeDelay(s)
	if err != nil || !reflect.DeepEqual(r, reversed) {
		t.Fatalf("order changed result: %+v, %v", reversed, err)
	}
}

func TestMergeDelayRejectsInvalidOrUnboundedSnapshots(t *testing.T) {
	for _, change := range []func(*MergeDelaySnapshot){
		func(s *MergeDelaySnapshot) { s.HeadSHA = "" },
		func(s *MergeDelaySnapshot) { s.TargetBranch = "" },
		func(s *MergeDelaySnapshot) { s.ProtectionCoverage = "" },
		func(s *MergeDelaySnapshot) { s.Required = append(s.Required, s.Required[0]) },
		func(s *MergeDelaySnapshot) { s.Observations[0].Attempt = 0 },
		func(s *MergeDelaySnapshot) { s.Required = make([]RequiredCheck, MaxRequiredChecks+1) },
		func(s *MergeDelaySnapshot) { s.Observations = make([]CheckObservation, MaxCheckObservations+1) },
	} {
		s := delayFixture()
		change(&s)
		if _, err := MergeDelay(s); err == nil {
			t.Fatal("accepted invalid snapshot")
		}
	}
}

func TestMergeDelayWithNoRequiredChecksIsNotAMergeReadinessVerdict(t *testing.T) {
	s := delayFixture()
	s.Required = nil
	r, err := MergeDelay(s)
	if err != nil || r.State != "no_required_checks" || r.LastFinishingCheckID != "" {
		t.Fatalf("got %+v, %v", r, err)
	}
}
