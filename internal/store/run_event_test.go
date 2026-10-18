package store

import (
	"context"
	"testing"
)

// workflow_job deliveries never say what started a run, so it arrives from a
// separate read of the run, and every job of that run has to carry it: the
// failure a person sees may be any one of them.
func TestTheRunEventIsKeptOnEveryJobOfTheRunAndOnNoOther(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	first := seedJob(t, s, 1, JobCompleted, "")
	second := seedJob(t, s, 2, JobCompleted, "")
	for _, j := range []*Job{first, second} {
		j.GitHubRunID, j.Repo = 70, "acme/site"
		if _, err := s.UpsertJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	other := seedJob(t, s, 3, JobCompleted, "")
	other.GitHubRunID, other.Repo = 71, "acme/site"
	if _, err := s.UpsertJob(ctx, other); err != nil {
		t.Fatal(err)
	}

	if err := s.SetRunEventForRun(ctx, "acme/site", 70, "push"); err != nil {
		t.Fatal(err)
	}
	for _, j := range []*Job{first, second} {
		got, err := s.GetJobByGitHubID(ctx, j.GitHubJobID)
		if err != nil || got.RunEvent != "push" {
			t.Errorf("job %d reads event %q (%v), want push", j.GitHubJobID, got.RunEvent, err)
		}
	}
	if got, _ := s.GetJobByGitHubID(ctx, 3); got.RunEvent != "" {
		t.Errorf("a job of another run reads event %q, want none", got.RunEvent)
	}

	// An empty answer must never erase what GitHub already said.
	if err := s.SetRunEventForRun(ctx, "acme/site", 70, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetJobByGitHubID(ctx, 1); got.RunEvent != "push" {
		t.Errorf("an empty event erased the stored one: %q", got.RunEvent)
	}
}
