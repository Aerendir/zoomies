package controller

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

func TestExperimentCollectionPreservesFailuresAndCannotAssertFullCoverage(t *testing.T) {
	h := newHarness(t)
	inst := h.installationOn("acme/repo", store.TargetRepo)
	for i, outcome := range []string{"failure", "success", "cancelled"} {
		_, err := h.st.UpsertJob(h.ctx, &store.Job{GitHubJobID: int64(100 + i), GitHubRunID: 10, RunAttempt: 1, InstallationID: inst.ID, Repo: inst.Target, QueuedAt: time.Now(), State: store.JobCompleted, Conclusion: outcome, JobName: "private workflow text"})
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := h.c.CollectExperimentAttempts(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{RunID: 10, Attempt: 1}, {RunID: 10, Attempt: 2}})
	if err != nil || r.SelectionComplete || len(r.Attempts) != 2 || len(r.Attempts[0].Jobs) != 3 || len(r.Attempts[1].Jobs) != 0 || r.Attempts[1].Coverage != "not_available" {
		t.Fatalf("got %+v, %v", r, err)
	}
	payload, err := json.Marshal(r)
	if err != nil || strings.Contains(string(payload), "private workflow text") {
		t.Fatalf("workflow text leaked: %s, %v", payload, err)
	}
	if r.Attempts[0].Coverage != "retained_only" || r.Attempts[0].Jobs[0].Conclusion != "failure" {
		t.Fatalf("got %+v", r)
	}
}

func TestExperimentCollectionRefusesCrossRepositoryAndDuplicateSelections(t *testing.T) {
	h := newHarness(t)
	inst := h.installationOn("acme/repo", store.TargetRepo)
	for _, tc := range []struct {
		repo     string
		selected []ExperimentAttemptSelection
	}{{"other/repo", []ExperimentAttemptSelection{{1, 1}}}, {inst.Target, []ExperimentAttemptSelection{{1, 1}, {1, 1}}}, {inst.Target, nil}, {inst.Target, []ExperimentAttemptSelection{{0, 1}}}, {inst.Target, make([]ExperimentAttemptSelection, 101)}} {
		_, err := h.c.CollectExperimentAttempts(h.ctx, inst.ID, tc.repo, tc.selected)
		if !errors.Is(err, ErrExperimentEvidence) {
			t.Fatalf("accepted %+v: %v", tc, err)
		}
	}
	org := h.installationOn("acme", store.TargetOrg)
	r, err := h.c.CollectExperimentAttempts(h.ctx, org.ID, "acme/other", []ExperimentAttemptSelection{{1, 1}})
	if err != nil || r.SelectionComplete {
		t.Fatalf("org report %+v, %v", r, err)
	}
}

func TestExperimentCollectionNamesPerAttemptAndWholeSelectionBounds(t *testing.T) {
	h := newHarness(t)
	inst := h.installationOn("acme/repo", store.TargetRepo)
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	for i := range 2002 {
		run := int64(10)
		if i >= 1001 {
			run = 11
		}
		if i == 2001 {
			run = 12
		}
		_, err := h.st.UpsertJob(h.ctx, &store.Job{GitHubJobID: int64(10000 + i), GitHubRunID: run, RunAttempt: 1, Repo: inst.Target, InstallationID: inst.ID, QueuedAt: now, State: store.JobCompleted, Conclusion: "failure"})
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := h.c.CollectExperimentAttempts(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 1}, {11, 1}, {12, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Attempts[0].Coverage != "truncated" || len(r.Attempts[0].Jobs) != 1000 || r.Attempts[1].Coverage != "retained_only" || len(r.Attempts[1].Jobs) != 1000 || r.Attempts[2].Coverage != "held_back" || len(r.Attempts[2].Jobs) != 0 || r.SelectionComplete {
		t.Fatalf("bounds not retained: %+v", r)
	}
}
