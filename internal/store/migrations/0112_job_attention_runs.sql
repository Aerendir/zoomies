-- GitHub can close a PR or settle checks without changing the retained job.
-- Keep that observation separate from a person's acknowledgement and the
-- historical conclusion. New failures after the observation remain visible.
CREATE TABLE job_attention_runs (
    repo TEXT NOT NULL,
    run_id INTEGER NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    checked_at INTEGER NOT NULL,
    attempted_at INTEGER NOT NULL,
    PRIMARY KEY (repo, run_id)
);

CREATE INDEX idx_jobs_attention_branch ON jobs(repo, head_branch, workflow, job_name, queued_at);
