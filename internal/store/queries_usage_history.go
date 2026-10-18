package store

import (
	"context"
	"database/sql"
	"fmt"
	"math/bits"
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

// usageCapacityChunk is how many observations the widest statement carries.
// SQLite refuses a statement with more than 32,766 bound values, which is 10,922
// of these; no fleet is a tenth of that size, and the chunk is far enough under
// the limit that none will meet it.
const usageCapacityChunk = 1024

// usageCapacityBatch is a statement for each width a batch can be rounded up to,
// the powers of two from one row to usageCapacityChunk, each a hot query: see
// hot.go.
//
// A statement as long as the fleet could not be one, because its text is a new
// statement for each length, and the driver parses every statement it is handed:
// 3% of a quiet reconcile pass at a hundred pools, every pass, for a text that
// only changes when a pool is added. So the rows are those of a VALUES list
// rounded up to the next width, and the rows past the end of the batch are NULLs
// that the statement's own WHERE drops, which is also what stops the ambiguity
// between a SELECT and the ON CONFLICT that follows it (SQLite asks for a WHERE
// there). A batch pays for the NULLs it binds, at most as many as it has samples,
// and not for parsing a statement of its own length.
var usageCapacityBatch = func() []hotQuery {
	var out []hotQuery
	for width := 1; width <= usageCapacityChunk; width *= 2 {
		rows := strings.TrimSuffix(strings.Repeat("(?,?,?),", width), ",")
		out = append(out, hot(`INSERT INTO usage_capacity_samples(pool_id,at,blocked)
			SELECT column1, column2, column3 FROM (VALUES `+rows+`) WHERE column1 IS NOT NULL`+mergeUsageCapacity))
	}
	return out
}()

// usageCapacityStatement is the statement that records n samples (1 to
// usageCapacityChunk), and how many rows it has.
func usageCapacityStatement(n int) (hotQuery, int) {
	i := bits.Len(uint(n - 1))
	return usageCapacityBatch[i], 1 << i
}

// RecordUsageCapacityBatch records a pass's observations exactly as
// RecordUsageCapacity would record them one after another, with one write.
//
// A reconcile pass makes one observation for every pool, so recording them
// singly made as many writes as the fleet has pools, each taking the writer,
// committing a transaction of its own and parsing the same statement afresh: a
// quarter of a quiet pass at a hundred pools. Here the observations are the
// rows of a single INSERT, so the writer is taken once, the statement is parsed
// once and one transaction is committed; and the INSERT is one of a few widths
// that are each prepared once (usageCapacityBatch), so the next pass does not
// parse it again either. A transaction of one prepared
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
		q, width := usageCapacityStatement(len(samples))
		if _, err := s.execHot(ctx, q, usageCapacityRows(samples, width)...); err != nil {
			return failed(err)
		}
		return nil
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		for chunk := range slices.Chunk(samples, usageCapacityChunk) {
			q, width := usageCapacityStatement(len(chunk))
			if _, err := s.hotTx(tx).Exec(ctx, q, usageCapacityRows(chunk, width)...); err != nil {
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

// usageCapacityRows is what a statement of that width binds for the samples: a
// row for each, then NULLs to fill the rest.
func usageCapacityRows(samples []UsageCapacitySample, width int) []any {
	args := make([]any, 0, 3*width)
	for _, sm := range samples {
		args = append(args, sm.PoolID, ms(sm.At.Truncate(time.Minute)), sm.Blocked)
	}
	for range width - len(samples) {
		args = append(args, nil, nil, nil)
	}
	return args
}

func (s *Store) PruneUsageCapacity(ctx context.Context, before time.Time) (int64, error) {
	r, err := s.exec(ctx, `DELETE FROM usage_capacity_samples WHERE at < ?`, ms(before))
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}
