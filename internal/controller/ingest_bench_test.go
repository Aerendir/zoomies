package controller

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

// A queued job reaches the fleet by two roads: GitHub delivers a workflow_job
// webhook, or the poller finds the job on a sweep. Both read the pools, claim
// the job for one of them, write its row and its timeline and tell the browsers
// that are watching, so what that costs is what the fleet pays for every job it
// is asked to run, and again for every job that is still waiting each time the
// poller comes round. The reconcile pass is the other cost, and the steadier
// one: it runs on a timer whether or not either road has anything to say.
// These benchmarks time all three on the controller the tests use.
//
// Each checks its scenario once before the timer starts, for the reason the
// scheduler's do: a delivery that was refused, or a job no pool claimed, would
// time a cheaper path under the name of a dearer one.

// benchLabel is the label pool i is asked for by. It is not taken from the
// pool's name, which the store rewrites when the pool is created.
func benchLabel(i int) string { return fmt.Sprintf("pool%03d", i) }

// benchPools seeds an installation and n pools, each asked for by a label of
// its own, so a job's claim has to be found among them.
func benchPools(h *harness, n int) (*store.Installation, []*store.Pool) {
	inst := h.installation()
	pools := make([]*store.Pool, n)
	for i := range pools {
		pools[i] = h.pool(inst, benchLabel(i), "self-hosted", "linux", "x64", benchLabel(i))
	}
	return inst, pools
}

// benchWebhook delivers the queued workflow_job for job id, which asks for the
// pool its number falls on among n, and reports what the endpoint answered.
func benchWebhook(h *harness, id int64, n int) int {
	body := jobEvent{
		Action: "queued", JobID: id, RunID: 9_000_000_000 + id/3,
		Name: "build (linux, x64)", Workflow: "CI",
		Labels: []string{"self-hosted", "linux", "x64", benchLabel(int(id) % n)},
	}.body()
	return h.deliver("workflow_job", body, testWebhookSecret).Code
}

// benchClaimed fails the benchmark unless the job with this GitHub ID has been
// claimed by the pool its number falls on among pools.
func benchClaimed(b *testing.B, h *harness, id int64, pools []*store.Pool) {
	b.Helper()
	job, err := h.st.GetJobByGitHubID(h.ctx, id)
	if err != nil || !job.Matched || job.PoolID != pools[int(id)%len(pools)].ID {
		b.Fatalf("job %d was not claimed by the pool it asked for: %+v, err = %v", id, job, err)
	}
}

// BenchmarkWebhookJob is one signed workflow_job delivery through HandleWebhook,
// from the signature check to the 202: the installation found, a secret
// unsealed, the pools read and one of them chosen, the job and its timeline
// written, the delivery logged and the browsers told. The request is built and
// signed inside the loop, because each delivery is a different job; that is a
// few microseconds of the figure.
func BenchmarkWebhookJob(b *testing.B) {
	b.Run("queued", func(b *testing.B) {
		for _, n := range []int{4, 20, 100} {
			b.Run(fmt.Sprintf("pools=%d", n), func(b *testing.B) {
				h := newHarness(b)
				_, pools := benchPools(h, n)

				if code := benchWebhook(h, 1, n); code != http.StatusAccepted {
					b.Fatalf("the first delivery was answered %d, want 202", code)
				}
				benchClaimed(b, h, 1, pools)

				b.ReportAllocs()
				for id := int64(2); b.Loop(); id++ {
					if code := benchWebhook(h, id, n); code != http.StatusAccepted {
						b.Fatalf("delivery %d was answered %d, want 202", id, code)
					}
				}
			})
		}
	})

	// The same job delivered again. GitHub delivers at least once, so this is
	// routine; nothing changes, but the pools are still read and the job
	// matched and written before the merge finds there is nothing to add.
	b.Run("redelivered", func(b *testing.B) {
		h := newHarness(b)
		_, pools := benchPools(h, 20)
		for range 2 {
			if code := benchWebhook(h, 1, len(pools)); code != http.StatusAccepted {
				b.Fatalf("a delivery was answered %d, want 202", code)
			}
		}
		benchClaimed(b, h, 1, pools)
		if _, total, err := h.st.ListJobs(h.ctx, store.JobFilter{}, store.Page{Limit: 10}); err != nil || total != 1 {
			b.Fatalf("two deliveries of one job left %d jobs, err = %v; want 1", total, err)
		}

		b.ReportAllocs()
		for b.Loop() {
			if code := benchWebhook(h, 1, len(pools)); code != http.StatusAccepted {
				b.Fatalf("a redelivery was answered %d, want 202", code)
			}
		}
	})
}

// benchRepo is the repository job number id belongs to when a sweep's jobs are
// spread over n of them in turn. One repository is the one the other benchmarks
// have always used.
func benchRepo(id int64, n int) string {
	if n <= 1 {
		return "acme/widgets"
	}
	return fmt.Sprintf("acme/repo-%03d", int(id)%n)
}

