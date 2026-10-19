package store

import (
	"context"
	"fmt"
	"time"
)

// SetHostAutoAdjust records an operator's answer to whether the controller keeps
// a host at its recommendation: AutoAdjustOn, AutoAdjustOff, or empty to follow
// the fleet again. It keeps the time of the last automatic move, so changing the
// answer does not make the controller forget it moved the host a moment ago.
func (s *Store) SetHostAutoAdjust(ctx context.Context, hostID, mode string) error {
	if mode != "" && mode != AutoAdjustOn && mode != AutoAdjustOff {
		return fmt.Errorf("host auto-adjust %q: %w", mode, ErrConflict)
	}
	res, err := s.exec(ctx, `INSERT INTO host_auto_adjust (host_id, mode)
		SELECT id, ? FROM hosts WHERE id = ?
		ON CONFLICT(host_id) DO UPDATE SET mode=excluded.mode`, mode, hostID)
	if err != nil {
		return wrapWrite(err)
	}
	return affected(res, "host", hostID)
}

// MarkHostAutoAdjusted records that the controller has just moved a host's
// capacity or reserve, which is what its cooldown is measured from.
func (s *Store) MarkHostAutoAdjusted(ctx context.Context, hostID string, at time.Time) error {
	res, err := s.exec(ctx, `INSERT INTO host_auto_adjust (host_id, adjusted_at)
		SELECT id, ? FROM hosts WHERE id = ?
		ON CONFLICT(host_id) DO UPDATE SET adjusted_at=excluded.adjusted_at`, ms(at), hostID)
	if err != nil {
		return wrapWrite(err)
	}
	return affected(res, "host", hostID)
}
