package scheduler

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// assign used to set every queued job against every pool. It now sets a job
// against them only if no job before it asked the same thing, and gives the rest
// that job's answer. The function below is the one it replaced, kept as it was,
// and the tests hold the new one to its answer over a great many random fleets in
// which many jobs do ask the same thing.

// assign maps every queued job onto the pool that will run it, and collects the
// ones nothing claims, each with the reason it went unclaimed.
func referenceAssign(pools []*store.Pool, jobs []*store.Job, targets map[string]string) (map[string][]*store.Job, []UnmatchedJob) {
	demand := make(map[string][]*store.Job, len(pools))
	var unmatched []UnmatchedJob
	var prepared [][]string
	for _, j := range sortedJobs(jobs) {
		if j.State != store.JobQueued || j.Provisioning != "" {
			continue
		}
		if prepared == nil {
			// Prepared for the first job that needs it. Most passes find nothing
			// waiting, and they should not pay to get the pools ready for a
			// comparison that never happens.
			prepared = preparedLabels(pools)
		}
		p, reason := bestOf(pools, prepared, j, labelsOf(store.NormalizeLabels(j.Labels)), targets)
		if p == nil {
			unmatched = append(unmatched, UnmatchedJob{Job: j, Reason: reason})
			continue
		}
		demand[p.ID] = append(demand[p.ID], j)
	}
	return demand, unmatched
}

// randomAssignFleet is pools and jobs drawn from a small vocabulary, so that a
// great many of the jobs ask the same thing as another and a great many ask what
// only some pools offer: labels in several cases and orders (which normalise to
// the same), two installations and none, three size classes and none, a few
// repositories, jobs in every state and some held back from provisioning, pools
// that are disabled, that decline the implicit labels and that are kept for a
// class of runner.
func randomAssignFleet(rng *rand.Rand) ([]*store.Pool, []*store.Job, map[string]string) {
	vocabulary := []string{"self-hosted", "linux", "x64", "arm64", "gpu", "Large", "docker", "windows"}
	installations := []string{"", "ins_a", "ins_b"}
	var pools []*store.Pool
	for i := range 1 + rng.Intn(12) {
		var labels []string
		for _, l := range vocabulary {
			if rng.Intn(3) == 0 {
				labels = append(labels, l)
			}
		}
		p := testPool(fmt.Sprintf("pool%02d", i), labels...)
		p.ID = fmt.Sprintf("pool_%02d", i)
		p.InstallationID = installations[1+rng.Intn(2)]
		p.Enabled = rng.Intn(8) != 0
		p.NoDefaultLabels = rng.Intn(6) == 0
		if rng.Intn(3) == 0 {
			p.AutoKey = store.AutoKeyFor([]string{"amd64", "arm64"}[rng.Intn(2)], store.SizeClasses()[rng.Intn(3)])
		}
		pools = append(pools, p)
	}
	if rng.Intn(5) == 0 {
		pools = append(pools, nil)
	}

	states := []store.JobState{store.JobQueued, store.JobQueued, store.JobQueued, store.JobInProgress, store.JobWaiting}
	var jobs []*store.Job
	for i := range rng.Intn(60) {
		var labels []string
		for _, l := range vocabulary {
			if rng.Intn(3) == 0 {
				labels = append(labels, []string{l, "  " + l + " "}[rng.Intn(2)])
			}
		}
		rng.Shuffle(len(labels), func(a, b int) { labels[a], labels[b] = labels[b], labels[a] })
		j := queued(fmt.Sprintf("job_%03d", i), time.Duration(rng.Intn(600))*time.Second, labels...)
		j.State = states[rng.Intn(len(states))]
		j.InstallationID = installations[rng.Intn(3)]
		j.Repo = []string{"acme/widgets", "acme/gadgets", "other/things"}[rng.Intn(3)]
		if rng.Intn(3) == 0 {
			j.RoutedClass = store.SizeClasses()[rng.Intn(3)]
		}
		if rng.Intn(10) == 0 {
			j.Provisioning = "paused"
		}
		jobs = append(jobs, j)
	}
	return pools, jobs, map[string]string{"ins_a": "acme", "ins_b": "other"}
}

func TestChoosingAPoolOnceForEachKindOfJobChoosesAsChoosingForEveryJobDid(t *testing.T) {
	var matched, unmatched, shared int
	for seed := range int64(1500) {
		rng := rand.New(rand.NewSource(seed))
		pools, jobs, targets := randomAssignFleet(rng)

		gotDemand, gotUnmatched := assign(pools, jobs, targets)
		wantDemand, wantUnmatched := referenceAssign(pools, jobs, targets)
		if !reflect.DeepEqual(gotDemand, wantDemand) || !reflect.DeepEqual(gotUnmatched, wantUnmatched) {
			t.Fatalf("seed %d: the jobs were assigned differently\n got: %v / %v\nwant: %v / %v", seed, gotDemand, gotUnmatched, wantDemand, wantUnmatched)
		}
		for _, js := range wantDemand {
			matched += len(js)
			if len(js) > 1 {
				shared++
			}
		}
		unmatched += len(wantUnmatched)
	}
	// The comparison says something only if the fleets gave the jobs somewhere to go,
	// somewhere not to, and more than one job to a pool.
	if matched == 0 || unmatched == 0 || shared == 0 {
		t.Fatalf("the random fleets exercised too little: %d jobs matched, %d unmatched, %d pools given more than one", matched, unmatched, shared)
	}
}

