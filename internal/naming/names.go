package naming

import (
	"strings"
)

// PoolName is the canonical name for a pool of the given shape.
//
// It carries no suffix: a pool *is* its shape, and two pools with the same
// shape are the same pool. That is also what makes the name usable in a
// workflow's runs-on, which is the name's main job.
func PoolName(spec Spec) string { return spec.WithSuffix("").String() }

// Labels are the labels a pool of this shape advertises.
//
// The canonical name comes first: it is the one a workflow author writes, and
// the only one that says how much machine they are asking for. The kernel and
// architecture labels follow because actions/runner advertises them anyway,
// and because a pool that declares them is a pool the scheduler will refuse to
// hand an x64 job to when it is arm64 -- which is the whole point of naming
// the architecture in the first place.
func Labels(spec Spec) []string {
	out := make([]string, 0, 3)
	if name := PoolName(spec); name != Prefix {
		out = append(out, name)
	}
	if k := Kernel(spec.OS); k != "" {
		out = append(out, k)
	}
	if a := ArchLabel(spec.Arch); a != "" {
		out = append(out, a)
	}
	return out
}

// Base is the branded form of a name that other names are built from: lowercase,
// hyphenated, and carrying the prefix exactly once.
//
// A pool called "gpu" and a pool called "zoomies-gpu" are the same pool as far
// as this grammar is concerned, and neither should produce "zoomies-zoomies-gpu".
func Base(name string) string {
	s := Slug(name)
	switch {
	case s == "":
		return Prefix
	case s == Prefix, strings.HasPrefix(s, Prefix+"-"):
		return s
	}
	return Prefix + "-" + s
}

// RunnerName is the name one runner registers under: what its pool is made of,
// then a discriminator that tells it from its siblings.
//
// GitHub shows this name in the runner list, in the job header, and in the "Set
// up job" step of every job's log -- the three places somebody looks when a job
// went somewhere surprising. "zoomies-4vcpu-ubuntu-2404-biscuit-a3f9qz2m" answers
// "how much machine, running what" there, without a page load; and because the
// shape is rendered by the same grammar as the pool's own name, Parse reads a
// runner's name back into the pool's Spec plus the discriminator.
//
// Two things must survive a name that will not fit. The prefix, because it is
// what marks a registration as this fleet's rather than one somebody made by
// hand -- store.IsRunnerName, and the reaper behind it, know a runner by no
// other sign. And the discriminator, because GitHub requires the name to be
// unique within the target and two runners that truncated to the same string
// would be one registration fighting itself. So the shape in the middle is what
// gives way, a whole segment at a time: a name cut mid-word ("...ubuntu-24")
// claims a platform that does not exist, where a name one segment shorter only
// says less than it could.
func RunnerName(pool, discriminator string) string {
	tail := Slug(discriminator)
	if room := MaxNameLength - len(Prefix) - 1; len(tail) > room {
		tail = strings.TrimRight(tail[:max(room, 0)], "-")
	}
	base := Base(pool)
	if tail == "" {
		return truncate(base)
	}
	parts := strings.Split(base, "-")
	for len(parts) > 1 && len(strings.Join(parts, "-"))+1+len(tail) > MaxNameLength {
		parts = parts[:len(parts)-1]
	}
	return truncate(strings.Join(parts, "-") + "-" + tail)
}

// MaxHostSegment is the most of a host's name a runner's name will carry. It is
// short on purpose: the runner's token must survive, and the shape in the middle
// is spent first, so a long host name would otherwise cost a runner the "how
// much machine" half of its name.
const MaxHostSegment = 20

// HostSegment is the part of a host's name a runner registered on it carries, so
// the name GitHub shows, in the runner list, the job header and the "Set up job"
// step of the log, says which machine ran the job. It is cut a whole word at a
// time for the reason RunnerName cuts the shape that way, and is empty when the
// name leaves nothing the grammar allows.
func HostSegment(machine string) string {
	s := Slug(machine)
	if len(s) <= MaxHostSegment {
		return s
	}
	parts := strings.Split(s, "-")
	for len(parts) > 1 && len(strings.Join(parts, "-")) > MaxHostSegment {
		parts = parts[:len(parts)-1]
	}
	return strings.TrimRight(truncateTo(strings.Join(parts, "-"), MaxHostSegment), "-")
}

func truncateTo(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
