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
	"time"
)

func cliRunnerPointer[T any](v T) *T { return &v }
func cliRunnerFixture() *cliRunnerEvidence {
	return &cliRunnerEvidence{Version: "runner-evidence-v1", Baseline: &cliRunnerArm{SelectedJobs: cliRunnerPointer(2), MatchedJobs: cliRunnerPointer(1), RecordedGrants: cliRunnerPointer(1), RunnerObservations: cliRunnerPointer(1), RowsOmitted: cliRunnerPointer(true), Jobs: []cliRunnerJob{{RunID: "10", Attempt: 1, GitHubJobID: "9223372036854775807", FleetMatched: cliRunnerPointer(true), RecordedGrant: &cliRunnerGrant{Basis: "retained_job_granted_size_v1", Source: "pool", CPUs: cliRunnerPointer(1.0), MemoryMB: cliRunnerPointer(int64(4096))}, Observation: &cliRunnerObservation{SHA256: strings.Repeat("a", 64), IssuedAt: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC), ReceivedAt: time.Date(2026, 10, 13, 0, 2, 0, 0, time.UTC), Requested: &cliRunnerRequest{Backend: "docker", CPUs: cliRunnerPointer(2.0), DockerMode: "dind", CPUTyped: cliRunnerPointer(false), MemoryTyped: cliRunnerPointer(false)}, DigestCoverage: "unsupported"}}}}, Candidate: &cliRunnerArm{SelectedJobs: cliRunnerPointer(1), MatchedJobs: cliRunnerPointer(0), RecordedGrants: cliRunnerPointer(0), RunnerObservations: cliRunnerPointer(0), RowsOmitted: cliRunnerPointer(false), Jobs: []cliRunnerJob{{RunID: "11", Attempt: 1, GitHubJobID: "111", FleetMatched: cliRunnerPointer(false)}}}}
}
func cliRunnerBody(t *testing.T, e *cliRunnerEvidence) string {
	t.Helper()
	var doc map[string]any
	d := json.NewDecoder(strings.NewReader(cliComparison))
	d.UseNumber()
	if err := d.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	s := doc["report"].(map[string]any)["snapshot"].(map[string]any)
	s["baseline"] = map[string]any{"attempts": []any{map[string]any{"run_id": 10, "attempt": 1}}}
	s["candidate"] = map[string]any{"attempts": []any{map[string]any{"run_id": 11, "attempt": 1}}}
	doc["runner_evidence"] = e
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestComparisonRunnerFactsAppearForCaptureAndReplayWithoutExtraRequests(t *testing.T) {
	for _, capture := range []bool{false, true} {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if capture && r.Method != "POST" || !capture && r.Method != "GET" {
				t.Errorf("unexpected method %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			if capture {
				w.WriteHeader(201)
			}
			w.Write([]byte(cliRunnerBody(t, cliRunnerFixture())))
		}))
		t.Cleanup(srv.Close)
		e, out, errOut := newTestEnv(t)
		args := []string{"comparisons", "get", "cmp_1", "--installation", "ins_1", "--repo", "acme/repo", "--url", srv.URL}
		if capture {
			e.in = strings.NewReader(cliCaptureSelection)
			args = []string{"comparisons", "capture", "--file", "-", "--url", srv.URL}
		}
		if code := dispatch(context.Background(), e, args); code != 0 {
			t.Fatalf("exit %d: %s", code, errOut.String())
		}
		for _, want := range []string{"1 CPUs / 4096 MB", "2 CPUs / unknown MB", "9223372036854775807", "2 enumerated jobs", "showing 1 jobs", "additional job rows were omitted", "not recorded", "no retained observation", "unsupported", "completed at (agent)", "unverified reports", "inconclusive", "0ms", "unknown"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("missing %q: %s", want, out.String())
			}
		}
		if calls.Load() != 1 {
			t.Fatal("extra capture or replay read")
		}
	}
}

func TestComparisonRunnerStructuredOutputPreservesFutureVersionsAndExactNumbers(t *testing.T) {
	e := cliRunnerFixture()
	e.Version = "future-runner-view"
	body := cliRunnerBody(t, e)
	srv := jsonRoutes(t, map[string]string{"/api/v1/comparisons/cmp_1": body})
	out, _ := runCLI(t, "comparisons", "get", "cmp_1", "--installation", "ins_1", "--repo", "acme/repo", "--url", srv.URL, "--output", "json")
	var got, want bytes.Buffer
	if err := json.Compact(&got, []byte(out)); err != nil {
		t.Fatal(err)
	}
	if err := json.Compact(&want, []byte(body)); err != nil {
		t.Fatal(err)
	}
	if got.String() != want.String() || !strings.Contains(out, "9007199254740993") {
		t.Fatal("changed raw structured evidence")
	}
}

func TestComparisonRunnerHumanOutputRefusesMalformedFactsBeforePrintingTheReport(t *testing.T) {
	for _, mode := range []string{"version", "count", "missing_flag", "duplicate", "foreign_run", "unmatched", "size", "grant_basis", "backend", "digest", "clock", "typed_flag"} {
		e := cliRunnerFixture()
		j := &e.Baseline.Jobs[0]
		switch mode {
		case "version":
			e.Version = "future"
		case "count":
			*e.Baseline.RecordedGrants = 0
		case "missing_flag":
			e.Baseline.RowsOmitted = nil
		case "duplicate":
			e.Candidate.Jobs[0].GitHubJobID = j.GitHubJobID
		case "foreign_run":
			j.RunID = "999"
		case "unmatched":
			*j.FleetMatched = false
		case "size":
			*j.RecordedGrant.CPUs = 0
		case "grant_basis":
			j.RecordedGrant.Basis = "future"
		case "backend":
			j.Observation.Requested.Backend = "future"
		case "digest":
			j.Observation.ImageDigest = cliRunnerPointer("private-legacy-value")
		case "clock":
			j.Observation.ReceivedAt = time.Time{}
		case "typed_flag":
			j.Observation.Requested.CPUTyped = nil
		}
		srv := jsonRoutes(t, map[string]string{"/api/v1/comparisons/cmp_1": cliRunnerBody(t, e)})
		env, out, _ := newTestEnv(t)
		if code := dispatch(context.Background(), env, []string{"comparisons", "get", "cmp_1", "--installation", "ins_1", "--repo", "acme/repo", "--url", srv.URL}); code == 0 || out.Len() != 0 {
			t.Errorf("printed malformed %s evidence: %s", mode, out.String())
		}
	}
}

func TestComparisonRunnerHumanOutputShowsOnlyWhitelistedTerminalSafeFacts(t *testing.T) {
	var doc map[string]any
	d := json.NewDecoder(strings.NewReader(cliRunnerBody(t, cliRunnerFixture())))
	d.UseNumber()
	if err := d.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	e := doc["runner_evidence"].(map[string]any)
	j := e["baseline"].(map[string]any)["jobs"].([]any)[0].(map[string]any)
	j["host_id"] = "private-host"
	j["future_secret"] = "\x1b[31mprivate-value"
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	srv := jsonRoutes(t, map[string]string{"/api/v1/comparisons/cmp_1": string(body)})
	out, _ := runCLI(t, "comparisons", "get", "cmp_1", "--installation", "ins_1", "--repo", "acme/repo", "--url", srv.URL)
	if strings.Contains(out, "private-host") || strings.Contains(out, "private-value") || strings.Contains(out, "future_secret") {
		t.Fatal("printed private or future runner fields")
	}
}
