package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// GitHub sends a workflow_job delivery for every change to a job, at least
// three of them (queued, in_progress, completed) and more when one is repeated
// or the poller finds a job the webhook already announced. Every one lands in
// ApplyJob, which UpsertJob wraps, through a single connection behind a single
// mutex, so how long it takes is how many jobs a second the fleet can hear
// about. These benchmarks time it down each path a delivery can take.
//
// Each path runs against a store in memory and a store on disk. The gap
// between the two is the file (the write-ahead log and its checkpoints) rather
// than the code, and it is the number to read before deciding that a faster
// upsert would let the fleet hear about its jobs any sooner.
//
// A delivery is built inside the loop, not before it: the insert path writes
// the job's ID, queued time and normalised labels back into the Job it is
// handed, so reusing one would measure a different call after the first. The
// few allocations that costs are in every path's allocs/op.

var (
	benchStart = time.Date(2025, 3, 4, 12, 0, 0, 0, time.UTC)
	benchEnd   = benchStart.Add(90 * time.Second)

	// benchStepNames are the steps of a typical build job.
	benchStepNames = []string{
		"Set up job", "Checkout", "Set up Go", "Restore cache", "Download modules", "Build",
		"Vet", "Test", "Lint", "Save cache", "Post Checkout", "Complete job",
	}

	// benchLifecycle is the order a job's deliveries normally arrive in.
	benchLifecycle = [...]JobState{JobQueued, JobInProgress, JobCompleted}
)

// benchSteps is a job's steps with the first completed of them finished and
// the next one running, as GitHub reports them part way through a job.
func benchSteps(completed int) JobSteps {
	steps := make(JobSteps, len(benchStepNames))
	for i, name := range benchStepNames {
		step := JobStep{Number: i + 1, Name: name, Status: "queued"}
		switch {
		case i < completed:
			step.Status, step.Conclusion = "completed", "success"
			step.StartedAt, step.CompletedAt = &benchStart, &benchEnd
		case i == completed:
			step.Status, step.StartedAt = "in_progress", &benchStart
		}
		steps[i] = step
	}
	return steps
}

// benchDelivery is a workflow_job delivery as the webhook handler hands it to
// the store: what GitHub sent, with the installation and pool the controller
// resolved before the upsert.
func benchDelivery(id int64, state JobState) *Job {
	j := &Job{
		GitHubJobID: id, GitHubRunID: 9_000_000_000 + id/3,
		Repo: "acme/widgets", Workflow: "CI", JobName: "build (linux, x64)",
		Labels: StringSlice{"self-hosted", "Linux", "X64", "zoomies"}, State: state,
		InstallationID: "ins_acme", PoolID: "pool_linux", Matched: true,
		HTMLURL:    "https://github.com/acme/widgets/actions/runs/9000000000/job/12345678901",
		HeadBranch: "main", HeadSHA: "3f2a9c41d07be6a5c8e1f0b2d4a7963e58c1b0fd", RunAttempt: 1,
	}
	switch state {
	case JobInProgress:
		j.StartedAt = &benchStart
		j.RunnerID, j.RunnerName = "run_bench", "zoomies-bench"
		j.Steps = benchSteps(0)
	case JobCompleted:
		j.StartedAt, j.CompletedAt = &benchStart, &benchEnd
		j.Conclusion = "success"
		j.Steps = benchSteps(len(benchStepNames))
	}
	return j
}

// benchStore opens a store for one benchmark. On disk it is in a directory of
// the benchmark's own and has the pragmas Open gives every real database.
func benchStore(b *testing.B, onDisk bool) *Store {
	b.Helper()
	path := ":memory:"
	if onDisk {
		path = filepath.Join(b.TempDir(), "zoomies.db")
	}
	s, err := Open(context.Background(), Options{Path: path})
	if err != nil {
		b.Fatalf("Open: %v", err)
	}
	b.Cleanup(func() { s.Close() })
	return s
}

