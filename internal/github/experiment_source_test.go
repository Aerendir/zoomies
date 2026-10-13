package github

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestPushWorkflowSourceUsesTheImmutableCommitAndRetainsOnlyDigests(t *testing.T) {
	for _, mode := range []string{"observed", "dispatch", "missing_path", "mutable_ref", "traversal", "url", "wrong_path", "bad_blob", "wrong_size", "directory", "symlink", "encoding", "oversized", "invalid_utf8", "permission", "missing", "submodule", "bad_base64", "empty", "movement"} {
		t.Run(mode, func(t *testing.T) {
			calls, runs, sources := 0, 0, 0
			content := "name: private workflow\njobs: {}\n# private token text\n"
			workflowPath := ".github/workflows/build.yml"
			event := "push"
			switch mode {
			case "dispatch":
				event = "workflow_dispatch"
			case "missing_path":
				workflowPath = ""
			case "mutable_ref":
				workflowPath += "@refs/heads/main"
			case "traversal":
				workflowPath = ".github/workflows/../workflows/build.yml"
			case "url":
				workflowPath = "https://other.example/workflow.yml"
			}
			c := delayTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" {
					t.Error("not a read")
				}
				switch r.URL.Path {
				case "/repos/acme/repo/actions/runs/10/attempts/2":
					runs++
					currentEvent := event
					if mode == "movement" && runs == 2 {
						currentEvent = "workflow_dispatch"
					}
					fmt.Fprint(w, strings.TrimSuffix(experimentRunJSON(delayTestSHA), "}")+fmt.Sprintf(`,"event":%q,"path":%q}`, currentEvent, workflowPath))
				case "/repos/acme/repo/actions/runs/10/attempts/2/jobs":
					fmt.Fprintf(w, `{"total_count":1,"jobs":[%s]}`, experimentJobJSON(1))
				case "/repos/acme/repo/contents/.github/workflows/build.yml":
					sources++
					if r.URL.Query().Get("ref") != delayTestSHA {
						t.Error("mutable or unpinned source ref")
					}
					if mode == "permission" || mode == "missing" {
						if mode == "missing" {
							w.WriteHeader(http.StatusNotFound)
						} else {
							w.WriteHeader(http.StatusForbidden)
						}
						fmt.Fprint(w, `{"message":"forbidden"}`)
						return
					}
					body := content
					if mode == "invalid_utf8" {
						body = string([]byte{0xff})
					}
					f := map[string]any{"type": "file", "path": workflowPath, "size": len(body), "sha": contextGitBlobSHA(body), "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(body)), "download_url": "https://other.example/private"}
					switch mode {
					case "wrong_path":
						f["path"] = ".github/workflows/other.yml"
					case "bad_blob":
						f["sha"] = strings.Repeat("b", 40)
					case "wrong_size":
						f["size"] = len(body) + 1
					case "directory":
						json.NewEncoder(w).Encode([]any{f})
						return
					case "symlink":
						f["type"] = "symlink"
						f["target"] = "private"
					case "encoding":
						f["encoding"] = "none"
					case "submodule":
						f["submodule_git_url"] = "https://other.example/repo"
					case "bad_base64":
						f["content"] = "%%%"
					case "empty":
						f["size"] = 0
					case "oversized":
						f["size"] = ExperimentWorkflowSourceBytes + 1
					}
					json.NewEncoder(w).Encode(f)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					w.WriteHeader(404)
				}
			})
			e, err := c.ReadExperimentAttempt(context.Background(), "acme/repo", 10, 2)
			skipped := mode == "dispatch" || mode == "missing_path" || mode == "mutable_ref" || mode == "traversal" || mode == "url"
			if skipped {
				if err != nil || e.WorkflowSource != nil || calls != 3 || sources != 0 {
					t.Fatalf("unsupported context fetched source: %+v %v %d", e, err, calls)
				}
				return
			}
			if mode != "observed" {
				if err == nil || calls > 4 || sources != 1 {
					t.Fatalf("invalid evidence accepted %v %d", err, calls)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte(content))
			s := e.WorkflowSource
			if calls != 4 || sources != 1 || s == nil || !ValidExperimentWorkflowSource(e) || s.ContentSHA256 != hex.EncodeToString(digest[:]) || s.BlobSHA != contextGitBlobSHA(content) || s.CommitSHA != delayTestSHA || s.Bytes != len(content) {
				t.Fatalf("lost or unpinned source: %+v calls=%d", e, calls)
			}
			raw, _ := json.Marshal(e)
			if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "download_url") {
				t.Fatal("source text or URL retained")
			}
		})
	}
}
