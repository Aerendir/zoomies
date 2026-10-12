package main

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

type prDelayItem struct {
	Repository  string `json:"repository"`
	PullNumber  int    `json:"pull_number"`
	WindowBasis string `json:"window_basis"`
	Result      struct {
		Version              string `json:"version"`
		State                string `json:"state"`
		HeadSHA              string `json:"head_sha"`
		TargetBranch         string `json:"target_branch"`
		ProtectionCoverage   string `json:"protection_coverage"`
		ObservationCoverage  string `json:"observation_coverage"`
		LastFinishingCheckID string `json:"last_finishing_check_id"`
		Checks               []struct {
			ID         string `json:"check_id"`
			State      string `json:"state"`
			Completion *int64 `json:"completion_delay_ms"`
			Timing     struct {
				PreStart  *int64 `json:"pre_start_ms"`
				Queue     *int64 `json:"queue_ms"`
				Startup   *int64 `json:"startup_ms"`
				Execution *int64 `json:"execution_ms"`
			} `json:"timing"`
		} `json:"checks"`
	} `json:"result"`
}

func jobsPRDelay(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "zoomies jobs pr-delay <job-id> --pull-number N", "Read observed required-check delay for this job's PR head.")
	cf := registerClientFlags(fs, true)
	pull := fs.Int("pull-number", 0, "PR number in the selected job's repository (required)")
	fs.example("zoomies jobs pr-delay job_01abc --pull-number 42", "zoomies jobs pr-delay job_01abc --pull-number 42 --output json")
	if err := fs.parse(args); err != nil {
		return err
	}
	id, err := fs.oneArg("a Zoomies job ID")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(id, "job_") || strings.ContainsAny(id, "/\\?#") || len(id) <= 4 || *pull < 1 || *pull > 2147483647 {
		return usagef("jobs pr-delay", "supply a Zoomies job ID and --pull-number with a positive PR number")
	}
	client, err := cf.client()
	if err != nil {
		return err
	}
	p, err := cf.printer(e)
	if err != nil {
		return err
	}
	var out prDelayItem
	raw, err := client.get(ctx, "/jobs/"+url.PathEscape(id)+"/pr-delay", url.Values{"pull_number": {strconv.Itoa(*pull)}}, &out)
	if err != nil {
		return err
	}
	if out.Result.Version == "" || out.Result.State == "" {
		return errors.New("this controller has no recognised PR delay result; upgrade the controller")
	}
	if p.structured() {
		return p.emit(raw)
	}
	p.keyValues([][2]string{{"repository", plain(out.Repository)}, {"PR", strconv.Itoa(out.PullNumber)},
		{"state", plain(out.Result.State)}, {"head", plain(out.Result.HeadSHA)}, {"target branch", plain(out.Result.TargetBranch)},
		{"protection coverage", plain(out.Result.ProtectionCoverage)}, {"observation coverage", plain(out.Result.ObservationCoverage)},
		{"window basis", plain(out.WindowBasis)}})
	rows := [][]string{}
	for _, check := range out.Result.Checks {
		rows = append(rows, []string{plain(check.ID), plain(check.State), prDelayMillis(check.Completion), prDelayMillis(check.Timing.PreStart),
			prDelayMillis(check.Timing.Queue), prDelayMillis(check.Timing.Startup), prDelayMillis(check.Timing.Execution)})
	}
	if len(rows) > 0 {
		p.table([]string{"check", "state", "completion delay", "pre-start", "queue", "startup", "execution"}, rows)
	}
	if out.Result.LastFinishingCheckID != "" {
		p.note("Latest completed check in observed evidence: %s", plain(out.Result.LastFinishingCheckID))
	}
	p.note("The window starts at the earliest observed job. Pre-start includes queue and startup.")
	p.note("Check results do not establish full merge readiness or a dependency critical path.")
	return nil
}

func prDelayMillis(v *int64) string {
	if v == nil {
		return "unknown"
	}
	if *v < 0 {
		return "invalid"
	}
	if *v == 0 {
		return "0s"
	}
	return millis(*v)
}
