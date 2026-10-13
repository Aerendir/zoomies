package controller

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// The demo is where a person first meets the four analyses, so each one is
// given a cause to find: a pool whose runners are slow to register, a pool
// held at its ceiling, a host whose failures came while it was overwhelmed,
// and a host that cannot pull the runner image. They are placed in one window
// a day back, from demoAnalysisFrom to demoAnalysisTo ago, outside the six
// hours the rest of the seed's jobs and runners cover, so that morning reads
// as it always has and an analysis asked about the window reads only what was
// put there for it. Host samples are the exception: seedHostSamples draws a
// whole day of them, and one host's are rewritten in the window.
//
// Every number below is chosen against what the controller reads for each
// analysis (internal/controller/analysis.go), not against what an analysis
// might be imagined to read: a fixture that only looked right would let the
// demo name the wrong cause, or the right one with a hedge.
//
// The jobs here carry no release. The demo's release comparison is meant to
// say its sample is too small to rank (decision 7), and stamping these would
// tip one side past the line.
const demoAnalysisFrom, demoAnalysisTo = 22 * time.Hour, 20 * time.Hour

// Where the fixture's GitHub numbers start: past every job and run the rest
// of the seed writes, and run numbers under the morning's 300, because these
// runs came first.
const (
	demoAnalysisGitHubJob = 82000
	demoAnalysisGitHubRun = 42000
	demoAnalysisRunNumber = 280
	// The runner names' index, past the dozen seedRunners names.
	demoAnalysisRunnerIndex = 20
)

// seedAnalyses writes the analysis fixture. It runs after seedHostSamples,
// because one host's samples in the window are rewritten here.
func (c *Controller) seedAnalyses(ctx context.Context, now time.Time, linux, arm *store.Pool, hosts []*store.Host) error {
	byID := hostsByID(hosts)
	a, b, arm64 := byID[demoHostPrefix+"a"], byID[demoHostPrefix+"b"], byID[demoHostPrefix+"c"]
	if a == nil || b == nil || arm64 == nil {
		return fmt.Errorf("seeding the analysis fixture: the demo hosts %sa, %sb and %sc are not all there", demoHostPrefix, demoHostPrefix, demoHostPrefix)
	}
	s := &analysisSeed{c: c, from: now.Add(-demoAnalysisFrom), to: now.Add(-demoAnalysisTo)}
	if err := s.slowStarts(ctx, linux, a); err != nil {
		return err
	}
	if err := s.poolCeiling(ctx, arm, arm64); err != nil {
		return err
	}
	if err := s.hostPressure(ctx, now, linux, b); err != nil {
		return err
	}
	return s.imageFailures(ctx, linux, b)
}

// analysisSeed is the window and the numbering the fixture's four parts
// share, so no two of them can hand out the same run or runner name.
type analysisSeed struct {
	c        *Controller
	from, to time.Time
	jobs     int
	runs     int
	runners  int
}

// run starts a new workflow run and returns its number in the fixture.
func (s *analysisSeed) run() int {
	s.runs++
	return s.runs - 1
}

// job is one of the fixture's jobs, claimed by pool the moment it was queued:
// eligible a second later, as a webhook claims one, and stamped so rather
// than left to the store, which would stamp it now.
func (s *analysisSeed) job(id string, pool *store.Pool, repo, workflow, name string, run int, queued time.Time) *store.Job {
	n := s.jobs
	s.jobs++
	eligible := queued.Add(time.Second)
	return &store.Job{
		ID:             id,
		GitHubJobID:    int64(demoAnalysisGitHubJob + n),
		GitHubRunID:    int64(demoAnalysisGitHubRun + run),
		RunNumber:      int64(demoAnalysisRunNumber + run),
		Repo:           repo,
		Workflow:       workflow,
		JobName:        name,
		Labels:         pool.Labels,
		InstallationID: pool.InstallationID,
		PoolID:         pool.ID,
		Matched:        true,
		QueuedAt:       queued,
		EligibleAt:     &eligible,
		HTMLURL:        fmt.Sprintf("https://github.com/%s/actions/runs/%d", repo, demoAnalysisGitHubRun+run),
		HeadBranch:     "main",
		HeadSHA:        fmt.Sprintf("%040x", 0xFACADE+run*7919),
		RunAttempt:     1,
	}
}

