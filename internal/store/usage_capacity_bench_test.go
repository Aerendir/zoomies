package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// A reconcile pass records one capacity observation for every pool, once in ten
// seconds. A minute therefore sees one pass that writes a row for each pool and
// five that find those rows already there and change nothing, and this times
// passes at that rhythm, with the observation recorded one write at a time and as
// a batch, for fleets of three sizes. Both end up with the same rows; the
// scenario is checked before anything is timed.
func BenchmarkRecordUsageCapacity(b *testing.B) {
	ctx := context.Background()
	epoch := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	for _, n := range []int{4, 20, 100} {
		samples := make([]UsageCapacitySample, n)
		for i := range samples {
			samples[i] = UsageCapacitySample{PoolID: fmt.Sprintf("pool_%03d", i), At: epoch, Blocked: i%5 == 0}
		}
		record := map[string]func(s *Store, at time.Time) error{
			"each": func(s *Store, at time.Time) error {
				for _, sm := range samples {
					if err := s.RecordUsageCapacity(ctx, sm.PoolID, at, sm.Blocked); err != nil {
						return err
					}
				}
				return nil
			},
			"batch": func(s *Store, at time.Time) error {
				for i := range samples {
					samples[i].At = at
				}
				return s.RecordUsageCapacityBatch(ctx, samples)
			},
		}
		for _, way := range []string{"each", "batch"} {
			b.Run(fmt.Sprintf("%s/pools=%d", way, n), func(b *testing.B) {
				s := newTestStore(b)
				if err := record[way](s, epoch); err != nil {
					b.Fatalf("recording a pass: %v", err)
				}
				rows := capacityRows(b, s)
				blocked := 0
				for _, r := range rows {
					blocked += r.Blocked
				}
				if len(rows) != n || blocked != (n+4)/5 {
					b.Fatalf("a pass left %d rows of which %d blocked, want %d rows of which %d", len(rows), blocked, n, (n+4)/5)
				}

				b.ReportAllocs()
				for i := 1; b.Loop(); i++ {
					if err := record[way](s, epoch.Add(time.Duration(i)*10*time.Second)); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
