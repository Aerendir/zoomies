package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

// UsageBucket uses fixed elapsed-hour or elapsed-day intervals anchored at the
// requested start. Counts are events; time is clipped to bucket boundaries.
type UsageBucket struct {
	From             time.Time `json:"from"`
	Queued           int       `json:"queued"`
	Started          int       `json:"started"`
	Succeeded        int       `json:"succeeded"`
	Failed           int       `json:"failed"`
	Cancelled        int       `json:"cancelled"`
	Unknown          int       `json:"unknown"`
	ExecutionSeconds float64   `json:"execution_seconds"`
	AllocatedSeconds float64   `json:"allocated_seconds"`
	CapacitySamples  int       `json:"capacity_samples"`
	CapacityReached  int       `json:"capacity_reached"`
}

// What an observation does to its minute has one definition, shared by the
// single write and the batch: a row for the pool and minute if there is none, and
// if there is, the larger of the two, so that a blocked observation is never
// erased by a free one.
const (
	insertUsageCapacity = `INSERT INTO usage_capacity_samples(pool_id,at,blocked) VALUES `
	mergeUsageCapacity  = ` ON CONFLICT(pool_id,at) DO UPDATE SET blocked=MAX(blocked,excluded.blocked) WHERE blocked < excluded.blocked`
	upsertUsageCapacity = insertUsageCapacity + `(?,?,?)` + mergeUsageCapacity
)

// RecordUsageCapacity coalesces reconcile observations. A blocked observation
// must not be erased by a successful placement later in the same minute.
func (s *Store) RecordUsageCapacity(ctx context.Context, pool string, at time.Time, blocked bool) error {
	_, err := s.exec(ctx, upsertUsageCapacity, pool, ms(at.Truncate(time.Minute)), blocked)
	return err
}

// UsageCapacitySample is one pool's observation for RecordUsageCapacityBatch.
type UsageCapacitySample struct {
	PoolID  string
	At      time.Time
	Blocked bool
}

// usageCapacityChunk is how many observations one statement carries. SQLite
// refuses a statement with more than 32,766 bound values, which is 10,922 of
// these; no fleet is a tenth of that size, and the chunk is far enough under the
// limit that none will meet it.
const usageCapacityChunk = 1000

// RecordUsageCapacityBatch records a pass's observations exactly as
// RecordUsageCapacity would record them one after another, with one write.
//
// A reconcile pass makes one observation for every pool, so recording them
// singly made as many writes as the fleet has pools, each taking the writer,
// committing a transaction of its own and parsing the same statement afresh: a
// quarter of a quiet pass at a hundred pools. Here the observations are the
// rows of a single INSERT, so the writer is taken once, the statement is parsed
// once and one transaction is committed. A transaction of one prepared
// statement run once for each pool was tried first and was slower at every
// size, because the transaction and each execution through database/sql cost
// more than the parsing they save: 467 microseconds against 308 for a hundred
// pools, on the machine this was measured on.
//
// A statement is atomic, so the batch succeeds or fails whole; more than a
// chunk's worth is split over statements in one transaction, which keeps it so.
// The schema accepts every sample (there is deliberately no foreign key, so that
// history outlives its pool), which leaves only the database itself failing, and
// that fails every sample alike.
func (s *Store) RecordUsageCapacityBatch(ctx context.Context, samples []UsageCapacitySample) error {
	failed := func(err error) error {
		return fmt.Errorf("recording the capacity of %d pools: %w", len(samples), err)
	}
	switch {
	case len(samples) == 0:
		return nil
	case len(samples) <= usageCapacityChunk:
		q, args := upsertUsageCapacityRows(samples)
		if _, err := s.exec(ctx, q, args...); err != nil {
			return failed(err)
		}
		return nil
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		for chunk := range slices.Chunk(samples, usageCapacityChunk) {
			q, args := upsertUsageCapacityRows(chunk)
			if _, err := tx.ExecContext(ctx, q, args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return failed(err)
	}
	return nil
}

// upsertUsageCapacityRows is the statement that records samples, one row each,
// and what it binds.
func upsertUsageCapacityRows(samples []UsageCapacitySample) (string, []any) {
	args := make([]any, 0, 3*len(samples))
	for _, sm := range samples {
		args = append(args, sm.PoolID, ms(sm.At.Truncate(time.Minute)), sm.Blocked)
	}
	rows := strings.TrimSuffix(strings.Repeat("(?,?,?),", len(samples)), ",")
	return insertUsageCapacity + rows + mergeUsageCapacity, args
}

func (s *Store) PruneUsageCapacity(ctx context.Context, before time.Time) (int64, error) {
	r, err := s.exec(ctx, `DELETE FROM usage_capacity_samples WHERE at < ?`, ms(before))
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}
