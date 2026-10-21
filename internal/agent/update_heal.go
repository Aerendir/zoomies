package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/updates"
	"github.com/eyupio/zoomies/internal/updates/channel"
)

// defaultRepairWait is how long an update task waits for root to answer a repair
// request. The repair settles for a few seconds after starting the path unit, so
// a healthy repair answers well inside it; one that does not answer has no repair
// unit listening, and the host's owner has to install it again.
const defaultRepairWait = 45 * time.Second

// healOutcome is what looking at a helper that did not read its request came to.
type healOutcome struct {
	// stuck is set when the helper was found stopped or busy; nil means nothing
	// was wrong that the agent can tell, and the ordinary refusal stands.
	stuck *UpdateStuck
	// written says the request was written after a repair.
	written bool
	// reason is the sentence for a failure, without a path.
	reason string
}

// healStuckHelper is what a task does instead of refusing outright when its
// request cannot be written because an earlier one is still there. The agent
// cannot touch the units and does not try: it looks, and when the helper is
// provably stopped it asks root, through the folder, to put it right, waits for
// the answer, and writes its request once if the helper is back.
//
// The caller holds a.update, which is what serialises two tasks: the second one
// to arrive sees the first's repair or the budget it spent, never a repair of
// its own on top.
func (a *Agent) healStuckHelper(ctx context.Context, task Task, dir string, req updates.Request) healOutcome {
	if a.opts.UnitRun == nil {
		return healOutcome{}
	}
	diag, err := channel.Diagnose(ctx, a.opts.UnitRun, dir, a.opts.ConfigDir, a.now(), a.opts.Processes)
	if err != nil {
		a.log.Warn("could not look at the update helper", "attempt", task.UpdateID, "error", err)
		return healOutcome{}
	}
	verdict := updates.Classify(diag, updates.OrphanAfter)
	if verdict.Kind == "" {
		return healOutcome{}
	}
	stuck := &UpdateStuck{
		Kind: verdict.Kind, RequestAgeSeconds: int64(diag.RequestAge / time.Second),
		Path: unitWords(diag.Path), Service: unitWords(diag.Service), LockPresent: diag.LockPresent,
	}
	a.log.Warn("the update request is waiting and the helper may be stopped",
		"attempt", task.UpdateID, "kind", verdict.Kind, "request_age", diag.RequestAge.Round(time.Second),
		"waiting_request", diag.RequestID, "lock", diag.LockPresent, "path_unit", stuck.Path, "service_unit", stuck.Service,
		"engine_running", diag.EngineRunning)

	if !verdict.Repair {
		// A helper at work may be reading the request, and the engine owns the lock.
		stuck.Note = "busy"
		return healOutcome{stuck: stuck, reason: "the update helper on this host is running another update, so this request was not written; it is safe to send again once that one finishes"}
	}
	if ok, next := a.repairs.Admit(a.now()); !ok {
		stuck.Note, stuck.Attempts = "held_back", len(a.repairs.Times)
		a.log.Warn("not asking for another repair of the update helper yet", "attempt", task.UpdateID, "next", next.UTC().Format(time.RFC3339))
		return healOutcome{stuck: stuck, reason: "the update helper on this host is stopped and was already repaired, or the repair was asked for, too recently to try again; the next repair may start at " +
			next.UTC().Format(time.RFC3339) + " (run " + helperInstallCommand + " on the host to put it right now)"}
	}
	stuck.Asked, stuck.Attempts = true, len(a.repairs.Times)

	repair := updates.RepairRequest{V: updates.WireVersion, ID: task.UpdateID, RequestedAt: a.now().UTC()}
	if err := channel.WriteRepair(dir, repair); err != nil && !errors.Is(err, channel.ErrRepairPending) {
		a.log.Warn("could not ask root to repair the update helper", "attempt", task.UpdateID, "error", err)
		stuck.Note = "no_answer"
		return healOutcome{stuck: stuck, reason: "the update helper on this host is stopped and the repair could not be asked for; run " + helperInstallCommand + " on the host"}
	}
	a.log.Info("asked root to repair the update helper", "attempt", task.UpdateID, "kind", verdict.Kind)

	res, answered := a.awaitRepair(ctx, dir, task.UpdateID)
	if !answered {
		channel.WithdrawRepair(dir, task.UpdateID)
		stuck.Note = "no_answer"
		a.log.Warn("root did not answer the repair request; the repair units may not be installed", "attempt", task.UpdateID)
		return healOutcome{stuck: stuck, reason: "the update helper on this host is stopped and nothing answered the request to repair it, which happens when the repair units are not installed on this host; run " +
			helperInstallCommand + " on the host, which also starts the helper"}
	}
	stuck.Repaired = append([]string(nil), res.Actions...)
	a.log.Info("root answered the repair request", "attempt", task.UpdateID, "healed", res.Healed, "actions", res.Actions, "verdict", res.Verdict)
	if !res.Healed {
		stuck.Note = "refused"
		return healOutcome{stuck: stuck, reason: "the update helper on this host is stopped, and the repair did not put it right: " + fieldSentence(res.Error) +
			"; run " + helperInstallCommand + " on the host"}
	}

	// One write, once: the helper is back and the folder is clear, or it is not.
	if err := channel.WriteRequest(dir, req); err != nil {
		a.log.Warn("the helper was repaired but the request could not be written", "attempt", task.UpdateID, "error", err)
		stuck.Note = "refused"
		return healOutcome{stuck: stuck, reason: "the update helper on this host was repaired, but an earlier request is still in its update folder, so this one was not written; it is safe to send again"}
	}
	stuck.Healed = true
	return healOutcome{stuck: stuck, written: true}
}

// awaitRepair waits for root's answer to this attempt's repair, and says whether
// it came. It looks once a second, because the answer is a file and there is
// nothing to be woken by.
func (a *Agent) awaitRepair(ctx context.Context, dir, id string) (updates.RepairResult, bool) {
	wait := a.opts.RepairWait
	if wait <= 0 {
		wait = defaultRepairWait
	}
	// Real time, not the agent's clock: this is a wait on another process.
	deadline := time.Now().Add(wait)
	for {
		if res, found, err := channel.ReadRepairResult(dir); err == nil && found && res.ID == id {
			return res, true
		}
		if !time.Now().Before(deadline) || !sleepCtx(ctx, repairPoll) {
			return updates.RepairResult{}, false
		}
	}
}

// repairPoll is how often an update task looks for root's answer. A variable so
// that a test does not wait a second for each look.
var repairPoll = time.Second

// unitWords is a unit's state as systemd's own words, for a sentence: what a
// person reads at "systemctl status".
func unitWords(u updates.UnitState) string {
	words := make([]string, 0, 3)
	for _, w := range []string{u.Active, u.Enabled} {
		if w = fieldText(w); w != "" {
			words = append(words, w)
		}
	}
	if u.Failed {
		words = append(words, "failed")
	}
	return strings.Join(words, ", ")
}

// fieldSentence is a sentence from root's answer, cleaned and cut.
func fieldSentence(s string) string {
	s = truncateRunes(printableText(s), maxUpdateErrorRunes)
	if s == "" {
		return "no reason was given"
	}
	return strings.TrimRight(s, ". ")
}
