package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// The attention view answers the question an operator arrives with (which
// failures have I not seen that are still standing), so it has to keep a
// failure only while all three remain true: it failed, nobody has acknowledged
// it, and no later attempt has replaced it. Each seeded job below is one way
// of failing one of those.
func TestAttentionKeepsOnlyTheFailuresNobodyHasDealtWith(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	s := newTestStoreAt(t, func() time.Time { return now })
	ctx := context.Background()
	started := now.Add(-time.Minute)
	done := now.Add(-30 * time.Second)
	old := now.Add(-8 * 24 * time.Hour)
	oldStart := old.Add(time.Second)
	oldDone := old.Add(time.Minute)

	seed := []*Job{
		{GitHubJobID: 1, GitHubRunID: 100, Repo: "acme/widgets", JobName: "recent-failure", State: JobCompleted, Conclusion: "failure", QueuedAt: now.Add(-2 * time.Hour), StartedAt: &started, CompletedAt: &done, RunAttempt: 1},
		{GitHubJobID: 2, GitHubRunID: 101, Repo: "acme/widgets", JobName: "recent-fleet", State: JobInProgress, QueuedAt: now.Add(-time.Hour), StartedAt: &started},
		{GitHubJobID: 3, GitHubRunID: 102, Repo: "acme/widgets", JobName: "known", State: JobCompleted, Conclusion: "failure", QueuedAt: now.Add(-3 * time.Hour), StartedAt: &started, CompletedAt: &done, RunAttempt: 1},
		{GitHubJobID: 4, GitHubRunID: 103, Repo: "acme/widgets", JobName: "fixed-by-rerun", State: JobCompleted, Conclusion: "failure", QueuedAt: now.Add(-5 * time.Hour), StartedAt: &started, CompletedAt: &done, RunAttempt: 1},
		{GitHubJobID: 5, GitHubRunID: 103, Repo: "acme/widgets", JobName: "fixed-by-rerun", State: JobCompleted, Conclusion: "success", QueuedAt: now.Add(-4 * time.Hour), StartedAt: &started, CompletedAt: &done, RunAttempt: 2},
		{GitHubJobID: 6, GitHubRunID: 104, Repo: "acme/widgets", JobName: "still-failing", State: JobCompleted, Conclusion: "failure", QueuedAt: now.Add(-6 * time.Hour), StartedAt: &started, CompletedAt: &done, RunAttempt: 1},
		{GitHubJobID: 7, GitHubRunID: 104, Repo: "acme/widgets", JobName: "still-failing", State: JobCompleted, Conclusion: "timed_out", QueuedAt: now.Add(-5 * time.Hour), StartedAt: &started, CompletedAt: &done, RunAttempt: 2},
		{GitHubJobID: 8, GitHubRunID: 105, Repo: "acme/widgets", JobName: "old", State: JobCompleted, Conclusion: "failure", QueuedAt: old, StartedAt: &oldStart, CompletedAt: &oldDone, RunAttempt: 1},
		{GitHubJobID: 9, GitHubRunID: 106, Repo: "acme/widgets", JobName: "cancelled", State: JobCompleted, Conclusion: "cancelled", QueuedAt: now.Add(-2 * time.Hour), StartedAt: &started, CompletedAt: &done, RunAttempt: 1},
		{GitHubJobID: 10, GitHubRunID: 107, Repo: "acme/widgets", JobName: "other-run", State: JobCompleted, Conclusion: "failure", QueuedAt: now.Add(-90 * time.Minute), StartedAt: &started, CompletedAt: &done, RunAttempt: 1},
	}
	for _, j := range seed {
		if _, err := s.UpsertJob(ctx, j); err != nil {
			t.Fatalf("seeding %s (job %d): %v", j.JobName, j.GitHubJobID, err)
		}
	}
	fleet, err := s.GetJobByGitHubID(ctx, 2)
	if err != nil {
		t.Fatalf("GetJobByGitHubID: %v", err)
	}
	if _, _, err := s.SetJobRunnerFault(ctx, fleet.ID, "runner exited with code 137", FaultOutOfMemory); err != nil {
		t.Fatalf("SetJobRunnerFault: %v", err)
	}
	known, err := s.GetJobByGitHubID(ctx, 3)
	if err != nil {
		t.Fatalf("GetJobByGitHubID: %v", err)
	}
	if _, _, err := s.AcknowledgeJob(ctx, known.ID, "alice"); err != nil {
		t.Fatalf("AcknowledgeJob: %v", err)
	}
	still, err := s.GetJobByGitHubID(ctx, 7)
	if err != nil {
		t.Fatalf("GetJobByGitHubID: %v", err)
	}
	if still.RunAttempt != 2 {
		t.Fatalf("the later attempt did not survive the upsert: %+v", still)
	}

	got, total, err := s.ListJobs(ctx, JobFilter{AttentionOnly: true}, Page{Sort: "queued_at", Desc: true})
	if err != nil {
		t.Fatalf("ListJobs attention: %v", err)
	}
	var names []string
	for _, j := range got {
		names = append(names, j.JobName)
	}
	want := []string{"recent-fleet", "other-run", "recent-failure", "still-failing"}
	if total != len(want) || len(names) != len(want) {
		t.Fatalf("attention listed %v (total %d), want %v", names, total, want)
	}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("attention[%d] = %q, want %q (order: newest queued first)", i, names[i], name)
		}
	}

	// The acknowledged job is still a failure and still in the failed view;
	// only the attention view has stopped asking. The superseded attempt is
	// in it too, because history is not what the attention view is for.
	if _, total, err := s.ListJobs(ctx, JobFilter{FailedOnly: true}, Page{}); err != nil || total != 8 {
		t.Fatalf("failed view total = %d (err %v), want 8", total, err)
	}
	if left, _, err := s.ListJobs(ctx, JobFilter{AttentionOnly: true, Repos: []string{"nope"}}, Page{}); err != nil || len(left) != 0 {
		t.Fatalf("attention for another repository returned %v (err %v), want none", left, err)
	}

	// The same question read at the run's level, which is what the Workflows
	// page's "Needs attention" view asks: a run is kept when any of its jobs
	// is one of these, and the run whose second attempt passed is not.
	runs, runTotal, err := s.ListWorkflowRuns(ctx, JobFilter{AttentionOnly: true}, Page{})
	if err != nil {
		t.Fatalf("ListWorkflowRuns attention: %v", err)
	}
	var runIDs []int64
	for _, r := range runs {
		runIDs = append(runIDs, r.GitHubRunID)
	}
	slices.Sort(runIDs)
	if runTotal != 4 || !slices.Equal(runIDs, []int64{100, 101, 104, 107}) {
		t.Fatalf("attention runs = %v (total %d), want 100, 101, 104 and 107", runIDs, runTotal)
	}
}

