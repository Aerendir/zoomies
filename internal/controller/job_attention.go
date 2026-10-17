package controller

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/zoomies/internal/github"
)

// Failure triage runs without personal Eli setup. Closing a PR or passing a
// rerun must clear the fleet's inbox even when nobody has configured repairs.
func (c *Controller) jobAttentionLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if c.mayAct() && !c.transferDraining.Load() {
			c.refreshJobAttention(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *Controller) refreshJobAttention(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	runs, err := c.st.JobAttentionRunsToCheck(ctx, 5)
	if err != nil {
		return
	}
	for _, run := range runs {
		if ctx.Err() != nil {
			return
		}
		checked := c.st.Now()
		reason, known := c.jobAttentionOnGitHub(ctx, run.Repo, run.RunID)
		if err := c.st.RecordJobAttentionRun(ctx, run.Repo, run.RunID, reason, checked, known); err != nil {
			return
		}
	}
	_ = c.st.PruneJobAttentionRuns(ctx)
}

func (c *Controller) jobAttentionOnGitHub(ctx context.Context, repo string, run int64) (string, bool) {
	inst, err := c.st.FindInstallationByTarget(ctx, repo)
	if err != nil {
		return "", false
	}
	raw, err := c.ClientFor(ctx, inst.ID)
	if err != nil {
		return "", false
	}
	state, err := raw.GetWorkflowRun(ctx, repo, run)
	if err != nil {
		return "", false
	}
	if state != nil && state.Status == "completed" && state.Conclusion == "success" {
		return "fixed", true
	}
	client, ok := raw.(github.RepairClient)
	if !ok {
		return "", false
	}
	pr, err := client.RepairPullForRun(ctx, repo, run)
	var changed *github.RepairTargetChanged
	if errors.As(err, &changed) {
		return changed.Code, true
	}
	if err != nil {
		// A push run without a PR can still have a real failure to fix.
		// Neither an unsupported repair nor a read failure proves it settled.
		return "", false
	}
	checks, err := client.RepairChecks(ctx, repo, pr.HeadSHA)
	if err != nil {
		return "", false
	}
	if checks == "passed" {
		return "fixed", true
	}
	// Pending or cancelled checks neither settle an unknown failure nor undo
	// an earlier observed success. A fresh failed job is visible by its time.
	return "", checks == "failed"
}
