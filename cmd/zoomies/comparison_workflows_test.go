package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func cliWorkflowFixture() *cliWorkflowEvidence {
	return &cliWorkflowEvidence{Version: "workflow-evidence-v1", Baseline: &cliWorkflowArm{SelectedAttempts: cliRunnerPointer(2), RootSources: cliRunnerPointer(1), RowsOmitted: cliRunnerPointer(true), Attempts: []cliWorkflowAttempt{{RunID: "9223372036854775807", Attempt: 1, Event: cliRunnerPointer("push"), WorkflowPathSHA256: cliRunnerPointer(strings.Repeat("d", 64)), RootSource: &cliRootSource{Basis: "push_head_sha_root_file", CommitSHA: strings.Repeat("a", 40), BlobSHA: strings.Repeat("b", 40), ContentSHA256: strings.Repeat("c", 64), Bytes: 128}}}}, Candidate: &cliWorkflowArm{SelectedAttempts: cliRunnerPointer(1), RootSources: cliRunnerPointer(0), RowsOmitted: cliRunnerPointer(false), Attempts: []cliWorkflowAttempt{{RunID: "11", Attempt: 1}}}}
}
func cliWorkflowBody(t *testing.T, e *cliWorkflowEvidence) string {
	t.Helper()
	var doc map[string]any
	d := json.NewDecoder(strings.NewReader(cliComparison))
	d.UseNumber()
	if err := d.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	s := doc["report"].(map[string]any)["snapshot"].(map[string]any)
	s["baseline"] = map[string]any{"attempts": []any{map[string]any{"run_id": json.Number("9223372036854775807"), "attempt": 1, "source_revision": strings.Repeat("a", 40)}, map[string]any{"run_id": 10, "attempt": 1}}}
	s["candidate"] = map[string]any{"attempts": []any{map[string]any{"run_id": 11, "attempt": 1}}}
	doc["workflow_evidence"] = e
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func cliWorkflowRequest(t *testing.T, body string, capture, structured bool) (int, string, int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if capture && r.Method != "POST" || !capture && r.Method != "GET" {
			t.Error("changed request method")
		}
		w.Header().Set("Content-Type", "application/json")
		if capture {
			w.WriteHeader(201)
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	e, out, _ := newTestEnv(t)
	args := []string{"comparisons", "get", "cmp_1", "--installation", "ins_1", "--repo", "acme/repo", "--url", srv.URL}
	if capture {
		e.in = strings.NewReader(cliCaptureSelection)
		args = []string{"comparisons", "capture", "--file", "-", "--url", srv.URL}
	}
	if structured {
		args = append(args, "--output", "json")
	}
	code := dispatch(context.Background(), e, args)
	return code, out.String(), calls.Load()
}

func TestComparisonWorkflowFactsAppearForCaptureAndReplayWithOneRequest(t *testing.T) {
	for _, capture := range []bool{false, true} {
		code, out, calls := cliWorkflowRequest(t, cliWorkflowBody(t, cliWorkflowFixture()), capture, false)
		if code != 0 || calls != 1 {
			t.Fatalf("failed or repeated request: %d %d", code, calls)
		}
		for _, v := range []string{"9223372036854775807", "2 selected attempts", "1 retained root sources", "showing 1 attempts", "additional workflow rows were omitted", "push", "root file only", "root content SHA-256", strings.Repeat("c", 64), "128", "not attested execution", "equivalent workloads", "unknown", "inconclusive", "0ms"} {
			if !strings.Contains(out, v) {
				t.Errorf("missing %q: %s", v, out)
			}
		}
	}
}
func TestComparisonWorkflowJSONPreservesFutureViewsAndExactNumbers(t *testing.T) {
	e := cliWorkflowFixture()
	e.Version = "future"
	body := cliWorkflowBody(t, e)
	code, out, calls := cliWorkflowRequest(t, body, false, true)
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(out)); err != nil {
		t.Fatal(err)
	}
	if code != 0 || calls != 1 || compact.String() != body || !strings.Contains(out, "9007199254740993") || !strings.Contains(out, "9223372036854775807") {
		t.Fatal("future/raw fields or exact numbers changed")
	}
}
func TestComparisonWorkflowHumanOutputRefusesMalformedFactsBeforePrinting(t *testing.T) {
	for _, mode := range []string{"version", "count", "flag", "rows", "roots", "duplicate", "run", "canonical", "event", "path", "basis", "commit", "blob", "content", "bytes", "nonpush", "no_path"} {
		e := cliWorkflowFixture()
		a := e.Baseline
		r := &a.Attempts[0]
		switch mode {
		case "version":
			e.Version = "future"
		case "count":
			*a.SelectedAttempts = 3
		case "flag":
			a.RowsOmitted = nil
		case "rows":
			a.Attempts = nil
		case "roots":
			*a.RootSources = 0
		case "duplicate":
			a.Attempts = append(a.Attempts, *r)
			*a.RowsOmitted = false
			*a.RootSources = 2
		case "run":
			r.RunID = "99"
		case "canonical":
			r.RunID = "01"
		case "event":
			r.Event = cliRunnerPointer("push\\nSYSTEM")
		case "path":
			r.WorkflowPathSHA256 = cliRunnerPointer("private")
		case "basis":
			r.RootSource.Basis = "executed_workflow"
		case "commit":
			r.RootSource.CommitSHA = strings.Repeat("b", 40)
		case "blob":
			r.RootSource.BlobSHA = "unknown"
		case "content":
			r.RootSource.ContentSHA256 = "unknown"
		case "bytes":
			r.RootSource.Bytes = 131073
		case "nonpush":
			r.Event = cliRunnerPointer("pull_request")
		case "no_path":
			r.WorkflowPathSHA256 = nil
		}
		code, out, calls := cliWorkflowRequest(t, cliWorkflowBody(t, e), false, false)
		if code == 0 || out != "" || calls != 1 {
			t.Fatalf("%s printed bad facts or repeated reads: %d %q %d", mode, code, out, calls)
		}
	}
}
func TestComparisonWorkflowHumanOutputExcludesFutureAndPrivateTerminalText(t *testing.T) {
	var doc map[string]any
	d := json.NewDecoder(strings.NewReader(cliWorkflowBody(t, cliWorkflowFixture())))
	d.UseNumber()
	if err := d.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	e := doc["workflow_evidence"].(map[string]any)
	a := e["baseline"].(map[string]any)
	r := a["attempts"].([]any)[0].(map[string]any)
	for _, v := range []map[string]any{e, a, r, r["root_source"].(map[string]any)} {
		v["source_text"] = "\x1b[31mprivate-script"
		v["workflow_path"] = "private-root.yml"
		v["future_secret"] = "private-secret"
	}
	body, _ := json.Marshal(doc)
	code, out, calls := cliWorkflowRequest(t, string(body), false, false)
	if code != 0 || calls != 1 || strings.Contains(out, "private-") || strings.Contains(out, "source_text") || strings.Contains(out, "future_secret") {
		t.Fatal("private or unreviewed terminal text leaked")
	}
}
func TestComparisonWorkflowLegacyViewsRemainExplicitlyUnknown(t *testing.T) {
	code, out, calls := cliWorkflowRequest(t, cliComparison, false, false)
	if code != 0 || calls != 1 || !strings.Contains(out, "Workflow evidence is unavailable from this controller; missing facts remain unknown.") {
		t.Fatal("invented legacy facts")
	}
}

func TestCLIInspectsDependencyCountsAndRejectsMissingFactsBeforePrinting(t *testing.T) {
	var doc map[string]any
	decoder := json.NewDecoder(strings.NewReader(cliWorkflowBody(t, cliWorkflowFixture())))
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	source := doc["workflow_evidence"].(map[string]any)["baseline"].(map[string]any)["attempts"].([]any)[0].(map[string]any)["root_source"].(map[string]any)
	counters := map[string]any{"declarations": 0, "commit_pins": 0, "digest_pins": 0, "floating": 0, "local": 0, "expressions": 0, "unknown": 0}
	source["dependencies"] = map[string]any{"version": "root-dependencies-v1", "basis": "root_uses_declarations", "source_sha256": strings.Repeat("c", 64), "coverage": "enumerated", "counts": counters, "rows_omitted": false, "references": []any{}}
	raw, _ := json.Marshal(doc)
	code, out, calls := cliWorkflowRequest(t, string(raw), false, false)
	if code != 0 || calls != 1 || !strings.Contains(out, "root declarations") || !strings.Contains(out, "Literal pins") && !strings.Contains(out, "literal pins") {
		t.Fatal("known zero dependency inventory not shown")
	}
	delete(counters, "local")
	raw, _ = json.Marshal(doc)
	code, out, calls = cliWorkflowRequest(t, string(raw), false, false)
	if code == 0 || out != "" || calls != 1 {
		t.Fatal("missing dependency counter became zero")
	}
}