// An acknowledgement names the person who looked, and the first one wins: a
// second press of the button is not a second triage. Clearing it asks the
// question again, and a re-delivery of the same webhook must not silently
// erase either decision.
func TestAcknowledgingKeepsTheFirstNameAndSurvivesRedelivery(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	later := now.Add(time.Hour)
	clock := now
	s := newTestStoreAt(t, func() time.Time { return clock })
	ctx := context.Background()
	done := now.Add(-time.Minute)

	j, err := s.UpsertJob(ctx, &Job{GitHubJobID: 1, GitHubRunID: 10, Repo: "acme/widgets", JobName: "red", State: JobCompleted, Conclusion: "failure", QueuedAt: now.Add(-time.Hour), CompletedAt: &done})
	if err != nil {
		t.Fatalf("UpsertJob: %v", err)
	}

	got, recorded, err := s.AcknowledgeJob(ctx, j.ID, "alice")
	if err != nil || !recorded {
		t.Fatalf("AcknowledgeJob = %+v, %v, %v; want the first acknowledgement recorded", got, recorded, err)
	}
	if got.AcknowledgedAt == nil || !got.AcknowledgedAt.Equal(now) || got.AcknowledgedBy != "alice" {
		t.Fatalf("acknowledgement = %v by %q, want %v by alice", got.AcknowledgedAt, got.AcknowledgedBy, now)
	}

	clock = later
	got, recorded, err = s.AcknowledgeJob(ctx, j.ID, "bob")
	if err != nil || recorded {
		t.Fatalf("second AcknowledgeJob recorded = %v (err %v), want false", recorded, err)
	}
	if got.AcknowledgedBy != "alice" || !got.AcknowledgedAt.Equal(now) {
		t.Fatalf("a second acknowledgement replaced the first: %v by %q", got.AcknowledgedAt, got.AcknowledgedBy)
	}

	if _, _, err := s.ApplyJob(ctx, &Job{GitHubJobID: 1, State: JobCompleted, Conclusion: "failure", CompletedAt: &done}); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	got, err = s.GetJob(ctx, j.ID)
	if err != nil || got.AcknowledgedBy != "alice" {
		t.Fatalf("a redelivery erased the acknowledgement: %+v (err %v)", got, err)
	}

	got, recorded, err = s.ClearJobAcknowledgement(ctx, j.ID)
	if err != nil || !recorded || got.AcknowledgedAt != nil || got.AcknowledgedBy != "" {
		t.Fatalf("ClearJobAcknowledgement = %+v, %v, %v; want the acknowledgement gone", got, recorded, err)
	}
	if _, recorded, err := s.ClearJobAcknowledgement(ctx, j.ID); err != nil || recorded {
		t.Fatalf("clearing twice recorded = %v (err %v), want false", recorded, err)
	}

	if _, _, err := s.AcknowledgeJob(ctx, "job_missing", "alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("acknowledging a job that does not exist: %v, want ErrNotFound", err)
	}
}

// An empty name is what an instance with authentication off has, and it must
// not turn into a person called "": the drawer reads the timestamp alone.
func TestAcknowledgingWithoutANameStillRecordsTheMoment(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	j, err := s.UpsertJob(ctx, &Job{GitHubJobID: 1, State: JobCompleted, Conclusion: "failure"})
	if err != nil {
		t.Fatalf("UpsertJob: %v", err)
	}
	got, recorded, err := s.AcknowledgeJob(ctx, j.ID, "")
	if err != nil || !recorded {
		t.Fatalf("AcknowledgeJob = %v, %v; want recorded", recorded, err)
	}
	if got.AcknowledgedAt == nil {
		t.Fatal("an acknowledgement with no name lost its time")
	}
}
