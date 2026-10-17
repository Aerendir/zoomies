package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AcknowledgeJob records that somebody has seen this job's failure and does
// not need it put in front of them again, and returns the job as it now is.
// It also reports whether this call is the one that recorded it.
//
// The first acknowledgement wins, decided inside the write rather than by a
// read the caller made first: two operators pressing the button at the same
// moment leave one name on the row, and the timeline entry and audit row the
// caller writes from the flag are also written once. Acknowledging is not
// resolving: the failure keeps its place in the failed view and its
// conclusion on GitHub, and only the attention list stops asking.
func (s *Store) AcknowledgeJob(ctx context.Context, jobID, by string) (*Job, bool, error) {
	return s.setJobAcknowledged(ctx, jobID, by, true)
}

// ClearJobAcknowledgement takes an acknowledgement back, so a failure somebody
// put away by mistake asks for attention again, and returns the job as it now
// is and whether this call removed anything. It is idempotent: clearing a
// failure nobody had acknowledged changes nothing and says so.
func (s *Store) ClearJobAcknowledgement(ctx context.Context, jobID string) (*Job, bool, error) {
	return s.setJobAcknowledged(ctx, jobID, "", false)
}

func (s *Store) setJobAcknowledged(ctx context.Context, jobID, by string, acknowledge bool) (*Job, bool, error) {
	var out *Job
	var recorded bool
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var (
			res sql.Result
			err error
		)
		if acknowledge {
			res, err = tx.ExecContext(ctx,
				`UPDATE jobs SET acknowledged_at = ?, acknowledged_by = ?
				 WHERE id = ? AND acknowledged_at IS NULL`,
				ms(s.Now()), by, jobID)
		} else {
			res, err = tx.ExecContext(ctx,
				`UPDATE jobs SET acknowledged_at = NULL, acknowledged_by = ''
				 WHERE id = ? AND acknowledged_at IS NOT NULL`, jobID)
		}
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			recorded = true
		}
		j, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = ?`, jobID))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("job %s: %w", jobID, ErrNotFound)
		}
		out = j
		return err
	})
	return out, recorded, err
}
