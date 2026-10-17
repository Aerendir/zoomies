package controller

import "context"

// RetireRecoveryHost fences even a queue held by a request authenticated before
// retirement. Unfence cannot resume delivery until this queue has been sealed.
func (c *Controller) RetireRecoveryHost(ctx context.Context, id, confirm, actor string) error {
	c.recoveryMu.Lock()
	defer c.recoveryMu.Unlock()
	if err := c.st.RetireRecoveryHost(ctx, id, confirm, actor); err != nil {
		return err
	}
	q := c.queues.get(id)
	q.mu.Lock()
	q.retired = true
	q.pending = nil
	clear(q.inflight)
	q.mu.Unlock()
	c.hostChecks.forget(id)
	c.dropMemoryState(id)
	return nil
}
