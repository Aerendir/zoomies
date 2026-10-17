package controller

import (
	"fmt"
	"math"
	"time"

	"github.com/eyupio/zoomies/internal/analysis"
	"github.com/eyupio/zoomies/internal/github"
)

type ExperimentComparisonRecord struct {
	Kind            string    `json:"kind"`
	Repository      string    `json:"repository"`
	InitiatingUser  string    `json:"initiating_user"`
	Hypothesis      string    `json:"hypothesis"`
	IntendedDelta   string    `json:"intended_delta"`
	EvidenceID      string    `json:"evidence_id"`
	RevisionDeltaID string    `json:"revision_delta_id"`
	Since           time.Time `json:"since"`
	Until           time.Time `json:"until"`
}

type ExperimentComparisonReport struct {
	WindowBasis    string                         `json:"window_basis"`
	ExecutionBasis string                         `json:"execution_basis"`
	Snapshot       analysis.ExperimentSnapshot    `json:"snapshot"`
	Measurement    analysis.ExperimentMeasurement `json:"measurement"`
}

// AssembleExperimentComparison does no collection or authorisation. Its caller
// must retain the input evidence beside the returned snapshot. An operator's
// intended delta is a hypothesis, not evidence of past runner configuration.
func AssembleExperimentComparison(record ExperimentComparisonRecord, a, b *ExperimentJoinedEvidence) (*ExperimentComparisonReport, error) {
	if a == nil || b == nil || a.Repository != record.Repository || b.Repository != record.Repository || a.InstallationID == "" || a.InstallationID != b.InstallationID {
		return nil, ErrExperimentEvidence
	}
	seenJobs := map[int64]bool{}
	seenRunners := map[string]bool{}
	for _, side := range []*ExperimentJoinedEvidence{a, b} {
		if len(side.Attempts) < 1 || len(side.Attempts) > maxExperimentSelectedAttempts {
			return nil, ErrExperimentEvidence
		}
		for _, attempt := range side.Attempts {
			if attempt.Remote == nil || len(attempt.Remote.Jobs) > 500 {
				return nil, ErrExperimentEvidence
			}
			if err := validateExperimentRunnerCreations(side.InstallationID, attempt, seenRunners); err != nil {
				return nil, err
			}
			if err := validateExperimentGrants(attempt); err != nil {
				return nil, err
			}
			for _, job := range attempt.Remote.Jobs {
				if seenJobs[job.ID] {
					return nil, fmt.Errorf("%w: a GitHub job appears in more than one selected attempt", ErrExperimentEvidence)
				}
				seenJobs[job.ID] = true
			}
		}
	}
	baseline, err := assembleExperimentArm(a)
	if err != nil {
		return nil, err
	}
	candidate, err := assembleExperimentArm(b)
	if err != nil {
		return nil, err
	}
	s := analysis.ExperimentSnapshot{RuleVersion: analysis.ExperimentRuleVersion, Kind: record.Kind, Repository: record.Repository, InitiatingUser: record.InitiatingUser, Hypothesis: record.Hypothesis, IntendedDelta: record.IntendedDelta, EvidenceID: record.EvidenceID, RevisionDeltaID: record.RevisionDeltaID, Since: record.Since, Until: record.Until, Baseline: baseline, Candidate: candidate}
	measured, err := analysis.MeasureExperiment(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrExperimentEvidence, err)
	}
	return &ExperimentComparisonReport{WindowBasis: "github_attempt_started_at", ExecutionBasis: "sum_of_all_github_job_elapsed_times", Snapshot: s, Measurement: measured}, nil
}

func assembleExperimentArm(e *ExperimentJoinedEvidence) (analysis.ExperimentArm, error) {
	arm := analysis.ExperimentArm{Attempts: []analysis.ExperimentAttempt{}}
	if len(e.Attempts) < 1 || len(e.Attempts) > maxExperimentSelectedAttempts {
		return arm, ErrExperimentEvidence
	}
	mixedSource := false
	for _, joined := range e.Attempts {
		remote := joined.Remote
		if remote == nil {
			return arm, ErrExperimentEvidence
		}
		if err := validateExperimentRemote(remote, e.Repository, ExperimentAttemptSelection{RunID: remote.RunID, Attempt: remote.Attempt}); err != nil || remote.RunID <= 0 || remote.Attempt <= 0 {
			return arm, ErrExperimentEvidence
		}
		if arm.SourceRevision == "" {
			arm.SourceRevision = remote.HeadSHA
		} else if arm.SourceRevision != remote.HeadSHA {
			mixedSource = true
		}
		a := analysis.ExperimentAttempt{SourceRevision: remote.HeadSHA, WorkflowID: remote.WorkflowID, RunID: remote.RunID, Attempt: remote.Attempt, Outcome: experimentAttemptOutcome(remote), Cache: "unknown", HostCondition: "unknown"}
		if remote.RunStartedAt != nil {
			a.ObservedAt = *remote.RunStartedAt
		}
		var err error
		a.ExecutionMS, err = experimentJobElapsed(remote)
		if err != nil {
			return arm, err
		}
		arm.Attempts = append(arm.Attempts, a)
	}
	if mixedSource {
		arm.SourceRevision = ""
	}
	// Workflow ID is not a workflow revision. Today's pool/image cannot recover
	// past configuration, and enumeration cannot prove fair cohort selection.
	return arm, nil
}
func experimentAttemptOutcome(e *github.ExperimentAttemptEvidence) string {
	if e.Status != "completed" {
		return "pending"
	}
	switch e.Conclusion {
	case "success":
		return "success"
	case "failure", "timed_out", "startup_failure":
		return "failure"
	case "cancelled":
		return "cancelled"
	default:
		return "unknown"
	}
}
func experimentJobElapsed(e *github.ExperimentAttemptEvidence) (*int64, error) {
	known := e.JobsCoverage == "complete" && len(e.Jobs) > 0
	var total int64
	for _, j := range e.Jobs {
		if j.StartedAt == nil || j.CompletedAt == nil || j.StartedAt.IsZero() || j.CompletedAt.IsZero() {
			known = false
			continue
		}
		d := j.CompletedAt.Sub(*j.StartedAt)
		if d < 0 || d == time.Duration(math.MaxInt64) {
			return nil, fmt.Errorf("%w: job elapsed time is reversed or exceeds the supported duration", ErrExperimentEvidence)
		}
		ms := d.Milliseconds()
		if ms > math.MaxInt64-total {
			return nil, ErrExperimentEvidence
		}
		total += ms
	}
	if !known {
		return nil, nil
	}
	return &total, nil
}
