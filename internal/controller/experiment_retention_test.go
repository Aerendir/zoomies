package controller

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eyupio/zoomies/internal/github"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

func retainedComparisonFixture(t *testing.T) (*Controller, ExperimentComparisonRecord, *ExperimentJoinedEvidence, *ExperimentJoinedEvidence) {
	t.Helper()
	s, err := store.Open(context.Background(), store.Options{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	record, a, b := experimentSnapshotFixture()
	inst := &store.Installation{AppID: 1, InstallationID: 2, Target: record.Repository, TargetType: store.TargetRepo}
	if err := s.CreateInstallation(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
	a.InstallationID = inst.ID
	b.InstallationID = inst.ID
	return &Controller{st: s}, record, a, b
}
func TestAStoredComparisonReplaysWithoutGitHubOrCurrentPoolSettings(t *testing.T) {
	c, record, a, b := retainedComparisonFixture(t)
	actor := &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user", ActorName: "operator"}
	saved, err := c.RetainExperimentComparison(context.Background(), actor, record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := c.ReplayExperimentComparison(context.Background(), a.InstallationID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(saved.Report)
	after, _ := json.Marshal(replay.Report)
	if string(before) != string(after) || replay.Report.Snapshot.InitiatingUser != actor.ActorID || replay.Report.Snapshot.EvidenceID != saved.ID || saved.ID == record.EvidenceID {
		t.Fatal("replay or actor identity changed")
	}
}
func TestReplayRefusesAnAlteredReportEvenWithAValidStorageDigest(t *testing.T) {
	c, record, a, b := retainedComparisonFixture(t)
	actor := &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}
	saved, err := c.RetainExperimentComparison(context.Background(), actor, record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	row, err := c.st.GetComparisonEvidence(context.Background(), a.InstallationID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var envelope experimentRetainedEnvelope
	if err := json.Unmarshal(row.Payload, &envelope); err != nil {
		t.Fatal(err)
	}
	id := store.NewID(store.PrefixComparison)
	envelope.Record.EvidenceID = id
	envelope.Report.Snapshot.EvidenceID = id
	envelope.Report.Measurement.Candidate.Execution.TotalMS = nil
	payload, _ := json.Marshal(envelope)
	row.ID = id
	row.Payload = payload
	if err := c.st.PutComparisonEvidence(context.Background(), row, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReplayExperimentComparison(context.Background(), row.InstallationID, row.Repository, id); !errors.Is(err, ErrExperimentEvidence) {
		t.Fatal("altered result accepted", err)
	}
}

func TestRetainedTriggerContextReplaysPrivatelyWithoutChangingTheReport(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "with_context", true: "legacy"}[legacy], func(t *testing.T) {
			c, record, a, b := retainedComparisonFixture(t)
			original, err := AssembleExperimentComparison(record, a, b)
			if err != nil {
				t.Fatal(err)
			}
			if !legacy {
				a.Attempts[0].Remote.Event = "workflow_dispatch"
				a.Attempts[0].Remote.WorkflowPath = ".github/workflows/private-baseline.yml@refs/heads/main"
				b.Attempts[0].Remote.Event = "push"
				b.Attempts[0].Remote.WorkflowPath = ".github/workflows/private-candidate.yml"
			}
			withContext, err := AssembleExperimentComparison(record, a, b)
			if err != nil {
				t.Fatal(err)
			}
			oldReport, _ := json.Marshal(original)
			newReport, _ := json.Marshal(withContext)
			if string(oldReport) != string(newReport) {
				t.Fatal("descriptive context changed eligibility or measurements")
			}
			actor := &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}
			saved, err := c.RetainExperimentComparison(context.Background(), actor, record, a, b)
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
			if retained.Baseline.Attempts[0].Remote.Event != a.Attempts[0].Remote.Event || retained.Candidate.Attempts[0].Remote.WorkflowPath != b.Attempts[0].Remote.WorkflowPath {
				t.Fatal("context lost during retention")
			}
			if legacy && (strings.Contains(string(row.Payload), `"event"`) || strings.Contains(string(row.Payload), `"workflow_path"`)) {
				t.Fatal("legacy bytes gained context")
			}
			a.Attempts[0].Remote.Event = "changed_after_capture"
			replay, err := c.ReplayExperimentComparison(context.Background(), a.InstallationID, record.Repository, saved.ID)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(saved)
			after, _ := json.Marshal(replay)
			if string(before) != string(after) || strings.Contains(string(after), "private-baseline") || strings.Contains(string(after), "workflow_dispatch") {
				t.Fatal("private context leaked or mutable source affected replay")
			}
		})
	}
}

func TestComparisonAssemblyRefusesInvalidContextFromAlternateReaders(t *testing.T) {
	for _, context := range []github.ExperimentAttemptEvidence{
		{Event: "push\nprivate"}, {Event: strings.Repeat("a", 65)},
		{WorkflowPath: strings.Repeat("x", 1025)}, {WorkflowPath: "file\x00"},
		{WorkflowPath: string([]byte{0xff})},
	} {
		record, a, b := experimentSnapshotFixture()
		b.Attempts[0].Remote.Event = context.Event
		b.Attempts[0].Remote.WorkflowPath = context.WorkflowPath
		if _, err := AssembleExperimentComparison(record, a, b); !errors.Is(err, ErrExperimentEvidence) {
			t.Fatalf("invalid context accepted: %v", err)
		}
	}
}