// finish completes a job with its steps, and writes it with its timeline.
func (s *analysisSeed) finish(ctx context.Context, j *store.Job, conclusion string, started, completed time.Time) error {
	j.State, j.Conclusion = store.JobCompleted, conclusion
	j.StartedAt, j.CompletedAt = &started, &completed
	j.Steps = demoSteps(j.JobName, conclusion, started, completed)
	saved, change, err := s.c.st.ApplyJob(ctx, j)
	if err != nil {
		return fmt.Errorf("seeding the analysis job %s: %w", j.ID, err)
	}
	return s.c.seedJobTimeline(ctx, saved, change)
}

// runner is a runner of pool on host, asked for at requested. How it came
// up and how it ended are the caller's.
func (s *analysisSeed) runner(id string, pool *store.Pool, host *store.Host, requested time.Time) *store.Runner {
	n := demoAnalysisRunnerIndex + s.runners
	s.runners++
	return &store.Runner{
		ID:                 id,
		PoolID:             pool.ID,
		HostID:             host.ID,
		Name:               demoRunnerName(pool, n),
		State:              store.RunnerRemoved,
		Ephemeral:          pool.Ephemeral,
		Labels:             pool.Labels,
		Image:              s.c.RunnerImage(pool),
		AllocatedCPUs:      pool.Resources.CPUs,
		AllocatedMemoryMB:  pool.Resources.MemoryMB,
		AllocationSource:   store.AllocationFromPool,
		ContainerID:        fmt.Sprintf("demo%032d", n),
		GitHubRunnerID:     int64(9000 + n),
		CreateTaskIssuedAt: &requested,
	}
}

// cameUp is a runner whose container started and which registered.
func cameUp(r *store.Runner, containerStarted, registered time.Time) {
	r.StartedAt, r.ContainerStartedAt, r.RegisteredAt = &containerStarted, &containerStarted, &registered
}

// ended is a runner that finished at, cleaned up on its host and on GitHub a
// few seconds later, as a finished runner is: a row left half cleaned would
// be something the reconcile loop went back to, and a problem the demo does
// not have.
func ended(r *store.Runner, at time.Time) {
	registrationDeleted, hostRemoved, cleaned := at.Add(time.Second), at.Add(2*time.Second), at.Add(3*time.Second)
	r.FinishedAt, r.RegistrationDeletedAt, r.HostRemovedAt, r.CleanedUpAt = &at, &registrationDeleted, &hostRemoved, &cleaned
}

// saveRunner writes a runner and moves its creation to when it was asked
// for. The store stamps creation itself, and the analyses read a runner by
// it: one created at seeding would be a runner of today, not of the window.
func (s *analysisSeed) saveRunner(ctx context.Context, r *store.Runner) error {
	if err := s.c.st.CreateRunner(ctx, r); err != nil {
		return fmt.Errorf("seeding the analysis runner %s: %w", r.ID, err)
	}
	if err := s.c.st.SetRunnerCreatedAt(ctx, r.ID, *r.CreateTaskIssuedAt); err != nil {
		return fmt.Errorf("dating the analysis runner %s: %w", r.ID, err)
	}
	return nil
}

// slowStarts is the queue-wait fixture's runner startup: twelve jobs on the
// linux pool, each given a runner of its own on the first builder that takes
// four minutes to register, most of it pulling the image. Startup is carved
// out of a wait from the runner's request to its registration, so with the
// request a second after the job became eligible and the job taken eight
// seconds after registration, each wait is about 96% startup, and twelve is
// past the ten jobs the analysis needs before it calls one cause dominant.
// Nine minutes apart, no two of them are alive at once, so the pool never
// comes near its ceiling of eight and its waits are not the ceiling's.
func (s *analysisSeed) slowStarts(ctx context.Context, linux *store.Pool, host *store.Host) error {
	names := []string{"build", "test"}
	run := 0
	for i := range 12 {
		if i%2 == 0 {
			run = s.run()
		}
		queued := s.from.Add(5*time.Minute + time.Duration(i)*9*time.Minute)
		requested := queued.Add(2 * time.Second)
		registered := requested.Add(4 * time.Minute)
		started := registered.Add(8 * time.Second)
		completed := started.Add(150*time.Second + time.Duration(i%4)*40*time.Second)

		r := s.runner(fmt.Sprintf("run_demowait%02d", i+1), linux, host, requested)
		pull := 3*time.Minute + 30*time.Second
		r.ImagePullDuration = &pull
		cameUp(r, requested.Add(pull+5*time.Second), registered)
		ended(r, completed.Add(4*time.Second))
		r.Message, r.JobsHandled = "ephemeral runner exited cleanly after its job", 1
		if err := s.saveRunner(ctx, r); err != nil {
			return err
		}

		j := s.job(fmt.Sprintf("job_demowait%02d", i+1), linux, demoRepos[0], "CI", names[i%2], run, queued)
		j.RunnerID, j.RunnerName, j.HostID = r.ID, r.Name, host.ID
		if err := s.finish(ctx, j, "success", started, completed); err != nil {
			return err
		}
	}
	return nil
}

