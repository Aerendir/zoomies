package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func experimentRunJSON(sha string) string {
	return fmt.Sprintf(`{"id":10,"run_attempt":2,"head_sha":%q,"workflow_id":20,"status":"completed","conclusion":"success","repository":{"full_name":"acme/repo"},"updated_at":"2026-10-13T01:00:00Z"}`, sha)
}
func experimentJobJSON(id int) string {
	return fmt.Sprintf(`{"id":%d,"run_id":10,"run_attempt":2,"head_sha":%q,"status":"completed","conclusion":"failure","name":"private workflow text","steps":[{"name":"private step"}]}`, id, delayTestSHA)
}

func TestAnExactAttemptReadPreservesFailuresWithoutWorkflowText(t *testing.T) {
	calls := 0
	c := delayTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" {
			t.Error(r.Method)
		}
		switch r.URL.Path {
		case "/repos/acme/repo/actions/runs/10/attempts/2":
			fmt.Fprint(w, experimentRunJSON(delayTestSHA))
		case "/repos/acme/repo/actions/runs/10/attempts/2/jobs":
			if r.URL.Query().Get("per_page") != "100" {
				t.Error("page size")
			}
			fmt.Fprintf(w, `{"total_count":1,"jobs":[%s]}`, experimentJobJSON(1))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	e, err := c.ReadExperimentAttempt(context.Background(), "acme/repo", 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(e)
	if e.JobsCoverage != "complete" || calls != 3 || len(e.Jobs) != 1 || e.Jobs[0].Conclusion != "failure" || strings.Contains(string(b), "private") {
		t.Fatalf("got %s calls=%d", b, calls)
	}
}
func TestAnAttemptReadRefusesMovementAndForeignIdentity(t *testing.T) {
	for _, mode := range []string{"movement", "foreign_job", "foreign_repo"} {
		t.Run(mode, func(t *testing.T) {
			runs := 0
			c := delayTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/jobs") {
					job := experimentJobJSON(1)
					if mode == "foreign_job" {
						job = strings.Replace(job, `"run_id":10`, `"run_id":11`, 1)
					}
					fmt.Fprintf(w, `{"total_count":1,"jobs":[%s]}`, job)
					return
				}
				runs++
				sha := delayTestSHA
				if mode == "movement" && runs > 1 {
					sha = strings.Repeat("b", 40)
				}
				payload := experimentRunJSON(sha)
				if mode == "foreign_repo" {
					payload = strings.Replace(payload, "acme/repo", "other/repo", 1)
				}
				fmt.Fprint(w, payload)
			})
			if _, err := c.ReadExperimentAttempt(context.Background(), "acme/repo", 10, 2); err == nil {
				t.Fatal("inconsistent evidence accepted")
			}
		})
	}
}
func TestAttemptJobCoverageCannotInventACompletePage(t *testing.T) {
	for _, mode := range []string{"missing_count", "duplicate", "cap", "pending"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c := delayTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if !strings.HasSuffix(r.URL.Path, "/jobs") {
					payload := experimentRunJSON(delayTestSHA)
					if mode == "pending" {
						payload = strings.Replace(payload, `"status":"completed"`, `"status":"in_progress"`, 1)
					}
					fmt.Fprint(w, payload)
					return
				}
				switch mode {
				case "missing_count":
					fmt.Fprintf(w, `{"jobs":[%s]}`, experimentJobJSON(1))
				case "duplicate":
					fmt.Fprintf(w, `{"total_count":2,"jobs":[%s,%s]}`, experimentJobJSON(1), experimentJobJSON(1))
				case "cap":
					page, _ := strconv.Atoi(r.URL.Query().Get("page"))
					w.Header().Set("Link", fmt.Sprintf(`<https://api.github.com/repos/acme/repo/actions/runs/10/attempts/2/jobs?page=%d>; rel="next"`, page+1))
					jobs := make([]string, 100)
					for i := range jobs {
						jobs[i] = experimentJobJSON((page-1)*100 + i + 1)
					}
					fmt.Fprintf(w, `{"total_count":501,"jobs":[%s]}`, strings.Join(jobs, ","))
				default:
					fmt.Fprintf(w, `{"total_count":1,"jobs":[%s]}`, experimentJobJSON(1))
				}
			})
			e, err := c.ReadExperimentAttempt(context.Background(), "acme/repo", 10, 2)
			if err != nil {
				t.Fatal(err)
			}
			if e.JobsCoverage != "partial" || calls > 7 || mode == "cap" && (calls != 7 || len(e.Jobs) != 500) {
				t.Fatalf("got %+v calls=%d", e, calls)
			}
		})
	}
}

