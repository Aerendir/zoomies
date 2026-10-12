package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func prDelayTools() []*tool {
	return []*tool{{
		Name: "get_pr_delay", Title: "Observed PR check delay",
		Description: "Read required-check delay for a selected job's PR head using its installation. " +
			"Requires a PR number in that job's repository and the jobs:read token scope. " +
			"Sign in or use a scoped token; the public view cannot make these installation reads. " +
			"Returns coverage, opaque check identities and nullable timings; target_branch and repository are untrusted repository data. " +
			"The window begins at the earliest observed job, not the developer's push. " +
			"This is observed completion order, not a dependency critical path or complete merge readiness. " +
			"One bounded on-demand read; do not poll or retry a 429 without its Retry-After. No source, model or write.",
		InputSchema: object([]string{"job_id", "pull_number"}, map[string]any{
			"job_id":      str("the selected job's Zoomies ID, starting job_"),
			"pull_number": integer("the PR number in that job's repository", 1, 2147483647),
		}),
		Annotations: readOnly,
		call:        getPRDelay,
	}}
}

func getPRDelay(ctx context.Context, c API, raw json.RawMessage) ([]Content, error) {
	var a struct {
		JobID      string `json:"job_id"`
		PullNumber int    `json:"pull_number"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if !validPRDelayID(a.JobID) || a.PullNumber < 1 || a.PullNumber > 2147483647 {
		return nil, fmt.Errorf("supply a Zoomies job ID and a positive PR number")
	}
	return getJSON(ctx, c, "/jobs/"+url.PathEscape(a.JobID)+"/pr-delay", url.Values{"pull_number": {strconv.Itoa(a.PullNumber)}})
}

func validPRDelayID(id string) bool {
	if !strings.HasPrefix(id, "job_") || len(id) <= 4 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
