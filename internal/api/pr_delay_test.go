package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

type prDelayAPIFactory struct {
	base   *fakeFactory
	reader *prDelayAPIReader
}

func (f *prDelayAPIFactory) For(ctx context.Context, inst *store.Installation, pem []byte) (github.Client, error) {
	raw, err := f.base.For(ctx, inst, pem)
	if err != nil {
		return nil, err
	}
	f.reader.Client = raw
	return f.reader, nil
}

type prDelayAPIReader struct {
	github.Client
	evidence   *github.PRDelayEvidence
	calls      atomic.Int32
	quotaCalls atomic.Int32
	quota      int
}

func (f *prDelayAPIReader) ReadPRDelay(context.Context, string, int) (*github.PRDelayEvidence, error) {
	f.calls.Add(1)
	return f.evidence, nil
}
func (f *prDelayAPIReader) RateLimit(context.Context) (*github.RateLimit, error) {
	f.quotaCalls.Add(1)
	return &github.RateLimit{Limit: 5000, Remaining: f.quota, ResetAt: time.Now().Add(time.Hour)}, nil
}

func prDelayAPIHarness(t *testing.T) (*harness, *store.Job, *prDelayAPIReader) {
	t.Helper()
	h := newHarness(t)
	inst := h.installation()
	now := time.Now().UTC().Truncate(time.Second)
	started, done := now.Add(-2*time.Minute), now.Add(-time.Minute)
	job, err := h.st.UpsertJob(h.ctx, &store.Job{GitHubJobID: 30, GitHubRunID: 20, Repo: "acme/repo", InstallationID: inst.ID,
		HeadSHA: "head", RunAttempt: 1, JobName: "private check name", State: store.JobCompleted, Conclusion: "success",
		QueuedAt: now.Add(-3 * time.Minute), StartedAt: &started, CompletedAt: &done})
	if err != nil {
		t.Fatal(err)
	}
	f := &prDelayAPIReader{quota: 5000, evidence: &github.PRDelayEvidence{Number: 1, HeadSHA: "head", CurrentHeadSHA: "head",
		Open: true, StillOpen: true, TargetBranch: "release", CurrentTargetBranch: "release", ProtectionCoverage: "complete", ChecksCoverage: "complete",
		Protection: &github.KennelProtection{Required: []string{"private check name"}},
		Checks: []github.PRDelayCheck{{ID: 40, AppID: github.GitHubActionsAppID, Name: "private check name", HeadSHA: "head",
			WorkflowRunID: 20, WorkflowJobID: 30, Status: "completed", Conclusion: "success", CompletedAt: &done}}}}
	ctrl, err := controller.New(controller.Options{Store: h.st, Config: h.cfg, Key: h.key, Auth: h.ctrl.Auth(),
		GitHub: &prDelayAPIFactory{base: &fakeFactory{gh: h.gh}, reader: f}, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	h.ctrl = ctrl
	h.api.ctrl = ctrl
	return h, job, f
}

func TestPRDelayAPIRequiresJobReadScopeBeforeSpendingGitHubReads(t *testing.T) {
	h, job, f := prDelayAPIHarness(t)
	path := "/api/v1/jobs/" + job.ID + "/pr-delay?pull_number=1"
	h.do(request{method: http.MethodGet, path: path}).mustStatus(t, http.StatusUnauthorized, "anonymous PR diagnosis")
	wrong := h.token("wrong-scope", store.RoleViewer, "hosts:read")
	h.do(request{method: http.MethodGet, path: path, token: wrong}).mustStatus(t, http.StatusForbidden, "missing jobs.read")
	if f.calls.Load() != 0 || f.quotaCalls.Load() != 0 {
		t.Fatal("unauthorised request spent GitHub reads")
	}
	viewer := h.token("viewer", store.RoleViewer, "jobs:read")
	resp := h.do(request{method: http.MethodGet, path: path, token: viewer})
	resp.mustStatus(t, http.StatusOK, "viewer PR diagnosis")
	doc := loadSpec(t)
	assertShape(t, doc, "PRDelayReport", resp.body)
	var shapeParts struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(resp.body, &shapeParts); err != nil {
		t.Fatal(err)
	}
	assertShape(t, doc, "PRDelayResult", shapeParts.Result)
	var resultParts struct {
		Checks []json.RawMessage `json:"checks"`
	}
	if err := json.Unmarshal(shapeParts.Result, &resultParts); err != nil {
		t.Fatal(err)
	}
	assertShape(t, doc, "PRDelayCheck", resultParts.Checks[0])
	var checkParts struct {
		Timing json.RawMessage `json:"timing"`
	}
	if err := json.Unmarshal(resultParts.Checks[0], &checkParts); err != nil {
		t.Fatal(err)
	}
	assertShape(t, doc, "PRDelayTiming", checkParts.Timing)
	var report controller.PRDelayReport
	if err := json.Unmarshal(resp.body, &report); err != nil {
		t.Fatal(err)
	}
	if report.WindowBasis != "earliest_observed_job" || report.Result.State != "checks_passed" || resp.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("got %s", resp.body)
	}
	if strings.Contains(string(resp.body), "private check name") || !strings.Contains(string(resp.body), `"queue_ms":null`) {
		t.Fatalf("unsafe or misleading result: %s", resp.body)
	}
}

func TestPRDelayAPIRejectsInvalidAndDuplicatePRNumbersBeforeReads(t *testing.T) {
	h, job, f := prDelayAPIHarness(t)
	token := h.token("viewer", store.RoleViewer, "jobs:read")
	for _, query := range []string{"", "?pull_number=0", "?pull_number=abc", "?pull_number=1&pull_number=2"} {
		h.do(request{method: http.MethodGet, path: "/api/v1/jobs/" + job.ID + "/pr-delay" + query, token: token}).mustStatus(t, http.StatusBadRequest, "invalid PR number")
	}
	if f.calls.Load() != 0 {
		t.Fatal("invalid request spent GitHub reads")
	}
}

func TestPRDelayAPIHasAnInstallationBudgetAndRetryAfter(t *testing.T) {
	h, job, f := prDelayAPIHarness(t)
	token := h.token("viewer", store.RoleViewer, "jobs:read")
	path := fmt.Sprintf("/api/v1/jobs/%s/pr-delay?pull_number=1", job.ID)
	for i := 0; i < 20; i++ {
		h.do(request{method: http.MethodGet, path: path, token: token}).mustStatus(t, http.StatusOK, "admitted diagnostic")
	}
	resp := h.do(request{method: http.MethodGet, path: path, token: token})
	resp.mustStatus(t, http.StatusTooManyRequests, "exhausted diagnostic allowance")
	if resp.header.Get("Retry-After") == "" || f.calls.Load() != 20 {
		t.Fatalf("retry %q, calls %d", resp.header.Get("Retry-After"), f.calls.Load())
	}
}

func TestPRDelayAPIReservesQuotaAndRefusesAChangedHead(t *testing.T) {
	h, job, f := prDelayAPIHarness(t)
	token := h.token("viewer", store.RoleViewer, "jobs:read")
	path := "/api/v1/jobs/" + job.ID + "/pr-delay?pull_number=1"
	f.quota = 2500
	h.do(request{method: http.MethodGet, path: path, token: token}).mustStatus(t, http.StatusTooManyRequests, "reserved GitHub quota")
	if f.calls.Load() != 0 {
		t.Fatal("spent below-reserve reads")
	}
	f.quota = 5000
	f.evidence.HeadSHA = "new"
	h.do(request{method: http.MethodGet, path: path, token: token}).mustStatus(t, http.StatusConflict, "changed PR head")
}
