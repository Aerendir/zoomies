package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/naming"
	"github.com/eyupio/zoomies/internal/store"
	"github.com/eyupio/zoomies/internal/updates"
)

// CodeHostUpdateHelperStuck is the problem for an update that could not be handed
// to a host because the helper there had stopped reading requests. It is the same
// code on the host's card, where it labels the failed attempt, and in the list.
const CodeHostUpdateHelperStuck = "host.update_helper_stuck"

// The audit action written when an agent has asked for the helper on its host to
// be repaired, or root has repaired it. One action for both, because the person
// reading the audit wants to know that the helper was touched and what became of it.
const auditUpdateHelperRepair = "update.helper_repair"

var knownRepairActions = []string{
	updates.ActionResetFailed, updates.ActionRewroteUnits, updates.ActionStartedPath,
	updates.ActionRemovedRequest, updates.ActionRemovedLock, updates.ActionRefusedBusy, updates.ActionRefusedRateLimit,
}

// cleanStuck is what an agent said about a stalled helper, made fit to keep and
// to show every role: the kind, the note and the actions are checked against the
// fixed words, the unit states are cut to a name's length, and the numbers are
// bounded. An agent is trusted to be honest about its host and no further.
func cleanStuck(s *agent.UpdateStuck, attempts int) *agent.UpdateStuck {
	if s == nil {
		return nil
	}
	out := agent.UpdateStuck{
		Kind: s.Kind, Path: unitWord(s.Path), Service: unitWord(s.Service),
		LockPresent: s.LockPresent, Healed: s.Healed, Asked: s.Asked, Attempts: min(max(s.Attempts, 0), 99),
	}
	if !slices.Contains([]string{updates.StuckPathDown, updates.StuckStaleRequest, updates.StuckBusy}, out.Kind) {
		return nil
	}
	if slices.Contains([]string{"busy", "held_back", "no_answer", "refused"}, s.Note) {
		out.Note = s.Note
	}
	out.RequestAgeSeconds = min(max(s.RequestAgeSeconds, 0), int64(10*365*24*time.Hour/time.Second))
	for _, a := range s.Repaired {
		if slices.Contains(knownRepairActions, a) && len(out.Repaired) < len(knownRepairActions) {
			out.Repaired = append(out.Repaired, a)
		}
	}
	return &out
}

// unitWord is a unit's state as an agent reported it: systemd's own words, so it
// is short and plain. Anything a terminal or a page would act on is dropped and
// the rest is cut to a name's length.
func unitWord(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || isBidiControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	return strings.TrimSpace(cutAt(strings.TrimSpace(s), 64))
}

// stuckOf reads the record on an attempt. Absent, or unreadable, is none.
func stuckOf(a store.UpdateAttempt) *agent.UpdateStuck {
	if a.Stuck == "" {
		return nil
	}
	var s agent.UpdateStuck
	if json.Unmarshal([]byte(a.Stuck), &s) != nil {
		return nil
	}
	return cleanStuck(&s, 0)
}

// stuckIsFault says the record is a helper that was stopped and stayed so, not
// one that was merely busy or that was repaired and then took the request: only
// a helper still stopped is a problem of its own.
func stuckIsFault(s *agent.UpdateStuck) bool {
	return s != nil && !s.Healed && s.Kind != updates.StuckBusy && s.Note != "busy"
}

// ageWords says how long a request had waited, roughly, because a person reading
// "3 days" knows what it means and "259214s" is for the log.
func ageWords(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	case d >= 2*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	case d >= 2*time.Minute:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	return "less than two minutes"
}

// repairWords is what root did, in a clause, in the order it did it.
func repairWords(actions []string) string {
	say := map[string]string{
		updates.ActionResetFailed:      "cleared the units' failed state",
		updates.ActionRewroteUnits:     "rewrote the helper's units",
		updates.ActionStartedPath:      "started the path unit",
		updates.ActionRemovedRequest:   "removed an abandoned request",
		updates.ActionRemovedLock:      "removed an abandoned upgrade lock",
		updates.ActionRefusedBusy:      "declined because an update was running",
		updates.ActionRefusedRateLimit: "declined because it had repaired this helper too recently",
	}
	var parts []string
	for _, a := range actions {
		if w, ok := say[a]; ok {
			parts = append(parts, w)
		}
	}
	return strings.Join(parts, ", ")
}

// stuckFacts is the sentence of what was found, with no path and nothing the
// host chose: fixed words, systemd's own words for the units, and numbers. It is
// safe to show to every role.
func stuckFacts(s *agent.UpdateStuck) string {
	var parts []string
	if s.Path != "" {
		parts = append(parts, "its path unit is "+s.Path)
	}
	if s.Service != "" && s.Service != "inactive, static" {
		parts = append(parts, "its service unit is "+s.Service)
	}
	if s.RequestAgeSeconds > 0 {
		parts = append(parts, "a request had been waiting for "+ageWords(s.RequestAgeSeconds))
	}
	if s.LockPresent {
		parts = append(parts, "upgrade.lock was present")
	}
	return strings.Join(parts, "; ")
}

