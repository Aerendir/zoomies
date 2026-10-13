package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// MarkTransferredHosts discards source liveness, never credentials or history.
func (s *Store) MarkTransferredHosts(ctx context.Context) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO host_recovery(host_id) SELECT id FROM hosts`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE host_recovery SET completed_at=NULL WHERE retired_at IS NULL AND host_id IN (SELECT id FROM hosts)`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE hosts SET last_heartbeat=0`)
		return err
	})
}

// DuplicateAgentWindow is shared with the controller's ownership warning.
const DuplicateAgentWindow = time.Hour

type RecoveryHost struct {
	HostID     string     `json:"host_id"`
	Name       string     `json:"name"`
	State      string     `json:"state"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	RetiredAt  *time.Time `json:"retired_at,omitempty"`
	RetiredBy  string     `json:"retired_by,omitempty"`
}

func (h *Host) RecoveryState(now time.Time) string {
	if h.Retired {
		return "retired"
	}
	if !h.RecoveryRequired {
		return ""
	}
	if h.LastHeartbeat.UnixMilli() > 0 && h.Healthy(now) && !h.Incompatible && h.TokenHash != "" && (h.AgentSessionAltAt == nil || now.Sub(*h.AgentSessionAltAt) >= DuplicateAgentWindow) {
		return "verified"
	}
	return "awaiting_recovery"
}

func (s *Store) RecoveryHosts(ctx context.Context) ([]RecoveryHost, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT host_id, COALESCE(h.name,''), retired_at, retired_by, completed_at FROM host_recovery r LEFT JOIN hosts h ON h.id=r.host_id ORDER BY host_id`)
	if err != nil {
		return nil, err
	}
	out := []RecoveryHost{}
	for rows.Next() {
		var v RecoveryHost
		var retired, completed sql.NullInt64
		if err := rows.Scan(&v.HostID, &v.Name, &retired, &v.RetiredBy, &completed); err != nil {
			rows.Close()
			return nil, err
		}
		v.RetiredAt = atp(retired)
		v.VerifiedAt = atp(completed)
		v.State = "awaiting_recovery"
		if completed.Valid {
			v.State = "verified"
		}
		if retired.Valid {
			v.State = "retired"
		}
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].State == "retired" || out[i].VerifiedAt != nil {
			continue
		}
		h, err := s.GetHost(ctx, out[i].HostID)
		if err == nil {
			out[i].State = h.RecoveryState(s.Now())
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	return out, nil
}

// RetireRecoveryHost changes destination records only. No task is queued and
// no runner cleanup confirmation is fabricated for a machine we cannot see.
func (s *Store) RetireRecoveryHost(ctx context.Context, id, confirm, actor string) error {
	if confirm != id || id == "" {
		return fmt.Errorf("confirm the exact host ID to retire: %w", ErrConflict)
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		var fenced string
		err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, SettingRecoveryFenced).Scan(&fenced)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err != nil || fenced != "true" {
			return fmt.Errorf("retirement requires the recovery fence: %w", ErrConflict)
		}
		var beat int64
		var retired sql.NullInt64
		err = tx.QueryRowContext(ctx, `SELECT h.last_heartbeat,r.retired_at FROM hosts h LEFT JOIN host_recovery r ON r.host_id=h.id WHERE h.id=?`, id).Scan(&beat, &retired)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var required int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM host_recovery WHERE host_id=? AND completed_at IS NULL`, id).Scan(&required); err != nil {
			return err
		}
		if required != 1 || retired.Valid || s.Now().Sub(at(beat)) < HeartbeatTimeout {
			return fmt.Errorf("only an unreachable carried-over host may be retired: %w", ErrConflict)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE host_recovery SET retired_at=?,retired_by=? WHERE host_id=?`, ms(s.Now()), actor, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE hosts SET cordoned=1,token_hash='' WHERE id=?`, id)
		return err
	})
}

func (s *Store) CheckHostRecovery(ctx context.Context) error {
	hosts, err := s.RecoveryHosts(ctx)
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if h.State == "awaiting_recovery" {
			return fmt.Errorf("host %s must reconnect and pass checks or be explicitly retired: %w", h.HostID, ErrConflict)
		}
	}
	return nil
}

// ReenrolRecoveryHost keeps historical runner references during cutover.
func (s *Store) ReenrolRecoveryHost(ctx context.Context, h *Host) error {
	if err := s.UpdateHost(ctx, h); err != nil {
		return err
	}
	res, err := s.exec(ctx, `UPDATE hosts SET token_hash=?,features=?,throttle=? WHERE id=? AND NOT EXISTS(SELECT 1 FROM host_recovery WHERE host_id=hosts.id AND retired_at IS NOT NULL)`, h.TokenHash, h.Features, h.Throttle, h.ID)
	if err != nil {
		return err
	}
	return affected(res, "host", h.ID)
}

// CompleteRecovery checks and lifts in one writer transaction: a concurrent
// heartbeat reporting incompatible or duplicate agents cannot slip between
// validation and the write that permits the controller to act.
func (s *Store) CompleteRecovery(ctx context.Context) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT r.host_id,r.retired_at,h.last_heartbeat,h.incompatible,h.token_hash,h.agent_session_alt_at FROM host_recovery r LEFT JOIN hosts h ON h.id=r.host_id WHERE r.completed_at IS NULL`)
		if err != nil {
			return err
		}
		now := s.Now()
		for rows.Next() {
			var id string
			var retired, beat, incompatible, alternated sql.NullInt64
			var token sql.NullString
			if err := rows.Scan(&id, &retired, &beat, &incompatible, &token, &alternated); err != nil {
				rows.Close()
				return err
			}
			if retired.Valid {
				continue
			}
			h := Host{ID: id, RecoveryRequired: true, LastHeartbeat: at(beat.Int64), Incompatible: incompatible.Int64 != 0, TokenHash: token.String, AgentSessionAltAt: atp(alternated)}
			if !beat.Valid || h.RecoveryState(now) != "verified" {
				rows.Close()
				return fmt.Errorf("host %s must reconnect and pass checks or be explicitly retired: %w", id, ErrConflict)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE host_recovery SET completed_at=? WHERE retired_at IS NULL AND completed_at IS NULL`, ms(now)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM settings WHERE key IN (?,?,?)`, SettingRecoveryFenced, SettingRecoveryFencedReason, SettingTransferDraining)
		return err
	})
}
