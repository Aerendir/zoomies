package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// pageKind is what became of a job, for the jobs a fleet's history is made of.
type pageKind int

const (
	pageManagedOK        pageKind = iota // ran on this fleet and passed
	pageManagedFailed                    // ran on this fleet and its workflow failed
	pageManagedCancelled                 // ran on this fleet and somebody stopped it
	pageFleetFault                       // its runner stopped under it
	pageHostedOK                         // somebody else's runner, passed
	pageHostedFailed                     // somebody else's runner, failed
)

// pageCycle is how a run of twenty jobs divides: three in five ran here and
// passed, and the rest are the other five kinds, so that a fifth of a history is
// somebody else's jobs, which is what an organisation that also uses hosted
// runners has.
var pageCycle = [20]pageKind{
	pageManagedOK, pageManagedOK, pageManagedOK, pageManagedOK, pageManagedOK, pageManagedOK,
	pageManagedOK, pageManagedOK, pageManagedOK, pageManagedOK, pageManagedOK, pageManagedOK,
	pageManagedFailed, pageManagedCancelled, pageFleetFault,
	pageHostedOK, pageHostedOK, pageHostedOK, pageHostedOK, pageHostedFailed,
}

// pageLive is how many of the newest jobs are still in hand: a quarter of them
// running, a quarter queued for a pool, a quarter queued with a label no pool
// claims, and a quarter queued for somebody else's runner, except that one of
// those is held by GitHub for a review instead.
const pageLive = 12

// pageWant is what each view of the Jobs page should say of a history, counted
// from what was written and not asked of the store, so that the benchmark is
// checked against something other than the statement it times.
type pageWant struct {
	running, queued, failed, finished, all, attention, lastDay, search int
}

// pageHistory puts n jobs in the store over the last seven days, oldest first as
// they arrive, and says what the views of the page should find. Every job is on a
// branch of its own, so that no failure has been replaced by a later one and the
// attention view is a question about the failures alone. It returns the jobs as
// the store kept them, for a test that wants to count them another way.
func pageHistory(tb testing.TB, s *Store, now time.Time, n int) ([]*Job, pageWant) {
	tb.Helper()
	ctx := context.Background()
	spacing := (7 * 24 * time.Hour / time.Duration(n)).Truncate(time.Millisecond)
	var want pageWant
	var written []*Job
	for i := n - 1; i >= 0; i-- {
		queued := now.Add(-time.Duration(i+1) * spacing)
		started := queued.Add(20 * time.Second)
		j := &Job{
			GitHubJobID: int64(5000 + i), GitHubRunID: int64(900000 + i/3),
			Repo: fmt.Sprintf("acme/svc-%d", i%5), Workflow: []string{"CI", "Release", "Nightly", "Docs"}[i%4],
			JobName: fmt.Sprintf("build-%d", i%5), HeadBranch: fmt.Sprintf("feature/%d", i), HeadSHA: fmt.Sprintf("%040x", i),
			RunAttempt: 1, QueuedAt: queued, StartedAt: &started,
		}
		managed, failed := false, false
		switch {
		case i < pageLive:
			switch i % 4 {
			case 0: // running here
				j.State, j.Matched, j.PoolID, j.RunnerID, j.RunnerName = JobInProgress, true, "pool_0", "run_live", "zoomies-live"
				j.Labels = StringSlice{"self-hosted", "zoomies-linux-x64"}
				managed = true
				want.running++
			case 1: // queued for a pool
				j.State, j.Matched, j.PoolID, j.StartedAt = JobQueued, true, "pool_0", nil
				j.Labels = StringSlice{"self-hosted", "zoomies-linux-x64"}
				managed = true
				want.queued++
			case 2: // queued with a label no pool claims
				j.State, j.StartedAt = JobQueued, nil
				j.Labels = StringSlice{"self-hosted", "gpu"}
				managed = true
				want.queued++
			default:
				if i == 7 { // held by GitHub for a deployment review
					j.State, j.Matched, j.PoolID, j.StartedAt = JobWaiting, true, "pool_0", nil
					j.Labels = StringSlice{"self-hosted", "zoomies-linux-x64"}
					managed = true
					break
				}
				// queued for somebody else's runner
				j.State, j.StartedAt = JobQueued, nil
				j.Labels = StringSlice{"ubuntu-latest"}
			}
		default:
			done := started.Add(time.Duration(30+i%90) * time.Second)
			j.State, j.CompletedAt = JobCompleted, &done
			j.Steps = pageSteps(started, done)
			switch kind := pageCycle[i%len(pageCycle)]; kind {
			case pageManagedOK, pageManagedFailed, pageManagedCancelled, pageFleetFault:
				j.Matched, j.PoolID = true, fmt.Sprintf("pool_%d", i%8)
				j.RunnerID, j.RunnerName = fmt.Sprintf("run_%d", i%40), fmt.Sprintf("zoomies-%d", i%40)
				j.Labels = StringSlice{"self-hosted", "zoomies-linux-x64"}
				managed = true
				switch kind {
				case pageManagedOK:
					j.Conclusion = "success"
				case pageManagedFailed:
					j.Conclusion, failed = "failure", true
				case pageManagedCancelled:
					j.Conclusion = "cancelled"
				case pageFleetFault:
					j.Conclusion, j.RunnerFault, j.FaultKind, failed = "failure", "the runner stopped under the job", FaultHostLost, true
				}
			case pageHostedOK:
				j.Conclusion, j.Labels = "success", StringSlice{"ubuntu-latest"}
			case pageHostedFailed:
				j.Conclusion, j.Labels = "failure", StringSlice{"ubuntu-latest"}
			}
		}
		kept, err := s.UpsertJob(ctx, j)
		if err != nil {
			tb.Fatalf("UpsertJob: %v", err)
		}
		written = append(written, kept)
		if !managed {
			continue
		}
		want.all++
		if j.JobName == "build-3" {
			want.search++
		}
		if j.State != JobCompleted {
			continue
		}
		want.finished++
		if !j.QueuedAt.Before(now.Add(-24 * time.Hour)) {
			want.lastDay++
		}
		if failed {
			want.failed++
			if !j.QueuedAt.Before(now.Add(-AttentionWindow)) {
				want.attention++
			}
		}
	}
	return written, want
}

