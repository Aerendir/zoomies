package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// completedJobs puts n completed jobs in the last day in the store, over four pools.
func completedJobs(tb testing.TB, s *Store, n int) {
	tb.Helper()
	now := time.Now()
	for i := range n {
		queued := now.Add(-time.Duration(1+i%600) * time.Minute)
		started, done := queued.Add(20*time.Second), queued.Add(time.Duration(30+i%90)*time.Second)
		if _, err := s.UpsertJob(context.Background(), &Job{
			GitHubJobID: int64(1000 + i), Repo: "acme/api", Workflow: "CI", JobName: fmt.Sprintf("build-%d", i%5),
			State: JobCompleted, Conclusion: "success", PoolID: fmt.Sprintf("pool_%d", i%4),
			QueuedAt: queued, StartedAt: &started, CompletedAt: &done,
		}); err != nil {
			tb.Fatalf("UpsertJob: %v", err)
		}
	}
}

// JobStats is what the dashboard asks of the jobs table every pass: how many ran
// in each pool, how they ended and how long they waited and took. It is six
// statements, and each is a page of SQL, so a table with nothing in it is the cost
// of the statements themselves and a table with a lot in it is the cost of running
// them. The scenario is checked before anything is timed: a table that did not
// hold the jobs it was meant to would time the empty one under another name.
func BenchmarkJobStats(b *testing.B) {
	ctx := context.Background()
	for _, n := range []int{0, 1000, 10000} {
		b.Run(fmt.Sprintf("jobs=%d", n), func(b *testing.B) {
			s := newTestStore(b)
			completedJobs(b, s, n)
			since := time.Now().Add(-24 * time.Hour)
			f := JobFilter{Since: &since}

			res, err := s.JobStats(ctx, f, []string{GroupByPool})
			if err != nil {
				b.Fatalf("JobStats: %v", err)
			}
			counted := 0
			for _, g := range res.Groups {
				counted += g.Count
			}
			if counted != n {
				b.Fatalf("the statistics counted %d jobs; the table holds %d", counted, n)
			}

			b.ReportAllocs()
			for b.Loop() {
				if _, err := s.JobStats(ctx, f, []string{GroupByPool}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// The same table read for one figure, which is what each reader of the dashboard's
// passes does: the pools' queue wait, the job names' duration, the pools' peak
// memory, the counts of jobs by pool and host. Every one is the statement that
// names the groups and the statement for its figure, so what they cost is read
// against the full answer's six at BenchmarkJobStats. The figure is checked before
// anything is timed, for the reason that one is.
func BenchmarkJobStatsOf(b *testing.B) {
	ctx := context.Background()
	for _, c := range []struct {
		name string
		want JobMeasures
		// read is what a group contributes, and total what the groups of a table of n
		// jobs add up to if the figure was worked out. The jobs have no peaks, so the
		// peaks are read as the groups themselves, which are the four pools.
		read  func(JobStatsGroup) int
		total func(n int) int
	}{
		{"counts", MeasureCounts, func(g JobStatsGroup) int { return g.Count }, func(n int) int { return n }},
		{"peaks", MeasurePeaks, func(JobStatsGroup) int { return 1 }, func(int) int { return 4 }},
		{"duration", MeasureDuration, func(g JobStatsGroup) int { return g.Duration.Samples }, func(n int) int { return n }},
		{"queue_wait", MeasureQueueWait, func(g JobStatsGroup) int { return g.QueueWait.Samples }, func(n int) int { return n }},
	} {
		for _, n := range []int{1000, 10000} {
			b.Run(fmt.Sprintf("figures=%s/jobs=%d", c.name, n), func(b *testing.B) {
				s := newTestStore(b)
				completedJobs(b, s, n)
				since := time.Now().Add(-24 * time.Hour)
				f := JobFilter{Since: &since}

				res, err := s.JobStatsOf(ctx, f, []string{GroupByPool}, c.want)
				if err != nil {
					b.Fatalf("JobStatsOf: %v", err)
				}
				read := 0
				for _, g := range res.Groups {
					read += c.read(g)
				}
				if want := c.total(n); read != want {
					b.Fatalf("the %s figure added up to %d over the table of %d jobs; want %d", c.name, read, n, want)
				}

				b.ReportAllocs()
				for b.Loop() {
					if _, err := s.JobStatsOf(ctx, f, []string{GroupByPool}, c.want); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
