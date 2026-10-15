package controller

import (
	"context"

	"github.com/eyupio/zoomies/internal/store"
)

// RecheckHostPriority recomputes what a host's specs currently rank it at and
// republishes the card for it.
//
// There is nothing stored to recompute against: the priority is read back out
// of the reported specs, which every heartbeat re-measures, so the score is
// always the machine's current one and an upgrade the host ever has is
// re-placed on its own with the next report after it. What this forces is the
// frame at once, for the UI's re-check button, and a nudge so the next pass
// places against the figure now on the card rather than by whatever the last
// one carried.
func (c *Controller) RecheckHostPriority(ctx context.Context, hostID string) (store.Host, error) {
	h, err := c.st.GetHost(ctx, hostID)
	if err != nil {
		return store.Host{}, err
	}
	c.publishHost(h)
	c.Nudge()
	return *h, nil
}
