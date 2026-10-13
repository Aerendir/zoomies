package store

import (
	"context"
	"fmt"
	"strings"
)

const MaxExperimentAttemptJobs = 1000

// ListExperimentAttemptJobs reads retained fleet rows, not GitHub's complete
// workflow. Truncation is explicit so a partial successful subset cannot be
// mistaken for a complete attempt. No outcome is filtered from the cohort.
func (s *Store) ListExperimentAttemptJobs(ctx context.Context, repo, installation string, runID int64, attempt, limit int) ([]*Job, bool, error) {
	if strings.TrimSpace(repo) == "" || strings.TrimSpace(installation) == "" || runID <= 0 || attempt <= 0 || limit < 1 || limit > MaxExperimentAttemptJobs {
		return nil, false, fmt.Errorf("provide repository, installation, positive run/attempt identities and a job limit from 1 to %d", MaxExperimentAttemptJobs)
	}
	rows, err := s.read.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE repo = ? AND installation_id = ? AND github_run_id = ? AND run_attempt = ? ORDER BY queued_at, id LIMIT ?`, repo, installation, runID, attempt, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]*Job, 0)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	cut := len(out) > limit
	if cut {
		out = out[:limit]
	}
	return out, cut, nil
}
