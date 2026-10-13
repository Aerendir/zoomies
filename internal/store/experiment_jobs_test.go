package store

import (
	"context"
	"testing"
	"time"
)

func TestAnExperimentReadIsBoundedAndKeepsEveryOutcomeOfOneAttempt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	for i, outcome := range []string{"success", "failure", "cancelled", "skipped", ""} {
		j := &Job{GitHubJobID: int64(100 + i), GitHubRunID: 10, RunAttempt: 1, Repo: "acme/repo", InstallationID: "ins_one", QueuedAt: now.Add(time.Duration(i) * time.Second), State: JobCompleted, Conclusion: outcome}
		if outcome == "" {
			j.State = JobQueued
		}
		if _, err := s.UpsertJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	for i, j := range []*Job{{Repo: "other/repo", InstallationID: "ins_one", GitHubRunID: 10, RunAttempt: 1}, {Repo: "acme/repo", InstallationID: "ins_other", GitHubRunID: 10, RunAttempt: 1}, {Repo: "acme/repo", InstallationID: "ins_one", GitHubRunID: 11, RunAttempt: 1}, {Repo: "acme/repo", InstallationID: "ins_one", GitHubRunID: 10, RunAttempt: 2}} {
		j.GitHubJobID = int64(200 + i)
		j.QueuedAt = now
		j.State = JobQueued
		if _, err := s.UpsertJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	rows, cut, err := s.ListExperimentAttemptJobs(ctx, "acme/repo", "ins_one", 10, 1, 3)
	if err != nil || !cut || len(rows) != 3 {
		t.Fatalf("rows=%d cut=%v err=%v", len(rows), cut, err)
	}
	if rows[0].Conclusion != "success" || rows[1].Conclusion != "failure" || rows[2].Conclusion != "cancelled" {
		t.Fatal("outcomes filtered or order changed")
	}
	rows, cut, err = s.ListExperimentAttemptJobs(ctx, "acme/repo", "ins_one", 10, 1, 5)
	if err != nil || cut || len(rows) != 5 {
		t.Fatalf("rows=%d cut=%v err=%v", len(rows), cut, err)
	}
	rows, cut, err = s.ListExperimentAttemptJobs(ctx, "acme/repo", "ins_one", 10, 2, 5)
	if err != nil || cut || len(rows) != 1 || rows[0].RunAttempt != 2 {
		t.Fatalf("retry rows=%d cut=%v err=%v", len(rows), cut, err)
	}
}

func TestAnExperimentReadRefusesUnscopedOrUnboundedArguments(t *testing.T) {
	s := newTestStore(t)
	for _, v := range []struct {
		repo, inst     string
		run            int64
		attempt, limit int
	}{{"", "ins", 1, 1, 1}, {"repo", "", 1, 1, 1}, {"repo", "ins", 0, 1, 1}, {"repo", "ins", 1, 0, 1}, {"repo", "ins", 1, 1, 0}, {"repo", "ins", 1, 1, 1001}} {
		if _, _, err := s.ListExperimentAttemptJobs(context.Background(), v.repo, v.inst, v.run, v.attempt, v.limit); err == nil {
			t.Fatalf("accepted %+v", v)
		}
	}
	rows, cut, err := s.ListExperimentAttemptJobs(context.Background(), "absent/repo", "ins", 1, 1, 1)
	if err != nil || cut || len(rows) != 0 {
		t.Fatalf("empty rows=%d cut=%v err=%v", len(rows), cut, err)
	}
}
