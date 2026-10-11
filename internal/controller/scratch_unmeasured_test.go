package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/backend"
	"github.com/eyupio/zoomies/internal/store"
)

// keepsTmpInMemory records that a runner was given its /tmp in memory, as the
// controller does when it builds the create task.
func (h *harness) keepsTmpInMemory(t *testing.T, r *store.Runner) {
	t.Helper()
	scratch := store.RunnerScratch{Folders: []store.ScratchFolder{{Kind: store.ScratchTmp, AskedMB: 1024, SizeMB: 1024}}}
	if err := h.st.SetRunnerScratch(h.ctx, r.ID, scratch); err != nil {
		t.Fatalf("SetRunnerScratch: %v", err)
	}
}

// sampleOf reports one sample for a runner, taken ago before now.
func (h *harness) sampleOf(t *testing.T, host *store.Host, r *store.Runner, ago time.Duration, s backend.Stats) {
	t.Helper()
	at := h.c.Now().Add(-ago)
	s.SampledAt = &at
	if s.MemoryBytes == 0 {
		s.MemoryBytes = 512 << 20
	}
	if err := h.c.ReportRunners(h.ctx, host.ID, []agent.RunnerReport{{RunnerID: r.ID, Stats: s}}); err != nil {
		t.Fatalf("ReportRunners: %v", err)
	}
}

func problemFor(problems []Problem, code, targetID string) *Problem {
	for i := range problems {
		if problems[i].Code == code && problems[i].TargetID == targetID {
			return &problems[i]
		}
	}
	return nil
}

// A folder that fills is only ever seen by a sample, so a host that cannot
// measure its folders is a host where the whole of full-folder detection is
// off, and it is off silently: no jobs.scratch_full ever appears, which reads
// as folders that never fill. The host says so, with the reason its agent gave,
// once its runners have had time to be sampled.
func TestAHostThatCannotMeasureItsFoldersSaysWhyOnceItHasHadTimeToSample(t *testing.T) {
	h := newHarness(t)
	_, pool, host := h.fleet()
	r := h.seedRunner(t, pool, host, store.RunnerIdle)
	h.keepsTmpInMemory(t, r)

	// A runner a few seconds old has not been sampled long enough to say.
	h.sampleOf(t, host, r, 0, backend.Stats{ScratchUnmeasured: backend.ScratchExecRefused})
	problems, err := h.c.Problems(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p := problemFor(problems, "host.scratch_unmeasured", host.ID); p != nil {
		t.Fatalf("flagged a runner seconds old: %+v", p)
	}

	h.advance(3 * time.Minute)
	h.sampleOf(t, host, r, 0, backend.Stats{ScratchUnmeasured: backend.ScratchExecRefused})
	problems, err = h.c.Problems(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	p := problemFor(problems, "host.scratch_unmeasured", host.ID)
	if p == nil {
		t.Fatalf("problems = %+v, want host.scratch_unmeasured on the host", problems)
	}
	if p.TargetKind != "host" || p.Severity != "info" || p.Audience != AudienceFleet {
		t.Fatalf("problem = %+v, want an info entry on the host for the fleet", p)
	}
	for _, want := range []string{host.Name, "refused", "exec"} {
		if !strings.Contains(p.Title+" "+p.Detail+" "+p.Fix, want) {
			t.Fatalf("problem %+v does not mention %q", p, want)
		}
	}
}

// Each reason is a different change, so each is told differently, and none of
// the sentences blames the operator for a setting they were never given.
func TestEachReasonAHostCannotMeasureItsFoldersHasItsOwnAdvice(t *testing.T) {
	seen := map[string]string{}
	for reason, wantInFix := range map[string]string{
		backend.ScratchExecRefused: "exec",
		backend.ScratchNoDF:        "df",
		backend.ScratchUnreadable:  "df -P -k",
		backend.ScratchTimedOut:    "overloaded",
	} {
		detail, fix := scratchUnmeasuredWhy(reason, "vm-1")
		if !strings.Contains(fix, wantInFix) || !strings.Contains(detail, "vm-1") {
			t.Fatalf("%s: detail %q fix %q, want the host named and %q in the fix", reason, detail, fix, wantInFix)
		}
		if strings.Contains(detail+fix, "\u2014") {
			t.Fatalf("%s: an em dash in %q %q", reason, detail, fix)
		}
		if other, dup := seen[fix]; dup {
			t.Fatalf("%s and %s give the same advice: %q", reason, other, fix)
		}
		seen[fix] = reason
	}
}

// One runner that can be measured is proof the host can, and the others are
// then a runner's own matter: an image without df, say. Only a host where none
// of its runners can be measured has lost detection.
func TestAHostThatMeasuresAnyRunnerIsNotFlagged(t *testing.T) {
	h := newHarness(t)
	_, pool, host := h.fleet()
	blind := h.seedRunner(t, pool, host, store.RunnerIdle)
	seeing := h.seedRunner(t, pool, host, store.RunnerIdle)
	h.keepsTmpInMemory(t, blind)
	h.keepsTmpInMemory(t, seeing)
	h.advance(3 * time.Minute)
	h.sampleOf(t, host, blind, 0, backend.Stats{ScratchUnmeasured: backend.ScratchNoDF})
	h.sampleOf(t, host, seeing, 0, backend.Stats{Scratch: []backend.ScratchUse{{Path: backend.RunnerTmpMount, UsedBytes: 1 << 20, SizeBytes: 1024 << 20}}})

	problems, err := h.c.Problems(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p := problemFor(problems, "host.scratch_unmeasured", host.ID); p != nil {
		t.Fatalf("flagged a host that measures one of its runners: %+v", p)
	}
}

// A runner whose pool keeps nothing in memory has nothing to measure, and an
// agent that predates the measurement says nothing either way: neither is a
// host that cannot measure.
func TestOnlyFoldersInMemoryAndAnAgentThatSaysWhyCountAsUnmeasured(t *testing.T) {
	h := newHarness(t)
	_, pool, host := h.fleet()
	onDisk := h.seedRunner(t, pool, host, store.RunnerIdle)
	oldAgent := h.seedRunner(t, pool, host, store.RunnerIdle)
	h.keepsTmpInMemory(t, oldAgent)
	h.advance(3 * time.Minute)
	h.sampleOf(t, host, onDisk, 0, backend.Stats{ScratchUnmeasured: backend.ScratchNoDF})
	h.sampleOf(t, host, oldAgent, 0, backend.Stats{})

	problems, err := h.c.Problems(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p := problemFor(problems, "host.scratch_unmeasured", host.ID); p != nil {
		t.Fatalf("flagged a host with no evidence either way: %+v", p)
	}
}

// A sample the agent has not refreshed in a long while is an agent that is
// failing to sample, which other entries say. It is no evidence about folders.
func TestAStaleSampleSaysNothingAboutWhetherFoldersCanBeMeasured(t *testing.T) {
	h := newHarness(t)
	_, pool, host := h.fleet()
	r := h.seedRunner(t, pool, host, store.RunnerIdle)
	h.keepsTmpInMemory(t, r)
	h.advance(time.Hour)
	h.sampleOf(t, host, r, 30*time.Minute, backend.Stats{ScratchUnmeasured: backend.ScratchExecRefused})

	problems, err := h.c.Problems(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p := problemFor(problems, "host.scratch_unmeasured", host.ID); p != nil {
		t.Fatalf("flagged on a half-hour-old sample: %+v", p)
	}
}
