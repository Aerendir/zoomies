//go:build unix

package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/updates"
	"github.com/eyupio/zoomies/internal/updates/channel"
)

// The repair is the root side of a stuck update helper. The service cannot
// reset a failed unit or rewrite one, and should not be able to, so when its
// agent finds the helper stopped it writes repair.json and the repair units
// start this. The request carries only the attempt it is for: the helper's state
// is looked at again here, as root, and what is done follows from what root
// finds. That is also why the order is fixed.
//
//  1. Units first: reset-failed, rewrite, start the path unit. A helper that
//     comes back reads a waiting request itself, so a request is never removed
//     from under one that is about to read it.
//  2. Only then a request or a lock that is provably abandoned: nothing running,
//     older than updates.OrphanAfter, no engine process.
//
// Nothing is touched while the helper's service or an engine is running.

const (
	repairLedgerFile = "repair-state.json"
	// repairSettle is how long the repair waits after starting the path unit for
	// the helper to take a waiting request, and repairPoll how often it looks.
	repairSettle = 6 * time.Second
	repairPoll   = 500 * time.Millisecond
)

// RepairOptions is what a repair needs. The helper's own options carry the
// folder, the lock and the binary, all read from root's pointer.
type RepairOptions struct {
	HelperOptions
	unitDir string
	run     commandRunner
	procs   func() [][]string
	settle  time.Duration
	sleep   func(time.Duration)
}

func (o RepairOptions) withDefaults() RepairOptions {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.ownerOf == nil {
		o.ownerOf = fileUID
	}
	if o.unitDir == "" {
		o.unitDir = systemdUnitDir
	}
	if o.run == nil {
		o.run = runCommand
	}
	if o.procs == nil {
		o.procs = channel.Processes
	}
	if o.settle == 0 {
		o.settle = repairSettle
	}
	if o.sleep == nil {
		o.sleep = time.Sleep
	}
	return o
}

func (o RepairOptions) logf(format string, args ...any) {
	fmt.Fprintf(o.Out, format+"\n", args...)
}

