// The documentation's sample repair history. This runs only against the
// screenshot harness's disposable database while its controller is stopped.
// It neither calls a model nor contacts GitHub. Terminal states are saved
// before restart, so the repair worker has nothing to execute.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eyupio/zoomies/internal/store"
)

func main() {
	if err := seed(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func seed() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("expected the screenshot database path")
	}
	path := filepath.Clean(os.Args[1])
	if filepath.Base(path) != "zoomies.db" || !strings.HasPrefix(filepath.Base(filepath.Dir(path)), "zoomies-screenshots-") {
		return fmt.Errorf("refusing to seed a database outside the screenshot fixture")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, store.Options{Path: path})
	if err != nil {
		return err
	}
	defer st.Close()
	user, err := st.GetUserByUsername(ctx, "alice")
	if err != nil {
		return err
	}
	inst, err := st.FindInstallationByTarget(ctx, "acme/widgets")
	if err != nil {
		return err
	}
	for _, sample := range []struct {
		pull                           int
		state, message, stage, failure string
	}{
		{42, "succeeded", "Sample repair: corrected a failing assertion; the PR checks passed. Normal review is still required.", "checks_passed", ""},
		{43, "blocked", "Sample repair: workflow edits are disabled for this repository. Review the policy before another attempt.", "prepare_fix", "unsupported_fix"},
	} {
		r := &store.EliRepair{
			DedupKey: fmt.Sprintf("screenshots:%d", sample.pull), InstallationID: inst.ID,
			Repo: "acme/widgets", PullNumber: sample.pull, UserID: user.ID,
			GitHubLogin: "alice", Trigger: "manual",
			Details: store.EliRepairDetails{Model: "demo (sample history)", Stage: sample.stage, FailureCode: sample.failure},
		}
		if _, err := st.EnqueueEliRepair(ctx, r, 5); err != nil {
			return err
		}
		r.State, r.Message = sample.state, sample.message
		if err := st.SaveEliRepair(ctx, r); err != nil {
			return err
		}
	}
	return nil
}
