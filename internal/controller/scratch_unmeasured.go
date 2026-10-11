package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/eyupio/zoomies/internal/backend"
	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/naming"
	"github.com/eyupio/zoomies/internal/store"
)

// A folder that fills is only ever seen by a sample, so a host whose agent
// cannot measure its runners' folders is a host where full-folder detection is
// off, and off silently: no jobs.scratch_full ever appears, which reads as
// folders that never fill. An agent says why it could not measure with every
// sample (backend.Stats.ScratchUnmeasured), and everything here turns that
// into a sentence on the host.

const (
	// scratchUnmeasuredAfter is how old a runner has to be before its folders
	// being unmeasured counts. The first samples arrive within a minute, and a
	// host judged on a runner's first would flap with every create.
	scratchUnmeasuredAfter = 2 * time.Minute
	// scratchSampleFresh is how recent a runner's sample has to be to be
	// evidence. The agent reports its last reading again when sampling fails,
	// with the time it was taken, and a reading from an hour ago says nothing
	// about whether the folders can be measured now; the host's other entries
	// say that its samples have stopped.
	scratchSampleFresh = 10 * time.Minute
)

// scratchUnmeasuredProblems names each host that cannot measure the in-memory
// folders of its runners.
func (c *Controller) scratchUnmeasuredProblems(ctx context.Context, out *[]Problem) error {
	hosts, err := c.st.ListHosts(ctx)
	if err != nil {
		return fmt.Errorf("listing hosts: %w", err)
	}
	now := c.Now()
	for _, h := range hosts {
		// A host already excluded from placement, or being dealt with, has its
		// own entries; this one would only repeat them.
		if h.Incompatible || h.Cordoned {
			continue
		}
		runners, err := c.st.ListRunnersForHost(ctx, h.ID, store.RunnerIdle, store.RunnerBusy)
		if err != nil {
			return fmt.Errorf("listing the runners on %s: %w", naming.ForSentence(h.Name), err)
		}
		reason, ok := hostScratchUnmeasured(runners, now)
		if !ok {
			continue
		}
		name := naming.ForSentence(h.Name)
		detail, fix := scratchUnmeasuredWhy(reason, name)
		*out = append(*out, Problem{
			Code:       "host.scratch_unmeasured",
			Severity:   config.SeverityInfo,
			Title:      name + " cannot tell how full its in-memory folders get",
			Detail:     detail,
			Fix:        fix,
			TargetKind: "host",
			TargetID:   h.ID,
		})
	}
	return nil
}

// hostScratchUnmeasured is why a host cannot measure its runners' folders, if
// it cannot. It cannot when it has runners that keep folders in memory, at
// least one has said why its folders were not measured, and none has had a
// folder measured: one that has proves the host can, and the others are that
// runner's own matter. A runner that keeps nothing in memory, one too young to
// have been sampled, one with no recent sample, and an agent that predates the
// measurement and says nothing either way are not evidence, so none of them
// counts for or against.
func hostScratchUnmeasured(runners []*store.Runner, now time.Time) (string, bool) {
	measured := false
	reasons := map[string]int{}
	for _, r := range runners {
		if r.Scratch.InMemory() == 0 || r.Age(now) < scratchUnmeasuredAfter {
			continue
		}
		var sample backend.Stats
		if len(r.ResourceSample) == 0 || json.Unmarshal(r.ResourceSample, &sample) != nil ||
			sample.SampledAt == nil || now.Sub(*sample.SampledAt) > scratchSampleFresh {
			continue
		}
		switch {
		case len(sample.Scratch) > 0:
			measured = true
		case sample.ScratchUnmeasured != "":
			reasons[sample.ScratchUnmeasured]++
		}
	}
	if measured || len(reasons) == 0 {
		return "", false
	}
	// The commonest reason is the one to fix first; a tie goes to the reason
	// that sorts first, so the sentence does not change between two reads of
	// the same fleet.
	var codes []string
	for code := range reasons {
		codes = append(codes, code)
	}
	slices.SortFunc(codes, func(a, b string) int {
		if reasons[a] != reasons[b] {
			return reasons[b] - reasons[a]
		}
		if a < b {
			return -1
		}
		return 1
	})
	return codes[0], true
}

// scratchUnmeasuredWhy is what happened and what to change for one of the
// reasons an agent gives. All four end the same way for a job: it fails as it
// would have, and nothing says the folder was why.
func scratchUnmeasuredWhy(reason, host string) (detail, fix string) {
	const consequence = " A job that fills one of those folders still fails with \"no space left on device\", but Zoomies cannot then say the folder was the cause, so jobs.scratch_full and the job's explanation stay silent."
	switch reason {
	case backend.ScratchExecRefused:
		return "The container engine on " + host + " refused to run a command in a runner. Zoomies measures an in-memory folder by running df in the runner's container, so it cannot see how full any of them get." + consequence,
			"allow exec through whatever answers for the engine's socket: a socket proxy that blocks the exec endpoint, or an authorisation plugin. The next sample clears this."
	case backend.ScratchNoDF:
		return "The runner image on " + host + " has no df command. Zoomies measures an in-memory folder by running df in the runner's container, so it cannot see how full any of them get." + consequence,
			"use an image that has df, which any with coreutils or busybox does and the stock runner image does. The next runner created from it clears this."
	case backend.ScratchTimedOut:
		return "The container engine on " + host + " did not answer, in time, a request to measure a runner's in-memory folders, on any runner it has. Zoomies cannot see how full any of them get." + consequence,
			"an engine that cannot answer a one-line command is overloaded or stalled; the host's other entries say how. This clears on its own once it answers."
	default:
		return "Zoomies ran df in a runner on " + host + " and could not read what it printed, so it cannot see how full the runner's in-memory folders get." + consequence,
			"on the host, run `docker exec <runner container> df -P -k /tmp` (or `podman exec`). Zoomies reads the table it prints, a row per folder with the mount point last; if the output is anything else, that image's df differs from the usual one."
	}
}