// benchPolled is n queued jobs as a sweep finds them, numbered from first,
// asking for the pools in turn and spread over repoCount repositories in turn.
func benchPolled(first, n int64, poolCount, repoCount int) []github.QueuedJob {
	queuedAt := time.Now().Add(-time.Minute)
	jobs := make([]github.QueuedJob, n)
	for i := range jobs {
		id := first + int64(i)
		repo := benchRepo(id, repoCount)
		jobs[i] = github.QueuedJob{
			ID: id, RunID: 9_000_000_000 + id/3, RunNumber: 1000 + id/3,
			Repo: repo, WorkflowName: "CI", JobName: "build (linux, x64)",
			Labels:   []string{"self-hosted", "linux", "x64", benchLabel(int(id) % poolCount)},
			QueuedAt: queuedAt, HTMLURL: "https://github.com/" + repo + "/actions/runs/9000000000",
			Status: "queued",
		}
	}
	return jobs
}

// BenchmarkIngestQueuedJobs is one poller sweep over the queued jobs a
// repository has, through ingestQueuedJobs. Each op is the whole sweep, so the
// per-job cost is the figure divided by the jobs.
func BenchmarkIngestQueuedJobs(b *testing.B) {
	// The sweep that matters most: every job is one the fleet already knows,
	// found again because it is still waiting. This is what the poller costs
	// on every round for as long as a queue lasts.
	b.Run("known", func(b *testing.B) {
		// repos is how many repositories the jobs are spread over; the sweeps
		// that do not say are of one repository, as they always were. The jobs
		// of a repository share the answer to which installation covers it.
		for _, tc := range []struct{ jobs, pools, repos int }{
			{10, 20, 1}, {100, 20, 1}, {500, 20, 1}, {100, 4, 1}, {100, 100, 1},
			{100, 20, 10}, {100, 20, 100},
		} {
			name := fmt.Sprintf("jobs=%d/pools=%d", tc.jobs, tc.pools)
			if tc.repos > 1 {
				name += fmt.Sprintf("/repos=%d", tc.repos)
			}
			b.Run(name, func(b *testing.B) {
				h := newHarness(b)
				inst, pools := benchPools(h, tc.pools)
				jobs := benchPolled(1, int64(tc.jobs), tc.pools, tc.repos)
				if left, err := h.c.ingestQueuedJobs(h.ctx, inst, jobs); err != nil || left != tc.jobs {
					b.Fatalf("the first sweep left %d of %d jobs waiting, err = %v", left, tc.jobs, err)
				}
				// A sweep counts a job nobody claimed as waiting too, so the
				// count above cannot say the pools were found.
				benchClaimed(b, h, 1, pools)
				benchClaimed(b, h, int64(tc.jobs), pools)
				if job, err := h.st.GetJobByGitHubID(h.ctx, int64(tc.jobs)); err != nil || job.InstallationID != inst.ID || job.Repo != jobs[len(jobs)-1].Repo {
					b.Fatalf("the last job was attributed to %+v, err = %v; want installation %s and repository %s", job, err, inst.ID, jobs[len(jobs)-1].Repo)
				}

				b.ReportAllocs()
				for b.Loop() {
					if _, err := h.c.ingestQueuedJobs(h.ctx, inst, jobs); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	})

	// A sweep that finds jobs for the first time, as the one after a burst
	// does, each of which is inserted, claimed and put on its timeline.
	b.Run("new", func(b *testing.B) {
		const jobs, poolCount = 100, 20
		h := newHarness(b)
		inst, pools := benchPools(h, poolCount)
		next := int64(1)
		sweep := func() (int, error) {
			batch := benchPolled(next, jobs, poolCount, 1)
			next += jobs
			return h.c.ingestQueuedJobs(h.ctx, inst, batch)
		}
		if left, err := sweep(); err != nil || left != jobs {
			b.Fatalf("the first sweep left %d of %d jobs waiting, err = %v", left, jobs, err)
		}
		benchClaimed(b, h, 1, pools)
		benchClaimed(b, h, jobs, pools)

		b.ReportAllocs()
		for b.Loop() {
			if _, err := sweep(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkReconcile is one pass of the reconcile loop over a fleet with
// nothing to do: pools and hosts, no runners and no queue. It is the steady
// cost of a controller that is simply running, since the loop makes this pass
// on every tick whether or not anything has happened, and it includes the
// derived figures (the stats, the problems list, the host views) that follow
// every pass.
func BenchmarkReconcile(b *testing.B) {
	b.Run("quiet", func(b *testing.B) {
		for _, n := range []int{4, 20, 100} {
			b.Run(fmt.Sprintf("pools=%d", n), func(b *testing.B) {
				h := newHarness(b)
				benchPools(h, n)
				for i := range max(n/2, 1) {
					h.host(fmt.Sprintf("vm-%03d", i))
				}
				if err := h.c.Reconcile(h.ctx); err != nil {
					b.Fatalf("Reconcile: %v", err)
				}
				if rs := h.runners(); len(rs) != 0 {
					b.Fatalf("a fleet with nothing queued started %d runners", len(rs))
				}

				b.ReportAllocs()
				for b.Loop() {
					if err := h.c.Reconcile(h.ctx); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	})
}
