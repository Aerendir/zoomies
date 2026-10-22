package scheduler

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// Decide runs on every reconcile pass whether or not anything is happening, so
// what it costs is paid continuously, and a change that makes it a little
// slower is paid for just as long. These benchmarks put a number on a pass at
// three fleet sizes in five situations, to be set beside the same run after a
// change to the scheduler (benchstat before.txt after.txt).
//
// Each snapshot is built before the timer starts and checked once. A fleet
// that was meant to be full but quietly had room would report a flattering
// number for a code path its name does not describe.

// benchScale is the size of fleet a pass decides over. The ladder shows how the
// cost grows rather than naming a fleet anyone runs: Decide sets every queued
// job against every pool, so one size cannot say whether the next is twice as
// slow or a hundred times.
type benchScale struct {
	name           string
	pools, hosts   int
	runnersPerPool int
	jobsPerPool    int
}

var benchScales = []benchScale{
	{name: "4pools", pools: 4, hosts: 2, runnersPerPool: 4, jobsPerPool: 2},
	{name: "20pools", pools: 20, hosts: 8, runnersPerPool: 8, jobsPerPool: 5},
	{name: "100pools", pools: 100, hosts: 40, runnersPerPool: 8, jobsPerPool: 10},
}

// benchFleet is what a situation changes about the fleet a scale describes.
type benchFleet struct {
	// idle is how many of each pool's runners are idle; the rest hold a job.
	idle int
	// queued says each pool has jobs waiting for it, and stray that as many
	// again wait for a label no pool advertises. Distinct gives each of those a
	// label of its own, so that no two jobs ask the same thing.
	queued, stray, distinct bool
	// spare is the slots every host has beyond the runners already on it.
	spare int
	// sized gives the hosts measured CPUs, memory and disk and the pools a limit
	// on each runner, so that placement does its resource accounting too.
	sized bool
}

// benchJob is a job as the store hands it to the scheduler, with its labels put
// through NormalizeLabels as every job's are on the way in. The tests' own
// queued helper leaves them as given, and labels out of order are ones the
// matching has to normalise again at every comparison, which no job that came
// from the store ever needs.
func benchJob(j *store.Job) *store.Job {
	j.Labels = store.NormalizeLabels(j.Labels)
	return j
}

// snapshot builds the fleet: every pool is asked for by a label of its own, and
// its runners are spread over the hosts in turn, which is what each host's
// active count is then taken from.
func (f benchFleet) snapshot(sc benchScale) Snapshot {
	hosts := make([]*store.Host, sc.hosts)
	for i := range hosts {
		id := fmt.Sprintf("host_%03d", i)
		if f.sized {
			// Far more than the pools can ask of it, so that the accounting
			// runs without ever being what refuses a runner.
			hosts[i] = sized(id, 0, 128, 512*1024, 4<<20)
		} else {
			hosts[i] = testHost(id, 0, 0)
		}
	}

	var pools []*store.Pool
	var runners []*store.Runner
	var jobs []*store.Job
	placed := 0
	for i := range sc.pools {
		name := fmt.Sprintf("pool%03d", i)
		p := testPool(name, "self-hosted", name)
		p.MaxRunners = sc.runnersPerPool + sc.jobsPerPool
		if f.sized {
			p.Resources = store.Resources{CPUs: 2, MemoryMB: 4096}
		}
		pools = append(pools, p)

		for k := range sc.runnersPerPool {
			id := fmt.Sprintf("run_%s_%02d", name, k)
			var r *store.Runner
			if k < f.idle {
				r = idleRunner(id, p, time.Minute)
			} else {
				r = testRunner(id, p, store.RunnerBusy, 10*time.Minute)
				job := benchJob(queued("job_"+id, 5*time.Minute, "self-hosted", name))
				job.State, job.RunnerID = store.JobInProgress, id
				jobs = append(jobs, job)
			}
			h := hosts[placed%len(hosts)]
			placed++
			r.HostID = h.ID
			h.ActiveRunners++
			runners = append(runners, r)
		}
		if f.queued {
			for k := range sc.jobsPerPool {
				jobs = append(jobs, benchJob(queued(fmt.Sprintf("job_%s_wait%02d", name, k), time.Minute, "self-hosted", name)))
			}
		}
		if f.stray {
			for k := range sc.jobsPerPool {
				label := "no-such-pool"
				if f.distinct {
					label = fmt.Sprintf("no-such-pool-%s-%02d", name, k)
				}
				jobs = append(jobs, benchJob(queued(fmt.Sprintf("job_%s_stray%02d", name, k), time.Minute, "self-hosted", label)))
			}
		}
	}
	for _, h := range hosts {
		h.Capacity = h.ActiveRunners + f.spare
	}
	return snap(pools, runners, jobs, hosts)
}

