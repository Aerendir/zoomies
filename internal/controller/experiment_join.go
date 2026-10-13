package controller

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

const maxExperimentRemoteAttempts = 10

var ErrExperimentAdmission = errors.New("workflow comparison diagnostic reads are temporarily limited")
var ErrExperimentUnsupported = errors.New("this installation cannot read workflow-attempt comparison evidence")

type ExperimentAdmissionError struct{ RetryAfter time.Duration }

func (e *ExperimentAdmissionError) Error() string { return ErrExperimentAdmission.Error() }
func (e *ExperimentAdmissionError) Unwrap() error { return ErrExperimentAdmission }

type ExperimentMatchedJob struct {
	RunnerCreation *store.RunnerCreationObservation `json:"runner_creation,omitempty"`
	RunnerID       string                           `json:"runner_id,omitempty"`
	PoolID         string                           `json:"pool_id,omitempty"`
	HostID         string                           `json:"host_id,omitempty"`
	RecordedGrant  *ExperimentRecordedGrant         `json:"recorded_grant,omitempty"`
	GitHubJobID    int64                            `json:"github_job_id"`
	FleetJobID     string                           `json:"fleet_job_id"`
}

type ExperimentJoinedAttempt struct {
	Remote                *github.ExperimentAttemptEvidence `json:"remote"`
	RetainedCoverage      string                            `json:"retained_coverage"`
	FleetIdentityCoverage string                            `json:"fleet_identity_coverage"`
	MatchedJobs           []ExperimentMatchedJob            `json:"matched_jobs"`
	MissingJobIDs         []int64                           `json:"missing_job_ids"`
	ExtraFleetJobIDs      []int64                           `json:"extra_fleet_job_ids"`
}

type ExperimentJoinedEvidence struct {
	Repository        string                    `json:"repository"`
	InstallationID    string                    `json:"installation_id"`
	SelectionComplete bool                      `json:"selection_complete"`
	Attempts          []ExperimentJoinedAttempt `json:"attempts"`
}