func TestAttemptEvidenceRefusesOversizedBodiesAndPermissionFailures(t *testing.T) {
	for _, mode := range []string{"oversized", "forbidden"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c := delayTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if mode == "forbidden" {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"message":"forbidden"}`)
					return
				}
				payload := experimentRunJSON(delayTestSHA)
				fmt.Fprint(w, strings.TrimSuffix(payload, "}")+`,"display_title":"`+strings.Repeat("x", prDelayResponseBytes)+`"}`)
			})
			if _, err := c.ReadExperimentAttempt(context.Background(), "acme/repo", 10, 2); err == nil || calls != 1 {
				t.Fatalf("accepted %s, calls=%d err=%v", mode, calls, err)
			}
		})
	}
}

func TestAttemptEvidenceRefusesInvalidIdentitiesBeforeAnyRead(t *testing.T) {
	calls := 0
	c := delayTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) })
	for _, tc := range []struct {
		repo    string
		run     int64
		attempt int
	}{{"acme/repo", 0, 2}, {"acme/repo", 10, 0}, {"", 10, 2}} {
		if _, err := c.ReadExperimentAttempt(context.Background(), tc.repo, tc.run, tc.attempt); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	if calls != 0 {
		t.Fatalf("spent %d reads", calls)
	}
}

func TestAttemptTriggerAndWorkflowPathArePinnedWithoutAdditionalReads(t *testing.T) {
	for _, mode := range []string{"present", "absent", "event_changes", "path_changes", "event_disappears", "path_disappears", "event_appears", "path_appears", "invalid_event", "oversized_path", "control_path"} {
		t.Run(mode, func(t *testing.T) {
			calls, runs := 0, 0
			c := delayTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if strings.HasSuffix(r.URL.Path, "/jobs") {
					fmt.Fprintf(w, `{"total_count":1,"jobs":[%s]}`, experimentJobJSON(1))
					return
				}
				runs++
				event, path := "workflow_dispatch", ".github/workflows/build.yml@refs/heads/main"
				if mode == "absent" {
					event, path = "", ""
				}
				if runs == 1 && mode == "event_appears" {
					event = ""
				}
				if runs == 1 && mode == "path_appears" {
					path = ""
				}
				if runs > 1 {
					switch mode {
					case "event_changes":
						event = "push"
					case "path_changes":
						path = ".github/workflows/other.yml"
					case "event_disappears":
						event = ""
					case "path_disappears":
						path = ""
					}
				}
				switch mode {
				case "invalid_event":
					event = "push\nprivate"
				case "oversized_path":
					path = strings.Repeat("x", 1025)
				case "control_path":
					path = "file\x1b[31m"
				}
				payload := strings.TrimSuffix(experimentRunJSON(delayTestSHA), "}") + fmt.Sprintf(`,"event":%q,"path":%q,"display_title":"private title","inputs":{"token":"private secret"}}`, event, path)
				fmt.Fprint(w, payload)
			})
			e, err := c.ReadExperimentAttempt(context.Background(), "acme/repo", 10, 2)
			if mode != "present" && mode != "absent" {
				if err == nil {
					t.Fatal("changed or invalid context accepted")
				}
				if strings.HasPrefix(mode, "invalid") || mode == "oversized_path" || mode == "control_path" {
					if calls != 1 {
						t.Fatalf("invalid context spent %d reads", calls)
					}
				} else if calls != 3 {
					t.Fatalf("identity recheck spent %d reads", calls)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if calls != 3 {
				t.Fatalf("spent %d reads", calls)
			}
			if mode == "present" && (e.Event != "workflow_dispatch" || e.WorkflowPath != ".github/workflows/build.yml@refs/heads/main") {
				t.Fatalf("lost context: %+v", e)
			}
			b, _ := json.Marshal(e)
			if strings.Contains(string(b), "private") || mode == "absent" && (strings.Contains(string(b), `"event"`) || strings.Contains(string(b), `"workflow_path"`)) {
				t.Fatalf("unexpected projection %s", b)
			}
		})
	}
}
