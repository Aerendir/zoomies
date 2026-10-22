package scheduler

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// A pass compares every queued job with every pool, so it normalises each
// side's labels once and compares those, rather than having every comparison
// normalise them, or check that it need not, for itself. That changes how the
// answer is reached and must not change the answer, which has a good many
// corners: case and spacing, duplicates, the labels every runner advertises, an
// operating system or architecture a pool declares, a pool that lists exactly
// what it has, a pool that is disabled or belongs to another installation, and
// ties.
//
// The reference below is the matching as a single comparison writes it,
// normalising both sides every time. It restates the rules rather than calling
// Eligible, PoolMatches or score, so that a change to those cannot move the
// reference with it.

// referenceLabelsMatch is the rule for one pool's labels against one job's, as it
// was written before a job's labels were classified once: a lookup in
// store.ImplicitLabels for every label at every comparison. It is restated here
// rather than called, so that changing how matches does it cannot move the
// reference with it.
func referenceLabelsMatch(pool, job []string, implicit bool) bool {
	for _, l := range job {
		if implicit && store.ImplicitLabels[l] {
			continue
		}
		if !slices.Contains(pool, l) {
			return false
		}
	}
	return !referenceContradicts(pool, job, osLabels) && !referenceContradicts(pool, job, archLabels)
}

func referenceContradicts(pool, job []string, dim map[string]bool) bool {
	if !slices.ContainsFunc(pool, func(l string) bool { return dim[l] }) {
		return false
	}
	return slices.ContainsFunc(job, func(l string) bool {
		return dim[l] && !slices.Contains(pool, l)
	})
}

func referenceMatches(p *store.Pool, jobLabels []string) bool {
	return referenceLabelsMatch(store.NormalizeLabels(p.Labels), store.NormalizeLabels(jobLabels), !p.NoDefaultLabels)
}

func referenceEligible(p *store.Pool, j *store.Job) (bool, string) {
	switch {
	case p == nil || j == nil:
		return false, ""
	case !p.Enabled:
		return false, "the pool is disabled"
	case j.InstallationID == "":
		return false, "no GitHub App installation here covers that repository"
	case p.InstallationID != j.InstallationID:
		return false, "the pool belongs to another GitHub App installation"
	case !referenceMatches(p, j.Labels):
		return false, "the pool does not advertise those labels"
	}
	return true, ""
}

func referenceScore(p *store.Pool, jobLabels []string) int {
	pool := store.NormalizeLabels(p.Labels)
	job := store.NormalizeLabels(jobLabels)
	if !referenceLabelsMatch(pool, job, !p.NoDefaultLabels) {
		return noMatch
	}
	surplus := 0
	for _, l := range pool {
		if !slices.Contains(job, l) {
			surplus++
		}
	}
	return max(exactScore-surplus, 0)
}

func referenceBestPool(pools []*store.Pool, j *store.Job, targets map[string]string) (*store.Pool, string) {
	var best, elsewhere *store.Pool
	bestScore := noMatch
	for _, p := range pools {
		if p == nil {
			continue
		}
		if ok, _ := referenceEligible(p, j); !ok {
			if p.Enabled && referenceMatches(p, j.Labels) && (elsewhere == nil || lessPool(p, elsewhere)) {
				elsewhere = p
			}
			continue
		}
		s := referenceScore(p, j.Labels)
		if s < 0 {
			continue
		}
		if best == nil || s > bestScore || (s == bestScore && lessPoolFor(j, p, best)) {
			best, bestScore = p, s
		}
	}
	if best != nil || elsewhere == nil {
		return best, ""
	}
	if j.InstallationID == "" {
		return nil, fmt.Sprintf("its labels match pool %s, but no GitHub App installation here covers %s", elsewhere.Name, j.Repo)
	}
	return nil, fmt.Sprintf("its labels match pool %s, which belongs to installation %s",
		elsewhere.Name, installationName(elsewhere.InstallationID, targets))
}

// matchingCase is one random fleet and one random job to put to it. The labels
// are written the way an operator writes them, in any case, padded, repeated
// and in any order, so that normalising has something to do.
type matchingCase struct {
	pools   []*store.Pool
	job     *store.Job
	targets map[string]string
}

