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

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/kennel/workflow"
	"github.com/eyupio/zoomies/internal/store"
)

type comparisonAPIFactory struct {
	base   *fakeFactory
	reader *comparisonAPIReader
}

func (f *comparisonAPIFactory) For(ctx context.Context, inst *store.Installation, pem []byte) (github.Client, error) {
	raw, err := f.base.For(ctx, inst, pem)
	f.reader.Client = raw
	return f.reader, err
}

type comparisonAPIReader struct {
	workflowSource bool
	github.Client
	at                time.Time
	calls, quotaCalls atomic.Int32
	quota             int
	err               error
}

func (f *comparisonAPIReader) RateLimit(context.Context) (*github.RateLimit, error) {
	f.quotaCalls.Add(1)
	return &github.RateLimit{Limit: 5000, Remaining: f.quota, ResetAt: f.at.Add(time.Hour)}, nil
}
func (f *comparisonAPIReader) ReadExperimentAttempt(_ context.Context, repo string, run int64, attempt int) (*github.ExperimentAttemptEvidence, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	end := f.at.Add(time.Minute)
	out := &github.ExperimentAttemptEvidence{Repository: repo, RunID: run, Attempt: attempt, HeadSHA: strings.Repeat("a", 40), WorkflowID: 20, Status: "completed", Conclusion: "failure", RunStartedAt: &f.at, UpdatedAt: &end, JobsCoverage: "complete", Jobs: []github.ExperimentAttemptJob{{ID: run + 100, Status: "completed", Conclusion: "failure", StartedAt: &f.at, CompletedAt: &end}}}
	if f.workflowSource {
		out.Event = "workflow_dispatch"
		if run == 10 {
			out.Event = "push"
			out.WorkflowPath = ".github/workflows/private-root.yml"
			out.WorkflowSource = &github.ExperimentWorkflowSource{Version: "workflow-source-v1", Basis: "push_head_sha_root_file", CommitSHA: out.HeadSHA, Path: out.WorkflowPath, BlobSHA: strings.Repeat("b", 40), ContentSHA256: strings.Repeat("c", 64), Bytes: 100}
			data := []byte("jobs: {private_job: {steps: [{uses: private_owner/private_repo@main}]}}")
			out.WorkflowSource.Dependencies = workflow.InspectDependencies(data)
			out.WorkflowSource.ContentSHA256 = out.WorkflowSource.Dependencies.SourceSHA256
			out.WorkflowSource.Bytes = len(data)
		}
	}
	return out, nil
}
func comparisonAPIHarness(t *testing.T, opts ...func(*config.Config)) (*harness, string, *comparisonAPIReader, map[string]any) {
	t.Helper()
	h := newHarness(t, opts...)
	inst := h.installation()
	at := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	f := &comparisonAPIReader{at: at, quota: 5000}
	ctrl, err := controller.New(controller.Options{Store: h.st, Config: h.cfg, Key: h.key, Auth: h.ctrl.Auth(), GitHub: &comparisonAPIFactory{base: &fakeFactory{gh: h.gh}, reader: f}})
	if err != nil {
		t.Fatal(err)
	}
	h.ctrl = ctrl
	h.api.ctrl = ctrl
	body := map[string]any{"installation_id": inst.ID, "repository": "acme/repo", "kind": "runner", "hypothesis": "lower wait", "intended_delta": "larger runner", "since": at.Add(-time.Minute), "until": at.Add(time.Hour), "baseline": []map[string]any{{"run_id": 10, "attempt": 1}}, "candidate": []map[string]any{{"run_id": 11, "attempt": 1}}}
	return h, inst.ID, f, body
}
func TestComparisonAPICaptureAndReplayUseOneBudgetAndRetainedEvidence(t *testing.T) {
	h, inst, f, body := comparisonAPIHarness(t)
	token := h.token("operator", store.RoleOperator, "jobs:read", "comparisons:write")
	resp := h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", token: token, body: body})
	resp.mustStatus(t, 201, "capture")
	var saved controller.RetainedExperimentComparison
	if err := json.Unmarshal(resp.body, &saved); err != nil {
		t.Fatal(err)
	}
	if f.calls.Load() != 2 || f.quotaCalls.Load() != 1 || saved.Report.Snapshot.Candidate.SelectionComplete || saved.Report.Measurement.Eligibility.State != "inconclusive" || saved.Report.Snapshot.InitiatingUser == "" || resp.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected capture %s calls=%d quota=%d", resp.body, f.calls.Load(), f.quotaCalls.Load())
	}
	path := fmt.Sprintf("/api/v1/comparisons/%s?installation_id=%s&repo=acme/repo", saved.ID, inst)
	readerToken := h.token("reader", store.RoleViewer, "jobs:read")
	read := h.do(request{method: http.MethodGet, path: path, token: readerToken})
	read.mustStatus(t, 200, "replay")
	if string(read.body) != string(resp.body) || f.calls.Load() != 2 || f.quotaCalls.Load() != 1 || read.header.Get("Cache-Control") != "no-store" {
		t.Fatal("replay refreshed or changed evidence")
	}
	if auditCount(t, h, "comparison.evidence_retained") != 1 {
		t.Fatal("replay wrote audit or capture missed audit")
	}
	assertShape(t, loadSpec(t), "RetainedExperimentComparison", resp.body)
	h.do(request{method: http.MethodGet, path: strings.Replace(path, "repo=acme/repo", "repo=other/repo", 1), token: token}).mustStatus(t, 404, "wrong scope")
}
func TestComparisonAPIRefusesAnonymousWrongScopesAndVisitorsBeforeReads(t *testing.T) {
	h, _, f, body := comparisonAPIHarness(t)
	h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", body: body}).mustStatus(t, 401, "anonymous capture")
	token := h.token("wrong", store.RoleViewer, "hosts:read")
	h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", body: body, token: token}).mustStatus(t, 403, "wrong scope")
	readonly := h.token("read-only", store.RoleOperator, "jobs:read")
	h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", body: body, token: readonly}).mustStatus(t, 403, "read-only token cannot capture")
	if f.calls.Load() != 0 || f.quotaCalls.Load() != 0 {
		t.Fatal("unauthorised reads")
	}
	public, _, pf, pbody := comparisonAPIHarness(t, func(c *config.Config) { c.Security.PublicView = true })
	public.do(request{method: http.MethodPost, path: "/api/v1/comparisons", body: pbody}).mustStatus(t, 403, "visitor capture")
	public.do(request{method: http.MethodGet, path: "/api/v1/comparisons/cmp_missing?installation_id=ins_x&repo=acme/repo"}).mustStatus(t, 403, "visitor read")
	if pf.calls.Load() != 0 || pf.quotaCalls.Load() != 0 {
		t.Fatal("visitor reads")
	}
}
func TestComparisonAPIStrictInputAndQuotaRefusalsSpendNoEvidenceReads(t *testing.T) {
	h, _, f, original := comparisonAPIHarness(t)
	token := h.token("operator", store.RoleOperator, "jobs:read", "comparisons:write")
	for _, mode := range []string{"actor", "empty", "duplicate", "oversized"} {
		body := map[string]any{}
		for k, v := range original {
			body[k] = v
		}
		switch mode {
		case "actor":
			body["initiating_user"] = "forged"
		case "empty":
			body["hypothesis"] = ""
		case "duplicate":
			body["candidate"] = body["baseline"]
		case "oversized":
			body["hypothesis"] = strings.Repeat("x", 33<<10)
		}
		h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", token: token, body: body}).mustStatus(t, 400, mode)
	}
	if f.calls.Load() != 0 || f.quotaCalls.Load() != 0 {
		t.Fatal("invalid input spent reads")
	}
	f.quota = 2505
	resp := h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", token: token, body: original})
	resp.mustStatus(t, 429, "whole request quota reserve")
	if f.calls.Load() != 0 || resp.header.Get("Retry-After") == "" {
		t.Fatal("quota guard failed")
	}
}

