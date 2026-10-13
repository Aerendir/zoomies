package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

func TestRunnerEvidenceViewSeparatesRecordedRequestedAndReportedFacts(t *testing.T) {
	h, inst, j, original := experimentCreationFixture(t, nil)
	if _, err := h.st.StampJobGranted(h.ctx, j.ID, 1, 4096, store.AllocationFromPool); err != nil {
		t.Fatal(err)
	}
	e, err := h.c.CollectExperimentAttempts(h.ctx, inst.ID, j.Repo, []ExperimentAttemptSelection{{10, 1}})
	if err != nil {
		t.Fatal(err)
	}
	record, a, b := experimentSnapshotFixture()
	a.InstallationID, b.InstallationID = inst.ID, inst.ID
	a.Attempts[0] = joinExperimentAttempt(a.Attempts[0].Remote, e.Attempts[0])
	saved, err := h.c.RetainExperimentComparison(h.ctx, &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}, record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	v := saved.RunnerEvidence
	if v == nil || v.Version != experimentRunnerViewVersion || v.Baseline.SelectedJobs != 1 || v.Baseline.RecordedGrants != 1 || v.Baseline.RunnerObservations != 1 || v.Candidate.MatchedJobs != 0 {
		t.Fatal("wrong coverage")
	}
	row := v.Baseline.Jobs[0]
	if *row.RecordedGrant.CPUs != 1 || *row.Observation.Requested.CPUs != 2 || row.Observation.SHA256 != original.SHA256 || row.Observation.ImageDigest == nil {
		t.Fatal("mixed assignment, request or claim")
	}
	*e.Attempts[0].Jobs[0].RecordedGrant.CPUs = 99
	*a.Attempts[0].MatchedJobs[0].RunnerCreation.Data.Issued.CPUs = 88
	if *row.RecordedGrant.CPUs != 1 || *row.Observation.Requested.CPUs != 2 {
		t.Fatal("public view aliases retained inputs")
	}
	payload, _ := json.Marshal(v)
	for _, secret := range []string{"host_id", "pool_id", "runner_id", "task_id", "installation_id", "actor", "private-jit-token", "private-env-token", "registry.example/runner:latest"} {
		if strings.Contains(string(payload), secret) {
			t.Errorf("public projection includes %s", secret)
		}
	}
	if err := h.st.DeleteRunner(h.ctx, j.RunnerID); err != nil {
		t.Fatal(err)
	}
	replay, err := h.c.ReplayExperimentComparison(h.ctx, inst.ID, j.Repo, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(saved)
	after, _ := json.Marshal(replay)
	if string(before) != string(after) {
		t.Fatal("offline projection changed")
	}
}

func TestRunnerEvidenceViewBoundsRowsWithoutTrimmingCoverageCounts(t *testing.T) {
	_, a, _ := experimentSnapshotFixture()
	a.Attempts[0].Remote.Jobs = nil
	grant, _ := experimentRecordedGrant(&store.Job{GrantedSource: store.AllocationFromPool, GrantedMemoryMB: 4096})
	for i := range 101 {
		id := int64(1000 + i)
		a.Attempts[0].Remote.Jobs = append(a.Attempts[0].Remote.Jobs, github.ExperimentAttemptJob{ID: id})
		a.Attempts[0].MatchedJobs = append(a.Attempts[0].MatchedJobs, ExperimentMatchedJob{GitHubJobID: id, FleetJobID: "job_fixture", RecordedGrant: grant})
	}
	v := experimentRunnerArmView(a)
	if v.SelectedJobs != 101 || v.MatchedJobs != 101 || v.RecordedGrants != 101 || len(v.Jobs) != 100 || !v.RowsOmitted || v.Jobs[99].GitHubJobID != 1099 {
		t.Fatal("view hid counts or lost deterministic row bound")
	}
	if v.Jobs[0].RecordedGrant.CPUs != nil || *v.Jobs[0].RecordedGrant.MemoryMB != 4096 || v.Jobs[0].Observation != nil {
		t.Fatal("unknown facts were filled")
	}
}

func TestRunnerEvidenceViewKeepsLargeGitHubIdentitiesExactForBrowsers(t *testing.T) {
	row := ExperimentRunnerEvidenceJob{RunID: 9007199254740993, GitHubJobID: 9223372036854775807, Attempt: 1}
	payload, err := json.Marshal(row)
	if err != nil || !strings.Contains(string(payload), `"run_id":"9007199254740993"`) || !strings.Contains(string(payload), `"github_job_id":"9223372036854775807"`) {
		t.Fatal("browser would round job identity", err)
	}
}

func TestOlderRetainedReportsGainAViewWithoutChangingTheirStoredBytes(t *testing.T) {
	c, record, a, b := retainedComparisonFixture(t)
	saved, err := c.RetainExperimentComparison(t.Context(), &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}, record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := c.st.GetComparisonEvidence(t.Context(), a.InstallationID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored.Payload), "runner_evidence") {
		t.Fatal("response view changed the immutable envelope")
	}
	replay, err := c.ReplayExperimentComparison(t.Context(), a.InstallationID, record.Repository, saved.ID)
	if err != nil || replay.SHA256 != stored.SHA256 || replay.RunnerEvidence.Baseline.Jobs[0].Observation != nil || replay.RunnerEvidence.Baseline.Jobs[0].RecordedGrant != nil {
		t.Fatal("older missing evidence was reconstructed", err)
	}
}
