package controller

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/eyupio/zoomies/internal/backend"
	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/store"
)

// A folder a pool keeps in memory has a ceiling, and a job that writes past it
// fails a step with "no space left on device". GitHub records that as an
// ordinary failure, the runner's own output never carries a step's words, and
// a compiler deletes its work directory as the link fails, so by the time the
// job is reported failed nothing says the folder was full. What survives is
// the agent's samples: the fullest each folder got is kept on the job, and
// everything here reads from that.

// scratchFullFraction is how full a folder has to have been, at some sample
// before a failed job ended, for the failure to be read as the folder's. The
// fill itself falls between samples: a cold Go build that died at the link in
// a 1 GB /tmp was measured at 94% a sample earlier, and a job that used half
// of its folder failed for its own reasons.
const scratchFullFraction = 0.9

// scratchSample is a backend's measurement of a runner's folders as the store
// keeps it, in megabytes.
func scratchSample(use []backend.ScratchUse) []store.ScratchPeak {
	out := make([]store.ScratchPeak, 0, len(use))
	for _, u := range use {
		out = append(out, store.ScratchPeak{Path: u.Path, PeakMB: u.UsedBytes >> 20, SizeMB: u.SizeBytes >> 20})
	}
	return out
}

// scratchFilled is the folder a failed job filled, if one of its folders came
// within scratchFullFraction of its ceiling, and how full it got.
func scratchFilled(job *store.Job) (store.ScratchPeak, float64, bool) {
	if job == nil || job.State != store.JobCompleted || !store.IsFailedConclusion(job.Conclusion) {
		return store.ScratchPeak{}, 0, false
	}
	peak, fraction, ok := job.ScratchPeaks.Fullest()
	if !ok || fraction < scratchFullFraction {
		return store.ScratchPeak{}, 0, false
	}
	return peak, fraction, true
}

// scratchPercent is a folder's fullness as the whole number an operator reads.
func scratchPercent(fraction float64) int {
	return int(math.Round(fraction * 100))
}

// scratchKindOf is which of a runner's folders a path in the container is.
func scratchKindOf(path string) (store.ScratchKind, bool) {
	switch path {
	case backend.RunnerWorkMount:
		return store.ScratchWork, true
	case backend.RunnerTmpMount:
		return store.ScratchTmp, true
	case backend.DaemonStoreMount:
		return store.ScratchDaemon, true
	}
	return "", false
}

// scratchName is a folder as a sentence names it: its path, and what it is
// where the path alone does not say.
func scratchName(path string) string {
	kind, ok := scratchKindOf(path)
	if !ok {
		return path
	}
	label, _, _ := scratchFolder(kind)
	return fmt.Sprintf("%s (the %s)", path, lowerFirst(label))
}

// scratchFullFix is what gives a folder room, which is the pool's choice three
// ways: a bigger runner fits a bigger folder, a typed size asks for one
// outright, and disk has no ceiling.
func scratchFullFix(pool string) string {
	return fmt.Sprintf("give the folder room: raise the pool's memory per runner, so the folder is fitted larger; type a larger size for it under the pool's in-memory folders; or keep it on disk (Placement: Auto, or zoomies pools edit %s --tmpfs-tmp=false for /tmp). A folder is charged to the runner's memory limit, so a bigger one may need a bigger limit, which the pool editor proposes.", pool)
}

// explainScratchFull adds a folder that ran out of room to a finished job's
// explanation, as explainOOM adds a kill: it is the one fact about the job
// the conclusion GitHub recorded cannot tell anybody. A kill registered after
// it still wins the summary, because a kill is the surer fact.
func explainScratchFull(job *store.Job, out *JobExplanation) {
	peak, fraction, ok := scratchFilled(job)
	if !ok {
		return
	}
	out.Summary = fmt.Sprintf("An in-memory folder on this job's runner ran out of room: %s, kept in memory at %s, was %d%% full at the last sample before the job failed.",
		scratchName(peak.Path), humanMB(peak.SizeMB), scratchPercent(fraction))
	out.Fix = scratchFullFix(job.PoolID)
}

