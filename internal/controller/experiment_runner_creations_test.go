package controller

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/analysis"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

func experimentCreationFixture(t *testing.T, clock func() time.Time) (*harness, *store.Installation, *store.Job, *store.RunnerCreationObservation) {
	t.Helper()
	h := newHarnessAt(t, clock)
	inst, pool, host := h.fleet()
	r := h.runnerRow(pool, host, store.RunnerProvisioning)
	task, result := creationProjectionFixture()
	task.RunnerID, task.PoolID = r.ID, pool.ID
	task.Spec.RunnerID, task.Spec.PoolID = r.ID, pool.ID
	result.RunnerID = r.ID
	d, err := buildRunnerCreationObservation(host.ID, task, result)
	if err != nil || d == nil {
		t.Fatalf("projection %v %v", d, err)
	}
	o, err := h.st.PutRunnerCreationObservation(h.ctx, inst.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	j, err := h.st.UpsertJob(h.ctx, &store.Job{GitHubJobID: 110, GitHubRunID: 10, RunAttempt: 1, Repo: "acme/repo", InstallationID: inst.ID, PoolID: pool.ID, RunnerID: r.ID, HostID: host.ID, HeadSHA: strings.Repeat("a", 40), State: store.JobCompleted, Conclusion: "failure"})
	if err != nil {
		t.Fatal(err)
	}
	return h, inst, j, o
}

func TestComparisonCaptureRetainsRunnerCreationUnderTheExistingSharedAdmission(t *testing.T) {
	h, inst, j, original := experimentCreationFixture(t, nil)
	record, a, _ := experimentSnapshotFixture()
	f := &experimentReaderFixture{Client: h.gh.Client(inst.Target, inst.TargetType), evidence: a.Attempts[0].Remote, dynamicIdentity: true, quota: &github.RateLimit{Limit: 5000, Remaining: 5000}}
	h.c.clients.entries[inst.ID] = &clientEntry{client: f, updatedAt: inst.UpdatedAt.Truncate(time.Millisecond)}
	saved, err := h.c.CaptureExperimentComparison(h.ctx, &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}, record, inst.ID, []ExperimentAttemptSelection{{10, 1}}, []ExperimentAttemptSelection{{11, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if f.calls != 2 || f.quotaCalls != 1 || !f.deadline {
		t.Fatal("observation join changed remote admission")
	}
	row, err := h.st.GetComparisonEvidence(h.ctx, inst.ID, j.Repo, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var envelope experimentRetainedEnvelope
	if err := json.Unmarshal(row.Payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Baseline.Attempts[0].MatchedJobs[0].RunnerCreation.SHA256 != original.SHA256 || len(envelope.Candidate.Attempts[0].MatchedJobs) != 0 {
		t.Fatal("capture lost or borrowed runner evidence")
	}
	for _, secret := range []string{"private-jit-token", "private-env-token", "private-handle", "registry.example/runner:latest"} {
		if strings.Contains(string(row.Payload), secret) {
			t.Errorf("private comparison retained forbidden value %q", secret)
		}
	}
	// A fresh outer digest cannot hide an altered source observation.
	row.ID = store.NewID(store.PrefixComparison)
	envelope.Record.EvidenceID = row.ID
	envelope.Report.Snapshot.EvidenceID = row.ID
	*envelope.Baseline.Attempts[0].MatchedJobs[0].RunnerCreation.Data.Issued.CPUs = 8
	row.Payload, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.st.PutComparisonEvidence(h.ctx, row, &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.ReplayExperimentComparison(h.ctx, inst.ID, j.Repo, row.ID); !errors.Is(err, ErrExperimentEvidence) {
		t.Fatal("altered observation replayed with a fresh outer digest", err)
	}
}

func TestComparisonRunnerCreationSurvivesPruningAndReplaysAfterObservationExpiry(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	h, inst, j, original := experimentCreationFixture(t, func() time.Time { return now })
	if err := h.st.DeleteRunner(h.ctx, j.RunnerID); err != nil {
		t.Fatal(err)
	}
	collected, err := h.c.CollectExperimentAttempts(h.ctx, inst.ID, j.Repo, []ExperimentAttemptSelection{{10, 1}})
	if err != nil {
		t.Fatal(err)
	}
	record, a, b := experimentSnapshotFixture()
	a.InstallationID, b.InstallationID = inst.ID, inst.ID
	a.Attempts[0] = joinExperimentAttempt(a.Attempts[0].Remote, collected.Attempts[0])
	kept := a.Attempts[0].MatchedJobs[0].RunnerCreation
	if kept == nil || kept.SHA256 != original.SHA256 || kept.Data.RunnerID != j.RunnerID {
		t.Fatal("lost pruned-runner evidence")
	}
	// A comparison has its own retention clock; expiry of the source observation
	// cannot change the report or require a current runner lookup during replay.
	now = now.Add(29 * 24 * time.Hour)
	saved, err := h.c.RetainExperimentComparison(h.ctx, &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}, record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	row, err := h.st.GetComparisonEvidence(h.ctx, inst.ID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var envelope experimentRetainedEnvelope
	if err := json.Unmarshal(row.Payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Baseline.Attempts[0].MatchedJobs[0].RunnerCreation.SHA256 != original.SHA256 {
		t.Fatal("observation not retained privately")
	}
	*kept.Data.Issued.CPUs = 99
	now = now.Add(2 * 24 * time.Hour)
	if _, err := h.st.GetRunnerCreationObservation(h.ctx, inst.ID, j.RunnerID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("observation did not expire", err)
	}
	replay, err := h.c.ReplayExperimentComparison(h.ctx, inst.ID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(saved.Report)
	after, _ := json.Marshal(replay.Report)
	if string(before) != string(after) || strings.Contains(string(after), "runner_creation") || replay.Report.Snapshot.Baseline.ResourceID != "" || replay.Report.Snapshot.Baseline.ImageID != "" || replay.Report.Snapshot.Baseline.ConfigurationID != "" || replay.Report.Snapshot.Baseline.SelectionComplete || replay.Report.Measurement.Eligibility.State != "inconclusive" || replay.Report.Measurement.ExecutionChange != analysis.NotEnough {
		t.Fatal("private observation changed the public report or eligibility")
	}
	collected, err = h.c.CollectExperimentAttempts(h.ctx, inst.ID, j.Repo, []ExperimentAttemptSelection{{10, 1}})
	if err != nil || collected.Attempts[0].Jobs[0].RunnerCreation != nil || collected.Attempts[0].Jobs[0].RunnerID != "" {
		t.Fatal("expired observation was reconstructed", err)
	}
}

func TestComparisonRunnerCreationRequiresScopedHistoricalJobLinks(t *testing.T) {
	h, inst, j, _ := experimentCreationFixture(t, nil)
	for _, mode := range []string{"missing_runner", "missing_pool", "unknown_runner", "foreign_scope", "wrong_pool", "wrong_host", "unknown_host"} {
		copy := *j
		scope := inst.ID
		switch mode {
		case "missing_runner":
			copy.RunnerID = ""
		case "missing_pool":
			copy.PoolID = ""
		case "unknown_runner":
			copy.RunnerID = "run_missing"
		case "foreign_scope":
			scope = "ins_foreign"
		case "wrong_pool":
			copy.PoolID = "pool_other"
		case "wrong_host":
			copy.HostID = "host_other"
		case "unknown_host":
			copy.HostID = ""
		}
		o, err := h.c.experimentRunnerCreation(h.ctx, scope, &copy)
		switch mode {
		case "wrong_pool", "wrong_host":
			if !errors.Is(err, ErrExperimentEvidence) {
				t.Errorf("accepted %s: %v", mode, err)
			}
		case "unknown_host":
			if err != nil || o == nil {
				t.Errorf("lost historical link without host stamp: %v", err)
			}
		default:
			if err != nil || o != nil {
				t.Errorf("invented %s evidence: %v", mode, err)
			}
		}
	}
}

func TestComparisonRunnerCreationJoinCopiesPointersAndRejectsAnotherRevision(t *testing.T) {
	h, inst, j, _ := experimentCreationFixture(t, nil)
	collected, err := h.c.CollectExperimentAttempts(h.ctx, inst.ID, j.Repo, []ExperimentAttemptSelection{{10, 1}})
	if err != nil {
		t.Fatal(err)
	}
	_, a, _ := experimentSnapshotFixture()
	joined := joinExperimentAttempt(a.Attempts[0].Remote, collected.Attempts[0])
	input := collected.Attempts[0].Jobs[0].RunnerCreation
	*input.Data.Issued.CPUs = 99
	*input.Data.Reported.ImageDigest = "changed"
	*input.Data.Reported.CompletedAt = time.Time{}
	if *joined.MatchedJobs[0].RunnerCreation.Data.Issued.CPUs != 2 || !store.ValidRetainedRunnerCreationObservation(joined.MatchedJobs[0].RunnerCreation) {
		t.Fatal("input pointers changed joined evidence")
	}
	collected.Attempts[0].Jobs[0].HeadSHA = strings.Repeat("b", 40)
	joined = joinExperimentAttempt(a.Attempts[0].Remote, collected.Attempts[0])
	if len(joined.MatchedJobs) != 0 {
		t.Fatal("attached observation to another revision")
	}
}

func TestComparisonRunnerCreationAssemblyRefusesAlteredOrDuplicatedProvenance(t *testing.T) {
	h, inst, j, original := experimentCreationFixture(t, nil)
	for _, mode := range []string{"scope", "runner", "pool", "host", "hash", "receipt", "version", "requested_size", "remote_job", "fleet_job", "duplicate", "duplicate_without_observation", "cross_arm_runner"} {
		record, a, b := experimentSnapshotFixture()
		a.InstallationID, b.InstallationID = inst.ID, inst.ID
		matched := ExperimentMatchedJob{GitHubJobID: j.GitHubJobID, FleetJobID: j.ID, RunnerID: j.RunnerID, PoolID: j.PoolID, HostID: j.HostID, RunnerCreation: cloneExperimentRunnerCreation(original)}
		switch mode {
		case "scope":
			matched.RunnerCreation.InstallationID = "ins_foreign"
		case "runner":
			matched.RunnerID = "run_other"
		case "pool":
			matched.PoolID = "pool_other"
		case "host":
			matched.HostID = "host_other"
		case "hash":
			matched.RunnerCreation.SHA256 = strings.Repeat("0", 64)
		case "receipt":
			matched.RunnerCreation.ReceivedAt = time.Time{}
		case "version":
			matched.RunnerCreation.Data.Version = "future"
		case "requested_size":
			*matched.RunnerCreation.Data.Issued.CPUs = 8
		case "remote_job":
			matched.GitHubJobID = 999
		case "fleet_job":
			matched.FleetJobID = ""
		case "duplicate":
			a.Attempts[0].MatchedJobs = append(a.Attempts[0].MatchedJobs, matched)
		case "duplicate_without_observation":
			other := matched
			other.RunnerCreation = nil
			a.Attempts[0].MatchedJobs = append(a.Attempts[0].MatchedJobs, other)
		case "cross_arm_runner":
			other := matched
			other.GitHubJobID = b.Attempts[0].Remote.Jobs[0].ID
			other.FleetJobID = "job_other"
			b.Attempts[0].MatchedJobs = []ExperimentMatchedJob{other}
		}
		a.Attempts[0].MatchedJobs = append(a.Attempts[0].MatchedJobs, matched)
		if _, err := h.c.RetainExperimentComparison(h.ctx, &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}, record, a, b); !errors.Is(err, ErrExperimentEvidence) {
			t.Errorf("accepted %s: %v", mode, err)
		}
	}
}
