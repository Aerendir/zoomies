package controller

import (
	"github.com/eyupio/zoomies/internal/analysis"
	"github.com/eyupio/zoomies/internal/github"
	"testing"
	"time"
)

func experimentSnapshotFixture() (ExperimentComparisonRecord, *ExperimentJoinedEvidence, *ExperimentJoinedEvidence) {
	at := time.Date(2026, 10, 13, 1, 0, 0, 0, time.UTC)
	end := at.Add(time.Minute)
	record := ExperimentComparisonRecord{Kind: "runner", Repository: "acme/repo", InitiatingUser: "user", Hypothesis: "lower wait", IntendedDelta: "larger runner", EvidenceID: "record", Since: at.Add(-time.Hour), Until: at.Add(time.Hour)}
	side := func(run int64) *ExperimentJoinedEvidence {
		return &ExperimentJoinedEvidence{Repository: record.Repository, InstallationID: "ins_one", SelectionComplete: true, Attempts: []ExperimentJoinedAttempt{{Remote: &github.ExperimentAttemptEvidence{Repository: record.Repository, RunID: run, Attempt: 1, HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", WorkflowID: 20, Status: "completed", Conclusion: "failure", UpdatedAt: &end, RunStartedAt: &at, JobsCoverage: "complete", Jobs: []github.ExperimentAttemptJob{{ID: run + 100, Status: "completed", Conclusion: "failure", StartedAt: &at, CompletedAt: &end}}}}}}
	}
	return record, side(10), side(11)
}
func TestObservedSnapshotsRetainFailuresWithoutInventingHistoricalMetadata(t *testing.T) {
	record, a, b := experimentSnapshotFixture()
	r, err := AssembleExperimentComparison(record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	attempt := r.Snapshot.Candidate.Attempts[0]
	if r.Measurement.Eligibility.State != "inconclusive" || r.Measurement.ExecutionChange != analysis.NotEnough || attempt.Outcome != "failure" || *attempt.ExecutionMS != 60000 || attempt.LatencyMS != nil || attempt.AllocatedIdleMS != nil || r.Snapshot.Candidate.SelectionComplete || r.Snapshot.Candidate.ConfigurationID != "" || r.Snapshot.Candidate.WorkflowRevision != "" || attempt.Cache != "unknown" || attempt.HostCondition != "unknown" {
		t.Fatalf("got %+v", r)
	}
}
func TestSnapshotsDistinguishMissingConsumptionFromObservedZero(t *testing.T) {
	for _, missing := range []bool{false, true} {
		record, a, b := experimentSnapshotFixture()
		job := &b.Attempts[0].Remote.Jobs[0]
		job.CompletedAt = job.StartedAt
		if missing {
			job.StartedAt = nil
		}
		r, err := AssembleExperimentComparison(record, a, b)
		if err != nil {
			t.Fatal(err)
		}
		value := r.Snapshot.Candidate.Attempts[0].ExecutionMS
		if missing && value != nil || !missing && (value == nil || *value != 0) {
			t.Fatalf("missing=%v value=%v", missing, value)
		}
	}
}
func TestSnapshotsRetainUnknownStartsAndCancellation(t *testing.T) {
	record, a, b := experimentSnapshotFixture()
	b.Attempts[0].Remote.RunStartedAt = nil
	b.Attempts[0].Remote.Conclusion = "cancelled"
	r, err := AssembleExperimentComparison(record, a, b)
	if err != nil || r.Measurement.Eligibility.Candidate.Cancelled != 1 || !r.Snapshot.Candidate.Attempts[0].ObservedAt.IsZero() {
		t.Fatalf("got %+v, %v", r, err)
	}
}
func TestSnapshotsRefuseMixedScopeDuplicatesAndReversedTiming(t *testing.T) {
	for _, mode := range []string{"repository", "installation", "duplicate", "timing", "bound", "job_bound", "overflow"} {
		record, a, b := experimentSnapshotFixture()
		switch mode {
		case "repository":
			b.Repository = "other/repo"
		case "installation":
			b.InstallationID = "other"
		case "duplicate":
			b.Attempts = a.Attempts
		case "bound":
			b.Attempts = make([]ExperimentJoinedAttempt, 101)
		case "job_bound":
			b.Attempts[0].Remote.Jobs = make([]github.ExperimentAttemptJob, 501)
		case "overflow":
			start := time.Date(2, 1, 2, 0, 0, 0, 0, time.UTC)
			b.Attempts[0].Remote.Jobs[0].StartedAt = &start
		}
		if mode == "timing" {
			at := b.Attempts[0].Remote.Jobs[0].StartedAt.Add(-time.Second)
			b.Attempts[0].Remote.Jobs[0].CompletedAt = &at
		}
		if _, err := AssembleExperimentComparison(record, a, b); err == nil {
			t.Fatalf("%s accepted", mode)
		}
	}
}

func TestSnapshotsKeepMixedHeadsAndPartialEnumerationExplicit(t *testing.T) {
	record, a, b := experimentSnapshotFixture()
	extra := *a.Attempts[0].Remote
	extra.RunID = 12
	extra.HeadSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	extra.Jobs = append([]github.ExperimentAttemptJob(nil), extra.Jobs...)
	extra.Jobs[0].ID = 112
	a.Attempts = append(a.Attempts, ExperimentJoinedAttempt{Remote: &extra})
	b.Attempts[0].Remote.JobsCoverage = "partial"
	r, err := AssembleExperimentComparison(record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	if r.Snapshot.Baseline.SourceRevision != "" || r.Snapshot.Baseline.Attempts[1].SourceRevision != extra.HeadSHA || r.Snapshot.Candidate.Attempts[0].ExecutionMS != nil || r.Measurement.ExecutionChange != analysis.NotEnough {
		t.Fatalf("got %+v", r)
	}
}
func TestObservedSnapshotsNeverRankEvenALargeCohortWithMissingWorkloadEvidence(t *testing.T) {
	record, a, b := experimentSnapshotFixture()
	grow := func(side *ExperimentJoinedEvidence, base int64) {
		original := *side.Attempts[0].Remote
		side.Attempts = nil
		for i := range 30 {
			copy := original
			copy.RunID = base + int64(i)
			copy.Jobs = append([]github.ExperimentAttemptJob(nil), copy.Jobs...)
			copy.Jobs[0].ID = base + 1000 + int64(i)
			side.Attempts = append(side.Attempts, ExperimentJoinedAttempt{Remote: &copy})
		}
	}
	grow(a, 100)
	grow(b, 200)
	r, err := AssembleExperimentComparison(record, a, b)
	if err != nil || r.Measurement.Eligibility.Baseline.Failed != 30 || r.Measurement.FailureChange != analysis.NotEnough || r.Measurement.ExecutionChange != analysis.NotEnough {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestSnapshotsCannotReuseOneJobToInflateDifferentAttemptSamples(t *testing.T) {
	record, a, b := experimentSnapshotFixture()
	b.Attempts[0].Remote.Jobs[0].ID = a.Attempts[0].Remote.Jobs[0].ID
	if _, err := AssembleExperimentComparison(record, a, b); err == nil {
		t.Fatal("reused measurement accepted")
	}
}
