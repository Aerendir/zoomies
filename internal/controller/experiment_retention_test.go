package controller

import (
	"context"
	"encoding/json"
	"errors"
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
