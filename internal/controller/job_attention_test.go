package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/assistant"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

type attentionFake struct {
	*repairFake
	pullErr   error
	checksErr error
	runState  github.WorkflowRunState
	runErr    error
}

func (f attentionFake) GetWorkflowRun(context.Context, string, int64) (*github.WorkflowRunState, error) {
	return &f.runState, f.runErr
}
func (f attentionFake) RepairPullForRun(ctx context.Context, repo string, run int64) (github.RepairPull, error) {
	if f.pullErr != nil {
		return github.RepairPull{}, f.pullErr
	}
	return f.repairFake.RepairPullForRun(ctx, repo, run)
}
func (f attentionFake) RepairChecks(ctx context.Context, repo, sha string) (string, error) {
	if f.checksErr != nil {
		return "", f.checksErr
	}
	return f.repairFake.RepairChecks(ctx, repo, sha)
}

func TestAttentionRechecksGitHubWithoutPersonalRepairSetup(t *testing.T) {
	for _, tc := range []struct {
		name, checks, runConclusion, reason string
		pullErr, checksErr, runErr          error
		known                               bool
	}{
		{name: "still failed", checks: "failed", known: true},
		{name: "passed checks", checks: "passed", reason: "fixed", known: true},
		{name: "passed push run", runConclusion: "success", pullErr: github.ErrRepairUnsupported, reason: "fixed", known: true},
		{name: "closed PR", pullErr: &github.RepairTargetChanged{Code: "closed"}, reason: "closed", known: true},
		{name: "newer commit", pullErr: &github.RepairTargetChanged{Code: "superseded"}, reason: "superseded", known: true},
		{name: "pending checks", checks: "waiting"},
		{name: "cancelled run", checks: "waiting", runConclusion: "cancelled"},
		{name: "push without PR", pullErr: github.ErrRepairUnsupported},
		{name: "checks unavailable", checksErr: errors.New("GitHub unavailable")},
		{name: "run unavailable", runErr: errors.New("GitHub unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			inst := h.installation()
			fake := attentionFake{repairFake: &repairFake{Client: h.gh.Client(inst.Target, inst.TargetType), check: tc.checks}, pullErr: tc.pullErr, checksErr: tc.checksErr, runErr: tc.runErr, runState: github.WorkflowRunState{Status: "completed", Conclusion: tc.runConclusion}}
			h.c.clients.entries[inst.ID] = &clientEntry{client: fake, updatedAt: inst.UpdatedAt.Truncate(time.Millisecond)}
			repo := inst.Target + "/repo"
			reason, known := h.c.jobAttentionOnGitHub(h.ctx, repo, 9)
			if reason != tc.reason || known != tc.known {
				t.Fatalf("reason %q, known %v; want %q, %v", reason, known, tc.reason, tc.known)
			}
			job := &store.Job{GitHubJobID: 1, GitHubRunID: 9, Repo: repo, State: store.JobCompleted, Conclusion: "failure", QueuedAt: h.st.Now().Add(-time.Minute)}
			if _, err := h.st.UpsertJob(h.ctx, job); err != nil {
				t.Fatal(err)
			}
			h.c.refreshJobAttention(h.ctx)
			_, count, err := h.st.ListJobs(h.ctx, store.JobFilter{AttentionOnly: true}, store.Page{})
			want := 1
			if tc.reason != "" {
				want = 0
			}
			if err != nil || count != want {
				t.Fatalf("attention count %d, error %v; want %d", count, err, want)
			}
		})
	}
}

func TestRepairRequestRechecksBeforeSpendingAnAttempt(t *testing.T) {
	for _, state := range []string{"passed", "waiting"} {
		t.Run(state, func(t *testing.T) {
			h := newHarness(t)
			inst := h.installation()
			fake := &repairFake{Client: h.gh.Client(inst.Target, inst.TargetType), check: state}
			h.c.clients.entries[inst.ID] = &clientEntry{client: fake, updatedAt: inst.UpdatedAt.Truncate(time.Millisecond)}
			repo := inst.Target + "/repo"
			if err := h.st.SetEliRepairPolicy(h.ctx, store.EliRepairPolicy{Repo: repo, InstallationID: inst.ID, Enabled: true, DailyLimit: 5}); err != nil {
				t.Fatal(err)
			}
			user := &store.User{Username: "owner", Role: store.RoleViewer}
			if err := h.st.CreateUser(h.ctx, user); err != nil {
				t.Fatal(err)
			}
			if err := h.st.SetEliIdentity(h.ctx, store.EliIdentity{UserID: user.ID, GitHubUserID: 42, GitHubLogin: "octo"}); err != nil {
				t.Fatal(err)
			}
			if err := h.st.ConfirmEliIdentity(h.ctx, user.ID, 42, true); err != nil {
				t.Fatal(err)
			}
			_, err := h.c.RequestPinnedEliRepair(h.ctx, user.ID, repo, 1, "", "old", false)
			var changed *github.RepairTargetChanged
			if !errors.As(err, &changed) {
				t.Fatalf("missing changed-target explanation: %v", err)
			}
			rows, _, err := h.st.ListEliRepairsPage(h.ctx, store.EliRepairFilter{Limit: store.MaxEliRepairPage})
			if err != nil || len(rows) != 0 || fake.commits != 0 {
				t.Fatalf("a settled or pending target consumed an attempt: %+v, %v", rows, err)
			}
			ready := h.c.EliReadiness(h.ctx, user.ID, repo, 1, 0)
			if ready.Ready {
				t.Fatal("a repair was offered without failed checks")
			}
			wantCode := "checks_pending"
			if state == "passed" {
				wantCode = "fixed"
			}
			for _, blocker := range ready.Blockers {
				if blocker.Code == wantCode {
					return
				}
			}
			t.Fatalf("missing current check outcome: %+v", ready.Blockers)
		})
	}
}

