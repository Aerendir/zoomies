package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/assistant"
	"github.com/eyupio/zoomies/internal/github"
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
	attempts, _, _ := h.st.ListEliRepairsPage(h.ctx, store.EliRepairFilter{Limit: store.MaxEliRepairPage})
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
	attempts, _, _ = h.st.ListEliRepairsPage(h.ctx, store.EliRepairFilter{Limit: store.MaxEliRepairPage})
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
	// An explicit limit, so that a cut is the end of the attempt: an automatic one asks again.
	m := repairModel{provider: repairEventProvider{events: []assistant.Event{{Usage: &assistant.Usage{InputTokens: 21, OutputTokens: 12, Reported: true}}, {Done: true, Cut: true}}}, maxTokens: 12000, record: func(s store.EliRepairStep) { steps = append(steps, s) }}
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

// roomProvider answers like a model that needs a given amount of room: asked
// for less it is cut at the ceiling, and asked for that much or more it
// answers. It records every ceiling it was asked for, and fails a request when
// refuse says to, so a test can watch the room a repair asks for.
type roomProvider struct {
	needs  int
	asked  *[]int
	refuse func(limit int) error
}

func (p roomProvider) Chat(_ context.Context, req assistant.Request) (assistant.Stream, error) {
	*p.asked = append(*p.asked, req.MaxTokens)
	if p.refuse != nil {
		if err := p.refuse(req.MaxTokens); err != nil {
			return nil, err
		}
	}
	if req.MaxTokens < p.needs {
		return &repairEventStream{events: []assistant.Event{{Usage: &assistant.Usage{InputTokens: 100, OutputTokens: req.MaxTokens, Reported: true}}, {Done: true, Cut: true}}}, nil
	}
	return &repairEventStream{events: []assistant.Event{{Delta: "{}"}, {Done: true}}}, nil
}
func (p roomProvider) Check(context.Context) (assistant.CheckResult, error) {
	return assistant.CheckResult{}, nil
}

// statusFailure is a provider answering with an HTTP status, which is how the
// adapters' errors carry it.
type statusFailure int

func (s statusFailure) Error() string   { return fmt.Sprintf("the provider answered %d", int(s)) }
func (s statusFailure) HTTPStatus() int { return int(s) }

// A thinking model can spend all of a repair's 12000 tokens on reasoning, and a
// repair that failed there could only be rescued by a person editing the
// provider and starting another attempt. An automatic one asks again with room
// enough, and keeps both attempts in its history.
func TestAnAutomaticRepairRaisesItsOwnOutputLimitWhenTheModelRunsOutOfRoom(t *testing.T) {
	var asked []int
	var steps []store.EliRepairStep
	m := &repairModel{provider: roomProvider{needs: 20000, asked: &asked}, record: func(s store.EliRepairStep) { steps = append(steps, s) }}
	got, err := m.Answer(context.Background(), []prrepair.Message{{Role: "user", Content: "evidence"}})
	if err != nil || got != "{}" {
		t.Fatalf("answer %q error %v", got, err)
	}
	// A repair starts at 12000, and twice that is 24000, which is enough.
	if !slices.Equal(asked, []int{12000, 24000}) {
		t.Fatalf("asked for %v", asked)
	}
	// The cut attempt keeps what the provider counted for it. The one after it
	// reported nothing, which is not zero.
	if len(steps) != 2 || !steps[0].UsageReported || steps[0].OutputTokens != 12000 || steps[1].UsageReported {
		t.Fatalf("steps %+v", steps)
	}
}

