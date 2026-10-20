package agent

import (
	"context"
	"testing"
)

// silentTransport is a transport whose Heartbeat answers nothing at all:
// no response and no error. Nothing in the tree does this today; the guard
// exists so that a future transport cannot do it by accident.
type silentTransport struct {
	*fakeTransport
}

func (s *silentTransport) Heartbeat(context.Context, HeartbeatRequest) (*HeartbeatResponse, error) {
	return nil, nil
}

// TestAHeartbeatSurvivesATransportThatAnswersNothing pins the guard at the
// top of heartbeat. The heartbeat loop is this process's whole report path
// and nothing recovers a panic in it, so a transport that ever returned
// (nil, nil) would take the agent down with it, ending host reporting for
// every running job on the host. The guard makes the beat end quietly
// instead; this test is the contract's tripwire.
func TestAHeartbeatSurvivesATransportThatAnswersNothing(t *testing.T) {
	a, tr, _, _ := newAgent(t, 1)
	a.tr = &silentTransport{tr}

	// A panic fails the test on its own; saying so keeps the test's point
	// readable: the call must survive, not merely return.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("heartbeat panicked: %v", r)
		}
	}()
	if err := a.heartbeat(context.Background()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
}