func TestQueuedRepairStopsBeforeOpeningAProviderWhenChecksAlreadyPassed(t *testing.T) {
	s := newCommandSetup(t, 5)
	s.link(t, true)
	s.fake.check = "passed"
	row := &store.EliRepair{DedupKey: "queued-fixed", Repo: s.repo, InstallationID: s.inst.ID, PullNumber: 1, HeadSHA: "old", Trigger: "mention", UserID: s.user.ID, GitHubUserID: 42, GitHubLogin: "octo"}
	if _, err := s.h.st.EnqueueEliRepair(s.h.ctx, row, 5); err != nil {
		t.Fatal(err)
	}
	s.h.c.runEliRepair(s.h.ctx, row)
	rows := s.rows(t)
	if len(rows) != 1 || rows[0].State != "superseded" || s.fake.commits != 0 {
		t.Fatalf("the already fixed target was repaired: %+v", rows)
	}
}

func TestRepairDoesNotPublishWhenChecksPassWhileTheModelWorks(t *testing.T) {
	s := newCommandSetup(t, 5)
	s.link(t, true)
	s.fake.check = "failed"
	personal := &store.AssistantProvider{Name: "Personal", OwnerID: s.user.ID, Kind: "openai", Enabled: true}
	if err := s.h.st.CreateAssistantProvider(s.h.ctx, personal); err != nil {
		t.Fatal(err)
	}
	s.fake.onRead = func() { s.fake.check = "passed" }
	s.h.c.openProvider = func(*store.AssistantProvider, string) (assistant.Provider, error) {
		return assistant.NewFake(assistant.FakeOptions{Script: func(assistant.Request) []assistant.Event {
			return []assistant.Event{{Delta: `{"summary":"Fix cause","files":[{"path":"main.go","content":"fixed"}]}`}}
		}}), nil
	}
	row := &store.EliRepair{DedupKey: "fixed-during-model", Repo: s.repo, InstallationID: s.inst.ID, PullNumber: 1, HeadSHA: "old", Trigger: "mention", UserID: s.user.ID, GitHubUserID: 42, GitHubLogin: "octo"}
	if _, err := s.h.st.EnqueueEliRepair(s.h.ctx, row, 5); err != nil {
		t.Fatal(err)
	}
	s.h.c.runEliRepair(s.h.ctx, row)
	if row.State != "superseded" || s.fake.commits != 0 {
		t.Fatalf("published over passing checks: %+v", row)
	}
}

func TestPendingChecksDoNotUndoAnObservedFix(t *testing.T) {
	now := time.Now().UTC()
	h := newHarnessAt(t, func() time.Time { return now })
	inst := h.installation()
	fake := &repairFake{Client: h.gh.Client(inst.Target, inst.TargetType), check: "waiting"}
	h.c.clients.entries[inst.ID] = &clientEntry{client: attentionFake{repairFake: fake}, updatedAt: inst.UpdatedAt.Truncate(time.Millisecond)}
	repo := inst.Target + "/repo"
	job := &store.Job{GitHubJobID: 1, GitHubRunID: 9, Repo: repo, State: store.JobCompleted, Conclusion: "failure", QueuedAt: h.st.Now().Add(-time.Hour)}
	if _, err := h.st.UpsertJob(h.ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := h.st.RecordJobAttentionRun(h.ctx, repo, 9, "fixed", h.st.Now(), true); err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Minute)
	h.c.refreshJobAttention(h.ctx)
	_, count, err := h.st.ListJobs(h.ctx, store.JobFilter{AttentionOnly: true}, store.Page{})
	if err != nil || count != 0 {
		t.Fatalf("pending checks resurrected an already fixed failure: %d, %v", count, err)
	}
	fake.check = "failed"
	now = now.Add(6 * time.Minute)
	h.c.refreshJobAttention(h.ctx)
	_, count, err = h.st.ListJobs(h.ctx, store.JobFilter{AttentionOnly: true}, store.Page{})
	if err != nil || count != 1 {
		t.Fatalf("a confirmed failure did not return to the inbox: %d, %v", count, err)
	}
}
