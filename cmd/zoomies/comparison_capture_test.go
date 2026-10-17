package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const cliCaptureSelection = `{"installation_id":"ins_1","repository":"acme/repo","kind":"runner","hypothesis":"lower latency","intended_delta":"larger runner","since":"2026-10-12T00:00:00Z","until":"2026-10-13T00:00:00Z","baseline":[{"run_id":9007199254740993,"attempt":1}],"candidate":[{"run_id":9223372036854775807,"attempt":2}]}`

func TestComparisonCaptureMakesOneExplicitPostAndPreservesIntegerEvidence(t *testing.T) {
	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" || r.URL.Path != "/api/v1/comparisons" || len(r.URL.Query()) != 0 {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				var body comparisonCaptureInput
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Baseline[0].RunID != 9007199254740993 || body.Candidate[0].RunID != 9223372036854775807 || body.InstallationID != "ins_1" || body.Repository != "acme/repo" || body.Hypothesis != "lower latency" {
					t.Errorf("changed input %+v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(cliComparison))
			}))
			t.Cleanup(srv.Close)
			path := filepath.Join(t.TempDir(), "selection.json")
			if err := os.WriteFile(path, []byte(cliCaptureSelection), 0600); err != nil {
				t.Fatal(err)
			}
			out, _ := runCLI(t, "comparisons", "capture", "--file", path, "--url", srv.URL, "--output", format)
			if calls.Load() != 1 {
				t.Fatalf("requests %d", calls.Load())
			}
			if format == "json" {
				var got, want bytes.Buffer
				json.Compact(&got, []byte(out))
				json.Compact(&want, []byte(cliComparison))
				if got.String() != want.String() {
					t.Fatal("changed raw report")
				}
			} else {
				for _, want := range []string{"cmp_1", "inconclusive", "selection_incomplete", "unknown", "0ms"} {
					if !strings.Contains(out, want) {
						t.Errorf("missing %q", want)
					}
				}
			}
		})
	}
}

func TestComparisonCaptureRefusesMalformedOrUnboundedSelectionsBeforePosting(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Error("invalid selection reached server")
	}))
	t.Cleanup(srv.Close)
	cases := []string{"null", "{}", cliCaptureSelection + `{}`, strings.Replace(cliCaptureSelection, `"kind":"runner"`, `"kind":"other"`, 1), strings.Replace(cliCaptureSelection, `"kind":"runner"`, `"kind":"workflow"`, 1), strings.Replace(cliCaptureSelection, "ins_1", "ins_../other", 1), strings.Replace(cliCaptureSelection, "acme/repo", "acme/repo?other=1", 1), strings.Replace(cliCaptureSelection, "lower latency", strings.Repeat("a", 4097), 1), strings.Repeat(" ", comparisonInputLimit+1), strings.Replace(cliCaptureSelection, "2026-10-13T00:00:00Z", "2026-10-11T00:00:00Z", 1), strings.Replace(cliCaptureSelection, "9223372036854775807", "9223372036854775808", 1), strings.Replace(cliCaptureSelection, "9223372036854775807", "1.5", 1), strings.Replace(cliCaptureSelection, `"attempt":2`, `"attempt":0`, 1), strings.Replace(cliCaptureSelection, `"candidate":[{"run_id":9223372036854775807,"attempt":2}]`, `"candidate":[]`, 1), strings.Replace(cliCaptureSelection, `"candidate":[{"run_id":9223372036854775807,"attempt":2}]`, `"candidate":[{"run_id":9007199254740993,"attempt":1}]`, 1), strings.TrimSuffix(cliCaptureSelection, "}") + `,"initiating_user":"other"}`}
	for i, body := range cases {
		e, _, _ := newTestEnv(t)
		e.in = strings.NewReader(body)
		if code := dispatch(context.Background(), e, []string{"comparisons", "capture", "--file", "-", "--url", srv.URL}); code != 2 {
			t.Errorf("case %d: exit %d", i, code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input spent requests")
	}
}

func TestComparisonCaptureStdinSupportsWorkflowDeltaAndCombinedBound(t *testing.T) {
	var input comparisonCaptureInput
	json.Unmarshal([]byte(cliCaptureSelection), &input)
	input.Kind = "workflow"
	input.RevisionDeltaID = "retained-delta"
	input.Baseline = []comparisonSelection{{RunID: 1, Attempt: 1}}
	input.Candidate = nil
	for n := 2; n <= 11; n++ {
		input.Candidate = append(input.Candidate, comparisonSelection{RunID: int64(n), Attempt: 1})
	}
	if err := validateComparisonCapture(input); err == nil {
		t.Fatal("accepted 11 attempts")
	}
	input.Candidate = input.Candidate[:9]
	if err := validateComparisonCapture(input); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(input)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var got comparisonCaptureInput
		json.NewDecoder(r.Body).Decode(&got)
		if got.RevisionDeltaID != "retained-delta" || len(got.Candidate) != 9 {
			t.Errorf("changed workflow %+v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		w.Write([]byte(cliComparison))
	}))
	t.Cleanup(srv.Close)
	e, _, _ := newTestEnv(t)
	e.in = bytes.NewReader(body)
	if code := dispatch(context.Background(), e, []string{"comparisons", "capture", "--file", "-", "--url", srv.URL}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if calls.Load() != 1 {
		t.Fatal("spent extra requests")
	}
}

func TestComparisonCaptureRefusalOrUnsupportedResultNeverRetries(t *testing.T) {
	for _, status := range []int{403, 429, 500, 201} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == 201 {
					w.Write([]byte(strings.ReplaceAll(cliComparison, "selected-runs-v2", "future-rule")))
				} else {
					w.Write([]byte(`{"error":{"code":"refused","message":"Capture refused"}}`))
				}
			}))
			t.Cleanup(srv.Close)
			e, _, stderr := newTestEnv(t)
			e.in = strings.NewReader(cliCaptureSelection)
			if code := dispatch(context.Background(), e, []string{"comparisons", "capture", "--file", "-", "--url", srv.URL}); code == 0 {
				t.Fatal("accepted refused capture")
			}
			if calls.Load() != 1 || !strings.Contains(stderr.String(), "do not automatically retry") {
				t.Fatalf("calls %d stderr %s", calls.Load(), stderr)
			}
		})
	}
}

func TestComparisonCaptureRequiresAnExplicitSelectionFile(t *testing.T) {
	e, _, _ := newTestEnv(t)
	if code := dispatch(context.Background(), e, []string{"comparisons", "capture"}); code != 2 {
		t.Fatalf("exit %d", code)
	}
}
