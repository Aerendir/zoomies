package controller

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/eyupio/zoomies/internal/analysis"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

var ErrPRDelayTargetChanged = errors.New("the PR target branch changed; refresh its delay evidence")
var ErrPRDelayClosed = errors.New("the PR is closed; its checks are not a current PR gate")
var ErrPRDelayEvidence = errors.New("PR delay evidence is incomplete or inconsistent")

// PRDelayJobEvidence includes an optional provisioning-start measurement, not
// the runner's registration time or a second copy of total queue wait.
type PRDelayJobEvidence struct {
	Job           *store.Job
	ProvisionedAt *time.Time
}

// PRDelayInput is assembled by an authorised reader. Identity checks here
// prevent an accidental cross-installation join; they do not grant read access.
type PRDelayInput struct {
	Repository     string
	InstallationID string
	WindowStart    time.Time
	ObservedAt     time.Time
	Evidence       *github.PRDelayEvidence
	Jobs           []PRDelayJobEvidence
	JobsCoverage   analysis.Coverage
}

// AnalysePRDelayEvidence joins transient GitHub check names to required checks
// and verifies job-link hints against fleet records before calling the pure
// core. It reads no source, executes nothing and returns no repository names
// chosen for checks, workflows or jobs.
func AnalysePRDelayEvidence(in PRDelayInput) (analysis.MergeDelayResult, error) {
	e := in.Evidence
	if e == nil || in.Repository == "" || in.InstallationID == "" {
		return analysis.MergeDelayResult{}, ErrPRDelayEvidence
	}
	if !e.Open || e.CurrentHeadSHA != "" && !e.StillOpen {
		return analysis.MergeDelayResult{}, ErrPRDelayClosed
	}
	if e.CurrentTargetBranch != "" && e.CurrentTargetBranch != e.TargetBranch {
		return analysis.MergeDelayResult{}, ErrPRDelayTargetChanged
	}
	if e.CurrentHeadSHA != "" && e.CurrentTargetBranch == "" {
		return analysis.MergeDelayResult{}, ErrPRDelayEvidence
	}
	if len(in.Jobs) > analysis.MaxCheckObservations || len(e.Checks) > analysis.MaxCheckObservations {
		return analysis.MergeDelayResult{}, ErrPRDelayEvidence
	}
	s := analysis.MergeDelaySnapshot{HeadSHA: e.HeadSHA, CurrentHeadSHA: e.CurrentHeadSHA,
		TargetBranch: e.TargetBranch, WindowStart: in.WindowStart, ObservedAt: in.ObservedAt,
		ProtectionCoverage:  analysis.Coverage(e.ProtectionCoverage),
		ObservationCoverage: analysis.Coverage(e.ChecksCoverage),
		Required:            []analysis.RequiredCheck{}, Observations: []analysis.CheckObservation{}}
	if in.JobsCoverage != analysis.CoverageComplete {
		if s.ObservationCoverage == analysis.CoverageComplete {
			s.ObservationCoverage = in.JobsCoverage
		}
	}
	if e.Protection == nil {
		if s.ProtectionCoverage == analysis.CoverageComplete {
			s.ProtectionCoverage = analysis.CoverageFailed
		}
		return analysis.MergeDelay(s)
	}
	if e.Protection.Partial && s.ProtectionCoverage == analysis.CoverageComplete {
		s.ProtectionCoverage = analysis.CoveragePartial
	}
	if e.Protection.Unpinned < 0 || len(e.Protection.Required) > analysis.MaxRequiredChecks ||
		e.Protection.Unpinned > analysis.MaxRequiredChecks-len(e.Protection.Required) {
		return analysis.MergeDelayResult{}, ErrPRDelayEvidence
	}
	names := append([]string(nil), e.Protection.Required...)
	sort.Strings(names)
	identities := map[string]string{}
	for i, name := range names {
		if name == "" || identities[name] != "" {
			return analysis.MergeDelayResult{}, ErrPRDelayEvidence
		}
		id := fmt.Sprintf("chk_%03d", i+1)
		identities[name] = id
		s.Required = append(s.Required, analysis.RequiredCheck{ID: id, Mapped: true})
	}
	for i := 0; i < e.Protection.Unpinned; i++ {
		s.Required = append(s.Required, analysis.RequiredCheck{ID: fmt.Sprintf("unmapped_%03d", i+1)})
	}
	jobs := map[[2]int64]PRDelayJobEvidence{}
	duplicates := map[[2]int64]bool{}
	for _, observed := range in.Jobs {
		job := observed.Job
		if job == nil || job.Repo != in.Repository || job.InstallationID != in.InstallationID || job.HeadSHA != e.HeadSHA {
			continue
		}
		key := [2]int64{job.GitHubRunID, job.GitHubJobID}
		if _, exists := jobs[key]; exists {
			duplicates[key] = true
		}
		jobs[key] = observed
	}
	for _, check := range e.Checks {
		id := identities[check.Name]
		if id == "" || check.AppID != github.GitHubActionsAppID {
			continue
		}
		if check.HeadSHA != e.HeadSHA {
			s.ObservationCoverage = incompleteObservationCoverage(s.ObservationCoverage)
			continue
		}
		key := [2]int64{check.WorkflowRunID, check.WorkflowJobID}
		observed, exists := jobs[key]
		if !exists || duplicates[key] || check.WorkflowRunID <= 0 || check.WorkflowJobID <= 0 || check.ID <= 0 {
			s.ObservationCoverage = incompleteObservationCoverage(s.ObservationCoverage)
			continue
		}
		job := observed.Job
		// Both the numeric identities and the check/job names must agree. A link
		// to an unrelated successful job cannot satisfy this required check.
		if job.JobName != check.Name || job.RunAttempt < 1 {
			s.ObservationCoverage = incompleteObservationCoverage(s.ObservationCoverage)
			continue
		}
		o := analysis.CheckObservation{ID: "check_" + strconv.FormatInt(check.ID, 10), CheckID: id,
			RunID: strconv.FormatInt(job.GitHubRunID, 10), InstanceID: job.JobName,
			Attempt: job.RunAttempt, HeadSHA: job.HeadSHA, QueuedAt: job.QueuedAt,
			ProvisionedAt: observed.ProvisionedAt, StartedAt: job.StartedAt,
			CompletedAt: job.CompletedAt, Conclusion: check.Conclusion}
		o.CheckCompletedAt = check.CompletedAt
		// A check result and a job delivery can race. Preserve known intervals,
		// but do not infer a current pass from mismatched or unfinished evidence.
		if check.Status == "completed" && (check.CompletedAt == nil || job.CompletedAt == nil || job.Conclusion != check.Conclusion) ||
			check.Status != "completed" && (job.CompletedAt != nil || check.CompletedAt != nil) || check.Status == "unknown" || check.Conclusion == "unknown" {
			s.ObservationCoverage = incompleteObservationCoverage(s.ObservationCoverage)
			o.Conclusion = ""
		}
		s.Observations = append(s.Observations, o)
	}
	return analysis.MergeDelay(s)
}

func incompleteObservationCoverage(c analysis.Coverage) analysis.Coverage {
	if c == analysis.CoverageComplete {
		return analysis.CoveragePartial
	}
	return c
}