func TestComparisonAPIRejectsDuplicateReadArgumentsAndIncompleteCaptureAuthority(t *testing.T) {
	h, inst, f, body := comparisonAPIHarness(t)
	readonly := h.token("read", store.RoleViewer, "jobs:read")
	for _, query := range []string{"", "?installation_id=" + inst + "&repo=acme/repo&repo=acme/repo", "?installation_id=" + inst + "&repo=acme/repo&refresh=true", "?installation_id=" + inst + "&repo=acme/repo&bad=%zz"} {
		h.do(request{method: http.MethodGet, path: "/api/v1/comparisons/cmp_missing" + query, token: readonly}).mustStatus(t, 400, "strict replay arguments")
	}
	writeOnly := h.token("write-only", store.RoleOperator, "comparisons:write")
	h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", token: writeOnly, body: body}).mustStatus(t, 403, "capture also needs job access")
	viewer := h.token("viewer-wide", store.RoleViewer, "*")
	h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", token: viewer, body: body}).mustStatus(t, 403, "scope cannot widen viewer role")
	if f.calls.Load() != 0 || f.quotaCalls.Load() != 0 {
		t.Fatal("invalid or unauthorised request spent reads")
	}
}

func TestComparisonAPICookieCaptureKeepsTheSameOriginGuard(t *testing.T) {
	h, _, f, body := comparisonAPIHarness(t)
	user, _ := h.user("comparison-operator", store.RoleOperator)
	cookie := h.session(user)
	h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", cookie: cookie, origin: "https://evil.example.com", body: body}).mustStatus(t, 403, "cross-origin capture")
	if f.calls.Load() != 0 || f.quotaCalls.Load() != 0 {
		t.Fatal("cross-origin request spent reads")
	}
	h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", cookie: cookie, body: body}).mustStatus(t, 201, "same-origin operator capture")
}

