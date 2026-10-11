package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/backend"
	"github.com/eyupio/zoomies/internal/store"
)

// The failure this was built for, end to end: a pool keeps /tmp in memory at
// 1 GB, a cold Go build fills it, the link fails with "no space left on
// device" and the runner exits cleanly after a failed job. GitHub records an
// ordinary failure, the runner's own output never carried the step's words,
// and the folder is empty again by the time anybody looks. The samples the
// agent took while the job ran have to be enough: the job comes out of it with
// the fullest each folder got, explained as a folder that ran out of room, and
// listed as a problem on its pool, because the pool's setting is the fix.
func TestAJobWhoseInMemoryFolderFilledIsRecordedExplainedAndReported(t *testing.T) {
	h := newHarness(t)
	_, pool, host := h.fleet()
	r := h.seedRunner(t, pool, host, store.RunnerBusy)
	started := h.c.Now().Add(-10 * time.Minute)
	job, err := h.st.UpsertJob(h.ctx, &store.Job{
		GitHubJobID: 5151, Repo: "eyupio/zoomies", Workflow: "CI", JobName: "Publish dev binaries",
		Labels: pool.Labels, State: store.JobInProgress, PoolID: pool.ID, Matched: true,
		RunnerID: r.ID, RunnerName: r.Name, QueuedAt: started, StartedAt: &started, HostID: host.ID,
		HTMLURL: "https://github.com/eyupio/zoomies/actions/runs/1/job/2",
	})
	if err != nil {
		t.Fatalf("UpsertJob: %v", err)
	}
	at := h.c.Now()
	for _, s := range []backend.Stats{
		{SampledAt: &at, MemoryBytes: 900 << 20, Scratch: []backend.ScratchUse{
			{Path: backend.RunnerWorkMount, UsedBytes: 300 << 20, SizeBytes: 4096 << 20},
			{Path: backend.RunnerTmpMount, UsedBytes: 400 << 20, SizeBytes: 1024 << 20}}},
		{SampledAt: &at, MemoryBytes: 1800 << 20, Scratch: []backend.ScratchUse{
			{Path: backend.RunnerWorkMount, UsedBytes: 310 << 20, SizeBytes: 4096 << 20},
			{Path: backend.RunnerTmpMount, UsedBytes: 980 << 20, SizeBytes: 1024 << 20}}},
		{SampledAt: &at, MemoryBytes: 600 << 20, Scratch: []backend.ScratchUse{
			{Path: backend.RunnerWorkMount, UsedBytes: 310 << 20, SizeBytes: 4096 << 20},
			{Path: backend.RunnerTmpMount, UsedBytes: 12 << 20, SizeBytes: 1024 << 20}}},
	} {
		if err := h.c.ReportRunners(h.ctx, host.ID, []agent.RunnerReport{{RunnerID: r.ID, Stats: s}}); err != nil {
			t.Fatalf("ReportRunners: %v", err)
		}
	}
	done := h.c.Now()
	if _, err := h.st.UpsertJob(h.ctx, &store.Job{GitHubJobID: 5151, State: store.JobCompleted, Conclusion: "failure",
		Repo: job.Repo, StartedAt: &started, CompletedAt: &done}); err != nil {
		t.Fatalf("completing the job: %v", err)
	}
	if err := h.c.ReportRunners(h.ctx, host.ID, []agent.RunnerReport{{
		RunnerID: r.ID, State: store.RunnerRemoved, Message: "ephemeral runner exited cleanly after its job",
	}}); err != nil {
		t.Fatalf("ReportRunners: %v", err)
	}

	got, err := h.st.GetJob(h.ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	fullest, fraction, ok := got.ScratchPeaks.Fullest()
	if !ok || fullest.Path != backend.RunnerTmpMount || fullest.PeakMB != 980 || fullest.SizeMB != 1024 || fraction < 0.95 {
		t.Fatalf("scratch peaks = %+v, want /tmp at 980 of 1024 MB as the fullest", got.ScratchPeaks)
	}

	why, err := h.c.ExplainJob(h.ctx, job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if why.Class != WhyScratchFull || why.ProblemCode != "jobs.scratch_full" || why.Confidence != WhyMedium {
		t.Fatalf("class %s (%s) code %q, want scratch-full at medium confidence with jobs.scratch_full", why.Class, why.Confidence, why.ProblemCode)
	}
	if !strings.Contains(why.Summary, "/tmp") || !strings.Contains(why.Summary, "1 GB") || !strings.Contains(why.Summary, "96%") {
		t.Fatalf("summary = %q, want the folder, its size and how full it got", why.Summary)
	}
	if !strings.Contains(why.Fix, "room") {
		t.Fatalf("fix = %q, want what gives the folder room", why.Fix)
	}
	if e := findEvidence(why.Evidence, "scratch_peak"); e.Value != "96" || e.Unit != "%" || !strings.Contains(e.Label, "/tmp") {
		t.Fatalf("evidence = %+v, want the fullest folder as a percentage", why.Evidence)
	}
	if len(why.NextSteps) == 0 || why.NextSteps[0].Kind != "change" || why.NextSteps[0].Link != "/pools/"+pool.ID {
		t.Fatalf("next_steps = %+v, want a change on the pool first", why.NextSteps)
	}

	problems, err := h.c.Problems(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found *Problem
	for i := range problems {
		if problems[i].Code == "jobs.scratch_full" {
			found = &problems[i]
		}
	}
	if found == nil || found.TargetKind != "pool" || found.TargetID != pool.ID || !strings.Contains(found.Detail, "/tmp") {
		t.Fatalf("problems = %+v, want jobs.scratch_full on the pool naming /tmp", problems)
	}
}

// A folder that merely had traffic is not a folder that filled. The threshold
// is nine tenths: a cold Go build that failed at the link was measured at 94%
// a sample before, and a job that used half of its folder failed for its own
// reasons.
func TestAFolderUsedButNotFilledLeavesTheWorkflowsFailureAlone(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	started, done := now.Add(-10*time.Minute), now.Add(-time.Minute)
	job := &store.Job{ID: "job_1", State: store.JobCompleted, Conclusion: "failure", QueuedAt: now.Add(-12 * time.Minute), StartedAt: &started, CompletedAt: &done, PoolID: "pool_1",
		ScratchPeaks: store.ScratchPeaks{{Path: backend.RunnerTmpMount, PeakMB: 512, SizeMB: 1024}}}
	if v := classify(whySnapshot{Job: job, Now: now}); v.Class != WhyWorkflowFailure || v.ProblemCode != "" {
		t.Fatalf("class %s code %q, want the workflow's own failure", v.Class, v.ProblemCode)
	}
	job.ScratchPeaks[0].PeakMB = 930
	if v := classify(whySnapshot{Job: job, Now: now}); v.Class != WhyScratchFull {
		t.Fatalf("class %s, want scratch-full at nine tenths", v.Class)
	}
	// A success with a full folder is a job that fitted, just: nothing to say.
	job.Conclusion = "success"
	if v := classify(whySnapshot{Job: job, Now: now}); v.Class != WhySucceeded {
		t.Fatalf("class %s, want succeeded", v.Class)
	}
}
