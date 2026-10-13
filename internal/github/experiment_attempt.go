package github

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode"
	"unicode/utf8"

	gh "github.com/google/go-github/v88/github"
)

// ExperimentAttemptReader is optional and on demand. Controller admission and
// the caller's repository permission must be checked before spending its reads.
type ExperimentAttemptReader interface {
	ReadExperimentAttempt(context.Context, string, int64, int) (*ExperimentAttemptEvidence, error)
}

var _ ExperimentAttemptReader = (*appClient)(nil)

const ExperimentAttemptMaximumRequests = 7

var ErrExperimentAttemptChanged = errors.New("github: comparison attempt changed during collection; refresh its evidence")

type ExperimentAttemptEvidence struct {
	// These are descriptive attempt metadata, not workflow contents or inputs.
	Event        string                 `json:"event,omitempty"`
	WorkflowPath string                 `json:"workflow_path,omitempty"`
	Repository   string                 `json:"repository"`
	RunID        int64                  `json:"run_id"`
	Attempt      int                    `json:"attempt"`
	HeadSHA      string                 `json:"head_sha"`
	WorkflowID   int64                  `json:"workflow_id"`
	Status       string                 `json:"status"`
	Conclusion   string                 `json:"conclusion"`
	RunStartedAt *time.Time             `json:"run_started_at"`
	UpdatedAt    *time.Time             `json:"updated_at"`
	JobsCoverage string                 `json:"jobs_coverage"`
	Jobs         []ExperimentAttemptJob `json:"jobs"`
}

type ExperimentAttemptJob struct {
	ID          int64      `json:"id"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// ReadExperimentAttempt pins the attempt endpoint rather than the latest run.
// It reads neither logs nor source and never follows a response's jobs URL.
func (c *appClient) ReadExperimentAttempt(ctx context.Context, repo string, runID int64, attempt int) (*ExperimentAttemptEvidence, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	if runID <= 0 || attempt <= 0 {
		return nil, fmt.Errorf("github: select a positive workflow run and attempt")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Reuse the isolated 4 MiB diagnostic body bound, with no conditional cache
	// or mutation of the shared installation client.
	bounded, err := c.prDelayClient()
	if err != nil {
		return nil, err
	}
	read := func() (*gh.WorkflowRun, error) {
		r, resp, err := bounded.asInstallation.Actions.GetWorkflowRunAttempt(ctx, owner, name, runID, attempt, nil)
		if err != nil {
			return nil, bounded.fail("read comparison workflow attempt", resp, err)
		}
		if r == nil || r.GetID() != runID || r.GetRunAttempt() != attempt || r.GetRepository().GetFullName() != repo || !validDelaySHA(r.GetHeadSHA()) || r.GetWorkflowID() <= 0 || !ValidExperimentAttemptContext(r.GetEvent(), r.GetPath()) {
			return nil, fmt.Errorf("github: comparison attempt has incomplete or inconsistent identity")
		}
		return r, nil
	}
	initial, err := read()
	if err != nil {
		return nil, err
	}
	stamp := func(t *gh.Timestamp) *time.Time {
		if t == nil || t.IsZero() {
			return nil
		}
		v := t.Time
		return &v
	}
	out := &ExperimentAttemptEvidence{Event: initial.GetEvent(), WorkflowPath: initial.GetPath(), Repository: repo, RunID: runID, Attempt: attempt, HeadSHA: initial.GetHeadSHA(), WorkflowID: initial.GetWorkflowID(), Status: initial.GetStatus(), Conclusion: initial.GetConclusion(), RunStartedAt: stamp(initial.RunStartedAt), UpdatedAt: stamp(initial.UpdatedAt), JobsCoverage: "partial", Jobs: []ExperimentAttemptJob{}}
	complete := initial.GetStatus() == "completed" && initial.GetConclusion() != "" && out.UpdatedAt != nil
	total := -1
	seen := map[int64]bool{}
	ended := false
	for page := 1; page <= 5; page++ {
		rows, resp, err := bounded.asInstallation.Actions.ListWorkflowJobsAttempt(ctx, owner, name, runID, int64(attempt), &gh.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return nil, bounded.fail("read comparison attempt jobs", resp, err)
		}
		if rows == nil || len(rows.Jobs) > 100 {
			return nil, fmt.Errorf("github: comparison job page is invalid")
		}
		if rows.TotalCount == nil || rows.GetTotalCount() < 0 {
			complete = false
		} else if total < 0 {
			total = rows.GetTotalCount()
		} else if total != rows.GetTotalCount() {
			complete = false
		}
		for _, j := range rows.Jobs {
			if j == nil || j.GetID() <= 0 || j.GetRunID() != runID || j.GetRunAttempt() != int64(attempt) || j.GetHeadSHA() != out.HeadSHA {
				return nil, fmt.Errorf("github: comparison job does not belong to the pinned run attempt")
			}
			if seen[j.GetID()] {
				complete = false
				continue
			}
			seen[j.GetID()] = true
			if j.GetStatus() != "completed" || j.GetConclusion() == "" {
				complete = false
			}
			out.Jobs = append(out.Jobs, ExperimentAttemptJob{ID: j.GetID(), Status: j.GetStatus(), Conclusion: j.GetConclusion(), StartedAt: stamp(j.StartedAt), CompletedAt: stamp(j.CompletedAt)})
		}
		if resp == nil {
			complete = false
			break
		}
		if resp.NextPage == 0 {
			ended = true
			break
		}
		// Sequential pages stay on the pinned endpoint, even if Link names a
		// different host. A jumping/backwards cursor does not establish coverage.
		if resp.NextPage != page+1 {
			complete = false
			break
		}
	}
	final, err := read()
	if err != nil {
		return nil, err
	}
	sameStamp := func(a, b *gh.Timestamp) bool {
		if a == nil || b == nil {
			return a == nil && b == nil
		}
		return a.Time.Equal(b.Time)
	}
	if final.GetEvent() != out.Event || final.GetPath() != out.WorkflowPath || final.GetHeadSHA() != out.HeadSHA || final.GetWorkflowID() != out.WorkflowID || final.GetStatus() != out.Status || final.GetConclusion() != out.Conclusion || !sameStamp(initial.UpdatedAt, final.UpdatedAt) || !sameStamp(initial.RunStartedAt, final.RunStartedAt) {
		return nil, ErrExperimentAttemptChanged
	}
	if complete && ended && total == len(out.Jobs) {
		out.JobsCoverage = "complete"
	}
	return out, nil
}

// ValidExperimentAttemptContext bounds optional, untrusted metadata before it is
// retained. A path is opaque: it is never resolved as a file, URL or source ref.
// Empty fields remain unknown, including in evidence captured before this slice.
func ValidExperimentAttemptContext(event, workflowPath string) bool {
	if len(event) > 64 || len(workflowPath) > 1024 || !utf8.ValidString(workflowPath) {
		return false
	}
	for _, c := range event {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	for _, c := range workflowPath {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}
