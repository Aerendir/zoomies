package store

import (
	"context"
	"time"
)

type JobAttentionRun struct {
	Repo  string
	RunID int64
}

// JobAttentionRunsToCheck rotates through runs, including ones previously
// settled on GitHub: a closed PR can reopen and checks can be run again.
func (s *Store) JobAttentionRunsToCheck(ctx context.Context, limit int) ([]JobAttentionRun, error) {
	rows, err := s.read.QueryContext(ctx, jobAttentionRunsToCheckSQL(),
		ms(s.Now().Add(-AttentionWindow)), ms(s.Now().Add(-5*time.Minute)), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JobAttentionRun{}
	for rows.Next() {
		var r JobAttentionRun
		if err := rows.Scan(&r.Repo, &r.RunID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// jobAttentionRunsToCheckSQL is a function so that a test can ask SQLite how it
// reads it: the replacement test inside is the part that is expensive to get
// wrong, and a plan is only visible from the statement.
func jobAttentionRunsToCheckSQL() string {
	return `SELECT jobs.repo, jobs.github_run_id
		FROM jobs LEFT JOIN job_attention_runs a ON a.repo=jobs.repo AND a.run_id=jobs.github_run_id
		WHERE jobs.github_run_id != 0 AND jobs.acknowledged_at IS NULL
			AND jobs.queued_at >= ? AND ` + failedJobSQL() + ` AND ` + unreplacedJobSQL() + `
			AND COALESCE(a.attempted_at,0) < ?
		GROUP BY jobs.repo, jobs.github_run_id
		ORDER BY COALESCE(a.attempted_at,0), MAX(jobs.queued_at) DESC, jobs.repo, jobs.github_run_id LIMIT ?`
}

// RecordJobAttentionRun records only evidence, never an acknowledgement. On
// a failed read, retain the last known disposition and rotate to the next run.
func (s *Store) RecordJobAttentionRun(ctx context.Context, repo string, run int64, reason string, checked time.Time, known bool) error {
	_, err := s.exec(ctx, `INSERT INTO job_attention_runs(repo,run_id,reason,checked_at,attempted_at) VALUES(?,?,?,CASE WHEN ? THEN ? ELSE 0 END,?)
		ON CONFLICT(repo,run_id) DO UPDATE SET reason=CASE WHEN ? THEN excluded.reason ELSE job_attention_runs.reason END,
		checked_at=CASE WHEN ? THEN excluded.checked_at ELSE job_attention_runs.checked_at END,
		attempted_at=excluded.attempted_at WHERE excluded.attempted_at >= job_attention_runs.attempted_at`, repo, run, reason, known, ms(checked), ms(checked), known, known)
	return err
}

func (s *Store) PruneJobAttentionRuns(ctx context.Context) error {
	_, err := s.exec(ctx, `DELETE FROM job_attention_runs WHERE NOT EXISTS
		(SELECT 1 FROM jobs WHERE jobs.repo=job_attention_runs.repo AND jobs.github_run_id=job_attention_runs.run_id
		AND jobs.queued_at >= ?)`, ms(s.Now().Add(-AttentionWindow)))
	return err
}
