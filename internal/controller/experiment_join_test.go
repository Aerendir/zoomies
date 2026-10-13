package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

type experimentReaderFixture struct {
	github.Client
	evidence          *github.ExperimentAttemptEvidence
	quota             *github.RateLimit
	err               error
	calls, quotaCalls int
	deadline          bool
	dynamicIdentity   bool
}

func (f *experimentReaderFixture) RateLimit(context.Context) (*github.RateLimit, error) {
	f.quotaCalls++
	return f.quota, nil
}
func (f *experimentReaderFixture) ReadExperimentAttempt(ctx context.Context, repo string, run int64, attempt int) (*github.ExperimentAttemptEvidence, error) {
	f.calls++
	_, f.deadline = ctx.Deadline()
	if f.dynamicIdentity {
		copy := *f.evidence
		copy.RunID = run
		copy.Attempt = attempt
		copy.Jobs = []github.ExperimentAttemptJob{{ID: run + 100, Status: "completed", Conclusion: "failure"}}
		return &copy, f.err
	}
	return f.evidence, f.err
}
func experimentJoinFixture(t *testing.T) (*harness, *store.Installation, *experimentReaderFixture) {
	t.Helper()
	h := newHarness(t)
	inst := h.installationOn("acme/repo", store.TargetRepo)
	now := time.Date(2026, 10, 13, 1, 0, 0, 0, time.UTC)
	h.c.clock = func() time.Time { return now }
	_, err := h.st.UpsertJob(h.ctx, &store.Job{GitHubJobID: 100, GitHubRunID: 10, RunAttempt: 2, Repo: inst.Target, InstallationID: inst.ID, HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", QueuedAt: now, State: store.JobCompleted, Conclusion: "failure"})
	if err != nil {
		t.Fatal(err)
	}
	f := &experimentReaderFixture{Client: h.gh.Client(inst.Target, inst.TargetType), quota: &github.RateLimit{Limit: 5000, Remaining: 5000, ResetAt: now.Add(time.Hour)}, evidence: &github.ExperimentAttemptEvidence{Repository: inst.Target, RunID: 10, Attempt: 2, HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", WorkflowID: 20, Status: "completed", Conclusion: "failure", JobsCoverage: "complete", UpdatedAt: &now, Jobs: []github.ExperimentAttemptJob{{ID: 100, Status: "completed", Conclusion: "failure"}}}}
	h.c.clients.entries[inst.ID] = &clientEntry{client: f, updatedAt: inst.UpdatedAt.Truncate(time.Millisecond)}
	return h, inst, f
}

func TestAnAdmittedAttemptJoinKeepsWorkflowAndFleetIdentityCoverageSeparate(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	r, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if r.SelectionComplete || f.calls != 1 || f.quotaCalls != 1 || !f.deadline || len(r.Attempts) != 1 || r.Attempts[0].FleetIdentityCoverage != "complete" || len(r.Attempts[0].MatchedJobs) != 1 || r.Attempts[0].Remote.Conclusion != "failure" {
		t.Fatalf("got %+v, reader %+v", r, f)
	}
}
func TestAnAttemptJoinNeverBorrowsTelemetryFromAnUnpinnedHead(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	f.evidence.HeadSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	r, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}})
	if err != nil || r.Attempts[0].FleetIdentityCoverage != "partial" || len(r.Attempts[0].MatchedJobs) != 0 || len(r.Attempts[0].MissingJobIDs) != 1 {
		t.Fatalf("got %+v, %v", r, err)
	}
	f.evidence.Jobs = append(f.evidence.Jobs, github.ExperimentAttemptJob{ID: 101, Status: "completed", Conclusion: "cancelled"})
	r, err = h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}})
	if err != nil || len(r.Attempts[0].MissingJobIDs) != 2 {
		t.Fatalf("got %+v, %v", r, err)
	}
}
func TestAnAttemptCollectionReservesQuotaAndReleasesItsFlightOnErrors(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	selected := []ExperimentAttemptSelection{{10, 2}}
	f.quota = &github.RateLimit{Limit: 100, Remaining: 56, ResetAt: h.c.Now().Add(time.Hour)}
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, selected); !errors.Is(err, ErrExperimentAdmission) || f.calls != 0 {
		t.Fatalf("reads=%d err=%v", f.calls, err)
	}
	f.quota.Remaining = 100
	f.err = context.Canceled
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, selected); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	f.err = nil
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, selected); err != nil {
		t.Fatal(err)
	}
	release, err := h.c.admitPRDelay(inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, selected); !errors.Is(err, ErrExperimentAdmission) {
		t.Fatal(err)
	}
	release()
}
func TestAnAttemptCollectionRefusesHoldsBoundsAndForeignTargetsBeforeRemoteReads(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	for _, tc := range []struct {
		repo     string
		selected []ExperimentAttemptSelection
	}{{"other/repo", []ExperimentAttemptSelection{{10, 2}}}, {inst.Target, make([]ExperimentAttemptSelection, 11)}, {inst.Target, []ExperimentAttemptSelection{{10, 2}, {10, 2}}}} {
		if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, tc.repo, tc.selected); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
	h.c.githubPaused = map[string]time.Time{inst.ID: h.c.Now().Add(time.Minute)}
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}}); !errors.Is(err, github.ErrRateLimited) {
		t.Fatal(err)
	}
	if f.calls != 0 || f.quotaCalls != 0 {
		t.Fatalf("spent reads %+v", f)
	}
}