// scratchFullProblems reports, pool by pool, the jobs whose in-memory folder
// filled in the last hour. Per pool because the fix is the pool's: the folder's
// size, or whether it is in memory at all, is a setting of the pool, and the
// jobs it fails are the ones the pool runs.
func (c *Controller) scratchFullProblems(ctx context.Context, out *[]Problem) error {
	since := c.Now().Add(-problemWindow)
	jobs, err := c.st.FailedJobsWithScratchSince(ctx, since, 100)
	if err != nil {
		return fmt.Errorf("listing jobs whose in-memory folder filled: %w", err)
	}
	byPool := map[string][]*store.Job{}
	var pools []string
	for _, j := range jobs {
		if _, _, ok := scratchFilled(j); !ok {
			continue
		}
		if byPool[j.PoolID] == nil {
			pools = append(pools, j.PoolID)
		}
		byPool[j.PoolID] = append(byPool[j.PoolID], j)
	}
	slices.Sort(pools)
	for _, poolID := range pools {
		jobs := byPool[poolID]
		example := jobs[0]
		peak, fraction, _ := scratchFilled(example)
		name := poolID
		if p, err := c.st.GetPool(ctx, poolID); err == nil {
			name = p.Name
		}
		title := fmt.Sprintf("1 job filled an in-memory folder in pool %s in the last hour", name)
		if len(jobs) > 1 {
			title = fmt.Sprintf("%d jobs filled an in-memory folder in pool %s in the last hour", len(jobs), name)
		}
		detail := fmt.Sprintf("The most recent is %s in %s: %s, kept in memory at %s, was %d%% full at the last sample before the job failed. "+
			"A job that writes past a folder's ceiling fails a step with \"no space left on device\", which GitHub records as an ordinary failure.",
			workflowText(example.JobName), workflowText(example.Repo), scratchName(peak.Path), humanMB(peak.SizeMB), scratchPercent(fraction))
		*out = append(*out, Problem{
			Code:       "jobs.scratch_full",
			Severity:   config.SeverityWarning,
			Title:      title,
			Detail:     detail,
			Fix:        scratchFullFix(name),
			TargetKind: "pool", TargetID: poolID, Since: example.CompletedAt,
		})
	}
	return nil
}

// ScratchPeakView is how full one of a job's in-memory folders got, for the
// job's page: the folder, what was written in it at most, its ceiling, and the
// one as a share of the other.
type ScratchPeakView struct {
	Kind    store.ScratchKind `json:"kind,omitempty"`
	Label   string            `json:"label,omitempty"`
	Path    string            `json:"path"`
	PeakMB  int64             `json:"peak_mb"`
	SizeMB  int64             `json:"size_mb"`
	Percent int               `json:"percent"`
}

func scratchPeakViews(peaks store.ScratchPeaks) []ScratchPeakView {
	if len(peaks) == 0 {
		return nil
	}
	out := make([]ScratchPeakView, 0, len(peaks))
	for _, p := range peaks {
		v := ScratchPeakView{Path: p.Path, PeakMB: p.PeakMB, SizeMB: p.SizeMB, Percent: scratchPercent(p.Fraction())}
		if kind, ok := scratchKindOf(p.Path); ok {
			v.Kind = kind
			v.Label, _, _ = scratchFolder(kind)
		}
		out = append(out, v)
	}
	return out
}

// scratchEvidence is the fullest folder as facts a person can check.
func scratchEvidence(peak store.ScratchPeak, fraction float64) []Evidence {
	return []Evidence{
		{Kind: "scratch_peak", Label: scratchName(peak.Path) + " in memory, at its fullest", Value: strconv.Itoa(scratchPercent(fraction)), Unit: "%"},
		{Kind: "scratch_size", Label: scratchName(peak.Path) + " ceiling", Value: strconv.FormatInt(peak.SizeMB, 10), Unit: "MB"},
	}
}

// scratchFilledFor is scratchFilled as the class table asks it.
func scratchFilledFor(job *store.Job) bool {
	_, _, ok := scratchFilled(job)
	return ok
}
