package store

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A reconcile pass records one capacity observation for every pool, so the
// number of writes it makes is the size of the fleet: a hundred pools were a
// hundred transactions, each with its statement parsed afresh, and in a profile
// of a quiet pass at a hundred pools that was a quarter of the pass.
// RecordUsageCapacityBatch takes the same observations as one write. These hold
// it to what recording them one at a time leaves behind, and to being one write.

type capacityRow struct {
	Pool    string
	At      int64
	Blocked int
}

func capacityRows(t testing.TB, s *Store) []capacityRow {
	t.Helper()
	rows, err := s.read.QueryContext(context.Background(), `SELECT pool_id, at, blocked FROM usage_capacity_samples ORDER BY pool_id, at`)
	if err != nil {
		t.Fatalf("reading the capacity samples: %v", err)
	}
	defer rows.Close()
	var out []capacityRow
	for rows.Next() {
		var r capacityRow
		if err := rows.Scan(&r.Pool, &r.At, &r.Blocked); err != nil {
			t.Fatalf("scanning a capacity sample: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the capacity samples: %v", err)
	}
	return out
}

// randomCapacitySamples is n observations over the given number of pools and
// minutes, at arbitrary seconds and milliseconds so that truncation to the minute
// is exercised, and mostly unblocked so that a blocked observation has to survive
// the unblocked ones that follow it in the same minute and an unblocked minute
// has to stay unblocked.
func randomCapacitySamples(rng *rand.Rand, n, pools, minutes int) []UsageCapacitySample {
	epoch := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	out := make([]UsageCapacitySample, n)
	for i := range out {
		out[i] = UsageCapacitySample{
			PoolID:  fmt.Sprintf("pool_%03d", rng.Intn(pools)),
			At:      epoch.Add(time.Duration(rng.Intn(minutes*60*1000)) * time.Millisecond),
			Blocked: rng.Intn(10) == 0,
		}
	}
	return out
}

func recordEachSample(t *testing.T, s *Store, samples []UsageCapacitySample) {
	t.Helper()
	for _, sm := range samples {
		if err := s.RecordUsageCapacity(context.Background(), sm.PoolID, sm.At, sm.Blocked); err != nil {
			t.Fatalf("RecordUsageCapacity: %v", err)
		}
	}
}

// bothKinds reports whether the rows are not all one kind. A comparison of two
// stores means something only if some minutes were observed blocked and some
// never were.
func bothKinds(rows []capacityRow) bool {
	blocked := 0
	for _, r := range rows {
		blocked += r.Blocked
	}
	return blocked > 0 && blocked < len(rows)
}

func TestRecordingUsageCapacityInBatchesLeavesWhatRecordingEachLeaves(t *testing.T) {
	ctx := context.Background()
	rng := rand.New(rand.NewSource(7))
	samples := randomCapacitySamples(rng, 600, 8, 10)

	each, batched := newTestStore(t), newTestStore(t)
	recordEachSample(t, each, samples)
	for rest := samples; len(rest) > 0; {
		n := min(1+rng.Intn(40), len(rest))
		if err := batched.RecordUsageCapacityBatch(ctx, rest[:n]); err != nil {
			t.Fatalf("RecordUsageCapacityBatch of %d: %v", n, err)
		}
		rest = rest[n:]
	}

	got, want := capacityRows(t, batched), capacityRows(t, each)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recording in batches left\n%v\nwant what recording each sample leaves\n%v", got, want)
	}
	if len(want) != 8*10 || !bothKinds(want) {
		t.Fatalf("the samples left %d rows; want %d, with blocked and free minutes among them", len(want), 8*10)
	}
}

// More observations than one statement carries are split over several, and the
// split must not show: the same rows, whichever side of a boundary an
// observation fell, including a minute whose observations straddle one.
func TestAUsageCapacityBatchLongerThanOneStatementLeavesWhatRecordingEachLeaves(t *testing.T) {
	ctx := context.Background()
	rng := rand.New(rand.NewSource(11))
	samples := randomCapacitySamples(rng, 2*usageCapacityChunk+500, 40, 6)

	each, batched := newTestStore(t), newTestStore(t)
	recordEachSample(t, each, samples)
	if err := batched.RecordUsageCapacityBatch(ctx, samples); err != nil {
		t.Fatalf("RecordUsageCapacityBatch of %d: %v", len(samples), err)
	}

	got, want := capacityRows(t, batched), capacityRows(t, each)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("a batch of %d left %d rows that differ from the %d recording each sample leaves", len(samples), len(got), len(want))
	}
	if len(want) != 40*6 || !bothKinds(want) {
		t.Fatalf("the samples left %d rows; want %d, with blocked and free minutes among them", len(want), 40*6)
	}
}

// The reason an observation is a MAX and not a plain overwrite: a pool that was
// held at its limit early in a minute and placed a runner later in it was held
// at its limit that minute, and the analysis counts the minute that way.
func TestAUsageCapacityBatchKeepsABlockedObservationWhateverFollowsIt(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 3, 1, 12, 0, 20, 0, time.UTC)
	for name, blocked := range map[string][]bool{
		"blocked then free": {true, false},
		"free then blocked": {false, true},
		"never blocked":     {false, false},
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestStore(t)
			var batch []UsageCapacitySample
			for i, b := range blocked {
				batch = append(batch, UsageCapacitySample{PoolID: "pool_a", At: at.Add(time.Duration(i) * time.Second), Blocked: b})
			}
			if err := s.RecordUsageCapacityBatch(ctx, batch); err != nil {
				t.Fatalf("RecordUsageCapacityBatch: %v", err)
			}
			rows := capacityRows(t, s)
			want := 0
			for _, b := range blocked {
				if b {
					want = 1
				}
			}
			if len(rows) != 1 || rows[0].Blocked != want || rows[0].At != ms(at.Truncate(time.Minute)) {
				t.Fatalf("rows = %+v, want one row for the minute with blocked = %d", rows, want)
			}
		})
	}
}

