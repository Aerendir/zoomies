package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/analysis"
	"github.com/eyupio/zoomies/internal/kennel/workflow"
)

func TestDependencyInspectionSharesItsRowBudgetAndCopiesFullCounts(t *testing.T) {
	_, a, _ := experimentSnapshotFixture()
	first := a.Attempts[0]
	a.Attempts = append(a.Attempts, first)
	data := []byte("jobs:\n  private:\n    steps:\n" + strings.Repeat("      - uses: private_owner/private_repo@main\n", 101))
	for i := range a.Attempts {
		r := *a.Attempts[i].Remote
		experimentSourceFixture(&r)
		r.WorkflowSource.Dependencies = workflow.InspectDependencies(data)
		r.WorkflowSource.ContentSHA256 = r.WorkflowSource.Dependencies.SourceSHA256
		a.Attempts[i].Remote = &r
	}
	v := experimentWorkflowArmView(a)
	rows := 0
	for _, r := range v.Attempts {
		d := r.RootSource.Dependencies
		rows += len(d.References)
		if *d.Counts.Declarations != 101 || !*d.RowsOmitted || !analysis.ValidRootDependencies(d, r.RootSource.ContentSHA256) {
			t.Fatal("counts, omission or binding changed")
		}
	}
	if rows != 50 {
		t.Fatal("arm budget not shared")
	}
	old, _ := json.Marshal(v)
	a.Attempts[0].Remote.WorkflowSource.Dependencies.Counts.Floating = 0
	after, _ := json.Marshal(v)
	if string(old) != string(after) || strings.Contains(string(after), "private_owner") {
		t.Fatal("private source borrowed or leaked")
	}
}