func TestComparisonAPIMetadataMovementReturnsAConflict(t *testing.T) {
	h, _, f, body := comparisonAPIHarness(t)
	f.err = github.ErrExperimentAttemptChanged
	token := h.token("operator", store.RoleOperator, "jobs:read", "comparisons:write")
	h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", token: token, body: body}).mustStatus(t, 409, "moving attempt")
	if f.calls.Load() != 1 || auditCount(t, h, "comparison.evidence_retained") != 0 {
		t.Fatal("moving attempt was retried or retained")
	}
}

func TestComparisonAPIProjectsRecordedGrantsWithoutPromotingEquivalenceOrRefreshingThem(t *testing.T) {
	h, inst, reader, body := comparisonAPIHarness(t)
	installation, err := h.st.GetInstallation(h.ctx, inst)
	if err != nil {
		t.Fatal(err)
	}
	pool := h.pool(installation, "runner-evidence")
	host := h.host("runner-evidence-host")
	runner := h.runner(pool, host, store.RunnerProvisioning)
	for _, run := range []int64{10, 11} {
		input := &store.Job{GitHubJobID: run + 100, GitHubRunID: run, RunAttempt: 1, Repo: "acme/repo", InstallationID: inst, HeadSHA: strings.Repeat("a", 40), QueuedAt: reader.at, State: store.JobCompleted, Conclusion: "failure"}
		if run == 10 {
			input.RunnerID, input.PoolID, input.HostID = runner.ID, pool.ID, host.ID
		}
		j, err := h.st.UpsertJob(h.ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.st.StampJobGranted(h.ctx, j.ID, 2, 8192, store.AllocationFromPool); err != nil {
			t.Fatal(err)
		}
	}
	cpu := 2.0
	if _, err := h.st.PutRunnerCreationObservation(h.ctx, inst, &store.RunnerCreationObservationData{Version: store.RunnerCreationObservationVersion, RunnerID: runner.ID, HostID: host.ID, PoolID: pool.ID, TaskID: "task_fixture", IssuedAt: reader.at, Issued: store.IssuedRunnerConfiguration{Backend: store.BackendDocker, CPUs: &cpu, DockerMode: store.DockerNone}, Reported: store.ReportedRunnerCreation{DigestCoverage: "unsupported"}}); err != nil {
		t.Fatal(err)
	}
	token := h.token("operator", store.RoleOperator, "jobs:read", "comparisons:write")
	captured := h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", token: token, body: body})
	captured.mustStatus(t, 201, "capture recorded grants")
	var saved controller.RetainedExperimentComparison
	if err := json.Unmarshal(captured.body, &saved); err != nil {
		t.Fatal(err)
	}
	row, err := h.st.GetComparisonEvidence(h.ctx, inst, "acme/repo", saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Baseline  *controller.ExperimentJoinedEvidence `json:"baseline"`
		Candidate *controller.ExperimentJoinedEvidence `json:"candidate"`
	}
	if err := json.Unmarshal(row.Payload, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, side := range []*controller.ExperimentJoinedEvidence{envelope.Baseline, envelope.Candidate} {
		if side == nil || len(side.Attempts) != 1 || len(side.Attempts[0].MatchedJobs) != 1 {
			t.Fatal("grant not joined to pinned job")
		}
		g := side.Attempts[0].MatchedJobs[0].RecordedGrant
		if g == nil || g.CPUs == nil || *g.CPUs != 2 || g.MemoryMB == nil || *g.MemoryMB != 8192 {
			t.Fatal("grant not durably retained")
		}
	}
	if saved.RunnerEvidence == nil || saved.RunnerEvidence.Baseline.RecordedGrants != 1 || saved.RunnerEvidence.Baseline.RunnerObservations != 1 || *saved.RunnerEvidence.Baseline.Jobs[0].RecordedGrant.CPUs != 2 || saved.Report.Snapshot.Baseline.ResourceID != "" || saved.Report.Measurement.Eligibility.State != "inconclusive" {
		t.Fatal("private grant promoted public equivalence")
	}
	spec := loadSpec(t)
	project := saved.RunnerEvidence
	for _, check := range []struct {
		name  string
		value any
	}{
		{"ExperimentRunnerEvidence", project}, {"ExperimentRunnerEvidenceArm", project.Baseline}, {"ExperimentRunnerEvidenceJob", project.Baseline.Jobs[0]}, {"ExperimentRecordedGrantView", project.Baseline.Jobs[0].RecordedGrant}, {"ExperimentRunnerObservationView", project.Baseline.Jobs[0].Observation}, {"ExperimentRunnerRequestView", project.Baseline.Jobs[0].Observation.Requested},
	} {
		payload, err := json.Marshal(check.value)
		if err != nil {
			t.Fatal(err)
		}
		assertShape(t, spec, check.name, payload)
	}
	if strings.Contains(string(captured.body), "task_fixture") || strings.Contains(string(captured.body), runner.ID) || strings.Contains(string(captured.body), host.ID) {
		t.Fatal("private runner identity leaked")
	}
	if err := h.st.DeleteRunner(h.ctx, runner.ID); err != nil {
		t.Fatal(err)
	}
	replay := h.do(request{method: http.MethodGet, path: fmt.Sprintf("/api/v1/comparisons/%s?installation_id=%s&repo=acme/repo", saved.ID, inst), token: token})
	replay.mustStatus(t, 200, "offline replay")
	if string(replay.body) != string(captured.body) || reader.calls.Load() != 2 || reader.quotaCalls.Load() != 1 {
		t.Fatal("replay changed evidence or collection budget")
	}
	assertShape(t, loadSpec(t), "RetainedExperimentComparison", captured.body)
}

func TestComparisonAPIProjectsRetainedWorkflowFactsWithoutReadingCurrentSource(t *testing.T) {
	h, inst, reader, body := comparisonAPIHarness(t)
	reader.workflowSource = true
	token := h.token("operator", store.RoleOperator, "jobs:read", "comparisons:write")
	capture := h.do(request{method: http.MethodPost, path: "/api/v1/comparisons", token: token, body: body})
	capture.mustStatus(t, 201, "capture workflow facts")
	var saved controller.RetainedExperimentComparison
	if err := json.Unmarshal(capture.body, &saved); err != nil {
		t.Fatal(err)
	}
	v := saved.WorkflowEvidence
	if v == nil || v.Baseline.RootSources != 1 || v.Candidate.RootSources != 0 || v.Candidate.Attempts[0].RootSource != nil || saved.Report.Snapshot.Baseline.WorkflowRevision != "" || saved.Report.Measurement.Eligibility.State != "inconclusive" {
		t.Fatalf("workflow facts promoted or lost: %+v", saved)
	}
	if strings.Contains(string(capture.body), "private-root") || strings.Contains(string(capture.body), `"workflow_path":`) {
		t.Fatal("private path leaked")
	}
	if v.Baseline.Attempts[0].RootSource.Dependencies == nil || *v.Baseline.Attempts[0].RootSource.Dependencies.Counts.Floating != 1 || strings.Contains(string(capture.body), "private_owner") {
		t.Fatal("dependency inspection lost or raw names leaked")
	}
	assertShape(t, loadSpec(t), "RetainedExperimentComparison", capture.body)
	reader.workflowSource = false
	viewer := h.token("viewer", store.RoleViewer, "jobs:read")
	replay := h.do(request{method: http.MethodGet, path: "/api/v1/comparisons/" + saved.ID + "?installation_id=" + inst + "&repo=acme%2Frepo", token: viewer})
	replay.mustStatus(t, 200, "replay workflow facts")
	if string(capture.body) != string(replay.body) || reader.calls.Load() != 2 || reader.quotaCalls.Load() != 1 {
		t.Fatal("replay enriched or recollected workflow facts")
	}
}
