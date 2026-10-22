package store

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

// The dashboard reads one figure of the six that JobStats works out, each by a
// statement over the whole window, and the dearest are the three percentiles. A
// caller can now ask for the figures it reads (JobStatsOf), and these tests hold
// that to the answer JobStats gives: the groups, their order and where the list is
// cut off do not move, the figures asked for are exactly the figures JobStats has,
// and the ones not asked for are left at zero.

// mixedJobs puts a history in the store that gives every figure something to count:
// n completed jobs over three days, in three pools (one of them blank), under three
// job names, from two releases and from jobs nothing stamped, on two hosts and on
// none, ending every way a job can, some lost to the fleet with a kind and some
// without, some measured and some not, some that never started, and some whose
// runner row is still kept. Durations come in whole seconds so that samples tie.
func mixedJobs(t *testing.T, s *Store, n int) {
	t.Helper()
	ctx := context.Background()
	rng := rand.New(rand.NewSource(7))
	_, pool, host := seedPool(t, s)

	// A runner whose container started before it was created has no startup: that is
	// two clocks disagreeing, and the figure leaves it out.
	var runners []string
	for i := range 12 {
		r := &Runner{PoolID: pool.ID, HostID: host.ID, Name: fmt.Sprintf("zoomies-%d", i), Ephemeral: true}
		if err := s.CreateRunner(ctx, r); err != nil {
			t.Fatalf("CreateRunner: %v", err)
		}
		created := versionsEpoch.Add(time.Duration(i) * time.Hour)
		started := created.Add(time.Duration(1+rng.Intn(9)) * time.Second)
		if i%5 == 4 {
			started = created.Add(-time.Second)
		}
		if _, err := s.exec(ctx, `UPDATE runners SET created_at = ?, container_started_at = ? WHERE id = ?`, ms(created), ms(started), r.ID); err != nil {
			t.Fatalf("dating runner %d: %v", i, err)
		}
		runners = append(runners, r.ID)
	}

	pools := []string{"pool_a", "pool_b", ""}
	names := []string{"build", "test", "lint"}
	conclusions := []string{"success", "success", "success", "success", "success", "success", "failure", "failure", "cancelled", "skipped"}
	releases := []any{"v1.0.0", "v1.1.0", nil}
	hosts := []any{"host_1", "host_2", nil}
	for i := range n {
		queued := versionsEpoch.Add(time.Duration(i) * 37 * time.Minute)
		started := queued.Add(time.Duration(rng.Intn(24)) * 5 * time.Second)
		done := started.Add(time.Duration(rng.Intn(120)) * 5 * time.Second)
		j := &Job{
			GitHubJobID: int64(2000 + i), Repo: "acme/widgets", Workflow: "ci", JobName: names[rng.Intn(len(names))],
			State: JobCompleted, Conclusion: conclusions[rng.Intn(len(conclusions))], Matched: true,
			PoolID: pools[rng.Intn(len(pools))], QueuedAt: queued, StartedAt: &started, CompletedAt: &done,
		}
		if rng.Intn(15) == 0 {
			// Cancelled before a runner took it.
			j.StartedAt = nil
		}
		out, err := s.UpsertJob(ctx, j)
		if err != nil {
			t.Fatalf("UpsertJob %d: %v", i, err)
		}

		var fault, kind string
		if rng.Intn(6) == 0 {
			fault, kind = "the runner stopped", []string{string(FaultRunnerExited), string(FaultOutOfMemory), ""}[rng.Intn(3)]
		}
		var source string
		var cpus float64
		var memory int64
		if rng.Intn(3) > 0 {
			source, cpus, memory = AllocationHistory, []float64{0, 1, 2, 4}[rng.Intn(4)], []int64{0, 2048, 3000, 8192}[rng.Intn(4)]
		}
		var peakCPUs float64
		var peakMemory, peakRunner, peakDaemon int64
		var oom int
		if rng.Intn(2) == 0 {
			peakCPUs, peakMemory = float64(1+rng.Intn(4))/2, int64(256*(1+rng.Intn(20)))
			if rng.Intn(3) == 0 {
				peakRunner, peakDaemon = peakMemory/3, peakMemory-peakMemory/3
			}
			if rng.Intn(8) == 0 {
				oom = 1
			}
		}
		runner := ""
		if rng.Intn(3) == 0 {
			runner = runners[rng.Intn(len(runners))]
		} else if rng.Intn(10) == 0 {
			runner = "run_gone"
		}
		if _, err := s.exec(ctx, `UPDATE jobs SET controller_version = ?, host_id = ?, runner_fault = ?, fault_kind = ?,
				granted_source = ?, granted_cpus = ?, granted_memory_mb = ?, peak_cpus = ?, peak_memory_mb = ?, oom_killed = ?,
				peak_runner_memory_mb = ?, peak_daemon_memory_mb = ?, runner_id = ? WHERE id = ?`,
			releases[rng.Intn(len(releases))], hosts[rng.Intn(len(hosts))], fault, kind, source, cpus, memory,
			peakCPUs, peakMemory, oom, peakRunner, peakDaemon, runner, out.ID); err != nil {
			t.Fatalf("measuring job %d: %v", i, err)
		}
	}
}