// poolCeiling is the queue-wait fixture's ceiling: the arm pool's four
// runners, its max_runners, taken from before the window to after it by a
// nightly run's four long shards, while twelve release jobs queue behind
// them. The pool keeps its runners, so they hold their places whether busy or
// idle; the shards are what make them busy, so the waiting jobs have nowhere
// to go. The twelve start only after the window, so every second of their
// waits inside it is the ceiling's, and none of them is a job that ran on a
// kept runner inside it, which the analysis could not split and would hedge
// on.
func (s *analysisSeed) poolCeiling(ctx context.Context, arm *store.Pool, host *store.Host) error {
	const holders = 4
	nightly := s.run()
	queued := s.from.Add(-31 * time.Minute)
	shardEnds := make([]time.Time, holders)
	runners := make([]*store.Runner, holders)
	for k := range holders {
		requested := s.from.Add(-30*time.Minute + time.Duration(k)*20*time.Second)
		registered := requested.Add(50 * time.Second)
		r := s.runner(fmt.Sprintf("run_demohold%d", k+1), arm, host, requested)
		cameUp(r, requested.Add(25*time.Second), registered)
		runners[k] = r
		shardEnds[k] = s.to.Add(10*time.Minute + time.Duration(k)*20*time.Second)

		j := s.job(fmt.Sprintf("job_demohold%d", k+1), arm, demoRepos[2], "Nightly", fmt.Sprintf("e2e-%d", k+1), nightly, queued)
		j.RunnerID, j.RunnerName, j.HostID = r.ID, r.Name, host.ID
		if err := s.finish(ctx, j, "success", registered.Add(5*time.Second), shardEnds[k]); err != nil {
			return err
		}
	}

	// Each runner takes three of the queued jobs once its shard is done, one
	// after the other, then idles out past the pool's ten-minute timeout.
	lastDone := slices.Clone(shardEnds)
	names := []string{"build", "test", "package"}
	run := 0
	for i := range 12 {
		if i%3 == 0 {
			run = s.run()
		}
		k, batch := i%holders, i/holders
		started := shardEnds[k].Add(15*time.Second + time.Duration(batch)*12*time.Minute)
		completed := started.Add(9*time.Minute + time.Duration(i)*5*time.Second)
		lastDone[k] = completed

		j := s.job(fmt.Sprintf("job_democeil%02d", i+1), arm, demoRepos[2], "Release", names[i%3], run,
			s.from.Add(10*time.Minute+time.Duration(i)*5*time.Minute))
		r := runners[k]
		j.RunnerID, j.RunnerName, j.HostID = r.ID, r.Name, host.ID
		if err := s.finish(ctx, j, "success", started, completed); err != nil {
			return err
		}
	}

	for k, r := range runners {
		ended(r, lastDone[k].Add(11*time.Minute))
		r.Message, r.JobsHandled = "idle for 11m, over the 10m idle timeout", 4
		if err := s.saveRunner(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

// hostPressure is the host-pressure fixture: the second builder's two hours
// of samples rewritten, the first twenty minutes at a load of 20 on its 8
// cores and the rest at 4, against the throttle's line of 2 a core, with six
// failed jobs completing in the busy minutes and one in a calm one. Six of
// the seven in a sixth of the minutes is a cluster by any reading, and seven
// is past the five the analysis needs to be sure of one.
//
// The rewrite overwrites the day's curve that seedHostSamples drew there,
// and leaves CPU and memory unmeasured on purpose: the curve's figures follow
// the local hour, so whichever line they drew could depend on when the demo
// was seeded, and load is the measure the throttle itself reads (P19).
//
// The jobs have no runner rows, so they bring no startups to judge, and the
// host's only starts in the window are the image fixture's. Each was queued
// as it started, so the queue-wait analysis of their pool reads them as jobs
// that did not wait, and neither counts them nor hedges on them (P17).
func (s *analysisSeed) hostPressure(ctx context.Context, now time.Time, linux *store.Pool, host *store.Host) error {
	const minutes, busy = 120, 20
	first := s.from.Truncate(time.Minute)
	alloc := host.Allocatable()
	samples := make([]store.HostSample, 0, minutes)
	for m := range minutes {
		at := first.Add(time.Duration(m) * time.Minute)
		load, active := 4.0, 1
		if m < busy {
			load, active = 20.0, host.Capacity
		}
		reservedCPUs, reservedMem := float64(active)*2, int64(active)*4096
		samples = append(samples, store.HostSample{
			HostID:              host.ID,
			At:                  at,
			Capacity:            host.Capacity,
			ActiveRunners:       active,
			LoadAverage1:        &load,
			CPUs:                int64(host.CPUs),
			MemoryMB:            host.MemoryMB,
			AllocatableCPUs:     alloc.CPUs,
			AllocatableMemoryMB: alloc.MemoryMB,
			ReservedCPUs:        &reservedCPUs,
			ReservedMemoryMB:    &reservedMem,
			DiskTotalMB:         host.DiskTotalMB,
			// The disk line seedHostSamples draws, so the rewrite does not
			// step it.
			DiskFreeMB: host.DiskFreeMB + int64(now.Truncate(time.Minute).Sub(at)/time.Minute)*8,
		})
	}
	if err := s.c.st.RecordHostSamples(ctx, samples); err != nil {
		return fmt.Errorf("seeding the pressured host's samples: %w", err)
	}

	// When each failure completed, from the window's first minute: six inside
	// the busy twenty, and one an hour into the calm.
	failures := []time.Duration{
		3*time.Minute + 10*time.Second, 5*time.Minute + 40*time.Second, 8*time.Minute + 20*time.Second,
		11*time.Minute + 5*time.Second, 13*time.Minute + 50*time.Second, 16*time.Minute + 30*time.Second,
		70*time.Minute + 15*time.Second,
	}
	for i, at := range failures {
		completed := first.Add(at)
		started := completed.Add(-(90*time.Second + time.Duration(i)*10*time.Second))
		j := s.job(fmt.Sprintf("job_demopress%d", i+1), linux, demoRepos[0], "Nightly", "integration", s.run(), started)
		j.EligibleAt, j.HostID = &started, host.ID
		if err := s.finish(ctx, j, "failure", started, completed); err != nil {
			return err
		}
	}
	return nil
}

// imageFailures is the startup fixture: the second builder starts one of the
// linux pool's runners and then fails six in a row before their containers
// start, because it cannot pull the runner image. The start that registered
// comes first, so the run of failures has a start before it to be failing
// since (P20). The pool's runners started on the first builder all window, so
// the host, not the pool, is what keeps failing: six of seven there, and
// under half of the pool's nineteen. Each failure records the image fault,
// which explains all six, so the finding has nothing to hedge on.
//
// They are failed rows, as a failed start stays until retention.runners
// prunes it, cleaned up on the host and on GitHub as the fleet does.
func (s *analysisSeed) imageFailures(ctx context.Context, linux *store.Pool, host *store.Host) error {
	requested := s.from.Add(30 * time.Minute)
	registered := requested.Add(40 * time.Second)
	r := s.runner("run_demopull1", linux, host, requested)
	cameUp(r, requested.Add(12*time.Second), registered)
	ended(r, registered.Add(6*time.Minute+5*time.Second))
	r.Message = "idle for 6m, over the 5m idle timeout"
	if err := s.saveRunner(ctx, r); err != nil {
		return err
	}

	image := s.c.RunnerImage(linux)
	for k := range 6 {
		requested := s.from.Add(45*time.Minute + time.Duration(k)*10*time.Minute)
		r := s.runner(fmt.Sprintf("run_demopull%d", k+2), linux, host, requested)
		// The pull is what failed, so there was never a container.
		r.State, r.FaultKind, r.ContainerID = store.RunnerFailed, store.FaultImage, ""
		ended(r, requested.Add(35*time.Second))
		r.Message = fmt.Sprintf("backend: pulling %s: Error response from daemon: unauthorized: authentication required", image)
		if err := s.saveRunner(ctx, r); err != nil {
			return err
		}
	}
	return nil
}
