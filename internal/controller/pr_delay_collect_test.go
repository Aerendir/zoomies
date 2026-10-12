package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

type prDelayReaderFixture struct {
	github.Client
	evidence *github.PRDelayEvidence
	err      error
	calls    int
	repo     string
	deadline bool
}

func (f *prDelayReaderFixture) ReadPRDelay(ctx context.Context, repo string, pull int) (*github.PRDelayEvidence, error) {
	f.calls++
	f.repo = repo
	_, f.deadline = ctx.Deadline()
	return f.evidence, f.err
}

func prDelayCollectorFixture(t *testing.T) (*harness, *store.Installation, *store.Job, *prDelayReaderFixture) {
	t.Helper()
	h := newHarness(t)
	inst := h.installationOn("acme/repo", store.TargetRepo)
	in := prDelayInputFixture()
	h.c.clock = func() time.Time { return in.ObservedAt }
	job := in.Jobs[0].Job
	job.ID = ""
	job.InstallationID = inst.ID
	job.State = store.JobCompleted
	stored, err := h.st.UpsertJob(h.ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	f := &prDelayReaderFixture{Client: h.gh.Client(inst.Target, inst.TargetType), evidence: in.Evidence}
	h.c.clients.entries[inst.ID] = &clientEntry{client: f, updatedAt: inst.UpdatedAt.Truncate(time.Millisecond)}
	return h, inst, stored, f
}

func TestPRDelayCollectorUsesTheSelectedJobsInstallationAndExplicitWindowBasis(t *testing.T) {
	h, inst, job, f := prDelayCollectorFixture(t)
	r, err := h.c.PRDelayForJob(h.ctx, job.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || f.repo != job.Repo || !f.deadline || r.InstallationID != inst.ID ||
		r.WindowBasis != "earliest_observed_job" || r.Result.State != "checks_passed" || r.Result.WindowStart != job.QueuedAt {
		t.Fatalf("report %+v, reader %+v", r, f)
	}
}

func TestPRDelayCollectorNeverBorrowsAMatchingJobFromAnotherInstallation(t *testing.T) {
	h, _, job, f := prDelayCollectorFixture(t)
	other := h.installationOn("other/repo", store.TargetRepo)
	foreign := *job
	foreign.ID = ""
	foreign.GitHubJobID = 40
	foreign.InstallationID = other.ID
	foreign.Repo = other.Target
	if _, err := h.st.UpsertJob(h.ctx, &foreign); err != nil {
		t.Fatal(err)
	}
	f.evidence.Checks[0].WorkflowJobID = 40
	r, err := h.c.PRDelayForJob(h.ctx, job.ID, 1)
	if err != nil || r.Result.State != "incomplete" {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestPRDelayCollectorKeepsExpiredJobHistoryIncomplete(t *testing.T) {
	h, _, job, f := prDelayCollectorFixture(t)
	f.evidence.Checks[0].WorkflowJobID = 999
	r, err := h.c.PRDelayForJob(h.ctx, job.ID, 1)
	if err != nil || r.Result.State != "incomplete" || r.Result.ObservationCoverage != "partial" {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestPRDelayCollectorDoesNotReadAnInvalidTargetOrHeldInstallation(t *testing.T) {
	h, inst, job, f := prDelayCollectorFixture(t)
	if _, err := h.c.PRDelayForJob(h.ctx, job.ID, 0); !errors.Is(err, ErrPRDelayEvidence) {
		t.Fatal(err)
	}
	if _, err := h.c.PRDelayForJob(h.ctx, "job_absent", 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	h.c.githubPaused = map[string]time.Time{inst.ID: h.c.Now().Add(time.Minute)}
	if _, err := h.c.PRDelayForJob(h.ctx, job.ID, 1); !errors.Is(err, github.ErrRateLimited) {
		t.Fatal(err)
	}
	if f.calls != 0 {
		t.Fatalf("spent %d reads", f.calls)
	}
}

func TestPRDelayCollectorRefusesANewerPRHeadAndPropagatesReadErrors(t *testing.T) {
	h, _, job, f := prDelayCollectorFixture(t)
	f.evidence.HeadSHA = "newer-head"
	if _, err := h.c.PRDelayForJob(h.ctx, job.ID, 1); !errors.Is(err, ErrPRDelayJobHeadChanged) {
		t.Fatal(err)
	}
	f.err = context.Canceled
	if _, err := h.c.PRDelayForJob(h.ctx, job.ID, 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPRDelayCollectorStopsAtItsEvidenceBound(t *testing.T) {
	h, _, job, f := prDelayCollectorFixture(t)
	f.evidence.Checks = make([]github.PRDelayCheck, maxPRDelayJobReads+1)
	if _, err := h.c.PRDelayForJob(h.ctx, job.ID, 1); !errors.Is(err, ErrPRDelayEvidence) {
		t.Fatal(err)
	}
}