// Two jobs that differ only in what bestOf never reads are given one answer, and
// two that differ in what it does read are not: the repository only where it is
// named in the reason.
func TestJobsAreGivenTheSameAnswerOnlyWhenTheyAskTheSameThing(t *testing.T) {
	a := testPool("a", "self-hosted", "gpu")
	a.ID, a.InstallationID = "pool_a", "ins_a"
	pools := []*store.Pool{a}
	targets := map[string]string{"ins_a": "acme", "ins_b": "other"}

	job := func(id, installation, repo string, labels ...string) *store.Job {
		j := queued(id, time.Minute, labels...)
		j.InstallationID, j.Repo = installation, repo
		return j
	}
	jobs := []*store.Job{
		job("job_1", "ins_a", "acme/widgets", "self-hosted", "gpu"), // goes to pool a
		job("job_2", "ins_a", "acme/gadgets", "gpu", "self-hosted"), // the same ask, in another order and repository: pool a
		job("job_3", "ins_b", "other/things", "self-hosted", "gpu"), // another installation: the near miss, not pool a
		job("job_4", "", "acme/widgets", "self-hosted", "gpu"),      // no installation, naming acme/widgets
		job("job_5", "", "acme/gadgets", "self-hosted", "gpu"),      // the same, naming acme/gadgets
	}
	for i := range jobs {
		jobs[i].QueuedAt = ago(time.Duration(len(jobs)-i) * time.Minute) // in the order written
	}
	demand, unmatched := assign(pools, jobs, targets)
	if len(demand["pool_a"]) != 2 {
		t.Errorf("pool a was given %d jobs; want the first two", len(demand["pool_a"]))
	}
	reasons := map[string]string{}
	for _, u := range unmatched {
		reasons[u.Job.ID] = u.Reason
	}
	if len(reasons) != 3 {
		t.Fatalf("%d jobs went unmatched; want three: %v", len(reasons), reasons)
	}
	if reasons["job_4"] == reasons["job_5"] {
		t.Errorf("two jobs in different repositories with no installation were told the same reason, %q; each reason names its repository", reasons["job_4"])
	}
	if reasons["job_3"] == reasons["job_4"] {
		t.Errorf("a job of another installation and one of none were told the same reason, %q", reasons["job_3"])
	}
}

// The answer is kept under the bytes of the question, so the bytes have to say where
// one label ends and the next begins: ab and c run together as a and bc do, and they
// are not the same ask.
func TestLabelsThatRunTogetherAreNotTheSameQuestion(t *testing.T) {
	x := testPool("x", "ab", "c")
	x.ID, x.InstallationID = "pool_x", "ins_a"
	y := testPool("y", "a", "bc")
	y.ID, y.InstallationID = "pool_y", "ins_a"

	job := func(id string, age time.Duration, labels ...string) *store.Job {
		j := queued(id, age, labels...)
		j.InstallationID, j.Repo = "ins_a", "acme/widgets"
		return j
	}
	jobs := []*store.Job{job("job_1", 2*time.Minute, "ab", "c"), job("job_2", time.Minute, "a", "bc")}

	demand, unmatched := assign([]*store.Pool{x, y}, jobs, nil)
	if len(unmatched) != 0 || len(demand["pool_x"]) != 1 || demand["pool_x"][0].ID != "job_1" ||
		len(demand["pool_y"]) != 1 || demand["pool_y"][0].ID != "job_2" {
		t.Errorf("the jobs asking for ab and c, and for a and bc, were given pool_x %v and pool_y %v, with %d unmatched; want one each",
			jobIDsOf(demand["pool_x"]), jobIDsOf(demand["pool_y"]), len(unmatched))
	}
}

func jobIDsOf(jobs []*store.Job) []string {
	out := make([]string, len(jobs))
	for i, j := range jobs {
		out[i] = j.ID
	}
	return out
}

// What a pass saves by it is the comparison, and a refusal is the part of the
// comparison that leaves something to count: the sentence saying which pool a job
// nearly went to is built each time bestOf has to word it. A thousand jobs refused
// for one reason cost the allocations of one sentence, not a thousand.
func TestAKindOfJobIsRefusedAndExplainedOnceHoweverManyAskIt(t *testing.T) {
	p := testPool("a", "self-hosted", "gpu")
	p.ID, p.InstallationID = "pool_a", "ins_a"
	pools := []*store.Pool{p}
	targets := map[string]string{"ins_a": "acme", "ins_b": "other"}

	refused := func(n int) []*store.Job {
		jobs := make([]*store.Job, n)
		for i := range jobs {
			// As the store hands them over, already normalised: labels out of order
			// are copied into order for every job, kind or no kind.
			j := queued(fmt.Sprintf("job_%04d", i), time.Duration(n-i)*time.Second, "gpu", "self-hosted")
			j.InstallationID, j.Repo = "ins_b", "other/things" // the near miss: the pool belongs to another installation
			jobs[i] = j
		}
		return jobs
	}
	few, many := refused(10), refused(1000)
	if _, unmatched := assign(pools, many, targets); len(unmatched) != len(many) || unmatched[0].Reason == "" {
		t.Fatalf("%d of %d jobs were refused, the first with %q; want all of them, with a reason", len(unmatched), len(many), unmatched[0].Reason)
	}

	allocsFew := testing.AllocsPerRun(10, func() { assign(pools, few, targets) })
	allocsMany := testing.AllocsPerRun(10, func() { assign(pools, many, targets) })
	// The list of refusals grows with the jobs, a few allocations for a hundredfold
	// more of them. Wording the reason for each would be several for every job.
	if allocsMany-allocsFew > 50 {
		t.Errorf("a thousand jobs asking the same thing took %.0f allocations and ten took %.0f; the reason for refusing them should be worded once", allocsMany, allocsFew)
	}
}
