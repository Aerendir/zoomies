package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func checkoutTestSource() string {
	return "on: push\njobs:\n  build:\n    name: Build\n    runs-on: ubuntu-latest\n    steps:\n      - name: Checkout\n        uses: actions/checkout@" + strings.Repeat("b", 40) + "\n        with:\n          fetch-depth: 0\n      - name: Test\n        run: npm test\n"
}
func checkoutReaderFixture(t *testing.T, mode string) (*appClient, *int) {
	t.Helper()
	calls, runs, jobs := 0, 0, 0
	sha, treeSHA := strings.Repeat("a", 40), strings.Repeat("c", 40)
	source := checkoutTestSource()
	if mode == "matrix" {
		source = strings.Replace(source, "    name: Build", "    strategy: {matrix: {os: [linux]}}\n    name: Build", 1)
	}
	if mode == "unnamed" {
		source = strings.Replace(source, "      - name: Checkout\n        uses:", "      - uses:", 1)
	}
	blob := contextGitBlobSHA(source)
	c := delayTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if mode == "redirect" {
			w.Header().Set("Location", "/unscoped")
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.Method != "GET" || r.Header.Get("If-None-Match") != "" {
			t.Error("mutable or conditional read")
		}
		var body any
		switch r.URL.Path {
		case "/repos/acme/repo/actions/runs/10/attempts/2":
			runs++
			body = map[string]any{"id": 10, "run_attempt": 2, "head_sha": sha, "workflow_id": 20, "event": "push", "path": ".github/workflows/private.yml", "status": "completed", "conclusion": "success", "repository": map[string]any{"full_name": "acme/repo"}, "run_started_at": "2026-10-14T01:00:00Z", "updated_at": "2026-10-14T01:00:10Z", "jobs_url": "https://attacker.invalid/jobs"}
			v := body.(map[string]any)
			switch mode {
			case "foreign_repo":
				v["repository"] = map[string]any{"full_name": "other/repo"}
			case "non_push":
				v["event"] = "pull_request"
			case "mutable_path":
				v["path"] = ".github/workflows/private.yml@main"
			case "missing_run_time":
				delete(v, "updated_at")
			case "moved_run":
				if runs == 2 {
					v["head_sha"] = strings.Repeat("d", 40)
				}
			}
		case "/repos/acme/repo/actions/jobs/100":
			jobs++
			step := map[string]any{"name": "Checkout", "number": 42, "status": "completed", "conclusion": "success", "started_at": "2026-10-14T01:00:02Z", "completed_at": "2026-10-14T01:00:06.200Z"}
			switch mode {
			case "step_failed":
				step["conclusion"] = "failure"
			case "missing_step_time":
				delete(step, "started_at")
			case "negative_time":
				step["completed_at"] = "2026-10-14T01:00:01Z"
			case "outside_job":
				step["completed_at"] = "2026-10-14T01:00:09Z"
			case "moved_step":
				if jobs == 2 {
					step["completed_at"] = "2026-10-14T01:00:07Z"
				}
			}
			steps := []any{map[string]any{"name": "Set up job", "number": 1}, step}
			if mode == "duplicate_name" {
				steps = append(steps, map[string]any{"name": "Checkout", "number": 99})
			}
			if mode == "duplicate_number" {
				steps = append(steps, map[string]any{"name": "other", "number": 42})
			}
			body = map[string]any{"id": 100, "run_id": 10, "run_attempt": 2, "head_sha": sha, "name": "Build", "status": "completed", "conclusion": "success", "started_at": "2026-10-14T01:00:01Z", "completed_at": "2026-10-14T01:00:08Z", "steps": steps, "runner_name": "private-runner", "labels": []string{"private-label"}}
			v := body.(map[string]any)
			switch mode {
			case "foreign_attempt":
				v["run_attempt"] = 1
			case "foreign_head":
				v["head_sha"] = strings.Repeat("d", 40)
			case "wrong_job_name":
				v["name"] = "Build (linux)"
			}
		case "/repos/acme/repo/contents/.github/workflows/private.yml":
			if r.URL.Query().Get("ref") != sha {
				t.Error("mutable contents ref")
			}
			body = map[string]any{"type": "file", "path": ".github/workflows/private.yml", "encoding": "base64", "size": len(source), "sha": blob, "content": base64.StdEncoding.EncodeToString([]byte(source)), "download_url": "https://attacker.invalid/source"}
			v := body.(map[string]any)
			if mode == "corrupt_source" {
				v["sha"] = strings.Repeat("d", 40)
			}
			if mode == "large_source" {
				v["size"] = 32769
			}
		case "/repos/acme/repo/git/commits/" + sha:
			body = map[string]any{"sha": sha, "tree": map[string]any{"sha": treeSHA}}
			if mode == "foreign_commit" {
				body.(map[string]any)["sha"] = strings.Repeat("d", 40)
			}
		case "/repos/acme/repo/git/trees/" + treeSHA:
			if r.URL.Query().Get("recursive") != "1" {
				t.Error("missing bounded tree enumeration")
			}
			entry := map[string]any{"path": ".github/workflows/private.yml", "mode": "100755", "type": "blob", "sha": blob}
			body = map[string]any{"sha": treeSHA, "truncated": false, "tree": []any{entry}}
			switch mode {
			case "truncated_tree":
				body.(map[string]any)["truncated"] = true
			case "unknown_tree":
				delete(body.(map[string]any), "truncated")
			case "symlink":
				entry["mode"] = "120000"
			case "foreign_blob":
				entry["sha"] = strings.Repeat("d", 40)
			case "foreign_tree":
				body.(map[string]any)["sha"] = strings.Repeat("d", 40)
			}
		default:
			t.Errorf("unexpected endpoint %s", r.URL)
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error(err)
		}
	})
	return c, &calls
}
func TestCheckoutEvidenceAssociatesUniqueNamesWithoutGuessingStepNumberOrFileMode(t *testing.T) {
	c, calls := checkoutReaderFixture(t, "valid")
	s := CheckoutSelection{RunID: 10, Attempt: 2, GitHubJobID: 100, JobIndex: 0, StepIndex: 0}
	e, err := c.ReadCheckoutEvidence(context.Background(), "acme/repo", s)
	if err != nil {
		t.Fatal(err)
	}
	if *calls != CheckoutEvidenceMaximumRequests || !ValidCheckoutEvidence(e, "acme/repo", s) || e.StepNumber != 42 || e.Source.File.Mode != "100755" || *e.Timing.CheckoutMS != 4200 || e.Timing.EvidenceID != "checkout:10:2:100:42" {
		t.Fatalf("invalid bounded evidence %+v calls=%d", e, *calls)
	}
	body, _ := json.Marshal(e)
	if strings.Contains(string(body), "private-runner") || strings.Contains(string(body), "attacker.invalid") {
		t.Fatal("unrelated metadata retained")
	}
}
func TestCheckoutEvidenceRefusesAmbiguousUnsupportedForeignAndMovingFacts(t *testing.T) {
	for _, mode := range []string{"redirect", "foreign_repo", "non_push", "mutable_path", "missing_run_time", "moved_run", "foreign_attempt", "foreign_head", "wrong_job_name", "step_failed", "missing_step_time", "negative_time", "outside_job", "moved_step", "duplicate_name", "duplicate_number", "corrupt_source", "large_source", "matrix", "unnamed", "foreign_commit", "truncated_tree", "unknown_tree", "symlink", "foreign_blob", "foreign_tree"} {
		t.Run(mode, func(t *testing.T) {
			c, calls := checkoutReaderFixture(t, mode)
			if e, err := c.ReadCheckoutEvidence(context.Background(), "acme/repo", CheckoutSelection{RunID: 10, Attempt: 2, GitHubJobID: 100}); err == nil || e != nil || *calls > CheckoutEvidenceMaximumRequests {
				t.Fatal("unsupported source/timing accepted", err, fmt.Sprint(*calls))
			}
		})
	}
}

func TestCheckoutEvidenceLocatesTheStepWithinTheSameRequestBudget(t *testing.T) {
	c, calls := checkoutReaderFixture(t, "valid")
	located := CheckoutSelection{RunID: 10, Attempt: 2, GitHubJobID: 100, Locate: true}
	e, err := c.ReadCheckoutEvidence(context.Background(), "acme/repo", located)
	if err != nil {
		t.Fatal(err)
	}
	want := CheckoutSelection{RunID: 10, Attempt: 2, GitHubJobID: 100}
	if *calls != CheckoutEvidenceMaximumRequests || e.Selection != want || e.Selection.Locate || !ValidCheckoutEvidence(e, "acme/repo", want) || e.StepNumber != 42 {
		t.Fatalf("a located read must cost the same and name the positions it found: %+v calls=%d", e.Selection, *calls)
	}
	if ValidCheckoutSelection(CheckoutSelection{RunID: 10, Attempt: 2, GitHubJobID: 100, StepIndex: 1, Locate: true}) {
		t.Fatal("a located selection must not also name a position")
	}
}
