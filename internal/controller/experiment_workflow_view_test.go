package controller

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestWorkflowProjectionPreservesExactIDsAndUnknownsWithoutPrivatePaths(t *testing.T) {
	_, a, b := experimentSnapshotFixture()
	r := a.Attempts[0].Remote
	r.RunID = math.MaxInt64
	experimentSourceFixture(r)
	view := experimentWorkflowView(a, b)
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"run_id":"9223372036854775807"`) || strings.Contains(string(raw), "private-root") || strings.Contains(string(raw), `"workflow_path":`) {
		t.Fatalf("identity or privacy changed: %s", raw)
	}
	if view.Baseline.RootSources != 1 || view.Baseline.SelectedAttempts != 1 || view.Baseline.Attempts[0].WorkflowPathSHA256 == nil || view.Candidate.Attempts[0].Event != nil || view.Candidate.Attempts[0].RootSource != nil || view.Candidate.Attempts[0].WorkflowPathSHA256 != nil {
		t.Fatalf("unknowns or coverage changed: %+v", view)
	}
	r.WorkflowSource.Bytes = 999
	r.Event = "changed"
	after, _ := json.Marshal(view)
	if string(raw) != string(after) {
		t.Fatal("response borrowed mutable private fields")
	}
}

func TestWorkflowProjectionKeepsFullCountsWhenRowsAreBounded(t *testing.T) {
	_, a, _ := experimentSnapshotFixture()
	experimentSourceFixture(a.Attempts[0].Remote)
	first := a.Attempts[0]
	for len(a.Attempts) < 101 {
		a.Attempts = append(a.Attempts, first)
	}
	v := experimentWorkflowArmView(a)
	if !v.RowsOmitted || v.SelectedAttempts != 101 || v.RootSources != 101 || len(v.Attempts) != 100 {
		t.Fatalf("counts changed with omitted rows: %+v", v)
	}
}
