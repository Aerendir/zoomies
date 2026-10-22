package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/backend"
	"github.com/eyupio/zoomies/internal/store"
)

// BenchmarkHeartbeat is one beat of one agent, from the controller's side: what
// Heartbeat does when a host that has been running a while says it is still here,
// the machine it is, its load, and how each of its runners is getting on. A fleet
// of hosts makes one of these every few seconds each, through the same single
// writer that webhook deliveries are waiting for.
//
// The beat is the steady one. The host's specification, backends and features were
// recorded by the beat before, so nothing in it is news, and every runner reports a
// fresh sample of what it is using. Idle is a host with runners waiting for a job;
// busy is the same host with every runner in the middle of one, which is the beat
// that records usage against a job as well.
func BenchmarkHeartbeat(b *testing.B) {
	for _, c := range []struct {
		name    string
		runners int
		state   store.RunnerState
	}{
		{"runners=0", 0, store.RunnerIdle},
		{"runners=8/idle", 8, store.RunnerIdle},
		{"runners=8/busy", 8, store.RunnerBusy},
	} {
		b.Run(c.name, func(b *testing.B) {
			h := newHarness(b)
			_, pools := benchPools(h, max(c.runners, 1))
			host := h.host("vm-000")
			now := time.Now()

			cpu, free := 37.5, int64(48*1024)
			req := agent.HeartbeatRequest{
				ProtocolVersion: agent.ProtocolVersion, Version: "v1.2.3", Capacity: host.Capacity,
				Features: []string{agent.FeatureSelfUpdate},
				CPUs:     host.CPUs, MemoryMB: host.MemoryMB, CPUMHz: 3000, MemoryMTs: 3200, DiskKind: "ssd",
				DiskTotalMB: host.DiskTotalMB, DiskFreeMB: host.DiskFreeMB,
				Backends: []backend.Info{{Kind: store.BackendDocker, Available: true, Version: "27.3.1", Endpoint: "unix:///var/run/docker.sock", Limits: store.LimitSupport{Known: true, CPU: true, Memory: true, Pids: true}}},
				Usage:    &store.HostUsage{CPUPercent: &cpu, MemoryAvailableMB: &free, SampledAt: now},
			}
			for i := range c.runners {
				r := h.seedRunner(b, pools[i], host, c.state)
				req.Runners = append(req.Runners, agent.RunnerReport{
					RunnerID: r.ID, Handle: backend.Handle(fmt.Sprintf("c%012d", i)), Phase: backend.PhaseRunning,
					Stats:      backend.Stats{SampledAt: &now, CPUPercent: 120 + float64(i), MemoryBytes: int64(900+i) << 20},
					ObservedAt: now,
				})
			}

			// The first beat records what the host is, and the runners' containers; the
			// timed ones are the beats after it, which is what a host does for as long as
			// it runs.
			resp, err := h.c.Heartbeat(h.ctx, host.ID, req)
			if err != nil || !resp.OK {
				b.Fatalf("Heartbeat = %+v, err = %v; want the host accepted", resp, err)
			}
			for _, r := range h.runners() {
				if r.State != c.state || len(r.ResourceSample) == 0 {
					b.Fatalf("after the first beat runner %s is %s with a sample of %q; want it %s and sampled", r.ID, r.State, r.ResourceSample, c.state)
				}
			}
			if got := len(h.runners()); got != c.runners {
				b.Fatalf("the host has %d runners; want %d", got, c.runners)
			}

			b.ReportAllocs()
			for b.Loop() {
				if _, err := h.c.Heartbeat(h.ctx, host.ID, req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