// RunUpdateRepair answers the repair request in the update folder. An error
// means it could not answer at all; every other outcome, a refusal included, is
// in repair-result.json.
func RunUpdateRepair(ctx context.Context, opts RepairOptions) error {
	o := opts.withDefaults()
	dir, err := openUpdateFolder(o.Dir, o.ServiceUID, o.ownerOf)
	if err != nil {
		return err
	}
	defer dir.Close()

	req, err := takeRepairRequest(dir, o.ServiceUID, o.ownerOf)
	if errors.Is(err, fs.ErrNotExist) {
		o.logf("there is no repair request in %s, so there is nothing to answer", o.Dir)
		return nil
	}
	started := o.Now()
	if err != nil {
		// No id to echo; the answer still says why, for whoever looks.
		o.logf("the repair request cannot be used: %v", err)
		return writeRepairResult(dir, updates.RepairResult{ID: "refused", Error: "the repair request cannot be used, and was removed; see the helper's log (journalctl -u " + UpdateRepairServiceUnit + ")", FinishedAt: started})
	}
	o.logf("repair %s asked for at %s", req.ID, req.RequestedAt.UTC().Format(time.RFC3339))

	res := updates.RepairResult{ID: req.ID}
	finish := func(r updates.RepairResult) error {
		r.ID, r.FinishedAt = req.ID, o.Now()
		o.logf("repair %s: healed=%v verdict=%s actions=%s %s", req.ID, r.Healed, r.Verdict, strings.Join(r.Actions, ","), r.Error)
		return writeRepairResult(dir, r)
	}

	ledger, err := loadRepairLedger(o.StateDir)
	if err != nil {
		o.logf("%v", err)
		res.Error = "the helper's repair record cannot be used, so no repair was made; the reason is in the log (journalctl -u " + UpdateRepairServiceUnit + ")"
		return finish(res)
	}
	diag, err := o.diagnose(ctx, dir)
	if err != nil {
		res.Error = "root could not look at the helper: " + err.Error()
		return finish(res)
	}
	verdict := updates.Classify(diag, updates.OrphanAfter)
	res.Verdict = verdict.Kind
	switch {
	case verdict.Kind == updates.StuckBusy:
		// A live helper may still be reading its request and a live engine owns
		// the lock. Nothing is touched, and nothing counts against the budget.
		res.Actions = []string{updates.ActionRefusedBusy}
		res.Error = "the helper is running an update now, so nothing was changed"
		return finish(res)
	case !verdict.Repair:
		res.Healed = true
		return finish(res)
	}
	if ok, next := ledger.Admit(o.Now()); !ok {
		res.Actions = []string{updates.ActionRefusedRateLimit}
		res.Error = "root has repaired this helper too recently or too often, so it did not try again; the next repair may start at " + next.UTC().Format(time.RFC3339)
		return finish(res)
	}
	// Saved before anything is changed, so a repair is counted however it ends.
	if err := ledger.save(o.StateDir); err != nil {
		o.logf("%v", err)
		res.Error = "the helper's repair record cannot be written, so no repair was made; the reason is in the log (journalctl -u " + UpdateRepairServiceUnit + ")"
		return finish(res)
	}

	actions, err := o.putUnitsRight(ctx)
	res.Actions = actions
	if err != nil {
		o.logf("%v", err)
		res.Error = "the helper's units could not be put right: " + err.Error()
		return finish(res)
	}

	// Give a helper that is back a moment to take what is waiting.
	for waited := time.Duration(0); waited < o.settle; waited += repairPoll {
		d, err := o.diagnose(ctx, dir)
		if err != nil || d.ServiceBusy() || !d.RequestPresent {
			break
		}
		o.sleep(repairPoll)
	}
	after, err := o.diagnose(ctx, dir)
	if err != nil {
		res.Error = "root could not look at the helper after repairing its units: " + err.Error()
		return finish(res)
	}
	if after.ServiceBusy() {
		// The helper started on the request it found. It is not ours to touch.
		res.Healed = true
		return finish(res)
	}
	removeRequest, removeLock := after.Orphaned()
	if removeRequest && o.stillIdle(ctx) {
		if err := dir.Remove(channel.RequestFile); err == nil {
			res.Actions = append(res.Actions, updates.ActionRemovedRequest)
		} else if !errors.Is(err, fs.ErrNotExist) {
			o.logf("cannot remove the abandoned request: %v", err)
		}
	}
	if removeLock && o.stillIdle(ctx) {
		if info, err := os.Lstat(o.LockPath); err == nil && info.Mode().IsRegular() {
			if err := os.Remove(o.LockPath); err == nil {
				res.Actions = append(res.Actions, updates.ActionRemovedLock)
			} else {
				o.logf("cannot remove the abandoned lock %s: %v", o.LockPath, err)
			}
		}
	}
	final, err := o.diagnose(ctx, dir)
	res.Healed = err == nil && final.PathWaiting() && (!final.RequestPresent || final.ServiceBusy())
	if !res.Healed && res.Error == "" {
		res.Error = "the helper's units were put right, but something is still in the way; run \"sudo zoomies updates helper status\" on this host"
	}
	return finish(res)
}

// diagnose is the same look the agent takes, taken again by root.
func (o RepairOptions) diagnose(ctx context.Context, dir *os.Root) (updates.Diagnosis, error) {
	_ = dir
	lockDir := ""
	if o.LockPath != "" {
		lockDir = filepath.Dir(o.LockPath)
	}
	return channel.Diagnose(ctx, channel.RunFunc(o.run), o.Dir, lockDir, o.Now(), o.procs)
}

// stillIdle is the last look before a file is removed: a helper that started
// since the last diagnosis is reading it.
func (o RepairOptions) stillIdle(ctx context.Context) bool {
	service := channel.QueryUnit(ctx, channel.RunFunc(o.run), UpdateServiceUnit)
	return !updates.Diagnosis{Service: service, EngineRunning: updates.EngineRunning(o.procs())}.ServiceBusy()
}