func randomMatchingCase(rng *rand.Rand) matchingCase {
	vocabulary := []string{
		"self-hosted", "linux", "windows", "macos", "x64", "arm", "arm64",
		"gpu", "cuda12", "docker", "big", "small", "team-a", "team-b",
	}
	spell := func(l string) string {
		switch rng.IntN(4) {
		case 0:
			return strings.ToUpper(l)
		case 1:
			return "  " + l + " "
		}
		return l
	}
	labels := func(most int) []string {
		out := make([]string, 0, most)
		for range rng.IntN(most + 1) {
			out = append(out, spell(vocabulary[rng.IntN(len(vocabulary))]))
		}
		rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		return out
	}

	installations := []string{"ins_a", "ins_b"}
	names := []string{"alpha", "beta", "gamma"} // few, so that ties on name happen
	c := matchingCase{targets: map[string]string{"ins_a": "acme", "ins_b": "other"}}
	for i := range rng.IntN(7) {
		if rng.IntN(12) == 0 {
			c.pools = append(c.pools, nil)
			continue
		}
		c.pools = append(c.pools, &store.Pool{
			ID: fmt.Sprintf("pool_%d", i), Name: names[rng.IntN(len(names))],
			InstallationID: installations[rng.IntN(len(installations))],
			Labels:         labels(5), Enabled: rng.IntN(10) != 0, NoDefaultLabels: rng.IntN(4) == 0,
		})
	}
	installation := ""
	if rng.IntN(8) != 0 {
		installation = installations[rng.IntN(len(installations))]
	}
	c.job = &store.Job{ID: "job_1", Repo: "acme/widgets", InstallationID: installation, Labels: labels(5)}
	return c
}

func TestPreparedMatchingAnswersAsEveryComparisonDoes(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261018, 1))
	claimed, unclaimed, nearMisses := 0, 0, 0
	for i := range 20000 {
		c := randomMatchingCase(rng)
		wantPool, wantWhy := referenceBestPool(c.pools, c.job, c.targets)

		gotPool, gotWhy := bestOf(c.pools, preparedLabels(c.pools), c.job, labelsOf(store.NormalizeLabels(c.job.Labels)), c.targets)
		if gotPool != wantPool || gotWhy != wantWhy {
			t.Fatalf("case %d: the prepared matching chose %s (%q), the reference %s (%q)\njob labels %q on installation %q\npools:\n%s",
				i, nameOf(gotPool), gotWhy, nameOf(wantPool), wantWhy, c.job.Labels, c.job.InstallationID, describePools(c.pools))
		}
		// The functions callers use keep the answer they always gave.
		if p, why := bestPool(c.pools, c.job, c.targets); p != wantPool || why != wantWhy {
			t.Fatalf("case %d: bestPool chose %s (%q), the reference %s (%q)", i, nameOf(p), why, nameOf(wantPool), wantWhy)
		}
		if p := BestPool(c.pools, c.job); p != wantPool {
			t.Fatalf("case %d: BestPool chose %s, the reference %s", i, nameOf(p), nameOf(wantPool))
		}
		for _, p := range c.pools {
			if p == nil {
				continue
			}
			gotOK, gotReason := Eligible(p, c.job)
			wantOK, wantReason := referenceEligible(p, c.job)
			if gotOK != wantOK || gotReason != wantReason {
				t.Fatalf("case %d: Eligible(%s) = %v, %q; the reference says %v, %q", i, p.Name, gotOK, gotReason, wantOK, wantReason)
			}
			if got, want := PoolMatches(p, c.job.Labels), referenceMatches(p, c.job.Labels); got != want {
				t.Fatalf("case %d: PoolMatches(%s) = %v, the reference %v", i, p.Name, got, want)
			}
			if got, want := Score(p.Labels, c.job.Labels), referenceScore(&store.Pool{Labels: p.Labels}, c.job.Labels); got != want {
				t.Fatalf("case %d: Score(%s) = %d, the reference %d", i, p.Name, got, want)
			}
		}

		switch {
		case wantPool != nil:
			claimed++
		case wantWhy != "":
			nearMisses++
		default:
			unclaimed++
		}
	}
	// A comparison that never saw a claim, a refusal and a near miss would have
	// proved nothing about the corners it was written for.
	if claimed < 1000 || unclaimed < 1000 || nearMisses < 100 {
		t.Fatalf("the generated cases claimed %d jobs, left %d unclaimed and explained %d near misses; want a spread of all three", claimed, unclaimed, nearMisses)
	}
}

