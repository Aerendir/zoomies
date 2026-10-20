package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/store"
)

func TestRunningJobsExpireOnlyBeyondGitHubsLimitAndMargin(t *testing.T) {
	h := newHarness(t)
	now := h.c.Now()
	h.c.clock = func() time.Time { return now }
	for i, age := range []time.Duration{121*time.Hour + time.Second, 121 * time.Hour, 120 * time.Hour, time.Hour} {
		start := now.Add(-age)
		if _, _, err := h.st.ApplyJob(h.ctx, &store.Job{GitHubJobID: int64(i + 1), State: store.JobInProgress, QueuedAt: start.Add(-time.Hour), StartedAt: &start}); err != nil {
			t.Fatal(err)
		}
	}
	for i, age := range []time.Duration{841*time.Hour + time.Second, 841 * time.Hour, 200 * time.Hour} {
		if _, _, err := h.st.ApplyJob(h.ctx, &store.Job{GitHubJobID: int64(i + 5), State: store.JobInProgress, QueuedAt: now.Add(-age)}); err != nil {
			t.Fatal(err)
		}
	}
	// Run the actual background pass: a sweep never wired into it leaves the
	// runner protected indefinitely, even if calling the helper would work.
	h.c.housekeep(h.ctx, &housekeeping{})
	h.c.housekeep(h.ctx, &housekeeping{})
	for id := int64(1); id <= 7; id++ {
		j, err := h.st.GetJobByGitHubID(h.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if id == 1 || id == 5 {
			if j.State != store.JobCompleted || j.Conclusion != "stale" || j.CompletedAt == nil || !j.CompletedAt.Equal(now) {
				t.Fatalf("job %d = %+v, want completed as stale now", id, j)
			}
			events := h.timeline(j.ID)
			if len(events) != 1 || events[0].Kind != store.JobEventCompleted || !strings.Contains(events[0].Message, "presumed cancelled or lost") {
				t.Fatalf("job %d timeline = %+v, want one explanation", id, events)
			}
		} else if j.State != store.JobInProgress {
			t.Fatalf("job %d retired too soon: %s", id, j.State)
		}
	}
}

func TestALostRunnersUnsettledJobEventuallyLetsItBeReaped(t *testing.T) {
	h := newHarness(t)
	r, job := lostRunnerOnAJob(t, h)
	now := job.StartedAt.Add(121 * time.Hour)
	h.c.clock = func() time.Time { return now }
	if err := h.c.Reconcile(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.c.lifecycleCalls.Wait()
	if h.hasTaskOfKind(r.HostID, agent.TaskRemoveRunner) {
		t.Fatal("removed the runner before its job was retired")
	}
	now = now.Add(time.Second)
	h.c.housekeep(h.ctx, &housekeeping{})
	if err := h.c.Reconcile(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.c.lifecycleCalls.Wait()
	after, err := h.st.GetRunner(h.ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != store.RunnerRemoved || !h.hasTaskOfKind(r.HostID, agent.TaskRemoveRunner) {
		t.Fatalf("runner state = %s, want removed with a remove task after stale completion", after.State)
	}
}