// pageSteps are the steps a job on a pool's runner reports, which are most of
// what the Jobs page's rows weigh.
func pageSteps(started, done time.Time) JobSteps {
	names := []string{"Set up job", "Run actions/checkout", "Run ./.github/actions/setup-go", "Run the suite", "Post Run actions/checkout", "Complete job"}
	steps := make(JobSteps, len(names))
	step := done.Sub(started) / time.Duration(len(names))
	for i, name := range names {
		from := started.Add(time.Duration(i) * step)
		to := from.Add(step)
		steps[i] = JobStep{Number: i + 1, Name: name, Status: "completed", Conclusion: "success", StartedAt: &from, CompletedAt: &to}
	}
	return steps
}

// BenchmarkListJobs is the Jobs page: a page of jobs, and the count of every job
// the page's filters match, for each of the views an operator can ask for. The page
// is the first fifty of the default order, as the grid sends it, and every view
// is the filter the page writes for it, narrowed to the jobs this fleet had a hand
// in as the page does unless it is asked for the rest.
//
// What each view should find is counted while the history is written and checked
// before anything is timed, so that a view that matched nothing, or somebody
// else's jobs as well, is not timed under the name of the one that did not.
func BenchmarkListJobs(b *testing.B) {
	ctx := context.Background()
	for _, n := range []int{2000, 10000} {
		b.Run(fmt.Sprintf("jobs=%d", n), func(b *testing.B) {
			now := time.Now().Truncate(time.Millisecond)
			s := newTestStoreAt(b, func() time.Time { return now })
			_, want := pageHistory(b, s, now, n)
			notCancelling := false
			lastDay := now.Add(-24 * time.Hour)
			views := []struct {
				name   string
				filter JobFilter
				total  int
			}{
				{"running", JobFilter{ManagedOnly: true, States: []JobState{JobInProgress}, Cancelling: &notCancelling}, want.running},
				{"queued", JobFilter{ManagedOnly: true, States: []JobState{JobQueued}, Cancelling: &notCancelling}, want.queued},
				{"failed", JobFilter{ManagedOnly: true, FailedOnly: true}, want.failed},
				{"attention", JobFilter{ManagedOnly: true, AttentionOnly: true}, want.attention},
				{"finished", JobFilter{ManagedOnly: true, States: []JobState{JobCompleted}}, want.finished},
				{"finished_last_day", JobFilter{ManagedOnly: true, States: []JobState{JobCompleted}, Since: &lastDay}, want.lastDay},
				{"all", JobFilter{ManagedOnly: true, States: []JobState{JobWaiting, JobQueued, JobInProgress, JobCompleted}}, want.all},
				{"search", JobFilter{ManagedOnly: true, States: []JobState{JobWaiting, JobQueued, JobInProgress, JobCompleted}, Search: "build-3"}, want.search},
			}
			page := Page{Limit: 50, Desc: true}
			for _, v := range views {
				b.Run("view="+v.name, func(b *testing.B) {
					jobs, total, err := s.ListJobs(ctx, v.filter, page)
					if err != nil {
						b.Fatalf("ListJobs: %v", err)
					}
					if total != v.total || len(jobs) != min(v.total, page.Limit) {
						b.Fatalf("the %s view found %d jobs and returned %d; the history holds %d for it", v.name, total, len(jobs), v.total)
					}
					for i := 1; i < len(jobs); i++ {
						if jobs[i].QueuedAt.After(jobs[i-1].QueuedAt) {
							b.Fatalf("the %s view is not newest first", v.name)
						}
					}
					if v.name == "search" && !strings.Contains(jobs[0].JobName, "build-3") {
						b.Fatalf("the search view returned %q", jobs[0].JobName)
					}

					b.ReportAllocs()
					for b.Loop() {
						if _, _, err := s.ListJobs(ctx, v.filter, page); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
