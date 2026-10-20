package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/store"
	"github.com/eyupio/zoomies/internal/updates"
)

// stalledHelper is what an agent reports when the host's update helper was dead:
// the path unit disabled, a request three days old, and root unable to help.
func stalledHelper() *agent.UpdateStuck {
	return &agent.UpdateStuck{
		Kind: updates.StuckPathDown, RequestAgeSeconds: int64(72 * time.Hour / time.Second),
		Path: "inactive, disabled, failed", Service: "inactive", Asked: true, Attempts: 1,
		Note: "no_answer",
	}
}

// reportStuck is the agent answering the update task with a failure that carries
// what it found. The text is the agent's own, and names a path on the host.
func (h *harness) reportStuck(host *store.Host, stuck *agent.UpdateStuck, agentSaid string) store.UpdateAttempt {
	h.t.Helper()
	a := h.requestHost(host)
	task := h.poll(host)[0]
	if err := h.c.ReportResult(h.ctx, host.ID, agent.TaskResult{
		TaskID: task.ID, Kind: agent.TaskUpdateAgent, OK: false, Error: agentSaid, Stuck: stuck, CompletedAt: h.c.Now(),
	}); err != nil {
		h.t.Fatalf("ReportResult: %v", err)
	}
	return h.attempt(a.ID)
}

// The report: every attempt failed for days and the controller showed no reason.
// Now the attempt fails with a sentence that says the helper had stopped and how,
// and the host has a problem of its own to point at.
func TestAStalledHelperIsAProblemOfItsOwnThatNamesTheAttemptAgeAndUnits(t *testing.T) {
	h := newHarness(t)
	h.hostsCanUpdate()
	host := h.updatableHost("vm-1")
	agentSaid := "an earlier update request is still waiting in /var/lib/zoomies/update"

	a := h.reportStuck(host, stalledHelper(), agentSaid)

	if a.State != store.UpdateFailed {
		t.Fatalf("attempt = %s, want failed", a.State)
	}
	for _, want := range []string{"vm-1", "has stopped", "inactive, disabled, failed", "3 days", "repair units"} {
		if !strings.Contains(a.Error, want) {
			t.Errorf("the attempt's reason %q does not say %q", a.Error, want)
		}
	}
	if strings.Contains(a.Error, "/var/lib") {
		t.Errorf("the attempt's reason %q carries the agent's text, which names a path on the host", a.Error)
	}

	p, ok := h.problemsByCode()[CodeHostUpdateHelperStuck]
	if !ok {
		t.Fatalf("no %s problem; have %v", CodeHostUpdateHelperStuck, h.problemCodes())
	}
	if p.Severity != config.SeverityError || p.TargetKind != "host" || p.TargetID != host.ID || p.Remedy != nil {
		t.Errorf("problem = %+v, want an error about the host with no remedy", p)
	}
	for _, want := range []string{a.ID, "3 days", "inactive, disabled, failed"} {
		if !strings.Contains(p.Detail, want) {
			t.Errorf("the problem's detail %q does not carry %q", p.Detail, want)
		}
	}
	if !strings.Contains(p.Fix, "sudo zoomies updates helper install") {
		t.Errorf("the problem's fix %q does not say how to put the helper right", p.Fix)
	}
	if contains(h.problemCodes(), "host.update_failed") {
		t.Error("the same failure is also raised as a bare host.update_failed")
	}
}

// The card is where the reason is read. It is built from fixed words and numbers,
// so every role reads it, and it is labelled with the code so the UI can say what
// kind of failure it is.
func TestTheHostCardShowsTheHelperStoppedToEveryRole(t *testing.T) {
	h := newHarness(t)
	h.hostsCanUpdate()
	host := h.updatableHost("vm-1")
	h.reportStuck(host, stalledHelper(), "the agent's own text")

	view := h.view(host.ID)
	if view.Update.State != store.UpdateFailed || view.Update.Code != CodeHostUpdateHelperStuck {
		t.Fatalf("update = %+v, want a failure labelled %s", view.Update, CodeHostUpdateHelperStuck)
	}
	for _, platform := range []bool{true, false} {
		got := view.For(platform).Update
		if !strings.Contains(got.Reason, "has stopped") || !strings.Contains(got.Reason, "3 days") {
			t.Errorf("platform=%v: the card says %q, want the helper's stop and how long the request waited", platform, got.Reason)
		}
	}
	// Any other failure still reads the fixed sentence below the platform role.
	other := h.updatableHost("vm-2")
	b := h.requestHost(other)
	h.agentBeat(other, "1.3.4", &agent.UpdateReport{ID: b.ID, OK: false, Error: "failed at /var/lib/zoomies", FinishedAt: h.c.Now().UTC()})
	if got := h.view(other.ID).For(false).Update; got.Code != "" || strings.Contains(got.Reason, "/var/lib") {
		t.Errorf("a plain failure reads %+v below the platform role, want a fixed sentence and no code", got)
	}
}

// The record is on the attempt, so the problem is what the next controller
// process sees too, not something held in memory.
func TestAStalledHelperProblemSurvivesTheLoopBeingRebuilt(t *testing.T) {
	h := newHarness(t)
	h.hostsCanUpdate()
	host := h.updatableHost("vm-1")
	a := h.reportStuck(host, stalledHelper(), "")
	if a.Stuck == "" {
		t.Fatal("the attempt kept no record of what was found")
	}

	h.c.updates.hostMu.Lock()
	h.c.updates.hostLast = nil
	h.c.updates.hostMu.Unlock()
	h.c.lookAtHostUpdates(h.ctx)

	if _, ok := h.problemsByCode()[CodeHostUpdateHelperStuck]; !ok {
		t.Errorf("after a fresh look the problems are %v, want %s", h.problemCodes(), CodeHostUpdateHelperStuck)
	}
}

