package mcp

import (
	"errors"
	"strings"
	"testing"
)

func TestPRDelayToolReadsOneBoundedDiagnosticRoute(t *testing.T) {
	api := &kennelAPI{body: `{"window_basis":"earliest_observed_job","result":{"state":"incomplete","checks":[]}}`}
	tl := kennelTool(t, "get_pr_delay")
	out, err := kennelCall(t, tl, api, `{"job_id":"job_1","pull_number":42}`)
	if err != nil {
		t.Fatal(err)
	}
	if api.calls != 1 || api.got != "GET /jobs/job_1/pr-delay" || api.query.Get("pull_number") != "42" {
		t.Fatalf("got %s %v, %d", api.got, api.query, api.calls)
	}
	if !tl.Annotations.ReadOnly || tl.action || !strings.Contains(out[0].Text, `"incomplete"`) {
		t.Fatalf("tool or result changed: %+v %v", tl, out)
	}
}

func TestPRDelayToolRefusesInvalidTargetsBeforeAnyRead(t *testing.T) {
	for _, args := range []string{`{}`, `{"job_id":"job_1","pull_number":0}`, `{"job_id":"job_../settings","pull_number":1}`,
		`{"job_id":"job_1","pull_number":2147483648}`, `{"job_id":"job_1","pull_number":1,"installation_id":"ins_other"}`} {
		api := &kennelAPI{}
		if _, err := kennelCall(t, kennelTool(t, "get_pr_delay"), api, args); err == nil || api.calls != 0 {
			t.Fatalf("accepted %s or spent reads: %v %d", args, err, api.calls)
		}
	}
}

func TestPRDelayToolDoesNotRetryARefusedDiagnostic(t *testing.T) {
	want := errors.New("429: diagnostic allowance exhausted")
	api := &kennelAPI{err: want}
	_, err := kennelCall(t, kennelTool(t, "get_pr_delay"), api, `{"job_id":"job_1","pull_number":1}`)
	if err == nil || api.calls != 1 {
		t.Fatalf("err %v, calls %d", err, api.calls)
	}
}
