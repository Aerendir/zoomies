package api

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/optimisation"
	"github.com/eyupio/zoomies/internal/prrepair"
	"github.com/eyupio/zoomies/internal/store"
)

const checkoutAPISecret = "private_workflow_name"

type checkoutAPIFactory struct {
	base   *fakeFactory
	reader *checkoutAPIReader
}

func (f *checkoutAPIFactory) For(ctx context.Context, inst *store.Installation, pem []byte) (github.Client, error) {
	raw, err := f.base.For(ctx, inst, pem)
	f.reader.Client = raw
	return f.reader, err
}

type checkoutAPIReader struct {
	github.Client
	evidence *github.CheckoutEvidence
	reads    atomic.Int32
	err      error
}

func (f *checkoutAPIReader) RateLimit(context.Context) (*github.RateLimit, error) {
	return &github.RateLimit{Limit: 5000, Remaining: 5000, ResetAt: time.Now().Add(time.Hour)}, nil
}
func (f *checkoutAPIReader) ReadCheckoutEvidence(context.Context, string, github.CheckoutSelection) (*github.CheckoutEvidence, error) {
	f.reads.Add(1)
	return f.evidence, f.err
}

func checkoutAPIEvidence(repo string, sel github.CheckoutSelection) *github.CheckoutEvidence {
	hash := func(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
	source := optimisation.Source{HeadSHA: strings.Repeat("a", 40), File: prrepair.File{Path: ".github/workflows/ci.yml", Mode: "100644", Content: "on: push\njobs:\n  build:\n    name: Build\n    runs-on: ubuntu-latest\n    steps:\n      - name: Checkout\n        uses: actions/checkout@" + strings.Repeat("b", 40) + "\n        with:\n          fetch-depth: 0\n      - name: Say\n        run: echo " + checkoutAPISecret + "\n"}}
	blob := sha1.Sum([]byte("blob " + strconv.Itoa(len(source.File.Content)) + "\x00" + source.File.Content))
	ms := int64(4200)
	return &github.CheckoutEvidence{Version: "checkout-evidence-v1", Basis: "push_source_unique_literal_names", Repository: repo, Selection: sel, StepNumber: 42, BlobSHA: hex.EncodeToString(blob[:]), JobNameSHA256: hash("Build"), StepNameSHA256: hash("Checkout"), Source: source, Timing: optimisation.Timing{EvidenceID: fmt.Sprintf("checkout:%d:%d:%d:42", sel.RunID, sel.Attempt, sel.GitHubJobID), HeadSHA: source.HeadSHA, SourceSHA256: hash(source.File.Content), RunID: strconv.FormatInt(sel.RunID, 10), Attempt: sel.Attempt, JobIndex: sel.JobIndex, StepIndex: sel.StepIndex, Outcome: "success", CheckoutMS: &ms}}
}

func checkoutAPIHarness(t *testing.T, enabled bool, opts ...func(*config.Config)) (*harness, string, *checkoutAPIReader, map[string]any) {
	t.Helper()
	opts = append(opts, func(c *config.Config) {
		c.Optimisation.SourceReads = enabled
		c.Optimisation.WorkflowEdits = enabled
	})
	h := newHarness(t, opts...)
	inst := h.installation()
	sel := github.CheckoutSelection{RunID: 10, Attempt: 2, GitHubJobID: 100}
	f := &checkoutAPIReader{evidence: checkoutAPIEvidence("acme/repo", sel)}
	ctrl, err := controller.New(controller.Options{Store: h.st, Config: h.cfg, Key: h.key, Auth: h.ctrl.Auth(), GitHub: &checkoutAPIFactory{base: &fakeFactory{gh: h.gh}, reader: f}})
	if err != nil {
		t.Fatal(err)
	}
	h.ctrl = ctrl
	h.api.ctrl = ctrl
	body := map[string]any{"installation_id": inst.ID, "repository": "acme/repo", "selection": map[string]any{"run_id": "10", "attempt": 2, "github_job_id": "100", "job_index": 0, "step_index": 0}}
	return h, inst.ID, f, body
}

func previewBody(inspection CheckoutInspectionView, body map[string]any) map[string]any {
	out := map[string]any{"history_reviewed": true, "head_sha": inspection.HeadSHA, "source_sha256": inspection.SourceSHA256}
	for k, v := range body {
		out[k] = v
	}
	return out
}

func TestCheckoutOptimisationInspectCaptureAndReplayKeepTheSourceToTheInspection(t *testing.T) {
	h, instID, f, body := checkoutAPIHarness(t, true)
	token := h.token("operator", store.RoleOperator, "jobs:read", "optimisations:write", "optimisations:read")
	resp := h.do(request{method: http.MethodPost, path: "/api/v1/optimisations/checkout/inspections", token: token, body: body})
	resp.mustStatus(t, 200, "inspect")
	var inspection CheckoutInspectionView
	if err := json.Unmarshal(resp.body, &inspection); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inspection.Content, checkoutAPISecret) || inspection.CheckoutMS != 4200 || resp.header.Get("Cache-Control") != "no-store" || f.reads.Load() != 1 {
		t.Fatalf("inspection %s", resp.body)
	}
	assertShape(t, loadSpec(t), "CheckoutInspection", resp.body)

	created := h.do(request{method: http.MethodPost, path: "/api/v1/optimisations/checkout/previews", token: token, body: previewBody(inspection, body)})
	created.mustStatus(t, 201, "capture")
	var saved CheckoutPreviewView
	if err := json.Unmarshal(created.body, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Preview.Before != "          fetch-depth: 0\n" || saved.Preview.After != "          fetch-depth: 1\n" || saved.Preview.Verification != "inconclusive" || saved.Currentness != "not_checked" || saved.ReviewedBy == "" || strings.Contains(string(created.body), checkoutAPISecret) || f.reads.Load() != 2 {
		t.Fatalf("capture %s", created.body)
	}
	assertShape(t, loadSpec(t), "CheckoutPreview", created.body)

	path := fmt.Sprintf("/api/v1/optimisations/checkout/previews/%s?installation_id=%s&repo=acme/repo", saved.ID, instID)
	read := h.do(request{method: http.MethodGet, path: path, token: token})
	read.mustStatus(t, 200, "replay")
	var replayed CheckoutPreviewView
	if err := json.Unmarshal(read.body, &replayed); err != nil {
		t.Fatal(err)
	}
	if replayed != saved && (replayed.Preview != saved.Preview || replayed.ID != saved.ID || replayed.SHA256 != saved.SHA256) {
		t.Fatalf("replay changed the preview: %s", read.body)
	}
	if f.reads.Load() != 2 || strings.Contains(string(read.body), checkoutAPISecret) {
		t.Fatal("replay read GitHub again or exposed the workflow file")
	}
	if auditCount(t, h, "optimisation.preview_retained") != 1 {
		t.Fatal("capture was not audited exactly once")
	}
	h.do(request{method: http.MethodGet, path: strings.Replace(path, "repo=acme/repo", "repo=other/repo", 1), token: token}).mustStatus(t, 404, "wrong scope")
}

func TestCheckoutOptimisationIsRefusedUntilTheSettingsAreOnWithoutReadingSource(t *testing.T) {
	h, _, f, body := checkoutAPIHarness(t, false)
	token := h.token("operator", store.RoleOperator, "jobs:read", "optimisations:write")
	resp := h.do(request{method: http.MethodPost, path: "/api/v1/optimisations/checkout/inspections", token: token, body: body})
	resp.mustStatus(t, 403, "inspect while off")
	if !strings.Contains(string(resp.body), "optimisation.source_reads") || f.reads.Load() != 0 {
		t.Fatalf("refusal must name the setting and read nothing: %s", resp.body)
	}
	pb := map[string]any{"history_reviewed": true, "head_sha": strings.Repeat("a", 40), "source_sha256": strings.Repeat("c", 64)}
	for k, v := range body {
		pb[k] = v
	}
	h.do(request{method: http.MethodPost, path: "/api/v1/optimisations/checkout/previews", token: token, body: pb}).mustStatus(t, 403, "capture while off")
	if f.reads.Load() != 0 {
		t.Fatal("a refused capture read source")
	}
}

func TestCheckoutOptimisationNeedsOperatorScopesAnIdentityAndNoVisitors(t *testing.T) {
	h, _, f, body := checkoutAPIHarness(t, true)
	path := "/api/v1/optimisations/checkout/inspections"
	h.do(request{method: http.MethodPost, path: path, body: body}).mustStatus(t, 401, "anonymous")
	h.do(request{method: http.MethodPost, path: path, token: h.token("viewer", store.RoleViewer, "jobs:read", "optimisations:write"), body: body}).mustStatus(t, 403, "viewer")
	h.do(request{method: http.MethodPost, path: path, token: h.token("no-scope", store.RoleOperator, "jobs:read"), body: body}).mustStatus(t, 403, "read-only token")
	h.do(request{method: http.MethodPost, path: path, token: h.token("no-jobs", store.RoleOperator, "optimisations:write"), body: body}).mustStatus(t, 403, "no jobs:read")
	if f.reads.Load() != 0 {
		t.Fatal("an unauthorised caller read source")
	}
	public, _, pf, pbody := checkoutAPIHarness(t, true, func(c *config.Config) { c.Security.PublicView = true })
	public.do(request{method: http.MethodPost, path: path, body: pbody}).mustStatus(t, 403, "visitor inspect")
	public.do(request{method: http.MethodGet, path: "/api/v1/optimisations/checkout/previews/opv_x?installation_id=a&repo=b"}).mustStatus(t, 403, "visitor replay")
	if pf.reads.Load() != 0 {
		t.Fatal("a visitor read source")
	}
}

func TestCheckoutOptimisationRefusesAnUnreviewedMovedOrMalformedRequest(t *testing.T) {
	h, _, f, body := checkoutAPIHarness(t, true)
	token := h.token("operator", store.RoleOperator, "jobs:read", "optimisations:write")
	inspect := "/api/v1/optimisations/checkout/inspections"
	capture := "/api/v1/optimisations/checkout/previews"
	with := func(sel map[string]any) map[string]any {
		out := map[string]any{"installation_id": body["installation_id"], "repository": body["repository"], "selection": sel}
		return out
	}
	badSelections := []map[string]any{
		{"run_id": 10, "attempt": 2, "github_job_id": "100", "job_index": 0, "step_index": 0},
		{"run_id": "010", "attempt": 2, "github_job_id": "100", "job_index": 0, "step_index": 0},
		{"run_id": "10", "attempt": 0, "github_job_id": "100", "job_index": 0, "step_index": 0},
		{"run_id": "10", "attempt": 2, "github_job_id": "9223372036854775808", "job_index": 0, "step_index": 0},
		{"run_id": "10", "attempt": 2, "github_job_id": "100", "job_index": -1, "step_index": 0},
	}
	for i, sel := range badSelections {
		h.do(request{method: http.MethodPost, path: inspect, token: token, body: with(sel)}).mustStatus(t, 400, fmt.Sprintf("selection %d", i))
	}
	forged := with(body["selection"].(map[string]any))
	forged["policy"] = map[string]any{"source_allowed": true}
	h.do(request{method: http.MethodPost, path: inspect, token: token, body: forged}).mustStatus(t, 400, "forged policy field")
	if f.reads.Load() != 0 {
		t.Fatal("invalid input spent reads")
	}
	pb := func(reviewed bool, sha string) map[string]any {
		out := map[string]any{"history_reviewed": reviewed, "head_sha": strings.Repeat("a", 40), "source_sha256": sha}
		for k, v := range body {
			out[k] = v
		}
		return out
	}
	h.do(request{method: http.MethodPost, path: capture, token: token, body: pb(false, strings.Repeat("c", 64))}).mustStatus(t, 400, "history not confirmed")
	if f.reads.Load() != 0 {
		t.Fatal("an unconfirmed review spent reads")
	}
	// A digest that is not the one the server reads is the source having moved.
	h.do(request{method: http.MethodPost, path: capture, token: token, body: pb(true, strings.Repeat("c", 64))}).mustStatus(t, 409, "stale digest")
	if auditCount(t, h, "optimisation.preview_retained") != 0 {
		t.Fatal("a refused capture was retained")
	}
}

func TestCheckoutOptimisationQuotaAndReadFailuresSayWhatToDo(t *testing.T) {
	h, _, f, body := checkoutAPIHarness(t, true)
	token := h.token("operator", store.RoleOperator, "jobs:read", "optimisations:write")
	f.err = github.ErrForbidden
	resp := h.do(request{method: http.MethodPost, path: "/api/v1/optimisations/checkout/inspections", token: token, body: body})
	resp.mustStatus(t, 403, "App lacks contents")
	if !strings.Contains(string(resp.body), "Contents") {
		t.Fatalf("a permission failure must name the permission: %s", resp.body)
	}
}

func TestOptimisationEnablementIsOnePressForAnAdministratorAndAudited(t *testing.T) {
	h, _, f, body := checkoutAPIHarness(t, false)
	admin := h.token("admin", store.RoleAdmin, "*")
	state := func(token string) OptimisationEnablement {
		resp := h.do(request{method: http.MethodGet, path: "/api/v1/optimisations/enablement", token: token})
		resp.mustStatus(t, 200, "state")
		var out OptimisationEnablement
		if err := json.Unmarshal(resp.body, &out); err != nil {
			t.Fatal(err)
		}
		assertShape(t, loadSpec(t), "OptimisationEnablement", resp.body)
		return out
	}
	if s := state(admin); s.Enabled || !s.CanChange {
		t.Fatalf("a fresh instance is off and an administrator can change it: %+v", s)
	}
	op := h.token("operator", store.RoleOperator, "jobs:read", "optimisations:read", "optimisations:write")
	if s := state(op); s.CanChange || !strings.Contains(s.Reason, "administrator") {
		t.Fatalf("an operator is told whom to ask: %+v", s)
	}
	// The operator is refused until the administrator presses the button, and
	// then the very next request goes through with no restart.
	h.do(request{method: http.MethodPost, path: "/api/v1/optimisations/checkout/inspections", token: op, body: body}).mustStatus(t, 403, "off")
	h.do(request{method: http.MethodPut, path: "/api/v1/optimisations/enablement", token: op, body: map[string]any{"enabled": true}}).mustStatus(t, 403, "operator cannot enable")
	put := h.do(request{method: http.MethodPut, path: "/api/v1/optimisations/enablement", token: admin, body: map[string]any{"enabled": true}})
	put.mustStatus(t, 200, "enable")
	var after OptimisationEnablement
	if err := json.Unmarshal(put.body, &after); err != nil || !after.Enabled || !after.SourceReads || !after.WorkflowEdits {
		t.Fatalf("both switches must come on together: %s", put.body)
	}
	h.do(request{method: http.MethodPost, path: "/api/v1/optimisations/checkout/inspections", token: op, body: body}).mustStatus(t, 200, "on")
	if f.reads.Load() != 1 {
		t.Fatal("the enabled instance did not read source")
	}
	if auditCount(t, h, "settings.update") != 1 {
		t.Fatal("turning the switches on must leave one settings audit row")
	}
	h.do(request{method: http.MethodPut, path: "/api/v1/optimisations/enablement", token: admin, body: map[string]any{"enabled": false}}).mustStatus(t, 200, "disable")
	h.do(request{method: http.MethodPost, path: "/api/v1/optimisations/checkout/inspections", token: op, body: body}).mustStatus(t, 403, "off again")
	h.do(request{method: http.MethodPut, path: "/api/v1/optimisations/enablement", token: admin, body: map[string]any{}}).mustStatus(t, 400, "missing enabled")
	h.do(request{method: http.MethodPut, path: "/api/v1/optimisations/enablement", token: admin, body: map[string]any{"enabled": true, "extra": 1}}).mustStatus(t, 400, "unknown field")
}

func TestOptimisationEnablementRefusesVisitorsAndAnonymous(t *testing.T) {
	h, _, _, _ := checkoutAPIHarness(t, false, func(c *config.Config) { c.Security.PublicView = true })
	h.do(request{method: http.MethodGet, path: "/api/v1/optimisations/enablement"}).mustStatus(t, 403, "visitor read")
	h.do(request{method: http.MethodPut, path: "/api/v1/optimisations/enablement", body: map[string]any{"enabled": true}}).mustStatus(t, 403, "visitor write")
}

func TestCheckoutOptimisationFindsTheStepWhenNoPositionsAreGiven(t *testing.T) {
	h, _, _, body := checkoutAPIHarness(t, true)
	token := h.token("operator", store.RoleOperator, "jobs:read", "optimisations:write")
	located := map[string]any{"installation_id": body["installation_id"], "repository": body["repository"], "selection": map[string]any{"run_id": "10", "attempt": 2, "github_job_id": "100"}}
	resp := h.do(request{method: http.MethodPost, path: "/api/v1/optimisations/checkout/inspections", token: token, body: located})
	resp.mustStatus(t, 200, "located")
	var got CheckoutInspectionView
	if err := json.Unmarshal(resp.body, &got); err != nil {
		t.Fatal(err)
	}
	if got.JobName != "Build" || got.StepName != "Checkout" || got.Selection.JobIndex == nil || got.Selection.StepIndex == nil {
		t.Fatalf("a person should see names and the positions used: %s", resp.body)
	}
	assertShape(t, loadSpec(t), "CheckoutInspection", resp.body)
	half := map[string]any{"installation_id": body["installation_id"], "repository": body["repository"], "selection": map[string]any{"run_id": "10", "attempt": 2, "github_job_id": "100", "job_index": 0}}
	h.do(request{method: http.MethodPost, path: "/api/v1/optimisations/checkout/inspections", token: token, body: half}).mustStatus(t, 400, "one position without the other")
}