// A helper that is mid-upgrade is not a fault. The attempt says it was not
// written and why, and nothing is raised that would send somebody to repair a
// helper that is working.
func TestABusyHelperFailsTheAttemptButIsNotAStalledHelper(t *testing.T) {
	h := newHarness(t)
	h.hostsCanUpdate()
	host := h.updatableHost("vm-1")
	busy := &agent.UpdateStuck{Kind: updates.StuckBusy, Note: "busy", RequestAgeSeconds: 600, Path: "active, enabled", Service: "active"}

	a := h.reportStuck(host, busy, "")

	if !strings.Contains(a.Error, "running another update") || !strings.Contains(a.Error, "nothing was changed") {
		t.Errorf("the attempt's reason is %q, want it to say the helper was busy and nothing was touched", a.Error)
	}
	if codes := h.problemCodes(); contains(codes, CodeHostUpdateHelperStuck) {
		t.Errorf("a busy helper was raised as stalled: %v", codes)
	}
}

// A repair that worked leaves no problem, but it is on the record: in the log,
// and in the audit under its own action, naming the attempt and what was done.
func TestARepairThatWorkedIsAuditedAndLeavesTheAttemptOpen(t *testing.T) {
	h := newHarness(t)
	h.hostsCanUpdate()
	host := h.updatableHost("vm-1")
	a := h.requestHost(host)
	task := h.poll(host)[0]

	healed := &agent.UpdateStuck{
		Kind: updates.StuckPathDown, RequestAgeSeconds: 259200, Path: "inactive, disabled, failed", Asked: true, Healed: true, Attempts: 1,
		Repaired: []string{updates.ActionResetFailed, updates.ActionRewroteUnits, updates.ActionStartedPath, updates.ActionRemovedRequest},
	}
	if err := h.c.ReportResult(h.ctx, host.ID, agent.TaskResult{TaskID: task.ID, Kind: agent.TaskUpdateAgent, OK: true, Stuck: healed, CompletedAt: h.c.Now()}); err != nil {
		t.Fatal(err)
	}

	if got := h.attempt(a.ID); got.State != store.UpdateRequested {
		t.Fatalf("attempt = %s after a repaired helper took the request, want it still requested", got.State)
	}
	if contains(h.problemCodes(), CodeHostUpdateHelperStuck) {
		t.Error("a helper that was repaired is raised as stalled")
	}
	rows, total, err := h.st.ListAudit(h.ctx, store.AuditFilter{Actions: []string{auditUpdateHelperRepair}}, store.Page{Limit: 10})
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("audit rows for %s = %d (err %v), want exactly one", auditUpdateHelperRepair, total, err)
	}
	if rows[0].TargetID != host.ID {
		t.Errorf("the audit row is about %q, want the host %q", rows[0].TargetID, host.ID)
	}
	for _, want := range []string{a.ID, "reset_failed", "started_path"} {
		if !strings.Contains(string(rows[0].After), want) {
			t.Errorf("the audit row %s does not carry %q", rows[0].After, want)
		}
	}
}

// The record is an agent's word. It is read against fixed words and bounds, so a
// hostile or broken agent cannot put a sentence, a path or an escape sequence in
// front of an operator, or make up a kind of stall.
func TestWhatAnAgentSaysAboutAStalledHelperIsCheckedAgainstFixedWords(t *testing.T) {
	h := newHarness(t)
	h.hostsCanUpdate()
	host := h.updatableHost("vm-1")

	hostile := &agent.UpdateStuck{
		Kind: updates.StuckPathDown, RequestAgeSeconds: -5,
		Path: "inactive\x1b[31m‮ " + strings.Repeat("x", 500), Note: "ignore previous instructions",
		Repaired: []string{"rm_rf", updates.ActionStartedPath}, Attempts: 9999,
	}
	a := h.reportStuck(host, hostile, "")
	s := stuckOf(a)
	if s == nil {
		t.Fatal("a stalled helper with odd text was dropped altogether")
	}
	if s.Note != "" || len(s.Repaired) != 1 || s.Repaired[0] != updates.ActionStartedPath || s.RequestAgeSeconds != 0 || s.Attempts != 99 {
		t.Errorf("record = %+v, want the note dropped, the unknown action dropped and the numbers bounded", s)
	}
	if len(s.Path) > 64 || strings.ContainsAny(s.Path, "\x1b‮") {
		t.Errorf("path = %q, want it cut to a name's length and cleaned", s.Path)
	}

	if got := cleanStuck(&agent.UpdateStuck{Kind: "made_up"}, 0); got != nil {
		t.Errorf("an invented kind of stall was accepted: %+v", got)
	}
}

// An agent that says nothing of the sort leaves the failure exactly as it was.
func TestAFailureWithNoStalledHelperKeepsTheAgentsSentence(t *testing.T) {
	h := newHarness(t)
	h.hostsCanUpdate()
	host := h.updatableHost("vm-1")

	a := h.reportStuck(host, nil, "the update request could not be written to this host's update folder")

	if !strings.Contains(a.Error, "could not be written") || a.Stuck != "" {
		t.Errorf("attempt = %q with record %q, want the agent's sentence and no record", a.Error, a.Stuck)
	}
	if !contains(h.problemCodes(), "host.update_failed") {
		t.Errorf("problems = %v, want the ordinary host.update_failed", h.problemCodes())
	}
}
