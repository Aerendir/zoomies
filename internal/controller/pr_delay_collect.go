package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eyupio/zoomies/internal/analysis"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

var ErrPRDelayUnsupported = errors.New("this installation cannot read PR delay evidence")
var ErrPRDelayJobHeadChanged = errors.New("the selected job is not on the current PR head; select a job from that head")

const prDelayReadTimeout = 20 * time.Second
const maxPRDelayJobReads = 500

type PRDelayReport struct {
	JobID          string
	Repository     string
	InstallationID string
	PullNumber     int
	// WindowBasis prevents the earliest retained job from being mistaken for
	// the time the developer pushed this revision.
	WindowBasis string
	Result      analysis.MergeDelayResult
}

// PRDelayForJob is on-demand collection for a job the caller may already read.
// A transport must authorise job access before calling it. It has no background
// trigger and cannot fall back to another installation's credentials.
func (c *Controller) PRDelayForJob(ctx context.Context, jobID string, pull int) (*PRDelayReport, error) {
	if pull < 1 {
		return nil, fmt.Errorf("%w: a PR number must be positive", ErrPRDelayEvidence)
	}
	focus, err := c.st.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if focus.InstallationID == "" || focus.Repo == "" || focus.HeadSHA == "" || focus.QueuedAt.IsZero() {
		return nil, fmt.Errorf("%w: the selected job has no complete repository, installation or revision identity", ErrPRDelayEvidence)
	}
	inst, err := c.st.GetInstallation(ctx, focus.InstallationID)
	if err != nil {
		return nil, err
	}
	owner, repo, kind := github.SplitTarget(focus.Repo)
	if kind != store.TargetRepo || !(inst.TargetType == store.TargetRepo && inst.Target == focus.Repo || inst.TargetType == store.TargetOrg && inst.Target == owner) || repo == "" {
		return nil, fmt.Errorf("%w: the selected job's installation does not cover its repository", ErrPRDelayEvidence)
	}
	if c.githubHeld(inst.ID, c.Now()) {
		return nil, github.ErrRateLimited
	}
	ctx, cancel := context.WithTimeout(ctx, prDelayReadTimeout)
	defer cancel()
	raw, err := c.ClientFor(ctx, inst.ID)
	if err != nil {
		return nil, err
	}
	reader, ok := raw.(github.PRDelayReader)
	if !ok {
		return nil, ErrPRDelayUnsupported
	}
	e, err := reader.ReadPRDelay(ctx, focus.Repo, pull)
	if err != nil {
		c.holdIfRateLimited(inst.ID, err, c.Now(), "reading PR delay evidence")
		return nil, err
	}
	if e == nil || e.Number != pull {
		return nil, ErrPRDelayEvidence
	}
	if e.HeadSHA != focus.HeadSHA {
		return nil, ErrPRDelayJobHeadChanged
	}
	if len(e.Checks) > maxPRDelayJobReads {
		return nil, ErrPRDelayEvidence
	}
	if e.ProtectionCoverage == "held_back" || e.ChecksCoverage == "held_back" {
		// The reader's coverage is bounded evidence rather than an error. Keep
		// it visible and also respect the controller's installation stand-down.
		c.holdIfRateLimited(inst.ID, github.ErrRateLimited, c.Now(), "reading PR delay evidence")
	}
	jobs := []PRDelayJobEvidence{}
	coverage := analysis.CoverageComplete
	seen := map[int64]bool{}
	windowStart := focus.QueuedAt
	for _, check := range e.Checks {
		if check.AppID != github.GitHubActionsAppID || check.HeadSHA != e.HeadSHA || check.WorkflowJobID <= 0 || seen[check.WorkflowJobID] {
			continue
		}
		seen[check.WorkflowJobID] = true
		job, err := c.st.GetJobByGitHubID(ctx, check.WorkflowJobID)
		if errors.Is(err, store.ErrNotFound) {
			coverage = analysis.CoveragePartial
			continue
		}
		if err != nil {
			return nil, err
		}
		if job.Repo != focus.Repo || job.InstallationID != inst.ID || job.HeadSHA != e.HeadSHA || job.GitHubRunID != check.WorkflowRunID {
			coverage = analysis.CoveragePartial
			continue
		}
		jobs = append(jobs, PRDelayJobEvidence{Job: job})
		if !job.QueuedAt.IsZero() && job.QueuedAt.Before(windowStart) {
			windowStart = job.QueuedAt
		}
	}
	result, err := AnalysePRDelayEvidence(PRDelayInput{Repository: focus.Repo, InstallationID: inst.ID,
		WindowStart: windowStart, ObservedAt: c.Now(), Evidence: e, Jobs: jobs, JobsCoverage: coverage})
	if err != nil {
		return nil, err
	}
	return &PRDelayReport{JobID: focus.ID, Repository: focus.Repo, InstallationID: inst.ID,
		PullNumber: pull, WindowBasis: "earliest_observed_job", Result: result}, nil
}