func BenchmarkApplyJob(b *testing.B) {
	for _, db := range []struct {
		name   string
		onDisk bool
	}{{"memory", false}, {"disk", true}} {
		b.Run(db.name, func(b *testing.B) {
			b.Run("insert", func(b *testing.B) { benchApplyInsert(b, benchStore(b, db.onDisk)) })
			b.Run("redelivery", func(b *testing.B) { benchApplyRedelivery(b, benchStore(b, db.onDisk)) })
			b.Run("stale", func(b *testing.B) { benchApplyStale(b, benchStore(b, db.onDisk)) })
			b.Run("lifecycle", func(b *testing.B) { benchApplyLifecycle(b, benchStore(b, db.onDisk)) })
		})
	}
}

// benchApplyInsert is the first delivery about a job: a read that finds
// nothing, an ID drawn and a row written to a table with a handful of indexes.
// It happens once per job and is the path that grows the table, which starts
// empty here and ends up as many rows as the run had iterations.
func benchApplyInsert(b *testing.B, s *Store) {
	ctx := context.Background()
	if _, change, err := s.ApplyJob(ctx, benchDelivery(1, JobQueued)); err != nil || !change.Created {
		b.Fatalf("a first delivery: created = %v, err = %v", change.Created, err)
	}

	b.ReportAllocs()
	for id := int64(2); b.Loop(); id++ {
		if _, _, err := s.ApplyJob(ctx, benchDelivery(id, JobQueued)); err != nil {
			b.Fatal(err)
		}
	}
}

// benchApplyRedelivery is the same state arriving again. GitHub delivers at
// least once, and the poller finds jobs the webhook already told the
// controller about, so this is routine rather than a corner. Nothing changes,
// but the row is still read, merged and rewritten.
func benchApplyRedelivery(b *testing.B, s *Store) {
	ctx := context.Background()
	if _, _, err := s.ApplyJob(ctx, benchDelivery(1, JobQueued)); err != nil {
		b.Fatal(err)
	}
	if _, change, err := s.ApplyJob(ctx, benchDelivery(1, JobQueued)); err != nil || change.Created || change.StateChanged {
		b.Fatalf("a redelivery: change = %+v, err = %v; want it to move nothing", change, err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := s.ApplyJob(ctx, benchDelivery(1, JobQueued)); err != nil {
			b.Fatal(err)
		}
	}
}

// benchApplyStale is a delivery that arrives after the job has moved on, a
// queued one after the completed: the merge that must not take the row
// backwards. The row is a finished job's, steps included, so it reads and
// rewrites more than the redelivery's row does, and part of the gap between
// the two is that.
func benchApplyStale(b *testing.B, s *Store) {
	ctx := context.Background()
	if _, _, err := s.ApplyJob(ctx, benchDelivery(1, JobCompleted)); err != nil {
		b.Fatal(err)
	}
	if got, change, err := s.ApplyJob(ctx, benchDelivery(1, JobQueued)); err != nil || change.StateChanged || got.State != JobCompleted {
		b.Fatalf("a stale delivery: change = %+v, err = %v; want the job left completed", change, err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := s.ApplyJob(ctx, benchDelivery(1, JobQueued)); err != nil {
			b.Fatal(err)
		}
	}
}

// benchApplyLifecycle is one job's whole history, queued then in_progress then
// completed, three deliveries to one op. It is what a fleet pays per job, so
// its reciprocal is the most jobs a second the one writer can hear about with
// nothing else asking it for anything.
func benchApplyLifecycle(b *testing.B, s *Store) {
	ctx := context.Background()
	var change JobChange
	for _, state := range benchLifecycle {
		var err error
		if _, change, err = s.ApplyJob(ctx, benchDelivery(1, state)); err != nil {
			b.Fatal(err)
		}
	}
	if change.PreviousState != JobInProgress || !change.StateChanged {
		b.Fatalf("a job's last delivery: change = %+v; want it moved on from in_progress", change)
	}

	b.ReportAllocs()
	for id := int64(2); b.Loop(); id++ {
		for _, state := range benchLifecycle {
			if _, _, err := s.ApplyJob(ctx, benchDelivery(id, state)); err != nil {
				b.Fatal(err)
			}
		}
	}
}
