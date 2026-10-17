package controller

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/kennel/workflow"
	"github.com/eyupio/zoomies/internal/store"
)

func experimentDependencyFixture(e *github.ExperimentAttemptEvidence) {
	experimentSourceFixture(e)
	data := []byte("jobs: {private_job: {steps: [{uses: 'private_owner/private_repo@" + strings.Repeat("a", 40) + "'}]}}")
	e.WorkflowSource.Dependencies = workflow.InspectDependencies(data)
	e.WorkflowSource.ContentSHA256 = e.WorkflowSource.Dependencies.SourceSHA256
	e.WorkflowSource.Bytes = len(data)
}
func TestDependencyInventoryRetainsPrivatelyAndReplaysWithoutPromotingWorkflowIdentity(t *testing.T) {
	c, record, a, b := retainedComparisonFixture(t)
	experimentSourceFixture(a.Attempts[0].Remote)
	before, err := AssembleExperimentComparison(record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	experimentDependencyFixture(a.Attempts[0].Remote)
	after, err := AssembleExperimentComparison(record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	oldReport, _ := json.Marshal(before)
	newReport, _ := json.Marshal(after)
	if string(oldReport) != string(newReport) {
		t.Fatal("root declaration pins promoted eligibility or identity")
	}
	saved, err := c.RetainExperimentComparison(context.Background(), &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}, record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	row, err := c.st.GetComparisonEvidence(context.Background(), a.InstallationID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var retained experimentRetainedEnvelope
	if err := json.Unmarshal(row.Payload, &retained); err != nil {
		t.Fatal(err)
	}
	inventory := retained.Baseline.Attempts[0].Remote.WorkflowSource.Dependencies
	if inventory == nil || inventory.Counts.CommitPins != 1 || strings.Contains(string(row.Payload), "private_owner") {
		t.Fatal("inventory lost or raw reference retained")
	}
	a.Attempts[0].Remote.WorkflowSource.Dependencies.Counts.CommitPins = 0
	replay, err := c.ReplayExperimentComparison(context.Background(), a.InstallationID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldView, _ := json.Marshal(saved)
	newView, _ := json.Marshal(replay)
	if string(oldView) != string(newView) || strings.Contains(string(newView), "private_owner") || saved.WorkflowEvidence.Baseline.Attempts[0].RootSource.Dependencies == nil {
		t.Fatal("private inventory leaked or changed immutable replay")
	}
}
func TestComparisonAssemblyRefusesUnboundOrContradictoryDependencyStamps(t *testing.T) {
	for _, mode := range []string{"digest", "counts", "version", "position", "fingerprint"} {
		record, a, b := experimentSnapshotFixture()
		experimentDependencyFixture(b.Attempts[0].Remote)
		d := b.Attempts[0].Remote.WorkflowSource.Dependencies
		switch mode {
		case "digest":
			d.SourceSHA256 = strings.Repeat("d", 64)
		case "counts":
			d.Counts.CommitPins = 0
		case "version":
			d.Version = "future"
		case "position":
			d.References[0].StepIndex = -1
		case "fingerprint":
			d.References[0].ReferenceSHA256 = "private"
		}
		if _, err := AssembleExperimentComparison(record, a, b); !errors.Is(err, ErrExperimentEvidence) {
			t.Fatalf("%s accepted: %v", mode, err)
		}
	}
}