// A job's labels are classified into a word, a bit for each label, and a job with
// more labels than the word has bits asks the map for the rest. Few jobs list
// sixty-eight labels, so the boundary needs a test of its own. Here linux is the
// last label the word holds, self-hosted the first it does not, x64 the next,
// and two explicit labels sort past them all.
func TestAJobWithManyLabelsIsMatchedAsEveryComparisonDoes(t *testing.T) {
	var early []string // 63 explicit labels that sort before linux
	for i := range 63 {
		early = append(early, fmt.Sprintf("a%02d", i))
	}
	late := []string{"y00", "z99"} // explicit labels that sort after x64
	explicit := append(slices.Clone(early), late...)
	every := append(slices.Clone(early), "linux", "self-hosted", "x64", "y00", "z99")
	job := queued("job_1", time.Minute, every...)
	job.Labels = store.NormalizeLabels(job.Labels)
	for label, at := range map[string]int{"linux": 63, "self-hosted": 64, "x64": 65, "y00": 66, "z99": 67} {
		if got := slices.Index(job.Labels, label); got != at {
			t.Fatalf("%s sorts at %d, want %d: the test means to straddle the 64 labels a word can classify", label, got, at)
		}
	}

	pool := func(name string, noDefaults bool, labels ...string) *store.Pool {
		p := testPool(name, labels...)
		p.NoDefaultLabels = noDefaults
		return p
	}
	without := func(labels []string, drop string) []string {
		return slices.DeleteFunc(slices.Clone(labels), func(l string) bool { return l == drop })
	}
	cases := []struct {
		name string
		pool *store.Pool
		want bool
	}{
		{"has every explicit label, and the implicit ones are not asked of it", pool("explicit", false, explicit...), true},
		{"lacks the last label, which sorts past the word", pool("missing-last", false, without(explicit, "z99")...), false},
		{"lacks an explicit label inside the word", pool("missing-first", false, without(explicit, "a00")...), false},
		{"lists exactly what the job asks, implicit labels included", pool("exact", true, every...), true},
		{"lacks linux, the last label in the word, and is asked for it", pool("no-linux", true, without(every, "linux")...), false},
		{"lacks self-hosted, the first label past the word, and is asked for it", pool("no-self-hosted", true, without(every, "self-hosted")...), false},
		{"lacks x64 and is asked for it", pool("no-x64", true, without(every, "x64")...), false},
		{"declares arm64 and the job asked for x64", pool("arm", false, append(slices.Clone(explicit), "arm64")...), false},
	}
	for _, c := range cases {
		if got := PoolMatches(c.pool, job.Labels); got != c.want {
			t.Errorf("%s: PoolMatches = %v, want %v", c.name, got, c.want)
		}
		if got := referenceMatches(c.pool, job.Labels); got != c.want {
			t.Errorf("%s: the reference says %v, want %v; the case is wrong", c.name, got, c.want)
		}
	}
	pools := make([]*store.Pool, len(cases))
	for i, c := range cases {
		pools[i] = c.pool
	}
	gotPool, gotWhy := bestPool(pools, job, nil)
	wantPool, wantWhy := referenceBestPool(pools, job, nil)
	if gotPool != wantPool || gotWhy != wantWhy {
		t.Errorf("bestPool chose %s (%q), the reference %s (%q)", nameOf(gotPool), gotWhy, nameOf(wantPool), wantWhy)
	}
}

// The controller asks about one job at a time, in loops of its own, and the
// labels it holds are the ones the store keeps, already in order. Getting the
// pools ready is worth doing for a pass that sets many jobs against them, but a
// single question that did it too would build a slice as long as the fleet at
// every call, for a comparison that has nothing to normalise.
func TestChoosingAPoolForOneJobAllocatesNothing(t *testing.T) {
	var pools []*store.Pool
	for _, label := range []string{"small", "big", "gpu"} {
		pools = append(pools, testPool(label, "self-hosted", label))
	}
	job := queued("job_1", time.Minute, "self-hosted", "big")
	job.Labels = store.NormalizeLabels(job.Labels)
	if p := BestPool(pools, job); p == nil || p.Name != "big" {
		t.Fatalf("the fleet was meant to give the job to pool big, got %s", nameOf(p))
	}

	if n := testing.AllocsPerRun(100, func() { BestPool(pools, job) }); n != 0 {
		t.Errorf("BestPool allocated %v times to choose among %d pools for one job whose labels are in order", n, len(pools))
	}
}

func nameOf(p *store.Pool) string {
	if p == nil {
		return "no pool"
	}
	return p.ID + "/" + p.Name
}

func describePools(pools []*store.Pool) string {
	var b strings.Builder
	for _, p := range pools {
		if p == nil {
			b.WriteString("  nil\n")
			continue
		}
		fmt.Fprintf(&b, "  %s enabled=%v installation=%s noDefaultLabels=%v labels=%q\n", nameOf(p), p.Enabled, p.InstallationID, p.NoDefaultLabels, p.Labels)
	}
	return b.String()
}
