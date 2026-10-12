package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const cliPRDelay = `{"repository":"acme/repo","pull_number":42,"window_basis":"earliest_observed_job","result":{"version":"merge-delay-v1","state":"incomplete","head_sha":"head","target_branch":"release","protection_coverage":"partial","observation_coverage":"complete","last_finishing_check_id":"chk_001","checks":[{"check_id":"chk_001","state":"passed","completion_delay_ms":60000,"timing":{"pre_start_ms":0,"queue_ms":null,"startup_ms":null,"execution_ms":60000}}]}}`

func TestJobsPRDelayPrintsCoverageAndUnknownPhasesWithoutGuessing(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/api/v1/jobs/job_1/pr-delay" || r.URL.Query().Get("pull_number") != "42" {
			t.Errorf("unexpected route %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cliPRDelay))
	}))
	t.Cleanup(srv.Close)
	out, _ := runCLI(t, "jobs", "pr-delay", "job_1", "--pull-number", "42", "--url", srv.URL)
	for _, want := range []string{"incomplete", "partial", "unknown", "0s", "earliest_observed_job", "do not establish full merge readiness"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("spent %d reads", calls.Load())
	}
}

func TestJobsPRDelayStructuredOutputKeepsTheControllersResult(t *testing.T) {
	srv := jsonRoutes(t, map[string]string{"/api/v1/jobs/job_1/pr-delay": cliPRDelay})
	out, _ := runCLI(t, "jobs", "pr-delay", "job_1", "--pull-number", "42", "--url", srv.URL, "--output", "json")
	var got, want any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal([]byte(cliPRDelay), &want)
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	if string(g) != string(w) {
		t.Fatalf("changed report: %s", out)
	}
}

func TestJobsPRDelayRequiresItsExplicitPRNumber(t *testing.T) {
	e, _, _ := newTestEnv(t)
	if code := dispatch(context.Background(), e, []string{"jobs", "pr-delay", "job_1"}); code != 2 {
		t.Fatalf("exit %d", code)
	}
}