// CollectExperimentEvidence requires an already authorised job reader. There
// is no transport here: an installation join cannot authorise a caller. Sharing
// PR diagnostic admission prevents two kinds of reader racing the same quota.
func (c *Controller) CollectExperimentEvidence(ctx context.Context, installation, repo string, selected []ExperimentAttemptSelection) (*ExperimentJoinedEvidence, error) {
	if len(selected) < 1 || len(selected) > maxExperimentRemoteAttempts {
		return nil, fmt.Errorf("%w: select 1 to %d attempts per remote collection", ErrExperimentEvidence, maxExperimentRemoteAttempts)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	retained, err := c.CollectExperimentAttempts(ctx, installation, repo, selected)
	if err != nil {
		return nil, err
	}
	if c.githubHeld(installation, c.Now()) {
		return nil, github.ErrRateLimited
	}
	raw, err := c.ClientFor(ctx, installation)
	if err != nil {
		return nil, err
	}
	reader, ok := raw.(github.ExperimentAttemptReader)
	if !ok {
		return nil, ErrExperimentUnsupported
	}
	release, err := c.admitPRDelay(installation)
	if err != nil {
		var limited *PRDelayAdmissionError
		if errors.As(err, &limited) {
			return nil, &ExperimentAdmissionError{RetryAfter: limited.RetryAfter}
		}
		return nil, err
	}
	defer release()
	quota, err := raw.RateLimit(ctx)
	if err != nil {
		c.holdIfRateLimited(installation, err, c.Now(), "reading comparison diagnostic quota")
		return nil, err
	}
	cost := len(selected) * github.ExperimentAttemptMaximumRequests
	if quota == nil || quota.Limit <= 0 || quota.Remaining < 0 || quota.Remaining > quota.Limit || quota.Remaining-cost < quota.Limit/2+quota.Limit%2 {
		retry := time.Minute
		if quota != nil && quota.ResetAt.After(c.Now()) {
			retry = quota.ResetAt.Sub(c.Now())
		}
		return nil, &ExperimentAdmissionError{RetryAfter: retry}
	}
	out := &ExperimentJoinedEvidence{Repository: repo, InstallationID: installation, Attempts: make([]ExperimentJoinedAttempt, 0, len(selected))}
	for i, s := range selected {
		if c.githubHeld(installation, c.Now()) {
			return nil, github.ErrRateLimited
		}
		remote, err := reader.ReadExperimentAttempt(ctx, repo, s.RunID, s.Attempt)
		if err != nil {
			c.holdIfRateLimited(installation, err, c.Now(), "reading comparison attempt evidence")
			return nil, err
		}
		if err := validateExperimentRemote(remote, repo, s); err != nil {
			return nil, err
		}
		out.Attempts = append(out.Attempts, joinExperimentAttempt(remote, retained.Attempts[i]))
	}
	// Workflow enumeration and fleet identity matches prove neither fair cohort
	// selection nor historical workload, cache or configuration equivalence.
	return out, nil
}

func validateExperimentRemote(e *github.ExperimentAttemptEvidence, repo string, s ExperimentAttemptSelection) error {
	if e == nil || e.Repository != repo || e.RunID != s.RunID || e.Attempt != s.Attempt || e.WorkflowID <= 0 || len(e.Jobs) > 500 || (e.JobsCoverage != "complete" && e.JobsCoverage != "partial") || !experimentStatus(e.Status) || !experimentConclusion(e.Conclusion) {
		return ErrExperimentEvidence
	}
	if len(e.HeadSHA) != 40 && len(e.HeadSHA) != 64 {
		return ErrExperimentEvidence
	}
	if _, err := hex.DecodeString(e.HeadSHA); err != nil {
		return ErrExperimentEvidence
	}
	if e.JobsCoverage == "complete" && (e.Status != "completed" || e.Conclusion == "" || e.UpdatedAt == nil) {
		return ErrExperimentEvidence
	}
	seen := map[int64]bool{}
	for _, j := range e.Jobs {
		if j.ID <= 0 || seen[j.ID] || !experimentStatus(j.Status) || !experimentConclusion(j.Conclusion) || e.JobsCoverage == "complete" && (j.Status != "completed" || j.Conclusion == "") {
			return ErrExperimentEvidence
		}
		seen[j.ID] = true
	}
	return nil
}
func experimentStatus(s string) bool {
	switch s {
	case "queued", "in_progress", "completed", "waiting", "pending", "requested":
		return true
	}
	return false
}
func experimentConclusion(s string) bool {
	switch s {
	case "", "success", "failure", "neutral", "cancelled", "skipped", "timed_out", "action_required", "stale", "startup_failure":
		return true
	}
	return false
}

func joinExperimentAttempt(remote *github.ExperimentAttemptEvidence, retained ExperimentRetainedAttempt) ExperimentJoinedAttempt {
	r := ExperimentJoinedAttempt{Remote: remote, RetainedCoverage: retained.Coverage, FleetIdentityCoverage: "partial", MatchedJobs: []ExperimentMatchedJob{}, MissingJobIDs: []int64{}, ExtraFleetJobIDs: []int64{}}
	local := map[int64]ExperimentRetainedJob{}
	for _, j := range retained.Jobs {
		local[j.GitHubJobID] = j
	}
	for _, j := range remote.Jobs {
		row, ok := local[j.ID]
		if ok && row.HeadSHA == remote.HeadSHA {
			r.MatchedJobs = append(r.MatchedJobs, ExperimentMatchedJob{RunnerCreation: cloneExperimentRunnerCreation(row.RunnerCreation), RunnerID: row.RunnerID, PoolID: row.PoolID, HostID: row.HostID, GitHubJobID: j.ID, FleetJobID: row.JobID, RecordedGrant: cloneExperimentGrant(row.RecordedGrant)})
			delete(local, j.ID)
		} else {
			r.MissingJobIDs = append(r.MissingJobIDs, j.ID)
		}
	}
	// Preserve the store's stable order, not map iteration order.
	for _, j := range retained.Jobs {
		if _, ok := local[j.GitHubJobID]; ok {
			r.ExtraFleetJobIDs = append(r.ExtraFleetJobIDs, j.GitHubJobID)
		}
	}
	if remote.JobsCoverage == "complete" && retained.Coverage == "retained_only" && len(r.MissingJobIDs) == 0 && len(r.ExtraFleetJobIDs) == 0 {
		r.FleetIdentityCoverage = "complete"
	}
	return r
}
