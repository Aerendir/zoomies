package analysis

import (
	"slices"
	"testing"
	"time"
)

func experimentFixture() ExperimentSnapshot {
	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	arm := ExperimentArm{SourceRevision: "commit", WorkflowRevision: "workflow", InputsID: "inputs", TestScopeID: "tests", ConfigurationID: "small", ImageID: "image", ResourceID: "resources", SelectionComplete: true}
	s := ExperimentSnapshot{RuleVersion: ExperimentRuleVersion, Kind: "runner", Repository: "owner/repo", InitiatingUser: "user", Hypothesis: "reduce wait", IntendedDelta: "larger runner", EvidenceID: "selection", Since: start, Until: start.Add(time.Hour), Baseline: arm, Candidate: arm}
	s.Candidate.ConfigurationID = "large"
	for i := 0; i < 30; i++ {
		a := ExperimentAttempt{RunID: int64(i + 1), Attempt: 1, ObservedAt: start.Add(time.Minute), Outcome: "success", Cache: "cold", HostCondition: "quiet"}
		s.Baseline.Attempts = append(s.Baseline.Attempts, a)
		a.RunID += 30
		s.Candidate.Attempts = append(s.Candidate.Attempts, a)
	}
	return s
}

func TestSelectedRunsExplainWhyAComparisonIsInconclusive(t *testing.T) {
	cases := []struct {
		name, reason string
		change       func(*ExperimentSnapshot)
	}{
		{"different source", "source_mismatch", func(s *ExperimentSnapshot) { s.Candidate.SourceRevision = "other" }},
		{"different workflow", "workflow_mismatch", func(s *ExperimentSnapshot) { s.Candidate.WorkflowRevision = "other" }},
		{"different inputs", "inputs_mismatch", func(s *ExperimentSnapshot) { s.Candidate.InputsID = "other" }},
		{"different tests", "test_scope_mismatch", func(s *ExperimentSnapshot) { s.Candidate.TestScopeID = "other" }},
		{"unknown cache", "cache_unknown", func(s *ExperimentSnapshot) { s.Candidate.Attempts[0].Cache = "unknown" }},
		{"warm candidate", "cache_mismatch", func(s *ExperimentSnapshot) {
			for i := range s.Candidate.Attempts {
				s.Candidate.Attempts[i].Cache = "warm"
			}
		}},
		{"busy host", "host_pressure", func(s *ExperimentSnapshot) { s.Candidate.Attempts[0].HostCondition = "busy" }},
		{"missing host", "host_unknown", func(s *ExperimentSnapshot) { s.Candidate.Attempts[0].HostCondition = "unknown" }},
		{"small sample", "insufficient_samples", func(s *ExperimentSnapshot) { s.Candidate.Attempts = s.Candidate.Attempts[:29] }},
		{"selection incomplete", "selection_incomplete", func(s *ExperimentSnapshot) { s.Candidate.SelectionComplete = false }},
		{"pending attempt", "outcome_unknown", func(s *ExperimentSnapshot) { s.Candidate.Attempts[0].Outcome = "pending" }},
		{"unrecorded delta", "revision_delta_missing", func(s *ExperimentSnapshot) { s.Kind = "workflow"; s.Candidate.SourceRevision = "other" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := experimentFixture()
			tc.change(&s)
			r, err := AssessExperiment(s)
			if err != nil {
				t.Fatal(err)
			}
			if r.State != "inconclusive" || !slices.Contains(r.Reasons, tc.reason) {
				t.Fatalf("got %+v", r)
			}
		})
	}
}

func TestEligibilityKeepsFailuresAndReportsExcludedAttempts(t *testing.T) {
	s := experimentFixture()
	s.Candidate.Attempts[0].Outcome = "failure"
	a := s.Candidate.Attempts[0]
	a.RunID = 100
	a.Outcome = "cancelled"
	s.Candidate.Attempts = append(s.Candidate.Attempts, a)
	a.RunID = 101
	a.Outcome = "superseded"
	s.Candidate.Attempts = append(s.Candidate.Attempts, a)
	r, err := AssessExperiment(s)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != "eligible" || r.Candidate.Completed != 30 || r.Candidate.Failed != 1 || r.Candidate.Cancelled != 1 || r.Candidate.Superseded != 1 {
		t.Fatalf("got %+v", r)
	}
}

func TestAnAttemptCannotAppearTwiceOrInBothArms(t *testing.T) {
	for _, cross := range []bool{false, true} {
		s := experimentFixture()
		if cross {
			s.Candidate.Attempts[0] = s.Baseline.Attempts[0]
		} else {
			s.Baseline.Attempts = append(s.Baseline.Attempts, s.Baseline.Attempts[0])
		}
		if _, err := AssessExperiment(s); err == nil {
			t.Fatal("duplicate accepted")
		}
	}
}

func TestAnExperimentRequiresAReproducibleBoundedRecord(t *testing.T) {
	changes := []func(*ExperimentSnapshot){
		func(s *ExperimentSnapshot) { s.RuleVersion = "future" }, func(s *ExperimentSnapshot) { s.Repository = "" }, func(s *ExperimentSnapshot) { s.EvidenceID = "" }, func(s *ExperimentSnapshot) { s.InitiatingUser = "" }, func(s *ExperimentSnapshot) { s.Hypothesis = "" }, func(s *ExperimentSnapshot) { s.IntendedDelta = "" }, func(s *ExperimentSnapshot) { s.Until = s.Since }, func(s *ExperimentSnapshot) { s.Candidate.ImageID = "" }, func(s *ExperimentSnapshot) { s.Candidate.Attempts[0].ObservedAt = s.Until }, func(s *ExperimentSnapshot) { s.Candidate.Attempts[0].Outcome = "invented" }, func(s *ExperimentSnapshot) { s.Candidate.Attempts = make([]ExperimentAttempt, MaxExperimentAttempts+1) },
	}
	for i, change := range changes {
		s := experimentFixture()
		change(&s)
		if _, err := AssessExperiment(s); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}

func TestWorkflowComparisonsRetainAnExplicitRevisionDelta(t *testing.T) {
	s := experimentFixture()
	s.Kind = "workflow"
	s.Candidate.WorkflowRevision = "changed"
	s.RevisionDeltaID = "retained-diff"
	r, err := AssessExperiment(s)
	if err != nil || r.State != "eligible" {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestRetriesRemainSeparateAndExclusionsCannotMeetTheSampleMinimum(t *testing.T) {
	s := experimentFixture()
	s.Candidate.Attempts[0].RunID = s.Baseline.Attempts[0].RunID
	s.Candidate.Attempts[0].Attempt = 2
	s.Candidate.Attempts[1].Outcome = "cancelled"
	r, err := AssessExperiment(s)
	if err != nil || r.Candidate.Completed != 29 || !slices.Contains(r.Reasons, "insufficient_samples") {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestEligibilityIsDeterministicAndDoesNotRewriteEvidence(t *testing.T) {
	s := experimentFixture()
	s.Candidate.Attempts[0].Cache = "unknown"
	s.Candidate.Attempts[0].HostCondition = "busy"
	s.Candidate.SelectionComplete = false
	first, err := AssessExperiment(s)
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		next, err := AssessExperiment(s)
		if err != nil || !slices.Equal(first.Reasons, next.Reasons) || !slices.IsSorted(next.Reasons) {
			t.Fatalf("unstable result %+v, %v", next, err)
		}
	}
	if s.Candidate.Attempts[0].Cache != "unknown" || s.Candidate.Attempts[0].HostCondition != "busy" || s.Candidate.SelectionComplete {
		t.Fatal("evidence rewritten")
	}
}
