package store

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"time"
)

// A percentile is the sample at index round((n-1)*q) of the group's samples in
// order, the position the Overview's percentiles use, so a figure here and the same
// one there cannot differ by a rounding rule. These tests hold the statistics to
// that, from jobs whose waits and durations are known because the test made them,
// and not from another run of the statement that is being held to it.

// rankOf is the index of the sample for q = 1/2 and q = 19/20, as integers. The
// statement works the same position out in floating point, (n-1)*q + 0.5 cut to an
// integer, which agrees with this wherever the product is a half, because a double
// is within half a unit of its last place of the exact product.
func rankOf(n int) (p50, p95 int) { return n / 2, ((n-1)*19 + 10) / 20 }

func spreadOfSamples(samples []int64) JobPercentiles {
	if len(samples) == 0 {
		return JobPercentiles{}
	}
	sorted := slices.Clone(samples)
	slices.Sort(sorted)
	i50, i95 := rankOf(len(sorted))
	return JobPercentiles{Samples: len(sorted), P50MS: &sorted[i50], P95MS: &sorted[i95]}
}

func TestPercentilesAreTheSamplesAtTheRankTheOverviewUses(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rng := rand.New(rand.NewSource(11))

	// One pool for each size of group from one job to thirty, so every position
	// the two ranks can fall on in a small group is taken, and one of a thousand.
	// Durations come in whole seconds so that samples tie, and a tenth of the jobs
	// are cancelled (they waited, and ran for as long as somebody let them, which
	// the duration leaves out) and a seventh never started (they have neither).
	sizes := make([]int, 0, 31)
	for n := 1; n <= 30; n++ {
		sizes = append(sizes, n)
	}
	sizes = append(sizes, 1000)

	waits, durs := map[string][]int64{}, map[string][]int64{}
	id := 0
	for _, size := range sizes {
		pool := fmt.Sprintf("pool_%04d", size)
		for i := range size {
			id++
			queued := versionsEpoch.Add(time.Duration(id) * time.Minute)
			wait := time.Duration(rng.Intn(40)) * time.Second
			ran := time.Duration(rng.Intn(60)) * time.Second
			started, done := queued.Add(wait), queued.Add(wait+ran)
			j := &Job{
				GitHubJobID: int64(4000 + id), Repo: "acme/widgets", Workflow: "ci", JobName: "build", PoolID: pool,
				State: JobCompleted, Conclusion: "success", Matched: true, QueuedAt: queued, StartedAt: &started, CompletedAt: &done,
			}
			switch {
			case i%7 == 6:
				j.StartedAt = nil
			case i%10 == 9:
				j.Conclusion = "cancelled"
			}
			if _, err := s.UpsertJob(ctx, j); err != nil {
				t.Fatalf("UpsertJob: %v", err)
			}
			if j.StartedAt != nil {
				waits[pool] = append(waits[pool], wait.Milliseconds())
				if j.Conclusion != "cancelled" {
					durs[pool] = append(durs[pool], ran.Milliseconds())
				}
			}
		}
	}

	check := func(name string, res *JobStatsResult, durationToo, waitToo bool) {
		t.Helper()
		if len(res.Groups) != len(sizes) {
			t.Fatalf("%s: %d groups, want %d", name, len(res.Groups), len(sizes))
		}
		for _, g := range res.Groups {
			pool := g.Keys[GroupByPool]
			if durationToo {
				if want := spreadOfSamples(durs[pool]); !equalSpread(g.Duration, want) {
					t.Errorf("%s: %s duration = %s, want %s", name, pool, showSpread(g.Duration), showSpread(want))
				}
			}
			if waitToo {
				if want := spreadOfSamples(waits[pool]); !equalSpread(g.QueueWait, want) {
					t.Errorf("%s: %s queue wait = %s, want %s", name, pool, showSpread(g.QueueWait), showSpread(want))
				}
			}
		}
	}
	full, err := s.JobStats(ctx, JobFilter{}, []string{GroupByPool})
	if err != nil {
		t.Fatal(err)
	}
	check("all figures", full, true, true)
	one, err := s.JobStatsOf(ctx, JobFilter{}, []string{GroupByPool}, MeasureDuration)
	if err != nil {
		t.Fatal(err)
	}
	check("the duration alone", one, true, false)

	// The comparison says something only if the groups gave it something to tell
	// apart: a median below its 95th percentile, and ties at the ranks, where a
	// wrong pick of one of the equal samples still gives the right figure and a
	// wrong rank usually does not.
	apart, tied := 0, 0
	for _, g := range full.Groups {
		if g.Duration.P50MS != nil && *g.Duration.P50MS != *g.Duration.P95MS {
			apart++
		}
		pool := g.Keys[GroupByPool]
		for _, samples := range [][]int64{waits[pool], durs[pool]} {
			sorted := slices.Clone(samples)
			slices.Sort(sorted)
			i50, i95 := rankOf(len(sorted))
			for _, i := range []int{i50, i95} {
				if len(sorted) > 1 && (i+1 < len(sorted) && sorted[i] == sorted[i+1] || i > 0 && sorted[i-1] == sorted[i]) {
					tied++
				}
			}
		}
	}
	if apart < 10 || tied < 10 {
		t.Fatalf("the groups exercised too little: %d with a median below their 95th percentile, %d ranks with a tie beside them", apart, tied)
	}
}

func equalSpread(a, b JobPercentiles) bool {
	if a.Samples != b.Samples || (a.P50MS == nil) != (b.P50MS == nil) || (a.P95MS == nil) != (b.P95MS == nil) {
		return false
	}
	return (a.P50MS == nil || *a.P50MS == *b.P50MS) && (a.P95MS == nil || *a.P95MS == *b.P95MS)
}

func showSpread(sp JobPercentiles) string {
	return fmt.Sprintf("%d samples, p50 %s, p95 %s", sp.Samples, p(sp.P50MS), p(sp.P95MS))
}

// A group's samples come back as one string and two of them are read out of it. It
// reads only those two, so a figure it was not asked for cannot make it fail, and
// it says so, and does not answer, when the string has fewer samples than it was
// asked for: a count and a list that disagree are a bug to be seen, not a figure.
func TestTwoSamplesAreReadOutOfTheListWithoutParsingTheRest(t *testing.T) {
	for _, c := range []struct {
		name string
		list string
		i, j int
		a, b int64
		// err is what the refusal says, and empty when there is to be none.
		err string
	}{
		{"one sample", "5", 0, 0, 5, 5, ""},
		{"the first and the last", "1,2,3", 0, 2, 1, 3, ""},
		{"the same one twice", "1,2,3", 1, 1, 2, 2, ""},
		{"samples of different widths stay in the order given", "9,10,100,1000", 1, 3, 10, 1000, ""},
		{"the largest a figure can be", "0,9223372036854775807", 0, 1, 0, 9223372036854775807, ""},
		{"the others are not looked at", "1,not a number,3", 0, 2, 1, 3, ""},
		{"one sample too few", "1,2", 0, 2, 0, 0, "has 2 samples"},
		{"a sample that is not a number", "1,x", 0, 1, 0, 0, "not a whole number"},
		{"a sample that is empty", "1,,3", 0, 1, 0, 0, "not a whole number"},
	} {
		a, b, err := samplesAt(c.list, c.i, c.j)
		if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) || a != c.a || b != c.b {
			t.Errorf("%s: samplesAt(%q, %d, %d) = %d, %d, %v; want %d, %d, error %q", c.name, c.list, c.i, c.j, a, b, err, c.a, c.b, c.err)
		}
	}
}