func TestAUsageCapacityBatchIsOneWrite(t *testing.T) {
	for _, n := range []int{1, 8, usageCapacityChunk, usageCapacityChunk + 1, 2*usageCapacityChunk + 500} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			s := newTestStore(t)
			var writes atomic.Int32
			s.ObserveWrites(func(_, _ time.Duration) { writes.Add(1) })

			at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
			batch := make([]UsageCapacitySample, n)
			for i := range batch {
				batch[i] = UsageCapacitySample{PoolID: fmt.Sprintf("pool_%04d", i), At: at, Blocked: i%7 == 0}
			}
			if err := s.RecordUsageCapacityBatch(context.Background(), batch); err != nil {
				t.Fatalf("RecordUsageCapacityBatch: %v", err)
			}
			if got := writes.Load(); got != 1 {
				t.Errorf("a batch of %d samples took %d writes, want 1: the writer is taken once for the whole batch", n, got)
			}
			if rows := capacityRows(t, s); len(rows) != n {
				t.Errorf("the batch left %d rows, want %d", len(rows), n)
			}
		})
	}
}

// A pass over no pools has nothing to record, and a store that is not to be
// written to is not asked to take the writer for it.
func TestAnEmptyUsageCapacityBatchWritesNothing(t *testing.T) {
	s := newTestStore(t)
	var writes atomic.Int32
	s.ObserveWrites(func(_, _ time.Duration) { writes.Add(1) })

	for _, batch := range [][]UsageCapacitySample{nil, {}} {
		if err := s.RecordUsageCapacityBatch(context.Background(), batch); err != nil {
			t.Errorf("RecordUsageCapacityBatch(%#v) = %v, want nil", batch, err)
		}
	}
	if got := writes.Load(); got != 0 {
		t.Errorf("empty batches took %d writes, want none", got)
	}
}

// The schema accepts every sample, on purpose: history outlives its pool. So the
// only way a batch fails is the database failing, and a trigger that refuses one
// pool stands in for that. What was written before the failure goes with it,
// whether the refused pool is in the only statement or in a later one, and the
// error carries the cause.
func TestAFailedUsageCapacityBatchRecordsNone(t *testing.T) {
	for name, n := range map[string]int{
		"within one statement": 5,
		"in the last of three": 2*usageCapacityChunk + 500,
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			if _, err := s.exec(ctx, `CREATE TRIGGER refuse_one_pool BEFORE INSERT ON usage_capacity_samples
				WHEN NEW.pool_id = 'pool_refused' BEGIN SELECT RAISE(ABORT, 'refused'); END`); err != nil {
				t.Fatalf("creating the trigger: %v", err)
			}

			at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
			batch := make([]UsageCapacitySample, n)
			for i := range batch {
				batch[i] = UsageCapacitySample{PoolID: fmt.Sprintf("pool_%04d", i), At: at}
			}
			batch[n-2] = UsageCapacitySample{PoolID: "pool_refused", At: at, Blocked: true}

			err := s.RecordUsageCapacityBatch(ctx, batch)
			if err == nil {
				t.Fatal("RecordUsageCapacityBatch = nil, want the refusal")
			}
			if !strings.Contains(err.Error(), "refused") {
				t.Errorf("the error is %q, want it to carry the cause", err)
			}
			if rows := capacityRows(t, s); len(rows) != 0 {
				t.Errorf("a failed batch left %d rows, want none: it is all or nothing", len(rows))
			}
		})
	}
}

func TestAUsageCapacityBatchHonoursACancelledContext(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.RecordUsageCapacityBatch(ctx, []UsageCapacitySample{{PoolID: "pool_a", At: time.Now()}})
	if err == nil {
		t.Fatal("RecordUsageCapacityBatch on a cancelled context = nil, want its error")
	}
	if rows := capacityRows(t, s); len(rows) != 0 {
		t.Errorf("a cancelled batch left %+v", rows)
	}
}

func TestAReadOnlyStoreRefusesAUsageCapacityBatch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "zoomies.db")
	rw, err := Open(ctx, Options{Path: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := rw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the database was not written: %v", err)
	}
	ro, err := Open(ctx, Options{Path: path, ReadOnly: true})
	if err != nil {
		t.Fatalf("Open read-only: %v", err)
	}
	defer ro.Close()

	for _, n := range []int{1, usageCapacityChunk + 1} {
		batch := make([]UsageCapacitySample, n)
		for i := range batch {
			batch[i] = UsageCapacitySample{PoolID: fmt.Sprintf("pool_%04d", i), At: time.Now()}
		}
		if err := ro.RecordUsageCapacityBatch(ctx, batch); !errors.Is(err, ErrReadOnly) {
			t.Errorf("RecordUsageCapacityBatch of %d on a read-only store = %v, want ErrReadOnly", n, err)
		}
	}
}
