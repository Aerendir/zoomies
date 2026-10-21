package updates

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// A helper that has stopped answering looks the same from the service as one
// that is slow: request.json sits in the update folder and nothing reads it.
// The service cannot tell the two apart by waiting, and it cannot repair the
// root-owned units itself, so this file is the shared vocabulary of the three
// parties that deal with it: the agent that diagnoses, the root helper that
// repairs, and the controller that tells the operator. Like the rest of this
// package it reads no clock and no file.

const (
	// StuckPathDown is a helper whose path unit is not waiting for a request:
	// disabled, inactive or failed, most often because systemd stopped it for
	// starting the service too often.
	StuckPathDown = "path_down"
	// StuckStaleRequest is a request nobody has read for longer than a healthy
	// helper takes to pick one up, with the units apparently fine.
	StuckStaleRequest = "stale_request"
	// StuckBusy is not a fault: the helper is running an update now, or the
	// engine is. It is a verdict so that nothing in the folder is touched.
	StuckBusy = "helper_busy"

	// OrphanAfter is how old a request or a lock must be, with no helper and no
	// engine running, before it is called abandoned. A healthy helper takes a
	// request within seconds and the engine finishes within fifty minutes while
	// its process is alive, so a file this old with nothing alive behind it is
	// left over.
	OrphanAfter = 15 * time.Minute

	// RepairMinInterval and RepairMaxPerDay bound how often a host's units are
	// repaired. They are enforced by root against its own record, and by the
	// agent against its own memory, so a loop in either is stopped by the other.
	RepairMinInterval = 10 * time.Minute
	RepairMaxPerDay   = 3

	// RepairFile is what the agent writes to ask root to look at the helper, and
	// RepairResultFile is what root answers in. They sit beside request.json and
	// are watched by their own units, so a helper whose path unit has died can
	// still be repaired.
	RepairFile       = "repair.json"
	RepairResultFile = "repair-result.json"

	// MaxRepairBytes bounds both documents.
	MaxRepairBytes = 8 << 10
)

// UnitState is what systemctl says of one unit, as its own words. An empty
// field is a question that could not be asked.
type UnitState struct {
	// Active is the is-active word: active, inactive, failed, activating...
	Active string `json:"active"`
	// Enabled is the is-enabled word: enabled, disabled, static, masked...
	Enabled string `json:"enabled"`
	// Failed says the unit is in the failed state, which is what a start limit
	// leaves behind and what reset-failed clears.
	Failed bool `json:"failed"`
}

// Diagnosis is everything looked at before a request is refused for one
// already waiting.
type Diagnosis struct {
	RequestPresent bool          `json:"request_present"`
	RequestAge     time.Duration `json:"request_age"`
	// RequestID is the waiting request's attempt, when it could be read.
	RequestID   string        `json:"request_id,omitempty"`
	LockPresent bool          `json:"lock_present"`
	LockAge     time.Duration `json:"lock_age"`
	Path        UnitState     `json:"path"`
	Service     UnitState     `json:"service"`
	// EngineRunning says a zoomies upgrade process is alive on the host.
	EngineRunning bool `json:"engine_running"`
}

var busyWords = []string{"active", "activating", "deactivating", "reloading"}

// ServiceBusy says the helper's service, or the engine it starts, is working.
func (d Diagnosis) ServiceBusy() bool {
	return d.EngineRunning || slices.Contains(busyWords, d.Service.Active)
}

// PathWaiting says the path unit is the one that would start the helper for the
// request: running and enabled, and not failed.
func (d Diagnosis) PathWaiting() bool {
	return d.Path.Active == "active" && !d.Path.Failed &&
		(d.Path.Enabled == "enabled" || d.Path.Enabled == "static" || d.Path.Enabled == "")
}

// UnitsKnown says systemd could be asked about the path unit at all. A host
// where it could not (no systemctl, a container, no bus) is one nothing can be
// concluded about, and is never repaired on a guess.
func (d Diagnosis) UnitsKnown() bool { return d.Path.Active != "" || d.Path.Enabled != "" }

// Verdict is Classify's answer.
type Verdict struct {
	// Kind is one of the Stuck constants, or empty for a helper that is merely
	// not done yet.
	Kind string
	// Repair says asking root to repair the helper is justified.
	Repair bool
}

// Classify says what a waiting request means. A busy helper is never repaired
// and nothing in the folder is touched, whatever else is wrong with it: a live
// helper may still be reading the request, and a live engine owns the lock.
func Classify(d Diagnosis, staleAfter time.Duration) Verdict {
	switch {
	case d.ServiceBusy():
		return Verdict{Kind: StuckBusy}
	case !d.UnitsKnown():
		return Verdict{}
	case !d.PathWaiting():
		return Verdict{Kind: StuckPathDown, Repair: true}
	case d.Service.Failed:
		return Verdict{Kind: StuckPathDown, Repair: true}
	case d.RequestPresent && d.RequestAge > staleAfter:
		return Verdict{Kind: StuckStaleRequest, Repair: true}
	}
	return Verdict{}
}

