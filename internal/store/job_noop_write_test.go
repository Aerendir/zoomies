package store

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"
)

// GitHub delivers a job's events at least once, and the poller finds jobs the
// webhook already announced, so most of the deliveries a job gets after its
// first say nothing the row does not already hold. ApplyJob used to put the row
// back anyway, in a write transaction of its own: a third of a poller sweep over
// known jobs in memory, and on disk most of what a redelivery cost.
//
// These tests ask the database what it was given to write, not the store what it
// meant to write: a trigger on the jobs table counts the UPDATEs it receives.

// countJobUpdates puts a canary on the jobs table and returns a function that
// reads how many UPDATEs the table has received since the canary went in.
func countJobUpdates(t *testing.T, s *Store) func() int {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`CREATE TABLE job_update_canary (n INTEGER)`,
		`CREATE TRIGGER job_update_canary_trigger AFTER UPDATE ON jobs BEGIN INSERT INTO job_update_canary VALUES (1); END`,
	} {
		if _, err := s.exec(ctx, q); err != nil {
			t.Fatalf("setting up the canary on the jobs table: %v", err)
		}
	}
	return func() int {
		t.Helper()
		var n int
		if err := s.read.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_update_canary`).Scan(&n); err != nil {
			t.Fatalf("reading the canary on the jobs table: %v", err)
		}
		return n
	}
}

// jobWriteClock is a clock that moves a second on every reading, so that a
// moment the store stamps is never the one it stamped before.
func jobWriteClock() func() time.Time {
	now := time.Date(2025, 3, 4, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		now = now.Add(time.Second)
		return now
	}
}

func jobJSON(t *testing.T, j *Job) string {
	t.Helper()
	b, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("encoding a job: %v", err)
	}
	return string(b)
}

func TestADeliveryThatSaysNothingNewDoesNotWriteTheJob(t *testing.T) {
	cases := []struct {
		name  string
		first func() *Job
		again func() *Job
	}{
		{"the same queued delivery again",
			func() *Job { return benchDelivery(1, JobQueued) },
			func() *Job { return benchDelivery(1, JobQueued) }},
		{"the same in_progress delivery again",
			func() *Job { return benchDelivery(1, JobInProgress) },
			func() *Job { return benchDelivery(1, JobInProgress) }},
		{"the same completed delivery again, steps and all",
			func() *Job { return benchDelivery(1, JobCompleted) },
			func() *Job { return benchDelivery(1, JobCompleted) }},
		{"labels in another case and order, which normalise to the stored set",
			func() *Job { return benchDelivery(1, JobQueued) },
			func() *Job {
				j := benchDelivery(1, JobQueued)
				j.Labels = StringSlice{"zoomies", "X64", "LINUX", "Self-Hosted"}
				return j
			}},
		{"a thinner view of the same job, as the poller sees it",
			func() *Job { return benchDelivery(1, JobQueued) },
			func() *Job {
				j := benchDelivery(1, JobQueued)
				j.Labels, j.HTMLURL, j.HeadBranch, j.HeadSHA, j.RunAttempt = nil, "", "", "", 0
				return j
			}},
		{"a queued delivery that arrives after the job completed",
			func() *Job { return benchDelivery(1, JobCompleted) },
			func() *Job { return benchDelivery(1, JobQueued) }},
		{"an in_progress delivery that arrives after the job completed, naming the runner it already has",
			func() *Job {
				j := benchDelivery(1, JobCompleted)
				j.RunnerID, j.RunnerName = "run_bench", "zoomies-bench"
				return j
			},
			func() *Job { return benchDelivery(1, JobInProgress) }},
		{"an in_progress delivery with no steps, after one that had them",
			func() *Job { return benchDelivery(1, JobInProgress) },
			func() *Job {
				j := benchDelivery(1, JobInProgress)
				j.Steps = nil
				return j
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := newTestStoreAt(t, jobWriteClock())
			updates := countJobUpdates(t, s)

			first := tc.first()
			if _, _, err := s.ApplyJob(ctx, first); err != nil {
				t.Fatalf("the first delivery: %v", err)
			}
			stored, err := s.GetJobByGitHubID(ctx, 1)
			if err != nil {
				t.Fatalf("reading the job back: %v", err)
			}
			wrote := updates()

			got, change, err := s.ApplyJob(ctx, tc.again())
			if err != nil {
				t.Fatalf("the delivery that follows: %v", err)
			}

			if n := updates() - wrote; n != 0 {
				t.Errorf("the delivery ran %d UPDATEs of the jobs table; want none, it says nothing the row does not already hold", n)
			}
			// Not writing is not the same as not answering. The caller is told what
			// it always was: the job as it now stands, and that nothing moved.
			if change.Created || change.StateChanged || change.Claimed || change.RunnerLinked || change.PreviousState != stored.State {
				t.Errorf("change = %+v; want nothing moved, from %s", change, stored.State)
			}
			if a, b := jobJSON(t, got), jobJSON(t, stored); a != b {
				t.Errorf("the job returned is not the job stored\n got: %s\nwant: %s", a, b)
			}
			after, err := s.GetJobByGitHubID(ctx, 1)
			if err != nil {
				t.Fatalf("reading the job back: %v", err)
			}
			if a, b := jobJSON(t, after), jobJSON(t, stored); a != b {
				t.Errorf("the stored job moved though nothing was written\n got: %s\nwant: %s", a, b)
			}
		})
	}
}

// Skipping the write is only right when the write would have changed nothing.
// Each case here is a delivery that differs from the stored row in one column
// the UPDATE writes, and each must still reach the row. The list of columns is
// the UPDATE's, so a column that stopped being compared would show up as the
// case that names it failing.
func TestADeliveryThatChangesAColumnStillWritesIt(t *testing.T) {
	completedWith := func(edit func(*Job)) func() *Job {
		return func() *Job {
			j := benchDelivery(1, JobCompleted)
			edit(j)
			return j
		}
	}
	cases := []struct {
		name    string
		first   func() *Job
		prepare string // SQL run after the first delivery, for a row the store would not leave like that itself
		change  func(*Job)
		took    func(got, stored *Job) bool
	}{
		{"repository", nil, "",
			func(j *Job) { j.Repo = "acme/gadgets" },
			func(g, _ *Job) bool { return g.Repo == "acme/gadgets" }},
		{"workflow", nil, "",
			func(j *Job) { j.Workflow = "Release" },
			func(g, _ *Job) bool { return g.Workflow == "Release" }},
		{"job name", nil, "",
			func(j *Job) { j.JobName = "build (linux, arm64)" },
			func(g, _ *Job) bool { return g.JobName == "build (linux, arm64)" }},
		{"labels", nil, "",
			func(j *Job) { j.Labels = append(j.Labels, "gpu") },
			func(g, _ *Job) bool { return slices.Contains(g.Labels, "gpu") }},
		{"state", nil, "",
			func(j *Job) { j.State = JobInProgress },
			func(g, _ *Job) bool { return g.State == JobInProgress }},
		{"conclusion",
			completedWith(func(j *Job) { j.Conclusion = "" }), "",
			func(j *Job) { j.Conclusion = "failure" },
			func(g, _ *Job) bool { return g.Conclusion == "failure" }},
		{"installation", nil, "",
			func(j *Job) { j.InstallationID = "ins_other" },
			func(g, _ *Job) bool { return g.InstallationID == "ins_other" }},
		{"pool", nil, "",
			func(j *Job) { j.PoolID = "pool_other" },
			func(g, _ *Job) bool { return g.PoolID == "pool_other" }},
		{"runner id", nil, "",
			func(j *Job) { j.RunnerID = "run_other" },
			func(g, _ *Job) bool { return g.RunnerID == "run_other" }},
		{"runner name", nil, "",
			func(j *Job) { j.RunnerName = "zoomies-other" },
			func(g, _ *Job) bool { return g.RunnerName == "zoomies-other" }},
		{"url", nil, "",
			func(j *Job) { j.HTMLURL = "https://github.com/acme/widgets/actions/runs/1/job/2" },
			func(g, _ *Job) bool { return g.HTMLURL == "https://github.com/acme/widgets/actions/runs/1/job/2" }},
		{"start", nil, "",
			func(j *Job) { j.StartedAt = &benchStart },
			func(g, _ *Job) bool { return g.StartedAt != nil && g.StartedAt.Equal(benchStart) }},
		{"completion",
			completedWith(func(j *Job) { j.CompletedAt = nil }), "",
			func(j *Job) { j.CompletedAt = &benchEnd },
			func(g, _ *Job) bool { return g.CompletedAt != nil && g.CompletedAt.Equal(benchEnd) }},
		{"match, and the moment the job became eligible",
			func() *Job {
				j := benchDelivery(1, JobQueued)
				j.Matched, j.PoolID = false, ""
				return j
			}, "",
			func(j *Job) { j.Matched = true },
			func(g, _ *Job) bool { return g.Matched && g.EligibleAt != nil }},
		// A job held for review is matched but not yet eligible: the match is all
		// that changes, and it has to reach the row on its own.
		{"match alone, while the job is held for review",
			func() *Job {
				j := benchDelivery(1, JobWaiting)
				j.Matched, j.PoolID = false, ""
				return j
			}, "",
			func(j *Job) { j.Matched = true },
			func(g, _ *Job) bool { return g.Matched && g.EligibleAt == nil }},
		// A row from before the eligible_at column existed is matched and has no
		// moment for it. The next delivery stamps one, and that is the only change.
		{"the moment a job became eligible, alone",
			nil, `UPDATE jobs SET eligible_at = NULL`,
			func(j *Job) {},
			func(g, _ *Job) bool { return g.EligibleAt != nil }},
		{"branch", nil, "",
			func(j *Job) { j.HeadBranch = "release/1.2" },
			func(g, _ *Job) bool { return g.HeadBranch == "release/1.2" }},
		{"commit", nil, "",
			func(j *Job) { j.HeadSHA = "0000000000000000000000000000000000000000" },
			func(g, _ *Job) bool { return g.HeadSHA == "0000000000000000000000000000000000000000" }},
		{"attempt", nil, "",
			func(j *Job) { j.RunAttempt = 2 },
			func(g, _ *Job) bool { return g.RunAttempt == 2 }},
		{"run number", nil, "",
			func(j *Job) { j.RunNumber = 77 },
			func(g, _ *Job) bool { return g.RunNumber == 77 }},
		{"run", nil, "",
			func(j *Job) { j.GitHubRunID++ },
			func(g, _ *Job) bool { return g.GitHubRunID == benchDelivery(1, JobQueued).GitHubRunID+1 }},
		{"steps",
			func() *Job { return benchDelivery(1, JobInProgress) }, "",
			func(j *Job) { j.Steps = benchSteps(3) },
			func(g, _ *Job) bool { return len(g.Steps) > 2 && g.Steps[2].Status == "completed" }},
		{"runner fault",
			completedWith(func(j *Job) {}), "",
			func(j *Job) { j.RunnerFault = "the runner exited before the job finished" },
			func(g, _ *Job) bool { return g.RunnerFault == "the runner exited before the job finished" }},
		{"fault kind",
			completedWith(func(j *Job) {}), "",
			func(j *Job) { j.FaultKind = FaultOutOfMemory },
			func(g, _ *Job) bool { return g.FaultKind == FaultOutOfMemory }},
		{"the moment a held job joined the queue",
			func() *Job {
				j := benchDelivery(1, JobWaiting)
				j.Matched, j.PoolID = false, ""
				return j
			}, "",
			func(j *Job) { j.State, j.Matched, j.PoolID = JobQueued, false, "" },
			func(g, was *Job) bool { return g.State == JobQueued && g.QueuedAt.After(was.QueuedAt) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := newTestStoreAt(t, jobWriteClock())
			updates := countJobUpdates(t, s)

			first := benchDelivery(1, JobQueued)
			if tc.first != nil {
				first = tc.first()
			}
			if _, _, err := s.ApplyJob(ctx, first); err != nil {
				t.Fatalf("the first delivery: %v", err)
			}
			if tc.prepare != "" {
				if _, err := s.exec(ctx, tc.prepare); err != nil {
					t.Fatalf("preparing the row: %v", err)
				}
			}
			stored, err := s.GetJobByGitHubID(ctx, 1)
			if err != nil {
				t.Fatalf("reading the job back: %v", err)
			}
			wrote := updates()

			// The delivery repeats the stored job and changes the one thing the
			// case names, so that nothing else can be what makes it write.
			next := *stored
			next.Labels = slices.Clone(stored.Labels)
			next.Steps = slices.Clone(stored.Steps)
			tc.change(&next)
			if _, _, err := s.ApplyJob(ctx, &next); err != nil {
				t.Fatalf("the delivery that changes the %s: %v", tc.name, err)
			}

			if n := updates() - wrote; n != 1 {
				t.Errorf("a delivery that changes the %s ran %d UPDATEs of the jobs table; want 1", tc.name, n)
			}
			got, err := s.GetJobByGitHubID(ctx, 1)
			if err != nil {
				t.Fatalf("reading the job back: %v", err)
			}
			if !tc.took(got, stored) {
				t.Errorf("the stored job did not take the new %s: %s", tc.name, jobJSON(t, got))
			}
		})
	}
}

// A delivery that arrives late may only fill in what the row does not have yet,
// and when it does fill something in, that is a write like any other: the
// runner a completed job was never linked to is exactly the kind of gap a late
// in_progress delivery closes.
func TestAStaleDeliveryThatFillsAGapStillWritesIt(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreAt(t, jobWriteClock())
	updates := countJobUpdates(t, s)

	if _, _, err := s.ApplyJob(ctx, benchDelivery(1, JobCompleted)); err != nil {
		t.Fatal(err)
	}
	wrote := updates()

	got, change, err := s.ApplyJob(ctx, benchDelivery(1, JobInProgress))
	if err != nil {
		t.Fatal(err)
	}
	if n := updates() - wrote; n != 1 {
		t.Errorf("a late delivery that links the job's runner ran %d UPDATEs of the jobs table; want 1", n)
	}
	if got.State != JobCompleted || !change.RunnerLinked || got.RunnerID != "run_bench" {
		t.Errorf("got state %s, runner %q, change %+v; want the job still completed, now linked to run_bench", got.State, got.RunnerID, change)
	}
	stored, err := s.GetJobByGitHubID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RunnerID != "run_bench" || stored.RunnerName != "zoomies-bench" {
		t.Errorf("the stored job has runner %q (%q); want the one the late delivery named", stored.RunnerID, stored.RunnerName)
	}
}
