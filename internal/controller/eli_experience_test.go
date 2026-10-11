package controller

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/assistant"
	"github.com/eyupio/zoomies/internal/prrepair"
	"github.com/eyupio/zoomies/internal/store"
)

func TestAskFirstOffersAreDeduplicatedAndCannotWriteAStaleHead(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	fake := &repairFake{Client: h.gh.Client(inst.Target, inst.TargetType)}
	h.c.clients.entries[inst.ID] = &clientEntry{client: fake, updatedAt: inst.UpdatedAt.Truncate(time.Millisecond)}
	repo := inst.Target + "/repo"
	if err := h.st.SetEliRepairPolicy(h.ctx, store.EliRepairPolicy{Repo: repo, InstallationID: inst.ID, Enabled: true, AskFirst: true, DailyLimit: 5}); err != nil {
		t.Fatal(err)
	}
	user := &store.User{Username: "owner", Role: store.RoleViewer}
	h.st.CreateUser(h.ctx, user)
	h.st.SetEliIdentity(h.ctx, store.EliIdentity{UserID: user.ID, GitHubUserID: 42, GitHubLogin: "octo"})
	h.st.ConfirmEliIdentity(h.ctx, user.ID, 42, true)
	for _, job := range []int{1, 2, 1} {
		body, _ := json.Marshal(map[string]any{"action": "completed", "repository": map[string]any{"full_name": repo}, "workflow_job": map[string]any{"id": job, "run_id": 9, "head_sha": "old", "conclusion": "failure", "status": "completed"}})
		if err := h.c.enqueueRepairWebhook(h.ctx, inst, "workflow_job", body); err != nil {
			t.Fatal(err)
		}
	}
	offers, err := h.c.EliOpportunities(h.ctx, user.ID)
	if err != nil || len(offers) != 1 {
		t.Fatalf("offers %+v %v", offers, err)
	}
	attempts, _ := h.st.ListEliRepairs(h.ctx)
	if len(attempts) != 0 || fake.commits != 0 {
		t.Fatal("offer spent an attempt or wrote")
	}
	fake.noWrite = true
	visible, err := h.c.EliOpportunities(h.ctx, user.ID)
	if err != nil || len(visible) != 0 {
		t.Fatal("revoked access can see offers")
	}
	fake.noWrite = false
	fake.commits = 1
	if _, err := h.c.ActEliOpportunity(h.ctx, user.ID, offers[0].ID, "accept", false); err == nil {
		t.Fatal("stale offer accepted")
	}
	attempts, _ = h.st.ListEliRepairs(h.ctx)
	if len(attempts) != 0 {
		t.Fatal("stale offer spent an attempt")
	}
}

type repairEventProvider struct{ events []assistant.Event }

func (p repairEventProvider) Chat(context.Context, assistant.Request) (assistant.Stream, error) {
	return &repairEventStream{events: p.events}, nil
}
func (p repairEventProvider) Check(context.Context) (assistant.CheckResult, error) {
	return assistant.CheckResult{}, nil
}

type repairEventStream struct{ events []assistant.Event }

func (s *repairEventStream) Next(context.Context) (assistant.Event, bool) {
	if len(s.events) == 0 {
		return assistant.Event{}, false
	}
	e := s.events[0]
	s.events = s.events[1:]
	return e, true
}
func (s *repairEventStream) Close() error { return nil }
func TestRepairUsageIsSavedEvenWhenTheProviderCutsItsAnswer(t *testing.T) {
	var steps []store.EliRepairStep
	m := repairModel{provider: repairEventProvider{events: []assistant.Event{{Usage: &assistant.Usage{InputTokens: 21, OutputTokens: 12, Reported: true}}, {Done: true, Cut: true}}}, record: func(s store.EliRepairStep) { steps = append(steps, s) }}
	_, err := m.Answer(context.Background(), []prrepair.Message{{Role: "user", Content: "evidence"}})
	if err == nil || !strings.Contains(err.Error(), "output limit") || len(steps) != 1 || steps[0].InputTokens != 21 || !steps[0].UsageReported {
		t.Fatalf("steps %+v error %v", steps, err)
	}
	m.provider = repairEventProvider{events: []assistant.Event{{Delta: "{}"}, {Done: true}}}
	if _, err := m.Answer(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if steps[1].UsageReported {
		t.Fatal("missing usage was reported as zero")
	}
}
