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

const cliComparison = `{"id":"cmp_1","sha256":"digest","created_at":"2026-10-13T00:00:00Z","report":{"window_basis":"github_attempt_started_at","execution_basis":"sum_of_all_github_job_elapsed_times","snapshot":{"repository":"acme/repo","evidence_id":"cmp_1","rule_version":"selected-runs-v2"},"measurement":{"version":"selected-measures-v1","eligibility":{"rule_version":"selected-runs-v2","state":"inconclusive","reasons":["selection_incomplete","cache_unknown"]},"baseline":{"all_attempt_execution":{"samples":1,"missing":0,"p50_ms":0,"p95_ms":0,"total_ms":0}},"candidate":{"all_attempt_execution":{"samples":0,"missing":1,"p50_ms":null,"p95_ms":null,"total_ms":null}},"latency_change":"not_enough","execution_mean_change":"not_enough","allocated_idle_mean_change":"not_enough","failure_rate_change":"not_enough"}},"future_field":9007199254740993}`

func TestComparisonsReplayReadsOnceWithExactScopeAndKeepsUnknowns(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/api/v1/comparisons/cmp_1" || r.URL.Query().Get("installation_id") != "ins_1" || r.URL.Query().Get("repo") != "acme/repo" || len(r.URL.Query()) != 2 {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cliComparison))
	}))
	t.Cleanup(srv.Close)
	out, _ := runCLI(t, "comparisons", "get", "cmp_1", "--installation", "ins_1", "--repo", "acme/repo", "--url", srv.URL)
	for _, want := range []string{"inconclusive", "selection_incomplete", "cache_unknown", "unknown", "0ms", "not_enough", "do not establish cohort completeness"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("reads=%d", calls.Load())
	}
}

func TestComparisonsStructuredReplayPreservesRawEvidence(t *testing.T) {
	srv := jsonRoutes(t, map[string]string{"/api/v1/comparisons/cmp_1": cliComparison})
	out, _ := runCLI(t, "comparisons", "get", "cmp_1", "--installation", "ins_1", "--repo", "acme/repo", "--url", srv.URL, "--output", "json")
	var got, want bytes.Buffer
	if err := json.Compact(&got, []byte(out)); err != nil {
		t.Fatal(err)
	}
	if err := json.Compact(&want, []byte(cliComparison)); err != nil {
		t.Fatal(err)
	}
	if got.String() != want.String() {
		t.Fatalf("changed retained response: %s", out)
	}
}

func TestComparisonsRejectInvalidScopeBeforeAnyRead(t *testing.T) {
	for _, args := range [][]string{
		{"cmp_1", "--repo", "acme/repo"}, {"cmp_1", "--installation", "ins_1"},
		{"cmp_1/other", "--installation", "ins_1", "--repo", "acme/repo"},
		{"cmp_1", "--installation", "ins_1", "--repo", "acme/repo?x=y"},
	} {
		e, _, _ := newTestEnv(t)
		if code := dispatch(context.Background(), e, append([]string{"comparisons", "get"}, args...)); code != 2 {
			t.Errorf("%v: exit=%d", args, code)
		}
	}
}

func TestComparisonsUnavailableHistoryDoesNotCaptureOrRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" {
			t.Errorf("mutation %s", r.Method)
		}
		http.Error(w, `{"error":"unavailable"}`, 404)
	}))
	t.Cleanup(srv.Close)
	e, _, _ := newTestEnv(t)
	if code := dispatch(context.Background(), e, []string{"comparisons", "get", "cmp_1", "--installation", "ins_1", "--repo", "acme/repo", "--url", srv.URL}); code == 0 {
		t.Fatal("accepted unavailable history")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestComparisonsRefuseUnsupportedOrMismatchedReports(t *testing.T) {
	for _, pair := range [][2]string{{"selected-runs-v2", "future-rule"}, {"selected-measures-v1", "future-measures"}, {"acme/repo", "other/repo"}, {`"evidence_id":"cmp_1"`, `"evidence_id":"cmp_2"`}, {`"state":"inconclusive"`, `"state":"future-state"`}} {
		body := strings.ReplaceAll(cliComparison, pair[0], pair[1])
		srv := jsonRoutes(t, map[string]string{"/api/v1/comparisons/cmp_1": body})
		e, _, _ := newTestEnv(t)
		if code := dispatch(context.Background(), e, []string{"comparisons", "get", "cmp_1", "--installation", "ins_1", "--repo", "acme/repo", "--url", srv.URL}); code == 0 {
			t.Errorf("accepted %v", pair)
		}
	}
}
