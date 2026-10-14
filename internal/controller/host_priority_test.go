package controller

import (
	"testing"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/backend"
	"github.com/eyupio/zoomies/internal/store"
)

// The speed figures are what a host's priority is scored from, so a heartbeat
// that reports new ones lands them and then republishes the host: the card
// carries the score it was worked out from, and it is One renderer's answer,
// not a stale row.
func TestAHeartbeatWithNewSpeedFiguresRecordsAndRepublishesThem(t *testing.T) {
	h := newHarness(t)
	tr := h.c.EmbeddedTransport()

	resp, err := tr.Join(h.ctx, agent.JoinRequest{
		ProtocolVersion: agent.ProtocolVersion,
		Name:            "builder-1",
		Capacity:        4,
		OS:              "linux",
		Arch:            "amd64",
		Backends:        []backend.Info{{Kind: store.BackendDocker, Available: true, Version: "27.1.1"}},
	})
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	tr.SetCredentials(resp.HostID, resp.AgentToken)

	if _, err := tr.Heartbeat(h.ctx, agent.HeartbeatRequest{
		ProtocolVersion: agent.ProtocolVersion,
		Capacity:        4,
		CPUs:            8,
		MemoryMB:        16 * 1024,
		CPUMHz:          2900,
		MemoryMTs:       2133,
		DiskKind:        "hdd",
		Backends:        []backend.Info{{Kind: store.BackendDocker, Available: true, Version: "27.1.1"}},
	}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	host, err := h.st.GetHost(h.ctx, resp.HostID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if host.CPUMHz != 2900 || host.MemoryMTs != 2133 || host.DiskKind != "hdd" {
		t.Errorf("host = clock %d mhz, %d MT/s, %q of disk, want what the agent reported",
			host.CPUMHz, host.MemoryMTs, host.DiskKind)
	}
	view := h.c.HostView(host)
	if view.Priority == 0 {
		t.Errorf("the card reads priority %d from a host with specs, want the score", view.Priority)
	}
	if got := view.PrioritySource; got != "computed" {
		t.Errorf("PrioritySource = %q, want computed", got)
	}

	// An upgrade that leaves the figures alone changes nothing: a heartbeat
	// that repeats the same figures rewrites nothing, and the score stays
	// what the machine earns.
	before := h.c.HostView(host)
	if _, err := tr.Heartbeat(h.ctx, agent.HeartbeatRequest{
		ProtocolVersion: agent.ProtocolVersion,
		Capacity:        4,
		CPUs:            8,
		MemoryMB:        16 * 1024,
		CPUMHz:          2900,
		MemoryMTs:       2133,
		DiskKind:        "hdd",
	}); err != nil {
		t.Fatalf("repeated Heartbeat: %v", err)
	}
	after := h.c.HostView(host)
	if after.Priority != before.Priority {
		t.Errorf("the priority moved from %d to %d on a friendly band", before.Priority, after.Priority)
	}
}

// A pin is the operator's: a heartbeat, join and re-join all leave it, and a
// rebuilt machine reclaiming its row keeps it the way it keeps its labels.
func TestAHeartbeatAndARejoinLeaveThePinAlone(t *testing.T) {
	h := newHarness(t)
	tr := h.c.EmbeddedTransport()

	resp, err := tr.Join(h.ctx, agent.JoinRequest{
		ProtocolVersion: agent.ProtocolVersion,
		Name:            "builder-1",
		Capacity:        4,
		OS:              "linux",
		Arch:            "amd64",
	})
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	tr.SetCredentials(resp.HostID, resp.AgentToken)
	pin := 90
	if err := h.st.PatchHost(h.ctx, resp.HostID, store.HostChanges{PriorityOverride: &pin}); err != nil {
		t.Fatalf("PatchHost: %v", err)
	}

	if _, err := tr.Heartbeat(h.ctx, agent.HeartbeatRequest{
		ProtocolVersion: agent.ProtocolVersion,
		Capacity:        4,
		CPUs:            8,
		CPUMHz:          1000,
	}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	// The agent re-joined: the row is fresh, and the pin must survive it.
	rejoined := joinRequest("builder-1", h.joinToken(t, nil, 0))
	rejoined.PreviousToken = resp.AgentToken
	rejoined.PreviousHostID = resp.HostID
	second, err := h.c.Join(h.ctx, rejoined, "10.0.0.9")
	if err != nil {
		t.Fatalf("re-join: %v", err)
	}
	if second.HostID != resp.HostID {
		t.Fatalf("re-joined, got %s; want the row reclaimed", second.HostID)
	}

	got, err := h.st.GetHost(h.ctx, second.HostID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if got.PriorityOverride == nil || *got.PriorityOverride != 90 {
		t.Errorf("the pin did not survive a re-join: %+v", got.PriorityOverride)
	}
	view := h.c.HostView(got)
	if view.PrioritySource != "overridden" || view.Priority != 90 {
		t.Errorf("the card reads %d from %q after the re-join, want the operator's 90", view.Priority, view.PrioritySource)
	}
}
