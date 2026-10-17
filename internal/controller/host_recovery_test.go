package controller

import (
	"errors"
	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/scheduler"
	"github.com/eyupio/zoomies/internal/store"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransferredHostReenrolmentProvesOwnershipAndKeepsHistory(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	h := newHarnessAt(t, func() time.Time { return now })
	first, err := h.c.Join(h.ctx, joinRequest("recovered", h.joinToken(t, nil, 0)), "10.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	host, err := h.st.GetHost(h.ctx, first.HostID)
	if err != nil {
		t.Fatal(err)
	}
	inst := h.installation()
	pool := h.pool(inst, "history")
	runner := h.runnerRow(pool, host, store.RunnerRemoved)
	if err := h.st.MarkTransferredHosts(h.ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.st.SetRecoveryFence(h.ctx, true, "transfer"); err != nil {
		t.Fatal(err)
	}
	if err := h.c.LoadFence(h.ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Unfence(h.ctx); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("unresolved host: %v", err)
	}
	req := joinRequest("recovered", h.joinToken(t, nil, 0))
	req.PreviousHostID = first.HostID
	if _, err := h.c.Join(h.ctx, req, "10.0.0.3"); err == nil {
		t.Fatal("join token claimed imported host without ownership proof")
	}
	req.PreviousToken = first.AgentToken
	second, err := h.c.Join(h.ctx, req, "10.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	if second.HostID != first.HostID {
		t.Fatal("re-enrolment changed historical identity")
	}
	if _, err := h.st.GetRunner(h.ctx, runner.ID); err != nil {
		t.Fatalf("re-enrolment destroyed history: %v", err)
	}
	if err := h.st.CheckHostRecovery(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.c.NoteAgentSession(h.ctx, second.HostID, "a")
	h.c.NoteAgentSession(h.ctx, second.HostID, "b")
	h.c.NoteAgentSession(h.ctx, second.HostID, "a")
	if err := h.c.Unfence(h.ctx); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate ownership warning ignored: %v", err)
	}
	now = now.Add(time.Hour + time.Minute)
	if _, err := h.c.Heartbeat(h.ctx, second.HostID, agent.HeartbeatRequest{ProtocolVersion: agent.ProtocolVersion, Capacity: 2}); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Unfence(h.ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRetirementNeverDeliversCleanupAndCannotReenrol(t *testing.T) {
	h := newHarness(t)
	first, err := h.c.Join(h.ctx, joinRequest("offline", h.joinToken(t, nil, 0)), "10.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.st.MarkTransferredHosts(h.ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.st.SetRecoveryFence(h.ctx, true, "transfer"); err != nil {
		t.Fatal(err)
	}
	if err := h.c.LoadFence(h.ctx); err != nil {
		t.Fatal(err)
	}
	q := h.c.queues.get(first.HostID)
	q.enqueue(agent.Task{Kind: agent.TaskStopRunner, RunnerID: "run_previous"})
	if err := h.c.RetireRecoveryHost(h.ctx, first.HostID, first.HostID, "administrator"); err != nil {
		t.Fatal(err)
	}
	if q.enqueue(agent.Task{Kind: agent.TaskCreateRunner, RunnerID: "run_new"}) {
		t.Fatal("retired queue accepted work")
	}
	if tasks := q.take(10, h.c.Now()); len(tasks) != 0 {
		t.Fatalf("a request holding the old queue could modify the remote machine: %+v", tasks)
	}
	req := joinRequest("offline", h.joinToken(t, nil, 0))
	req.PreviousHostID = first.HostID
	req.PreviousToken = first.AgentToken
	if _, err := h.c.Join(h.ctx, req, "10.0.0.3"); err == nil {
		t.Fatal("retired host re-enrolled")
	}
	if _, err := h.c.Heartbeat(h.ctx, first.HostID, agent.HeartbeatRequest{}); !errors.Is(err, agent.ErrHostGone) {
		t.Fatalf("retired host heartbeat: %v", err)
	}
	if err := h.c.Unfence(h.ctx); err != nil {
		t.Fatal(err)
	}
	if tasks, err := h.c.PollTasks(h.ctx, first.HostID, 0); !errors.Is(err, agent.ErrHostGone) || tasks != nil {
		t.Fatalf("retirement sent work or cleanup: %+v %v", tasks, err)
	}
	host, err := h.st.GetHost(h.ctx, first.HostID)
	if err != nil {
		t.Fatal(err)
	}
	if host.Available(h.c.Now()) || host.Healthy(h.c.Now()) {
		t.Fatal("retired host became active")
	}
}

func TestRecoveryWaitsForAnInflightTaskPollBeforeRetiring(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	var block atomic.Bool
	var enteredOnce sync.Once
	entered, release := make(chan struct{}), make(chan struct{})
	clock := func() time.Time {
		if block.Load() {
			enteredOnce.Do(func() { close(entered) })
			<-release
		}
		return now
	}
	h := newHarnessAt(t, clock)
	joined, err := h.c.Join(h.ctx, joinRequest("polling", h.joinToken(t, nil, 0)), "10.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.st.MarkTransferredHosts(h.ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.st.SetRecoveryFence(h.ctx, true, "transfer"); err != nil {
		t.Fatal(err)
	}
	h.c.queues.get(joined.HostID).enqueue(agent.Task{Kind: agent.TaskStopRunner, RunnerID: "run_previous"})
	block.Store(true)
	pollDone := make(chan struct {
		batch *agent.TaskBatch
		err   error
	}, 1)
	go func() {
		batch, err := h.c.PollTasks(h.ctx, joined.HostID, 0)
		pollDone <- struct {
			batch *agent.TaskBatch
			err   error
		}{batch, err}
	}()
	<-entered
	// The fence rides into the controller only now, with the poll already
	// admitted: the recovery row is written before the block goes on, and
	// this is the read that activates it, so nothing in it answers the
	// clock. A poll admitted before the fence is the ordering the
	// serialisation is for; with the fence in memory before the poll began,
	// PollTasks would return at mayAct before it reached the clock and the
	// hook above would never fire.
	if err := h.c.LoadFence(h.ctx); err != nil {
		t.Fatal(err)
	}
	retireDone := make(chan error, 1)
	go func() {
		retireDone <- h.c.RetireRecoveryHost(h.ctx, joined.HostID, joined.HostID, "administrator")
	}()
	select {
	case err := <-retireDone:
		t.Fatalf("retirement overtook the in-flight task poll: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	block.Store(false)
	close(release)
	poll := <-pollDone
	if poll.err != nil || poll.batch == nil || len(poll.batch.Tasks) != 1 {
		t.Fatalf("task poll failed: %+v", poll)
	}
	if err := <-retireDone; err != nil {
		t.Fatal(err)
	}
}

func TestAgentReportsWaitForRecoveryRetirementLock(t *testing.T) {
	h := newHarness(t)
	joined, err := h.c.Join(h.ctx, joinRequest("reporting", h.joinToken(t, nil, 0)), "10.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	h.c.recoveryMu.Lock()
	resultDone := make(chan error, 1)
	go func() {
		resultDone <- h.c.ReportResult(h.ctx, joined.HostID, agent.TaskResult{})
	}()
	runnersDone := make(chan error, 1)
	go func() {
		runnersDone <- h.c.ReportRunners(h.ctx, joined.HostID, nil)
	}()
	select {
	case err := <-resultDone:
		t.Fatalf("task result crossed the recovery lock: %v", err)
	case err := <-runnersDone:
		t.Fatalf("runner report crossed the recovery lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	h.c.recoveryMu.Unlock()
	if err := <-resultDone; err != nil {
		t.Fatal(err)
	}
	if err := <-runnersDone; err != nil {
		t.Fatal(err)
	}
}

func TestRetiredHostDoesNotCauseAutomaticMachineRelease(t *testing.T) {
	p := demandProvider("lab")
	pool := demandPool("alpha")
	host := demandHost("host_1", 2, 0)
	host.LastHeartbeat = machineNow.Add(-24 * time.Hour)
	m := demandMachine("mach_1", p, store.MachineReady, 48*time.Hour)
	m.HostID = host.ID
	m.IdleSince = ptrTime(machineNow.Add(-24 * time.Hour))
	s := MachineSnapshot{Now: machineNow, Pools: []*store.Pool{pool}, Hosts: []*store.Host{host}, Providers: []*store.Provider{p}, Machines: []*store.Machine{m}, Limits: demandLimits(), Plan: scheduler.Plan{Pools: []scheduler.PoolPlan{demandPoolPlan(pool, 0, 0, 0, "")}}}
	if got := planFor(t, DecideMachines(s), p.ID); len(got.Drain) != 1 {
		t.Fatalf("fixture did not qualify for release: %+v", got)
	}
	host.Retired = true
	if got := planFor(t, DecideMachines(s), p.ID); len(got.Drain) != 0 {
		t.Fatalf("retirement would modify remote machine: %+v", got)
	}
}

func TestRecoveryWaitsForAnInflightHeartbeatBeforeRetiringOrResuming(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	var block atomic.Bool
	var enteredOnce sync.Once
	entered, release := make(chan struct{}), make(chan struct{})
	clock := func() time.Time {
		if block.Load() {
			enteredOnce.Do(func() { close(entered) })
			<-release
		}
		return now
	}
	h := newHarnessAt(t, clock)
	joined, err := h.c.Join(h.ctx, joinRequest("returning", h.joinToken(t, nil, 0)), "10.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.st.MarkTransferredHosts(h.ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.st.SetRecoveryFence(h.ctx, true, "transfer"); err != nil {
		t.Fatal(err)
	}
	if err := h.c.LoadFence(h.ctx); err != nil {
		t.Fatal(err)
	}
	block.Store(true)
	type beatResult struct {
		response *agent.HeartbeatResponse
		err      error
	}
	beatDone := make(chan beatResult, 1)
	go func() {
		response, err := h.c.Heartbeat(h.ctx, joined.HostID, agent.HeartbeatRequest{ProtocolVersion: agent.ProtocolVersion, Capacity: 2})
		beatDone <- beatResult{response, err}
	}()
	<-entered
	retireDone, unfenceDone := make(chan error, 1), make(chan error, 1)
	go func() { retireDone <- h.c.RetireRecoveryHost(h.ctx, joined.HostID, joined.HostID, "administrator") }()
	go func() { unfenceDone <- h.c.Unfence(h.ctx) }()
	select {
	case err := <-retireDone:
		t.Errorf("retirement overtook the returning host's heartbeat: %v", err)
		retireDone <- err
	case err := <-unfenceDone:
		t.Errorf("recovery resumed before the in-flight heartbeat completed: %v", err)
		unfenceDone <- err
	case <-time.After(50 * time.Millisecond):
	}
	block.Store(false)
	close(release)
	beat := <-beatDone
	if beat.err != nil || beat.response == nil || !beat.response.MutationsPaused || len(beat.response.UnknownRunners) != 0 {
		t.Fatalf("in-flight response authorised work or cleanup: %+v %v", beat.response, beat.err)
	}
	if err := <-retireDone; !errors.Is(err, store.ErrConflict) {
		t.Fatalf("a returning host was retired: %v", err)
	}
	if err := <-unfenceDone; err != nil {
		t.Fatal(err)
	}
}