// only is a full answer with the figures that were not asked for put back to
// zero, which is what asking for the others must give. It is written out figure
// by figure and does not share anything with JobStatsOf, so that a figure the
// real thing leaks or loses is the difference.
func only(full *JobStatsResult, want JobMeasures) *JobStatsResult {
	out := &JobStatsResult{Truncated: full.Truncated}
	for _, g := range full.Groups {
		if want&MeasureCounts == 0 {
			g.Count, g.Succeeded, g.Failed, g.Cancelled, g.FleetFailed, g.FleetFailureRate = 0, 0, 0, 0, 0, 0
		}
		if want&MeasureFaultKinds == 0 {
			g.FleetFailedByKind = nil
		}
		if want&MeasurePeaks == 0 {
			g.PeakCPUs, g.PeakMemoryMB, g.OOMKilled, g.MeasuredMemory = nil, nil, 0, 0
			g.PeakRunnerMemoryMB, g.PeakDaemonMemoryMB, g.MeasuredHalves = 0, 0, 0
		}
		if want&MeasureDuration == 0 {
			g.Duration = JobPercentiles{}
		}
		if want&MeasureQueueWait == 0 {
			g.QueueWait = JobPercentiles{}
		}
		if want&MeasureStartup == 0 {
			g.Startup = JobPercentiles{}
		}
		out.Groups = append(out.Groups, g)
	}
	return out
}

func TestAskingForSomeOfTheJobFiguresGivesThoseAndLeavesTheRestAtZero(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mixedJobs(t, s, 150)

	later := versionsEpoch.Add(24 * time.Hour)
	cases := []struct {
		name string
		f    JobFilter
		by   []string
	}{
		{"as one total", JobFilter{}, nil},
		{"by pool", JobFilter{}, []string{GroupByPool}},
		{"by job name and pool, after the first day", JobFilter{Since: &later}, []string{GroupByJobName, GroupByPool}},
		{"by release, which reads in the order they were first seen", JobFilter{}, []string{GroupByControllerVersion}},
		{"by day and host", JobFilter{}, []string{GroupByDay, GroupByHost}},
		{"by size, in one pool", JobFilter{PoolIDs: []string{"pool_a"}}, []string{GroupBySize}},
	}

	// The comparison says something only if the history gave each figure something
	// to be different about.
	var counted, lost, named, unnamed, cancelled, measured, killed, halves, timed, waited, started, fewer int
	for _, c := range cases {
		full, err := s.JobStats(ctx, c.f, c.by)
		if err != nil {
			t.Fatalf("%s: JobStats: %v", c.name, err)
		}
		if len(full.Groups) == 0 {
			t.Fatalf("%s: the history gave no groups", c.name)
		}
		for _, g := range full.Groups {
			counted += g.Count
			lost += g.FleetFailed
			cancelled += g.Cancelled
			named += g.FleetFailedByKind[string(FaultRunnerExited)]
			unnamed += g.FleetFailedByKind["unclassified"]
			killed += g.OOMKilled
			halves += g.MeasuredHalves
			timed += g.Duration.Samples
			waited += g.QueueWait.Samples
			started += g.Startup.Samples
			if g.PeakMemoryMB != nil {
				measured++
			}
			if g.Duration.Samples < g.Count {
				fewer++
			}
		}

		for want := JobMeasures(0); want <= MeasureAll; want++ {
			got, err := s.JobStatsOf(ctx, c.f, c.by, want)
			if err != nil {
				t.Fatalf("%s, figures %06b: JobStatsOf: %v", c.name, want, err)
			}
			if exp := only(full, want); !reflect.DeepEqual(got, exp) {
				t.Fatalf("%s, figures %06b: JobStatsOf differs from JobStats with the other figures put back to zero\n got: %+v\nwant: %+v", c.name, want, got.Groups, exp.Groups)
			}
		}
	}
	if counted == 0 || lost == 0 || cancelled == 0 || named == 0 || unnamed == 0 || measured == 0 || killed == 0 || halves == 0 ||
		timed == 0 || waited == 0 || started == 0 || fewer == 0 {
		t.Fatalf("the history exercised too little: counted %d, lost %d, cancelled %d, named kinds %d, unclassified %d, measured groups %d, killed %d, halves %d, timed %d, waited %d, started %d, groups with fewer durations than jobs %d",
			counted, lost, cancelled, named, unnamed, measured, killed, halves, timed, waited, started, fewer)
	}

	// Each figure is a statement over the whole window, so the saving is in not
	// running the ones that are not read. The groups come from the first.
	statements := func(want JobMeasures, f JobFilter) int64 {
		t.Helper()
		before := s.texts.hits.Load() + s.texts.misses.Load()
		if _, err := s.JobStatsOf(ctx, f, []string{GroupByPool}, want); err != nil {
			t.Fatalf("JobStatsOf: %v", err)
		}
		return s.texts.hits.Load() + s.texts.misses.Load() - before
	}
	for _, c := range []struct {
		name string
		want JobMeasures
		ran  int64
	}{
		{"nothing but the groups", 0, 1},
		{"the counts", MeasureCounts, 1},
		// A figure nearly every job has is gathered by the statement that names
		// the groups when it is the only one, because that statement goes through
		// every job anyway.
		{"the duration alone", MeasureDuration, 1},
		{"the queue wait alone", MeasureQueueWait, 1},
		{"the startup alone, which few jobs have", MeasureStartup, 2},
		{"the peaks", MeasurePeaks, 2},
		{"the fault kinds", MeasureFaultKinds, 2},
		{"the counts and the duration", MeasureCounts | MeasureDuration, 2},
		{"the duration and the queue wait", MeasureDuration | MeasureQueueWait, 3},
		{"queue wait and startup", MeasureQueueWait | MeasureStartup, 3},
		{"everything", MeasureAll, 6},
	} {
		if ran := statements(c.want, JobFilter{}); ran != c.ran {
			t.Errorf("asking for %s ran %d statements, want %d", c.name, ran, c.ran)
		}
	}
	// A window with no job in it has no group for the other statements to fill in.
	nothing := versionsEpoch.Add(30 * 24 * time.Hour)
	if ran := statements(MeasureAll, JobFilter{Since: &nothing}); ran != 1 {
		t.Errorf("a window with no job in it ran %d statements, want only the one that found there were no groups", ran)
	}
}

