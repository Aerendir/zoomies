package channel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/eyupio/zoomies/internal/updates"
)

// LockFile is the engine's lock, in the deployment's configuration directory
// and not in the update folder. Its name is the engine's own.
const LockFile = "upgrade.lock"

// ErrRepairPending is returned by WriteRepair while an earlier repair request is
// still in the folder: root has not read it, and a second would only replace it.
var ErrRepairPending = errors.New("a repair request is already waiting for root")

// Facts is what the folder itself says of a stuck helper. The unit states and the
// processes are not in it: they are not files, and the agent asks systemd for them.
type Facts struct {
	RequestPresent bool
	RequestAge     time.Duration
	// RequestID is the waiting request's attempt, empty when it cannot be read.
	RequestID   string
	LockPresent bool
	LockAge     time.Duration
}

// Look reads the folder's request and the configuration directory's lock without
// following a link or opening anything but the request, which is read with the
// helper's own bounds. now is the caller's clock. A configDir of "" means the lock
// is not looked for.
func Look(dir, configDir string, now time.Time) (Facts, error) {
	var f Facts
	root, err := os.OpenRoot(dir)
	if err != nil {
		return f, fmt.Errorf("cannot open the update folder %s: %w", dir, err)
	}
	defer root.Close()
	switch info, err := root.Lstat(RequestFile); {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return f, fmt.Errorf("cannot look for the request in %s: %w", dir, err)
	default:
		f.RequestPresent, f.RequestAge = true, ageOf(now, info.ModTime())
	}
	// Unreadable is an answer too: the age is what matters, and a request that is
	// not a request is exactly the kind a stuck helper leaves.
	if req, found, err := PendingRequest(dir); err == nil && found {
		f.RequestID = req.ID
	}
	if configDir != "" {
		switch info, err := os.Lstat(filepath.Join(configDir, LockFile)); {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return f, fmt.Errorf("cannot look for %s in %s: %w", LockFile, configDir, err)
		default:
			f.LockPresent, f.LockAge = true, ageOf(now, info.ModTime())
		}
	}
	return f, nil
}

// ageOf is how long ago t was, never negative: a file stamped in the future by a
// skewed clock is brand new, not old.
func ageOf(now, t time.Time) time.Duration {
	if d := now.Sub(t); d > 0 {
		return d
	}
	return 0
}

// WriteRepair asks root to look at the helper. It is WriteRequest's counterpart
// for the repair units, with the same promises: written under a temporary name and
// renamed, so the path unit never sees half a document, and never making the
// folder.
func WriteRepair(dir string, r updates.RepairRequest) error {
	body, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("cannot encode the repair request: %w", err)
	}
	if _, err := updates.ParseRepairRequest(body); err != nil {
		return fmt.Errorf("refusing to write a repair request root would refuse: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("cannot open the update folder %s: %w; %s", dir, err, installHint)
	}
	defer root.Close()
	if _, err := root.Lstat(updates.RepairFile); err == nil {
		return fmt.Errorf("%w: %s is still in %s", ErrRepairPending, updates.RepairFile, dir)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cannot look for an earlier repair request in %s: %w", dir, err)
	}
	tmp, f, err := createTemp(root)
	if err != nil {
		return fmt.Errorf("cannot write in the update folder %s: %w; %s", dir, err, installHint)
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = root.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o640); err != nil {
		_ = f.Close()
		return fmt.Errorf("cannot set the mode of the repair request in %s: %w", dir, err)
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return fmt.Errorf("cannot write the repair request in %s: %w", dir, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("cannot finish the repair request in %s: %w", dir, err)
	}
	if err := root.Rename(tmp, updates.RepairFile); err != nil {
		return fmt.Errorf("cannot put the repair request in place in %s: %w", dir, err)
	}
	renamed = true
	return nil
}

// WithdrawRepair takes back a repair request root has not read, when it is the
// one with this id. It is for an attempt that ends before root answers, so that a
// request nobody wants is not acted on later.
func WithdrawRepair(dir, id string) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return
	}
	defer root.Close()
	info, err := root.Lstat(updates.RepairFile)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	f, err := root.Open(updates.RepairFile)
	if err != nil {
		return
	}
	body, err := readOpened(f, info, updates.MaxRepairBytes)
	_ = f.Close()
	if err != nil {
		return
	}
	if r, err := updates.ParseRepairRequest(body); err == nil && r.ID == id {
		_ = root.Remove(updates.RepairFile)
	}
}

// ReadRepairResult reads root's answer. Absent is not an error.
func ReadRepairResult(dir string) (updates.RepairResult, bool, error) {
	var r updates.RepairResult
	found, err := readOptional(filepath.Join(dir, updates.RepairResultFile), updates.MaxRepairBytes, &r)
	if err != nil || !found {
		return updates.RepairResult{}, false, err
	}
	if r.V != updates.WireVersion {
		return updates.RepairResult{}, false, fmt.Errorf("%s is version %d and this release reads version %d", updates.RepairResultFile, r.V, updates.WireVersion)
	}
	return r, true, nil
}
