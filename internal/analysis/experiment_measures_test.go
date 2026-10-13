package analysis

import (
	"math"
	"testing"
)

func measuredExperiment() ExperimentSnapshot {
	s := experimentFixture()
	for _, arm := range []*ExperimentArm{&s.Baseline, &s.Candidate} {
		for i := range arm.Attempts {
			arm.Attempts[i].LatencyMS = experimentMS(100)
			arm.Attempts[i].ExecutionMS = experimentMS(100)
			arm.Attempts[i].AllocatedIdleMS = experimentMS(0)
		}
	}
	return s
}
func experimentMS(v int64) *int64 { return &v }

func TestFasterLatencyDoesNotHideHigherConsumption(t *testing.T) {
	s := measuredExperiment()
	for i := range s.Candidate.Attempts {
		s.Candidate.Attempts[i].LatencyMS = experimentMS(50)
		s.Candidate.Attempts[i].ExecutionMS = experimentMS(200)
	}
	r, err := MeasureExperiment(s)
	if err != nil {
		t.Fatal(err)
	}
	if r.LatencyChange != "improved" || r.ExecutionChange != "regressed" || *r.Candidate.Execution.TotalMS != 6000 || r.Candidate.Latency.Samples != 30 {
		t.Fatalf("got %+v", r)
	}
}
func TestFailureRegressionRetainsFailedAndCancelledConsumption(t *testing.T) {
	s := measuredExperiment()
	s.Candidate.Attempts[0].Outcome = "failure"
	s.Candidate.Attempts[1].Outcome = "failure"
	a := s.Candidate.Attempts[0]
	a.RunID = 100
	a.Outcome = "cancelled"
	a.ExecutionMS = experimentMS(500)
	s.Candidate.Attempts = append(s.Candidate.Attempts, a)
	r, err := MeasureExperiment(s)
	if err != nil {
		t.Fatal(err)
	}
	if r.FailureChange != "regressed" || r.Candidate.Latency.Samples != 28 || *r.Candidate.Execution.TotalMS != 3500 || r.Candidate.FailureRate == nil {
		t.Fatalf("got %+v", r)
	}
}
func TestMissingConsumptionDoesNotBecomeZeroOrAnImprovement(t *testing.T) {
	s := measuredExperiment()
	s.Candidate.Attempts[0].ExecutionMS = nil
	s.Candidate.Attempts[1].LatencyMS = nil
	r, err := MeasureExperiment(s)
	if err != nil {
		t.Fatal(err)
	}
	if r.Candidate.Execution.TotalMS != nil || r.Candidate.Execution.Missing != 1 || r.ExecutionChange != "not_enough" || r.LatencyChange != "not_enough" || r.Candidate.Latency.Missing != 1 || *r.Candidate.AllocatedIdle.TotalMS != 0 {
		t.Fatalf("got %+v", r)
	}
}
func TestIneligibleCohortsKeepTheirFiguresWithoutRanking(t *testing.T) {
	s := measuredExperiment()
	s.Candidate.InputsID = "different"
	r, err := MeasureExperiment(s)
	if err != nil || r.LatencyChange != "not_enough" || r.ExecutionChange != "not_enough" || r.FailureChange != "not_enough" || r.Candidate.Latency.P50MS == nil {
		t.Fatalf("got %+v, %v", r, err)
	}
}
func TestInvalidMeasurementsAndOverflowAreRefused(t *testing.T) {
	for _, kind := range []string{"negative", "overflow"} {
		s := measuredExperiment()
		if kind == "negative" {
			s.Candidate.Attempts[0].LatencyMS = experimentMS(-1)
		} else {
			s.Candidate.Attempts[0].ExecutionMS = experimentMS(math.MaxInt64)
		}
		if _, err := MeasureExperiment(s); err == nil {
			t.Fatalf("%s accepted", kind)
		}
	}
}
func TestLatencyUsesNearestRankAndIncludesRealZero(t *testing.T) {
	s := measuredExperiment()
	for i := range s.Baseline.Attempts {
		s.Baseline.Attempts[i].LatencyMS = experimentMS(int64(i))
	}
	r, err := MeasureExperiment(s)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Baseline.Latency
	if *d.MinMS != 0 || *d.P50MS != 14 || *d.P95MS != 28 || *d.MaxMS != 29 {
		t.Fatalf("got %+v", d)
	}
}

func TestMaterialLatencyBoundaryAndZeroAreExplicit(t *testing.T) {
	for _, tc := range []struct {
		from, to int64
		want     string
	}{{100, 110, "regressed"}, {100, 109, "unchanged"}, {110, 100, "improved"}, {0, 1, "regressed"}, {1, 0, "improved"}, {0, 0, "unchanged"}} {
		s := measuredExperiment()
		for i := range s.Baseline.Attempts {
			s.Baseline.Attempts[i].LatencyMS = experimentMS(tc.from)
			s.Candidate.Attempts[i].LatencyMS = experimentMS(tc.to)
		}
		r, err := MeasureExperiment(s)
		if err != nil || r.LatencyChange != tc.want {
			t.Fatalf("%+v: %+v, %v", tc, r, err)
		}
	}
}

func TestConsumptionRanksMeansRatherThanDifferentCohortTotals(t *testing.T) {
	s := measuredExperiment()
	for i := range 10 {
		a := s.Candidate.Attempts[0]
		a.RunID = int64(100 + i)
		s.Candidate.Attempts = append(s.Candidate.Attempts, a)
	}
	r, err := MeasureExperiment(s)
	if err != nil || r.ExecutionChange != "unchanged" || *r.Candidate.Execution.TotalMS == *r.Baseline.Execution.TotalMS {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestFivePercentagePointFailureBoundaryDoesNotRoundAway(t *testing.T) {
	s := measuredExperiment()
	for i := range 10 {
		a := s.Baseline.Attempts[0]
		a.RunID = int64(100 + i)
		s.Baseline.Attempts = append(s.Baseline.Attempts, a)
		a.RunID += 100
		s.Candidate.Attempts = append(s.Candidate.Attempts, a)
	}
	for i := range 4 {
		s.Baseline.Attempts[i].Outcome = "failure"
	}
	for i := range 6 {
		s.Candidate.Attempts[i].Outcome = "failure"
	}
	r, err := MeasureExperiment(s)
	if err != nil || r.FailureChange != "regressed" {
		t.Fatalf("got %+v, %v", r, err)
	}
}