// Only the first MaxJobStatsGroups groups are kept, in the order of their keys, and
// a group with no sample of the figure asked for is still a group: it takes its
// place in the list and its turn to be cut off. Taking the groups from the figure's
// own statement would move the line, and lose the answer's Truncated.
func TestAskingForSomeOfTheJobFiguresCutsTheGroupsOffWhereAskingForAllDoes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for i := range MaxJobStatsGroups + 20 {
		queued := versionsEpoch.Add(time.Duration(i) * time.Minute)
		started, done := queued.Add(5*time.Second), queued.Add(time.Duration(10+i)*time.Second)
		conclusion := "success"
		if i < 30 {
			// The first thirty names took no measured time.
			conclusion = "skipped"
		}
		if _, err := s.UpsertJob(ctx, &Job{
			GitHubJobID: int64(3000 + i), Repo: "acme/widgets", Workflow: "ci", JobName: fmt.Sprintf("job-%04d", i),
			State: JobCompleted, Conclusion: conclusion, Matched: true, PoolID: "pool_a",
			QueuedAt: queued, StartedAt: &started, CompletedAt: &done,
		}); err != nil {
			t.Fatalf("UpsertJob %d: %v", i, err)
		}
	}
	full, err := s.JobStats(ctx, JobFilter{}, []string{GroupByJobName})
	if err != nil {
		t.Fatalf("JobStats: %v", err)
	}
	if !full.Truncated || len(full.Groups) != MaxJobStatsGroups || full.Groups[0].Duration.Samples != 0 {
		t.Fatalf("the history gave %d groups, truncated %v, first with %d duration samples; want %d, truncated, the first with none",
			len(full.Groups), full.Truncated, full.Groups[0].Duration.Samples, MaxJobStatsGroups)
	}
	for _, want := range []JobMeasures{0, MeasureDuration, MeasureQueueWait | MeasurePeaks} {
		got, err := s.JobStatsOf(ctx, JobFilter{}, []string{GroupByJobName}, want)
		if err != nil {
			t.Fatalf("figures %06b: JobStatsOf: %v", want, err)
		}
		if exp := only(full, want); !reflect.DeepEqual(got, exp) {
			t.Fatalf("figures %06b: the %d groups kept, truncated %v, are not the %d groups, truncated %v, that JobStats keeps",
				want, len(got.Groups), got.Truncated, len(exp.Groups), exp.Truncated)
		}
	}
}