// putUnitsRight is the units half of "zoomies updates helper install": clear a
// failed state, write the units root rendered from its own pointer, and start
// the path unit. It reports what it did in fixed words.
func (o RepairOptions) putUnitsRight(ctx context.Context) ([]string, error) {
	var actions []string
	if _, err := o.run(ctx, "systemctl", "reset-failed", UpdateServiceUnit, UpdatePathUnit); err == nil {
		actions = append(actions, updates.ActionResetFailed)
	}
	pathUnit, serviceUnit := RenderUpdateUnits(o.BinaryPath, o.Dir)
	repairPath, repairService := RenderRepairUnits(o.BinaryPath, o.Dir)
	rewrote := false
	for _, u := range [][2]string{{UpdatePathUnit, pathUnit}, {UpdateServiceUnit, serviceUnit},
		{UpdateRepairPathUnit, repairPath}, {UpdateRepairServiceUnit, repairService}} {
		path := filepath.Join(o.unitDir, u[0])
		if old, err := os.ReadFile(path); err == nil && string(old) == u[1] {
			continue
		}
		if err := writeFileAtomic(path, []byte(u[1]), 0o644); err != nil {
			return actions, fmt.Errorf("cannot write %s: %w", path, err)
		}
		rewrote = true
	}
	if rewrote {
		actions = append(actions, updates.ActionRewroteUnits)
		if _, err := o.run(ctx, "systemctl", "daemon-reload"); err != nil {
			return actions, err
		}
		_, _ = o.run(ctx, "systemctl", "reset-failed", UpdateServiceUnit, UpdatePathUnit)
	}
	if _, err := o.run(ctx, "systemctl", "enable", "--now", UpdatePathUnit); err != nil {
		return actions, err
	}
	return append(actions, updates.ActionStartedPath), nil
}

// takeRepairRequest reads repair.json through the folder's handle and removes
// it before anything is done about it, so that a repair that goes wrong is not
// started again by the file it was started by.
func takeRepairRequest(dir *os.Root, wantUID int, ownerOf func(os.FileInfo) (int, bool)) (updates.RepairRequest, error) {
	f, info, err := openPlainFile(dir, updates.RepairFile)
	if err != nil {
		return updates.RepairRequest{}, err
	}
	defer f.Close()
	defer dir.Remove(updates.RepairFile)
	if uid, ok := ownerOf(info); !ok || uid != wantUID {
		return updates.RepairRequest{}, fmt.Errorf("it is not owned by the account the helper was installed for (uid %d)", wantUID)
	}
	body, err := io.ReadAll(io.LimitReader(f, updates.MaxRepairBytes+1))
	if err != nil {
		return updates.RepairRequest{}, err
	}
	if len(body) > updates.MaxRepairBytes {
		return updates.RepairRequest{}, fmt.Errorf("it is over the limit of %d bytes", updates.MaxRepairBytes)
	}
	return updates.ParseRepairRequest(body)
}

func writeRepairResult(dir *os.Root, r updates.RepairResult) error {
	r.V = updates.WireVersion
	r.ID = headOf(printable(r.ID, false), maxResultFieldBytes)
	r.Error = headOf(printable(r.Error, false), maxResultErrorBytes)
	r.Verdict = headOf(printable(r.Verdict, false), maxResultFieldBytes)
	body, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("cannot encode the repair result: %w", err)
	}
	return replaceFile(dir, updates.RepairResultFile, body, 0o644)
}

type repairLedger struct{ updates.RepairLedger }

// loadRepairLedger reads root's record of past repairs. Absent is a fresh
// start; a record that cannot be trusted is an error and not a fresh start,
// because treating it as one would reset the limit.
func loadRepairLedger(stateDir string) (*repairLedger, error) {
	root, err := openStateDir(stateDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	l := &repairLedger{}
	if _, err := root.Lstat(repairLedgerFile); errors.Is(err, fs.ErrNotExist) {
		return l, nil
	}
	path := filepath.Join(stateDir, repairLedgerFile)
	f, info, err := openPlainFile(root, repairLedgerFile)
	if err != nil {
		return nil, fmt.Errorf("the update helper's repair record %s cannot be used: %w", path, err)
	}
	defer f.Close()
	if err := checkStateOwner(path, info); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(f, maxHelperStateBytes+1))
	if err == nil && len(body) > maxHelperStateBytes {
		err = errors.New("it is over the size limit")
	}
	if err == nil {
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		err = dec.Decode(&l.RepairLedger)
	}
	if err != nil {
		return nil, fmt.Errorf("the update helper's repair record %s cannot be read: %v; inspect it, and remove it only if you mean to reset the limit", path, err)
	}
	return l, nil
}

func (l *repairLedger) save(stateDir string) error {
	root, err := openStateDir(stateDir)
	if err != nil {
		return err
	}
	defer root.Close()
	body, err := json.Marshal(l.RepairLedger)
	if err != nil {
		return err
	}
	return replaceFile(root, repairLedgerFile, body, 0o600)
}
