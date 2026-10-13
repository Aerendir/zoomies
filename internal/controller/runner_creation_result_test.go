package controller

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/store"
)

func creationResultHarness(t *testing.T) (*harness, *store.Installation, *store.Pool, *store.Host, *store.Runner, agent.Task, agent.TaskResult) {
	t.Helper()
	h := newHarness(t)
	inst, pool, host := h.fleet()
	runner := h.runnerRow(pool, host, store.RunnerProvisioning)
	task, res := creationProjectionFixture()
	task.RunnerID = runner.ID
	task.PoolID = pool.ID
	task.Spec.RunnerID = runner.ID
	task.Spec.PoolID = pool.ID
	h.c.enqueue(host.ID, task)
	batch, err := h.c.PollTasks(h.ctx, host.ID, time.Second)
	if err != nil || len(batch.Tasks) != 1 {
		t.Fatalf("poll %+v %v", batch, err)
	}
	sent := batch.Tasks[0]
	res.TaskID = sent.ID
	res.RunnerID = runner.ID
	res.State = store.RunnerRegistering
	return h, inst, pool, host, runner, sent, res
}

func TestCreateResultRetainsTheDispatchedWhitelistBeforeAcknowledgingItsLease(t *testing.T) {
	h, inst, pool, host, r, task, res := creationResultHarness(t)
	// The embedded transport may prepare the delivered Spec. Current settings
	// and those shared pointers cannot rewrite the dispatch snapshot.
	task.Spec.Resources.CPUs = 99
	task.Spec.Image = "different:latest"
	pool.Image = "current:latest"
	if err := h.st.UpdatePool(h.ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := h.c.ReportResult(h.ctx, host.ID, res); err != nil {
		t.Fatal(err)
	}
	recorded, err := h.st.GetRunnerCreationObservation(h.ctx, inst.ID, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *recorded.Data.Issued.CPUs != 2 || recorded.Data.Issued.ImageReferenceSHA256 == "" || recorded.Data.TaskID != task.ID || recorded.Data.Reported.ImageDigest == nil {
		t.Fatal("lost frozen provenance")
	}
	if _, known := h.c.queues.get(host.ID).peek(task.ID); known {
		t.Fatal("lease not acknowledged")
	}
	after, err := h.st.GetRunner(h.ctx, r.ID)
	if err != nil || after.State != store.RunnerRegistering {
		t.Fatal("observation blocked lifecycle", err)
	}
	// A duplicate delivered after acknowledgement cannot renew the observation.
	if err := h.c.ReportResult(h.ctx, host.ID, res); err != nil {
		t.Fatal(err)
	}
	replay, err := h.st.GetRunnerCreationObservation(h.ctx, inst.ID, r.ID)
	if err != nil || replay.SHA256 != recorded.SHA256 || !replay.ReceivedAt.Equal(recorded.ReceivedAt) {
		t.Fatal("duplicate changed observation", err)
	}
}

func TestCreateResultDoesNotInventEvidenceForUnknownForeignFailedOrNotStartedTasks(t *testing.T) {
	for _, mode := range []string{"unknown", "foreign", "failed", "not_started", "mismatched_kind", "mismatched_runner", "missing_spec"} {
		t.Run(mode, func(t *testing.T) {
			h, inst, pool, host, r, task, res := creationResultHarness(t)
			reporter := host.ID
			switch mode {
			case "unknown":
				h.c.queues = newTaskQueues()
			case "foreign":
				reporter = h.host("foreign").ID
			case "failed":
				res.OK = false
				res.Error = "fixture failure"
			case "not_started":
				res.OK = false
				res.NotStarted = true
			case "mismatched_kind":
				res.Kind = agent.TaskStopRunner
				res.State = ""
			case "mismatched_runner":
				other := h.runnerRow(pool, host, store.RunnerProvisioning)
				res.RunnerID = other.ID
				res.State = ""
			case "missing_spec":
				q := h.c.queues.get(host.ID)
				q.mu.Lock()
				q.inflight[task.ID].creationTask = agent.Task{}
				q.mu.Unlock()
			}
			err := h.c.ReportResult(h.ctx, reporter, res)
			if mode == "foreign" && err == nil {
				t.Fatal("foreign host accepted")
			}
			if mode != "foreign" && err != nil {
				t.Fatal(err)
			}
			if _, err := h.st.GetRunnerCreationObservation(h.ctx, inst.ID, r.ID); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("invented %s evidence: %v", mode, err)
			}
		})
	}
}

func TestCreateResultObservationConflictPreservesFirstEvidenceAndDoesNotTrapARunner(t *testing.T) {
	h, inst, _, host, r, task, res := creationResultHarness(t)
	frozen, _ := h.c.queues.get(host.ID).creationTaskSnapshot(task.ID)
	data, err := buildRunnerCreationObservation(host.ID, frozen, res)
	if err != nil {
		t.Fatal(err)
	}
	first, err := h.st.PutRunnerCreationObservation(h.ctx, inst.ID, data)
	if err != nil {
		t.Fatal(err)
	}
	res.Digest = "sha256:" + strings.Repeat("b", 64)
	if err := h.c.ReportResult(h.ctx, host.ID, res); err != nil {
		t.Fatal("evidence conflict blocked lifecycle", err)
	}
	read, err := h.st.GetRunnerCreationObservation(h.ctx, inst.ID, r.ID)
	if err != nil || read.SHA256 != first.SHA256 {
		t.Fatal("overwrote immutable first result", err)
	}
	if _, known := h.c.queues.get(host.ID).peek(task.ID); known {
		t.Fatal("conflict trapped task lease")
	}
}

func TestCreateResultObservationScopeFailureDoesNotAcknowledgeLifecycleLease(t *testing.T) {
	h, _, _, host, r, task, res := creationResultHarness(t)
	// Closing the store makes ownership storage unavailable. The observer
	// must not acknowledge the lease or panic; required lifecycle writes still fail.
	if err := h.st.Close(); err != nil {
		t.Fatal(err)
	}
	h.c.recordRunnerCreationResult(h.ctx, host.ID, r, res)
	if _, known := h.c.queues.get(host.ID).peek(task.ID); !known {
		t.Fatal("observer acknowledged a lease directly")
	}
	if err := h.c.ReportResult(h.ctx, host.ID, res); err == nil {
		t.Fatal("required closed-store result read unexpectedly succeeded")
	}
	if _, known := h.c.queues.get(host.ID).peek(task.ID); !known {
		t.Fatal("required lifecycle failure lost retryable lease")
	}
}

func TestCreateResultLegacyDigestRemainsUnsupportedWhileNormalProgressContinues(t *testing.T) {
	h, inst, _, host, r, _, res := creationResultHarness(t)
	res.Digest = "legacy digest text"
	res.Kind = ""
	res.CompletedAt = time.Time{}
	res.ContainerStartedAt = nil
	if err := h.c.ReportResult(h.ctx, host.ID, res); err != nil {
		t.Fatal(err)
	}
	got, err := h.st.GetRunnerCreationObservation(h.ctx, inst.ID, r.ID)
	if err != nil || got.Data.Reported.DigestCoverage != "unsupported" || got.Data.Reported.ImageDigest != nil || got.Data.Reported.CompletedAt != nil {
		t.Fatal("invented legacy digest or time", err)
	}
}
