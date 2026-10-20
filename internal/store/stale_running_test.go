package store

import (
	"context"
	"testing"
	"time"
)

func TestStaleRunningJobsUseTheStartAndKeepOtherStates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	fallback := cutoff.Add(-30 * 24 * time.Hour)
	old, edge, fresh := cutoff.Add(-time.Second), cutoff, cutoff.Add(time.Second)
	for _, tc := range []struct {
		id     int64
		state  JobState
		queued time.Time
		start  *time.Time
	}{
		{1, JobInProgress, fallback, &old}, {2, JobInProgress, fallback, &edge},
		{3, JobInProgress, fallback, &fresh}, {4, JobQueued, fallback.Add(-time.Second), nil},
		{5, JobCompleted, fallback, &old}, {6, JobInProgress, fallback.Add(-time.Second), nil},
		{7, JobInProgress, fallback, nil}, {8, JobInProgress, fallback.Add(time.Second), nil},
	} {
		if _, _, err := s.ApplyJob(ctx, &Job{GitHubJobID: tc.id, State: tc.state, QueuedAt: tc.queued, StartedAt: tc.start}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListStaleRunningJobs(ctx, cutoff, fallback)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	for _, j := range got {
		ids[j.GitHubJobID] = true
	}
	if len(got) != 2 || !ids[1] || !ids[6] {
		t.Fatalf("stale jobs = %v, want only old started and old missing-start jobs", ids)
	}
}

// A delivery can finish a job between the sweep's read and its write. The
// presumption must never replace a result GitHub has already confirmed.
func TestAStaleCompletionDoesNotOverwriteAKnownOutcome(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ended := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	for i, conclusion := range []string{"success", "failure"} {
		id := int64(i + 20)
		if _, _, err := s.ApplyJob(ctx, &Job{GitHubJobID: id, State: JobCompleted, Conclusion: conclusion, CompletedAt: &ended}); err != nil {
			t.Fatal(err)
		}
		later := ended.Add(time.Hour)
		got, change, err := s.ApplyJob(ctx, &Job{GitHubJobID: id, State: JobCompleted, Conclusion: "stale", CompletedAt: &later}, JobCondition{State: JobInProgress})
		if err != nil {
			t.Fatal(err)
		}
		if change.StateChanged || got.Conclusion != conclusion || got.CompletedAt == nil || !got.CompletedAt.Equal(ended) {
			t.Fatalf("stale sweep replaced %s: %+v (%+v)", conclusion, got, change)
		}
		saved, err := s.GetJobByGitHubID(ctx, id)
		if err != nil || saved.Conclusion != conclusion || !saved.CompletedAt.Equal(ended) {
			t.Fatalf("stored completion = %+v (%v)", saved, err)
		}

		corrected, _, err := s.ApplyJob(ctx, &Job{GitHubJobID: id, State: JobCompleted, Conclusion: "cancelled", CompletedAt: &later})
		if err != nil || corrected.Conclusion != "cancelled" || !corrected.CompletedAt.Equal(later) {
			t.Fatalf("a real completion correction was refused: %+v (%v)", corrected, err)
		}
	}
}

func TestAStaleCompletionDoesNotRetireAJobWhoseStartWasJustLearned(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	if _, _, err := s.ApplyJob(ctx, &Job{GitHubJobID: 30, State: JobInProgress, QueuedAt: now.Add(-36 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	// The sweep selected this row with no start; a delivery fills it before
	// the sweep writes. That new evidence must keep the workload protected.
	if _, _, err := s.ApplyJob(ctx, &Job{GitHubJobID: 30, State: JobInProgress, StartedAt: &now}); err != nil {
		t.Fatal(err)
	}
	got, change, err := s.ApplyJob(ctx, &Job{GitHubJobID: 30, State: JobCompleted, Conclusion: "stale", CompletedAt: &now}, JobCondition{State: JobInProgress})
	if err != nil {
		t.Fatal(err)
	}
	if change.StateChanged || got.State != JobInProgress || got.CompletedAt != nil {
		t.Fatalf("newly learned start was ignored: %+v (%+v)", got, change)
	}
}