// Only the step that needs more room should pay to find out how much: the steps
// after it are the same model reading the same evidence.
func TestARepairKeepsTheRoomItFoundForItsNextStep(t *testing.T) {
	var asked []int
	m := &repairModel{provider: roomProvider{needs: 20000, asked: &asked}}
	for range 2 {
		if _, err := m.Answer(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(asked, []int{12000, 24000, 24000}) {
		t.Fatalf("asked for %v", asked)
	}
}

// A number on the provider is the operator's, and may be a cap on what an
// attempt costs, so it is kept exactly and a cut ends the repair as it did.
func TestAnOutputLimitAnOperatorSetIsNeverRaised(t *testing.T) {
	var asked []int
	m := &repairModel{provider: roomProvider{needs: 20000, asked: &asked}, maxTokens: 12000}
	_, err := m.Answer(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "output limit") || !slices.Equal(asked, []int{12000}) {
		t.Fatalf("asked for %v, error %v", asked, err)
	}
}

// Room is not asked for without end. A model still cut at the most a provider
// may be given has nothing more to be asked, and the repair says where it stopped.
func TestAnAutomaticRepairStopsAtTheMostItMayAsk(t *testing.T) {
	var asked []int
	m := &repairModel{provider: roomProvider{needs: 1 << 30, asked: &asked}}
	_, err := m.Answer(context.Background(), nil)
	// 12000 doubled is 24000 and then 48000, and doubled again is 96000, which is
	// held to the 65536 that a provider may be given.
	if !slices.Equal(asked, []int{12000, 24000, 48000, 65536}) {
		t.Fatalf("asked for %v", asked)
	}
	if err == nil || !strings.Contains(err.Error(), "65536") || repairFailureCode(err.Error()) != "output_limit" {
		t.Fatalf("error %v", err)
	}
}

// A provider that took the room it was asked for before and refuses more has
// reached its model's own ceiling, or has no context left beside the prompt.
// Where the answer was cut is then the honest account, and the next step starts
// from the room that worked and not from the one that was refused.
func TestAProviderThatRefusesMoreRoomLeavesTheCutAsTheAnswer(t *testing.T) {
	var asked []int
	refuse := func(limit int) error {
		if limit > 12000 {
			return statusFailure(400)
		}
		return nil
	}
	m := &repairModel{provider: roomProvider{needs: 20000, asked: &asked, refuse: refuse}}
	_, err := m.Answer(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "output limit") || !strings.Contains(err.Error(), "12000") {
		t.Fatalf("error %v", err)
	}
	_, _ = m.Answer(context.Background(), nil)
	if !slices.Equal(asked, []int{12000, 24000, 12000, 24000}) {
		t.Fatalf("asked for %v", asked)
	}
}

// A refusal means "no more room" only after a cut. On the first request it is
// whatever it says: a model name the provider does not know, or a request it
// cannot take for some other reason.
func TestARefusalOnTheFirstRequestIsNotReadAsAnOutputLimit(t *testing.T) {
	var asked []int
	m := &repairModel{provider: roomProvider{needs: 20000, asked: &asked, refuse: func(int) error { return statusFailure(400) }}}
	_, err := m.Answer(context.Background(), nil)
	if err == nil || strings.Contains(err.Error(), "output limit") || !slices.Equal(asked, []int{12000}) {
		t.Fatalf("asked for %v, error %v", asked, err)
	}
}

// Only a refusal of the request itself says the room was too much. A rate limit
// while asking again is a rate limit, and the person is told so.
func TestAnotherFailureWhileAskingForMoreRoomIsNotHiddenBehindTheCut(t *testing.T) {
	var asked []int
	refuse := func(limit int) error {
		if limit > 12000 {
			return statusFailure(429)
		}
		return nil
	}
	m := &repairModel{provider: roomProvider{needs: 20000, asked: &asked, refuse: refuse}}
	_, err := m.Answer(context.Background(), nil)
	if err == nil || repairFailureCode(err.Error()) != "rate_limited" {
		t.Fatalf("error %v", err)
	}
}

type unresolvedRepairRun struct {
	*repairFake
	err error
}

func (f unresolvedRepairRun) RepairPullForRun(context.Context, string, int64) (github.RepairPull, error) {
	return github.RepairPull{Number: 313, HeadSHA: "old"}, f.err
}

func TestReadinessExplainsAClosedPRAndDistinguishesGitHubFailure(t *testing.T) {
	for _, tc := range []struct {
		name, code, message string
		err                 error
	}{
		{"closed PR", "pull", "PR #313 is closed or merged.", fmt.Errorf("%w: PR #313 is closed or merged.", github.ErrRepairUnsupported)},
		{"GitHub unavailable", "github", "GitHub could not resolve this repair target. Try checking readiness again.", errors.New("transport failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			inst := h.installation()
			fake := unresolvedRepairRun{repairFake: &repairFake{Client: h.gh.Client(inst.Target, inst.TargetType)}, err: tc.err}
			h.c.clients.entries[inst.ID] = &clientEntry{client: fake, updatedAt: inst.UpdatedAt.Truncate(time.Millisecond)}
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
			ready := h.c.EliReadiness(h.ctx, user.ID, inst.Target+"/repo", 0, 9)
			if ready.Ready || ready.PullNumber != 313 {
				t.Fatalf("readiness lost the blocked PR: %+v", ready)
			}
			for _, blocker := range ready.Blockers {
				if blocker.Code == tc.code && blocker.Message == tc.message {
					return
				}
			}
			t.Fatalf("missing actionable blocker: %+v", ready.Blockers)
		})
	}
}

// readinessSetup is a person who could repair: linked and confirmed, with a
// provider of their own, on a repository whose policy allows repairs. A test
// then breaks the one thing it is about.
func readinessSetup(t *testing.T) *commandSetup {
	t.Helper()
	s := newCommandSetup(t, 5)
	s.link(t, true)
	personal := &store.AssistantProvider{Name: "Personal", OwnerID: s.user.ID, Kind: "openai", Enabled: true}
	if err := s.h.st.CreateAssistantProvider(s.h.ctx, personal); err != nil {
		t.Fatal(err)
	}
	s.fake.check = "failed"
	return s
}

// checkSummary is "code:status" for each check, in the order readiness makes them.
func checkSummary(r EliReadiness) []string {
	out := make([]string, 0, len(r.Checks))
	for _, c := range r.Checks {
		out = append(out, c.Code+":"+c.Status)
	}
	return out
}

func blockerCodes(r EliReadiness) []string {
	out := []string{}
	for _, b := range r.Blockers {
		out = append(out, b.Code)
	}
	return out
}

const nineOK = "identity:ok provider:ok installation:ok access:ok policy:ok budget:ok pull:ok checks:ok permissions:ok"

// A page that says what Eli can do needs the pull request's own words and a
// finding for every check, so a person sees what passed and not only what failed.
func TestReadinessReturnsThePullRequestAndEveryCheckWhenReady(t *testing.T) {
	s := readinessSetup(t)
	s.fake.title, s.fake.author, s.fake.url, s.fake.base = "Retry transient 502s", "dana", "https://github.com/acme/repo/pull/1", "main"
	ready := s.h.c.EliReadiness(s.h.ctx, s.user.ID, s.repo, 1, 0)
	if !ready.Ready || len(ready.Blockers) != 0 {
		t.Fatalf("not ready: %+v", ready)
	}
	if got := strings.Join(checkSummary(ready), " "); got != nineOK {
		t.Fatalf("checks: %s", got)
	}
	for _, c := range ready.Checks {
		if c.Message == "" || c.Message == "Not checked yet." {
			t.Errorf("an ok check has no finding: %+v", c)
		}
	}
	want := &EliReadinessPull{Number: 1, Title: "Retry transient 502s", Author: "dana", Branch: "feature", Base: "main", URL: "https://github.com/acme/repo/pull/1", Open: true}
	if ready.Pull == nil || *ready.Pull != *want {
		t.Fatalf("pull: %+v, want %+v", ready.Pull, want)
	}
}

// A person whose GitHub account cannot write to the repository is not told what
// is in it, so the pull request is not looked up for them, and the checks after
// the one that stopped them say they were never reached.
func TestReadinessStopsAtWriteAccessAndLeavesTheRestUnchecked(t *testing.T) {
	s := readinessSetup(t)
	s.fake.noWrite = true
	s.fake.title = "A private title"
	ready := s.h.c.EliReadiness(s.h.ctx, s.user.ID, s.repo, 1, 0)
	want := "identity:ok provider:ok installation:ok access:blocked policy:unchecked budget:unchecked pull:unchecked checks:unchecked permissions:unchecked"
	if got := strings.Join(checkSummary(ready), " "); got != want {
		t.Fatalf("checks: %s", got)
	}
	if ready.Ready || ready.Pull != nil {
		t.Fatalf("readiness gave a pull request to someone who cannot write: %+v", ready)
	}
	if len(ready.Blockers) != 1 || ready.Blockers[0] != (EliBlocker{"access", "Your linked GitHub account needs write access to this repository."}) {
		t.Fatalf("blockers changed: %+v", ready.Blockers)
	}
	if ready.Checks[3].Message != ready.Blockers[0].Message {
		t.Fatalf("the blocked check does not carry the blocker's words: %q", ready.Checks[3].Message)
	}
	if ready.Checks[4].Message != "Not checked yet." {
		t.Fatalf("an unreached check should say so: %q", ready.Checks[4].Message)
	}
}

// With no linked account the provider and the installation are still looked at,
// because they are separate things to fix and a person should fix them in one go.
func TestReadinessWithNoLinkedAccountBlocksIdentityAndStillChecksProvider(t *testing.T) {
	s := newCommandSetup(t, 5)
	personal := &store.AssistantProvider{Name: "Personal", OwnerID: s.user.ID, Kind: "openai", Enabled: true}
	if err := s.h.st.CreateAssistantProvider(s.h.ctx, personal); err != nil {
		t.Fatal(err)
	}
	ready := s.h.c.EliReadiness(s.h.ctx, s.user.ID, s.repo, 1, 0)
	want := "identity:blocked provider:ok installation:ok access:unchecked policy:unchecked budget:unchecked pull:unchecked checks:unchecked permissions:unchecked"
	if got := strings.Join(checkSummary(ready), " "); got != want {
		t.Fatalf("checks: %s", got)
	}
	if ready.Pull != nil || len(ready.Blockers) != 1 || ready.Blockers[0] != (EliBlocker{"identity", "Link and confirm your GitHub account in repair setup."}) {
		t.Fatalf("blockers or pull changed: %+v", ready)
	}
}

type closedRepairPull struct{ *repairFake }

func (closedRepairPull) RepairPull(context.Context, string, int) (github.RepairPull, error) {
	return github.RepairPull{Number: 7, Title: "Old work", Author: "dana", Branch: "feature", HeadSHA: "old"},
		&github.RepairTargetChanged{Code: "closed", Message: "PR #7 is closed or merged."}
}

// A closed pull request is the answer to what the person asked, and it reads
// better with its title. The blockers keep their old shape: the changed target
// stands alone, as it always has.
func TestReadinessKeepsThePullRequestWhenItIsClosed(t *testing.T) {
	s := readinessSetup(t)
	s.h.c.clients.entries[s.inst.ID] = &clientEntry{client: closedRepairPull{s.fake}, updatedAt: s.inst.UpdatedAt.Truncate(time.Millisecond)}
	ready := s.h.c.EliReadiness(s.h.ctx, s.user.ID, s.repo, 7, 0)
	if ready.Pull == nil || ready.Pull.Title != "Old work" || ready.Pull.Open {
		t.Fatalf("the closed pull request was lost: %+v", ready.Pull)
	}
	want := "identity:ok provider:ok installation:ok access:ok policy:ok budget:ok pull:blocked checks:unchecked permissions:unchecked"
	if got := strings.Join(checkSummary(ready), " "); got != want {
		t.Fatalf("checks: %s", got)
	}
	if len(ready.Blockers) != 1 || ready.Blockers[0] != (EliBlocker{"closed", "PR #7 is closed or merged."}) {
		t.Fatalf("blockers: %+v", ready.Blockers)
	}
}

// Checks that have passed, or have not finished, are not a failure to repair,
// and the checklist says which of the two it is on the check that decided it.
func TestReadinessSaysWhetherTheChecksHavePassedOrAreStillRunning(t *testing.T) {
	for _, tc := range []struct{ check, code string }{{"passed", "fixed"}, {"waiting", "checks_pending"}} {
		t.Run(tc.check, func(t *testing.T) {
			s := readinessSetup(t)
			s.fake.check = tc.check
			ready := s.h.c.EliReadiness(s.h.ctx, s.user.ID, s.repo, 1, 0)
			if ready.Ready || ready.Pull == nil {
				t.Fatalf("readiness: %+v", ready)
			}
			var checks EliCheck
			for _, c := range ready.Checks {
				if c.Code == "checks" {
					checks = c
				}
			}
			if checks.Status != EliCheckBlocked || checks.Message == "" {
				t.Fatalf("the checks check: %+v", checks)
			}
			if !slices.Contains(blockerCodes(ready), tc.code) {
				t.Fatalf("blockers %v lack %s", blockerCodes(ready), tc.code)
			}
		})
	}
}

// A missing App permission is a check of its own, with the sentence it always had.
func TestReadinessNamesAMissingAppPermission(t *testing.T) {
	s := readinessSetup(t)
	s.h.gh.SetPermissions(map[string]string{"contents": "read", "pull_requests": "write", "actions": "write"})
	ready := s.h.c.EliReadiness(s.h.ctx, s.user.ID, s.repo, 1, 0)
	last := ready.Checks[len(ready.Checks)-1]
	if last.Code != "permissions" || last.Status != EliCheckBlocked || last.Message != "Update the GitHub App permission: contents write" {
		t.Fatalf("permissions check: %+v", last)
	}
	if got := blockerCodes(ready); len(got) != 1 || got[0] != "permissions" {
		t.Fatalf("blockers: %v", got)
	}
}