// Orphaned says the request and the lock left in the folder are provably
// abandoned: nothing is running, and they are older than OrphanAfter. It is
// asked only after the units have been put right, since a helper that comes
// back reads a request itself, and the answer is per file.
func (d Diagnosis) Orphaned() (request, lock bool) {
	if d.ServiceBusy() {
		return false, false
	}
	return d.RequestPresent && d.RequestAge > OrphanAfter, d.LockPresent && d.LockAge > OrphanAfter
}

// EngineRunning says one of the command lines is a zoomies upgrade engine. The
// caller reads /proc; this only reads the arguments, so the match is a table
// test.
func EngineRunning(cmdlines [][]string) bool {
	for _, argv := range cmdlines {
		if len(argv) < 2 || !strings.HasPrefix(filepath.Base(argv[0]), "zoomies") {
			continue
		}
		// A flag may take a value ahead of the subcommand, so the word is looked
		// for anywhere; the helper's own "updates helper run" never carries it.
		if slices.Contains(argv[1:], "upgrade") && !slices.Contains(argv[1:], "updates") {
			return true
		}
	}
	return false
}

// RepairRequest is what the agent leaves for root. It carries nothing root acts
// on but the attempt it is for: the diagnosis is made again by root.
type RepairRequest struct {
	V           int       `json:"v"`
	ID          string    `json:"id"`
	RequestedAt time.Time `json:"requested_at"`
}

// RepairResult is root's account of a repair.
type RepairResult struct {
	V  int    `json:"v"`
	ID string `json:"id"`
	// Healed says the helper is waiting for requests again and the folder holds
	// nothing that would stop a new one.
	Healed bool `json:"healed"`
	// Actions are the steps taken, in order, as fixed words.
	Actions []string `json:"actions,omitempty"`
	// Verdict is what root found, one of the Stuck constants.
	Verdict    string    `json:"verdict,omitempty"`
	Error      string    `json:"error,omitempty"`
	FinishedAt time.Time `json:"finished_at"`
}

// The actions a repair reports.
const (
	ActionResetFailed       = "reset_failed"
	ActionRewroteUnits      = "rewrote_units"
	ActionStartedPath       = "started_path"
	ActionRemovedRequest    = "removed_request"
	ActionRemovedLock       = "removed_lock"
	ActionRefusedBusy       = "refused_busy"
	ActionRefusedRateLimit  = "refused_rate_limit"
	maxRepairActionsAllowed = 8
)

// ParseRepairRequest reads a repair.json. The folder belongs to the service,
// so anything may be in it: the document is read strictly and checked.
func ParseRepairRequest(body []byte) (RepairRequest, error) {
	var r RepairRequest
	if err := decodeStrict(body, &r); err != nil {
		return RepairRequest{}, fmt.Errorf("the repair request is not a document this release reads: %w", err)
	}
	if r.V != WireVersion {
		return RepairRequest{}, fmt.Errorf("the repair request is version %d and this release reads version %d", r.V, WireVersion)
	}
	if err := checkID(r.ID); err != nil {
		return RepairRequest{}, fmt.Errorf("the repair request's id: %w", err)
	}
	return r, nil
}

// ParseRepairResult reads a repair-result.json.
func ParseRepairResult(body []byte) (RepairResult, error) {
	var r RepairResult
	if err := decodeStrict(body, &r); err != nil {
		return RepairResult{}, fmt.Errorf("the repair result is not a document this release reads: %w", err)
	}
	if r.V != WireVersion {
		return RepairResult{}, fmt.Errorf("the repair result is version %d and this release reads version %d", r.V, WireVersion)
	}
	if len(r.Actions) > maxRepairActionsAllowed {
		return RepairResult{}, fmt.Errorf("the repair result lists %d actions, over the limit of %d", len(r.Actions), maxRepairActionsAllowed)
	}
	return r, nil
}

func checkID(id string) error {
	switch {
	case id == "":
		return errors.New("is empty")
	case len(id) > 64:
		return errors.New("is over 64 bytes")
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return fmt.Errorf("has %q in it", r)
		}
	}
	return nil
}

func decodeStrict(body []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("has more in it than one document")
	}
	return nil
}

// RepairLedger is the record that bounds repairs: the times of the ones made.
// Root keeps its own, in a folder only root writes; the agent keeps one in
// memory. Both ask Admit.
type RepairLedger struct {
	Times []time.Time `json:"times"`
}

// Admit says whether a repair may start at now, and when the next may if not.
// An admitted repair is recorded; the ledger is pruned to the last day.
func (l *RepairLedger) Admit(now time.Time) (ok bool, next time.Time) {
	kept := l.Times[:0]
	for _, t := range l.Times {
		if now.Sub(t) < 24*time.Hour {
			kept = append(kept, t)
		}
	}
	l.Times = kept
	if n := len(l.Times); n > 0 {
		if next = l.Times[n-1].Add(RepairMinInterval); now.Before(next) {
			return false, next
		}
	}
	if len(l.Times) >= RepairMaxPerDay {
		return false, l.Times[0].Add(24 * time.Hour)
	}
	l.Times = append(l.Times, now)
	return true, time.Time{}
}
