package controller

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/analysis"
	"github.com/eyupio/zoomies/internal/store"
)

func TestRecordedGrantsPreserveUnknownDimensionsAndRejectInvalidFigures(t *testing.T) {
	for _, tc := range []struct {
		source         string
		cpu            float64
		mem            int64
		known, invalid bool
	}{
		{store.AllocationFromPool, 2, 8192, true, false}, {store.AllocationFromProfile, 0, 4096, true, false}, {store.AllocationFromHost, 1, 0, true, false},
		{store.AllocationHistory, 0, 0, false, false}, {"", 2, 4096, false, false}, {"future-source", 2, 4096, false, false},
		{store.AllocationReduced, -1, 4096, false, true}, {store.AllocationFromPool, math.NaN(), 4096, false, true}, {store.AllocationFromHost, math.Inf(1), 4096, false, true}, {store.AllocationFromPool, 1, -1, false, true},
	} {
		g, err := experimentRecordedGrant(&store.Job{GrantedSource: tc.source, GrantedCPUs: tc.cpu, GrantedMemoryMB: tc.mem})
		if tc.invalid {
			if !errors.Is(err, ErrExperimentEvidence) {
				t.Errorf("invalid %+v: %v", tc, err)
			}
			continue
		}
		if err != nil || (g != nil) != tc.known {
			t.Errorf("%+v: %+v %v", tc, g, err)
			continue
		}
		if g != nil && (g.Basis != experimentGrantBasis || g.Source != tc.source || (g.CPUs != nil) != (tc.cpu > 0) || (g.MemoryMB != nil) != (tc.mem > 0)) {
			t.Errorf("invented provenance/dimension %+v", g)
		}
	}
}

func TestRetainedGrantCollectionUsesFirstJobStampWithoutReadingCurrentConfiguration(t *testing.T) {
	h := newHarness(t)
	inst := h.installationOn("acme/repo", store.TargetRepo)
	j, err := h.st.UpsertJob(h.ctx, &store.Job{GitHubJobID: 100, GitHubRunID: 10, RunAttempt: 1, Repo: inst.Target, InstallationID: inst.ID, HeadSHA: strings.Repeat("a", 40), QueuedAt: time.Now(), State: store.JobCompleted, Conclusion: "failure"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.StampJobGranted(h.ctx, j.ID, 2, 8192, store.AllocationFromPool); err != nil {
		t.Fatal(err)
	}
	// A later observer cannot substitute today's size for the first assignment.
	if _, err := h.st.StampJobGranted(h.ctx, j.ID, 8, 32768, store.AllocationFromHost); err != nil {
		t.Fatal(err)
	}
	collected, err := h.c.CollectExperimentAttempts(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 1}})
	if err != nil {
		t.Fatal(err)
	}
	g := collected.Attempts[0].Jobs[0].RecordedGrant
	if g == nil || *g.CPUs != 2 || *g.MemoryMB != 8192 || g.Source != store.AllocationFromPool || collected.SelectionComplete {
		t.Fatalf("changed or invented grant %+v", g)
	}
	// No pool, host or runner row exists in this fixture to supply a fallback.
}

func TestMatchedGrantsBindOnlyToPinnedJobIdentityAndAreCopied(t *testing.T) {
	_, a, _ := experimentSnapshotFixture()
	remote := a.Attempts[0].Remote
	g, _ := experimentRecordedGrant(&store.Job{GrantedSource: store.AllocationFromHost, GrantedCPUs: 2, GrantedMemoryMB: 4096})
	retained := ExperimentRetainedAttempt{Coverage: "retained_only", Jobs: []ExperimentRetainedJob{{JobID: "job_one", GitHubJobID: remote.Jobs[0].ID, HeadSHA: remote.HeadSHA, RecordedGrant: g}}}
	joined := joinExperimentAttempt(remote, retained)
	if len(joined.MatchedJobs) != 1 || joined.MatchedJobs[0].RecordedGrant == nil {
		t.Fatal("lost matching grant")
	}
	*g.CPUs = 8
	if *joined.MatchedJobs[0].RecordedGrant.CPUs != 2 {
		t.Fatal("retained pointer mutated joined evidence")
	}
	retained.Jobs[0].HeadSHA = strings.Repeat("b", 40)
	joined = joinExperimentAttempt(remote, retained)
	if len(joined.MatchedJobs) != 0 || len(joined.MissingJobIDs) != 1 {
		t.Fatal("attached grant to another revision")
	}
}

func TestRecordedGrantsRetainOfflineWithoutPromotingComparisonEligibility(t *testing.T) {
	c, record, a, b := retainedComparisonFixture(t)
	g, _ := experimentRecordedGrant(&store.Job{GrantedSource: store.AllocationFromPool, GrantedCPUs: 2, GrantedMemoryMB: 8192})
	a.Attempts[0].MatchedJobs = []ExperimentMatchedJob{{GitHubJobID: a.Attempts[0].Remote.Jobs[0].ID, FleetJobID: "job_one", RecordedGrant: g}}
	ctx := context.Background()
	saved, err := c.RetainExperimentComparison(ctx, &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}, record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	row, err := c.st.GetComparisonEvidence(ctx, a.InstallationID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var envelope experimentRetainedEnvelope
	if err := json.Unmarshal(row.Payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if kept := envelope.Baseline.Attempts[0].MatchedJobs[0].RecordedGrant; kept == nil || *kept.CPUs != 2 || *kept.MemoryMB != 8192 {
		t.Fatal("grant not durably retained")
	}
	*g.CPUs = 99
	replay, err := c.ReplayExperimentComparison(ctx, a.InstallationID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	s := replay.Report.Snapshot
	m := replay.Report.Measurement
	if s.Baseline.ResourceID != "" || s.Baseline.ImageID != "" || s.Baseline.ConfigurationID != "" || s.Baseline.SelectionComplete || m.Eligibility.State != "inconclusive" || m.ExecutionChange != analysis.NotEnough {
		t.Fatal("grant manufactured controlled equivalence")
	}
	before, _ := json.Marshal(saved.Report)
	after, _ := json.Marshal(replay.Report)
	if string(before) != string(after) {
		t.Fatal("offline replay changed")
	}
}

func TestReplayAssemblyRefusesInvalidGrantProvenance(t *testing.T) {
	for _, mode := range []string{"basis", "source", "missing", "zero", "identity", "duplicate"} {
		record, a, b := experimentSnapshotFixture()
		g, _ := experimentRecordedGrant(&store.Job{GrantedSource: store.AllocationFromPool, GrantedCPUs: 2, GrantedMemoryMB: 4096})
		j := ExperimentMatchedJob{GitHubJobID: a.Attempts[0].Remote.Jobs[0].ID, FleetJobID: "job_one", RecordedGrant: g}
		switch mode {
		case "basis":
			g.Basis = "future"
		case "source":
			g.Source = "future"
		case "missing":
			g.CPUs = nil
			g.MemoryMB = nil
		case "zero":
			*g.CPUs = 0
		case "identity":
			j.GitHubJobID++
		case "duplicate":
			a.Attempts[0].MatchedJobs = append(a.Attempts[0].MatchedJobs, j)
		}
		a.Attempts[0].MatchedJobs = append(a.Attempts[0].MatchedJobs, j)
		if _, err := AssembleExperimentComparison(record, a, b); !errors.Is(err, ErrExperimentEvidence) {
			t.Errorf("accepted %s: %v", mode, err)
		}
	}
}
