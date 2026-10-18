package store

import (
	"context"
	"fmt"
	"math/bits"
	"testing"
	"time"
)

// A batch is recorded by the statement of the next width up, because a statement
// as long as the fleet is a new statement for each length and the driver parses
// every one it is handed. The rows past the end of the batch are NULLs the
// statement drops. These hold that to recording exactly the samples it was given,
// whichever side of a width it falls.

func TestAUsageCapacityBatchRecordsExactlyItsSamplesWhateverItsWidth(t *testing.T) {
	sizes := []int{1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 17, 31, 33, 63, 65, 100, 127, 128, 129, 255, 257, 511, 513,
		usageCapacityChunk - 1, usageCapacityChunk, usageCapacityChunk + 1, 2*usageCapacityChunk - 1, 2*usageCapacityChunk + 1}
	for _, n := range sizes {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			s := newTestStore(t)
			at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
			batch := make([]UsageCapacitySample, n)
			for i := range batch {
				batch[i] = UsageCapacitySample{PoolID: fmt.Sprintf("pool_%05d", i), At: at, Blocked: i%3 == 0}
			}
			if err := s.RecordUsageCapacityBatch(context.Background(), batch); err != nil {
				t.Fatalf("RecordUsageCapacityBatch of %d: %v", n, err)
			}
			rows := capacityRows(t, s)
			if len(rows) != n {
				t.Fatalf("a batch of %d left %d rows; want one for each sample and none for the padding", n, len(rows))
			}
			for i, r := range rows {
				// The rows come back in pool order, which is the order they were made in.
				if want := (capacityRow{Pool: batch[i].PoolID, At: at.UnixMilli(), Blocked: boolInt(batch[i].Blocked)}); r != want {
					t.Fatalf("row %d is %+v; want %+v", i, r, want)
				}
			}
		})
	}
}

// An empty pool ID is a value, and NULL is the padding. A filter that took one
// for the other would drop a sample that recording it alone would have kept.
func TestAUsageCapacitySampleWithAnEmptyPoolIDIsStillRecorded(t *testing.T) {
	for _, n := range []int{1, 3, 5} {
		t.Run(fmt.Sprintf("among %d", n), func(t *testing.T) {
			s := newTestStore(t)
			at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
			batch := make([]UsageCapacitySample, n)
			for i := range batch {
				batch[i] = UsageCapacitySample{PoolID: fmt.Sprintf("pool_%d", i), At: at}
			}
			batch[n-1].PoolID = ""
			if err := s.RecordUsageCapacityBatch(context.Background(), batch); err != nil {
				t.Fatal(err)
			}
			if rows := capacityRows(t, s); len(rows) != n || rows[0].Pool != "" {
				t.Errorf("a batch of %d with an empty pool ID left %+v; want %d rows, the empty ID among them", n, rows, n)
			}
		})
	}
}

func TestTheWidthOfAUsageCapacityStatementIsTheNextPowerOfTwo(t *testing.T) {
	for n := 1; n <= usageCapacityChunk; n++ {
		q, width := usageCapacityStatement(n)
		if width < n || width/2 >= n || bits.OnesCount(uint(width)) != 1 {
			t.Fatalf("a statement for %d samples has %d rows; want the next power of two", n, width)
		}
		if got, want := countPlaceholders(q.sql), 3*width; got != want {
			t.Fatalf("the statement for %d samples binds %d values; want %d, three for each of its %d rows", n, got, want, width)
		}
	}
}

func countPlaceholders(sql string) int {
	n := 0
	for _, r := range sql {
		if r == '?' {
			n++
		}
	}
	return n
}
