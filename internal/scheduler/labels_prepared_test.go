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

func referenceMatches(p *store.Pool, jobLabels []string) bool {
	return matches(store.NormalizeLabels(p.Labels), store.NormalizeLabels(jobLabels), !p.NoDefaultLabels)
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
	if !matches(pool, job, !p.NoDefaultLabels) {
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

		gotPool, gotWhy := bestOf(c.pools, preparedLabels(c.pools), c.job, store.NormalizeLabels(c.job.Labels), c.targets)
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