// stuckSentence is what the controller says of an attempt the stalled helper
// stopped, on the host's card and in the failed row. The agent's own error text
// is not used: this is built from the checked record.
func stuckSentence(host string, s *agent.UpdateStuck) string {
	subject := "The update helper on " + host
	facts := stuckFacts(s)
	if facts != "" {
		facts = " (" + facts + ")"
	}
	repaired := repairWords(s.Repaired)
	if repaired != "" {
		repaired = " Root " + repaired + "."
	}
	const install = " Somebody with root on the host can run sudo zoomies updates helper install, which resets and restarts it, and then update again."
	switch {
	case s.Kind == updates.StuckBusy || s.Note == "busy":
		return subject + " is running another update, so this request was not written and nothing was changed. Update again once that one has finished."
	case s.Note == "held_back":
		return subject + " has stopped" + facts + ", and was repaired too recently or too often to try again yet." + repaired + install
	case s.Note == "no_answer":
		return subject + " has stopped" + facts + ", and the request to repair it was not answered, which happens when the repair units are not installed on that host." + install
	case s.Note == "refused":
		return subject + " has stopped" + facts + ", and the repair did not put it right." + repaired + install
	}
	return subject + " had stopped" + facts + "." + repaired
}

// stuckProblem is the problem for a host whose latest attempt the stalled helper
// stopped. It carries what an operator needs to act without opening the host: the
// attempt, how long the request had waited, and the state of the units. Like
// every host update problem it has no Remedy: the autopilot applies every remedy
// it finds, and nothing here is one it applies.
func stuckProblem(h *store.Host, a store.UpdateAttempt, s *agent.UpdateStuck) Problem {
	label := naming.ForSentence(h.Name)
	return Problem{
		Code:     "host.update_helper_stuck",
		Severity: config.SeverityError,
		Title:    fmt.Sprintf("the update helper on host %s has stopped, so its update to %s could not be handed over", label, a.ToVersion),
		Detail: fmt.Sprintf("attempt %s: %s. The host's agent still runs %s and places work as normal.%s",
			a.ID, stuckFacts(s), reportedVersion(h.Version), problemRepairClause(s)),
		Fix: fmt.Sprintf("have somebody with root on %s run `%s`, which clears the units' failed state, rewrites them and starts the path unit again, "+
			"then `sudo zoomies updates helper status` to see that it is waiting. Update it again from the Hosts page once it is.", label, helperInstallCommand),
		TargetKind: "host", TargetID: h.ID,
	}
}

func problemRepairClause(s *agent.UpdateStuck) string {
	switch {
	case s.Note == "no_answer":
		return " The request to repair it was not answered, so the repair units are probably not installed on that host."
	case s.Note == "held_back":
		return " It was repaired too recently or too often for the agent to ask again."
	case len(s.Repaired) > 0:
		return " Root tried: it " + repairWords(s.Repaired) + "."
	}
	return ""
}

// noteHelperStuck records what an update task found wrong with the helper and,
// when the agent asked for or saw a repair, says so where an operator reads: the
// log, the audit and the host's card. It reports whether the attempt's record was
// written.
func (c *Controller) noteHelperStuck(ctx context.Context, hostID, attemptID string, raw *agent.UpdateStuck) *agent.UpdateStuck {
	s := cleanStuck(raw, 0)
	if s == nil {
		return nil
	}
	if body, err := json.Marshal(s); err == nil {
		if err := c.st.SetUpdateAttemptStuck(ctx, attemptID, string(body)); err != nil {
			c.log.Warn("could not record what was found wrong with a host's update helper", "host", hostID, "attempt", attemptID, "error", err)
		}
	}
	if s.Asked || len(s.Repaired) > 0 {
		c.log.Warn("a host's update helper had stopped and a repair was asked for",
			"host", hostID, "attempt", attemptID, "kind", s.Kind, "request_age_seconds", s.RequestAgeSeconds,
			"path_unit", s.Path, "service_unit", s.Service, "repaired", s.Repaired, "healed", s.Healed, "note", s.Note)
		c.systemAudit(ctx, auditUpdateHelperRepair, "host", hostID, nil, map[string]any{
			"attempt_id": attemptID, "kind": s.Kind, "request_age_seconds": s.RequestAgeSeconds,
			"path_unit": s.Path, "service_unit": s.Service, "repaired": s.Repaired, "healed": s.Healed, "note": s.Note,
		})
	}
	return s
}