// benchSituations are the shapes of pass a fleet spends its time in. Quiet is
// the commonest, since most passes find nothing to do, so it is the number
// that sets the steady cost; the others are the passes an operator is waiting
// on.
var benchSituations = []struct {
	name  string
	fleet benchFleet
	// expect says what the pass must have done for the number to mean what
	// the name claims, and returns the complaint when it did not.
	expect func(sc benchScale, plan Plan) string
}{
	{
		// Nothing is waiting and nothing has outlived its welcome, so the
		// pass reads the whole fleet and changes none of it.
		name:  "quiet",
		fleet: benchFleet{idle: 1, spare: 4},
		expect: func(sc benchScale, plan Plan) string {
			if len(plan.Actions) != 0 || len(plan.Unmatched) != 0 {
				return fmt.Sprintf("%d actions and %d unmatched jobs", len(plan.Actions), len(plan.Unmatched))
			}
			return ""
		},
	},
	{
		// A queue arrives where the hosts have room, so the pass places
		// runners, up to the shipped cap on creates per pass.
		name:   "burst",
		fleet:  benchFleet{queued: true, spare: 20},
		expect: expectCreates,
	},
	{
		// The same burst on hosts that report their size, so every placement
		// is weighed against CPU, memory and disk as well as slots.
		name:   "burst_sized",
		fleet:  benchFleet{queued: true, spare: 20, sized: true},
		expect: expectCreates,
	},
	{
		// The queue is there and the hosts are full, so every pool has to
		// work out and word why nothing can be started for it. This is the
		// pass an operator is looking at when jobs are waiting.
		name:  "saturated",
		fleet: benchFleet{queued: true},
		expect: func(sc benchScale, plan Plan) string {
			blocked := 0
			for _, pp := range plan.Pools {
				if pp.Blocked != "" {
					blocked++
				}
			}
			if n := countOf(plan.Actions, ActionCreate); n != 0 || blocked != sc.pools {
				return fmt.Sprintf("%d creates and %d of %d pools blocked", n, blocked, sc.pools)
			}
			return ""
		},
	},
	{
		// Jobs asking for labels no pool has: the pass has to try every pool
		// before it can say so.
		name:   "stray_jobs",
		fleet:  benchFleet{stray: true, spare: 4},
		expect: expectUnmatched,
	},
	{
		// The same jobs, each asking for a label of its own, so that none can be
		// given another's answer: a pass that answers a kind of job once pays for
		// every job here, and this is what it costs when that never helps.
		name:   "distinct_jobs",
		fleet:  benchFleet{stray: true, distinct: true, spare: 4},
		expect: expectDistinct,
	},
}

// expectUnmatched is what a pass over jobs no pool will take must do: say so of
// every one of them.
func expectUnmatched(sc benchScale, plan Plan) string {
	if want := sc.pools * sc.jobsPerPool; len(plan.Unmatched) != want {
		return fmt.Sprintf("%d unmatched jobs, want %d", len(plan.Unmatched), want)
	}
	return ""
}

// expectDistinct is expectUnmatched for jobs of which no two ask for the same.
func expectDistinct(sc benchScale, plan Plan) string {
	if msg := expectUnmatched(sc, plan); msg != "" {
		return msg
	}
	asked := map[string]bool{}
	for _, u := range plan.Unmatched {
		key := fmt.Sprint(u.Job.Labels)
		if asked[key] {
			return fmt.Sprintf("two jobs ask for %s", key)
		}
		asked[key] = true
	}
	return ""
}

// expectCreates is what a burst on hosts with room must do: start as many
// runners as there are jobs waiting, or as many as one pass may start.
func expectCreates(sc benchScale, plan Plan) string {
	want := min(sc.pools*sc.jobsPerPool, testPolicy().MaxCreatesPerTick)
	if n := countOf(plan.Actions, ActionCreate); n != want {
		return fmt.Sprintf("%d creates, want %d", n, want)
	}
	return ""
}

func BenchmarkDecide(b *testing.B) {
	for _, sit := range benchSituations {
		b.Run(sit.name, func(b *testing.B) {
			for _, sc := range benchScales {
				b.Run(sc.name, func(b *testing.B) {
					s := sit.fleet.snapshot(sc)
					first := Decide(s)
					if msg := sit.expect(sc, first); msg != "" {
						b.Fatalf("this fleet is not a %s one: %s", sit.name, msg)
					}
					// What the store hands over is already normalised, and the
					// matching has a fast path for exactly that. A fleet whose
					// labels are not would time the slow path, which no stored
					// job or pool takes.
					for _, j := range s.Jobs {
						if !slices.Equal(j.Labels, store.NormalizeLabels(j.Labels)) {
							b.Fatalf("job %s has labels the store would have normalised: %v", j.ID, j.Labels)
						}
					}
					for _, p := range s.Pools {
						if !slices.Equal(p.Labels, store.NormalizeLabels(p.Labels)) {
							b.Fatalf("pool %s has labels the store would have normalised: %v", p.Name, p.Labels)
						}
					}
					if sit.fleet.sized && Reserved(s.Hosts[0], s.Pools, s.Runners).MemoryMB == 0 {
						b.Fatal("the hosts report their size but the runners on them are charged nothing against it")
					}
					// Decide is meant to be deterministic and to leave its
					// snapshot alone. If it did not, every iteration after the
					// first would be measuring a different fleet.
					if !reflect.DeepEqual(first, Decide(s)) {
						b.Fatal("a second pass over the same snapshot decided differently")
					}

					b.ReportAllocs()
					for b.Loop() {
						Decide(s)
					}
				})
			}
		})
	}
}