func TestAttemptAdmissionSharesThePRDiagnosticHourlyBudget(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	for range prDelayReadsPerHour {
		release, err := h.c.admitPRDelay(inst.ID)
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}}); !errors.Is(err, ErrExperimentAdmission) {
		t.Fatal(err)
	}
	if f.calls != 0 || f.quotaCalls != 0 {
		t.Fatalf("spent reads %+v", f)
	}
}
func TestAttemptJoinsKeepExtraFleetRowsAndRemotePartialCoverageVisible(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	_, err := h.st.UpsertJob(h.ctx, &store.Job{GitHubJobID: 101, GitHubRunID: 10, RunAttempt: 2, Repo: inst.Target, InstallationID: inst.ID, HeadSHA: f.evidence.HeadSHA, QueuedAt: h.c.Now(), State: store.JobCompleted, Conclusion: "cancelled"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}})
	if err != nil || r.Attempts[0].FleetIdentityCoverage != "partial" || len(r.Attempts[0].ExtraFleetJobIDs) != 1 {
		t.Fatalf("got %+v, %v", r, err)
	}
	f.evidence.JobsCoverage = "partial"
	r, err = h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}})
	if err != nil || r.Attempts[0].FleetIdentityCoverage != "partial" {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestAttemptCollectionsRejectMalformedQuotaAndReserveEverySelectedAttempt(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	for _, quota := range []*github.RateLimit{nil, {Limit: 0, Remaining: 100}, {Limit: 100, Remaining: -1}, {Limit: 100, Remaining: 101}} {
		f.quota = quota
		if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}}); !errors.Is(err, ErrExperimentAdmission) {
			t.Fatalf("quota %+v: %v", quota, err)
		}
	}
	f.quota = &github.RateLimit{Limit: 100, Remaining: 60}
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}, {11, 1}}); !errors.Is(err, ErrExperimentAdmission) {
		t.Fatal(err)
	}
	if f.calls != 0 {
		t.Fatalf("spent %d evidence reads", f.calls)
	}
}
func TestAttemptCollectionsRefuseInvalidReaderEvidenceAndReleaseTheGate(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	original := *f.evidence
	changes := []func(*github.ExperimentAttemptEvidence){func(e *github.ExperimentAttemptEvidence) { e.Repository = "other/repo" }, func(e *github.ExperimentAttemptEvidence) { e.RunID = 11 }, func(e *github.ExperimentAttemptEvidence) { e.Attempt = 1 }, func(e *github.ExperimentAttemptEvidence) { e.HeadSHA = "unknown" }, func(e *github.ExperimentAttemptEvidence) { e.JobsCoverage = "trusted" }, func(e *github.ExperimentAttemptEvidence) { e.UpdatedAt = nil }, func(e *github.ExperimentAttemptEvidence) { e.Jobs = append(e.Jobs, e.Jobs[0]) }}
	for _, change := range changes {
		copy := original
		change(&copy)
		f.evidence = &copy
		if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}}); !errors.Is(err, ErrExperimentEvidence) {
			t.Fatalf("accepted %+v, %v", copy, err)
		}
	}
	f.evidence = &original
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}}); err != nil {
		t.Fatal(err)
	}
}
func TestAttemptReadRateLimitsEstablishAnInstallationHold(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	f.err = github.ErrRateLimited
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}}); !errors.Is(err, github.ErrRateLimited) {
		t.Fatal(err)
	}
	before := f.quotaCalls
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}}); !errors.Is(err, github.ErrRateLimited) || f.quotaCalls != before || f.calls != 1 {
		t.Fatalf("hold ignored: %+v, %v", f, err)
	}
}

func TestAttemptCollectionsBoundATenAttemptBatchToOneQuotaRead(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	f.dynamicIdentity = true
	selected := make([]ExperimentAttemptSelection, 10)
	for i := range selected {
		selected[i] = ExperimentAttemptSelection{RunID: int64(i + 10), Attempt: 2}
	}
	r, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, selected)
	if err != nil || len(r.Attempts) != 10 || r.SelectionComplete || f.calls != 10 || f.quotaCalls != 1 {
		t.Fatalf("got %+v, reader %+v, %v", r, f, err)
	}
}

type experimentUnsupportedFixture struct {
	github.Client
	quotaCalls int
}

func (f *experimentUnsupportedFixture) RateLimit(context.Context) (*github.RateLimit, error) {
	f.quotaCalls++
	return nil, nil
}
func TestAttemptCollectionsNeverFallbackWhenTheReaderIsUnsupported(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	unsupported := &experimentUnsupportedFixture{Client: f.Client}
	h.c.clients.entries[inst.ID] = &clientEntry{client: unsupported, updatedAt: inst.UpdatedAt.Truncate(time.Millisecond)}
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}}); !errors.Is(err, ErrExperimentUnsupported) {
		t.Fatal(err)
	}
	if unsupported.quotaCalls != 0 || f.calls != 0 {
		t.Fatal("unsupported reader spent remote reads")
	}
}
