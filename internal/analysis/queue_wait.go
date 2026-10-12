package analysis

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
)

const QueueWaitVersion = "queue-wait-v1"

// MaxEvidencePools and MaxLongestPerPool bound the evidence, so a finding
// over a large fleet still fits a reader's window; the totals keep the rest.
const (
	MaxEvidencePools  = 50
	MaxLongestPerPool = 5
)

// WaitCause is what a stretch of a job's wait is put down to. Each second of
// a wait has exactly one, so the causes add up to the wait.
type WaitCause string

const (
	CauseHeldForReview WaitCause = "held_for_review"
	CauseUnmatched     WaitCause = "unmatched"
	CausePoolCeiling   WaitCause = "pool_ceiling"
	CauseThrottled     WaitCause = "throttled"
	CauseNoRoom        WaitCause = "no_room"
	CauseRunnerStartup WaitCause = "runner_startup"
	CauseGitHubDelay   WaitCause = "github_delay"
	// CauseUnattributed is time the fleet keeps no record to explain, such
	// as a pool with no eligible host. Naming a cause for it would be a guess.
	CauseUnattributed WaitCause = "unattributed"
)

// waitCauses is the order ties are broken in, so the same snapshot always
// reads the same way.
var waitCauses = []WaitCause{CauseHeldForReview, CauseUnmatched, CausePoolCeiling, CauseThrottled,
	CauseNoRoom, CauseRunnerStartup, CauseGitHubDelay, CauseUnattributed}

// WaitJob is one job's timeline. Repo, Workflow and JobName are written by a
// workflow, so whoever shows them to a model treats them as untrusted.
type WaitJob struct {
	ID, PoolID, HostID, Repo, Workflow, JobName string
	QueuedAt                                    time.Time
	HeldAt, ApprovedAt, EligibleAt              *time.Time
	// FirstStartFailureAt is the first runner for this job that failed to
	// start; the wait after it is that failure's.
	FirstStartFailureAt                   *time.Time
	RunnerRequestedAt, RunnerRegisteredAt *time.Time
	// StartedAt is nil while the job is still waiting at Until.
	StartedAt *time.Time
}

// PoolConditions is what held a pool back over the period.
type PoolConditions struct {
	PoolID, PoolName string
	Deleted          bool
	// AtCeiling is when the pool's live runners were at or above its current
	// max_runners.
	AtCeiling []Interval
	// NoRoom holds the minutes recorded blocked at capacity, each covering
	// [m, m+1m).
	NoRoom []time.Time
	// Throttled is when a host that fits the pool was on a throttle rung. A
	// zero To is an episode still open at Until.
	Throttled []Interval
}

type QueueWaitSnapshot struct {
	Since, Until time.Time
	Jobs         []WaitJob
	Pools        []PoolConditions
	// JobsTruncated and RunnersTruncated say a read stopped at MaxReadJobs. A
	// cut runner read thins AtCeiling as surely as a cut job read thins Jobs.
	JobsTruncated    bool
	RunnersTruncated bool
}

type CauseShare struct {
	Cause   WaitCause `json:"cause"`
	Seconds int64     `json:"seconds"`
	// Share is the fraction of the wait, to two places.
	Share float64 `json:"share"`
}

// LongWait names one job. Repo, Workflow and JobName are workflow-written
// and untrusted.
type LongWait struct {
	JobID       string    `json:"job_id"`
	Repo        string    `json:"repo"`
	Workflow    string    `json:"workflow"`
	JobName     string    `json:"job_name"`
	WaitSeconds int64     `json:"wait_seconds"`
	Cause       WaitCause `json:"cause"`
}

type PoolWait struct {
	PoolID      string       `json:"pool_id"`
	PoolName    string       `json:"pool_name"`
	Deleted     bool         `json:"deleted"`
	Jobs        int          `json:"jobs"`
	WaitSeconds int64        `json:"wait_seconds"`
	Causes      []CauseShare `json:"causes"`
	// Dominant is "" for a pool whose jobs did not wait at all.
	Dominant WaitCause  `json:"dominant"`
	Longest  []LongWait `json:"longest"`
}

type QueueWaitEvidence struct {
	Fleet []CauseShare `json:"fleet"`
	// Pools is ordered by WaitSeconds, longest first, and cut to
	// MaxEvidencePools; PoolsCut says how many were left out. Fleet, Jobs
	// and WaitSeconds still count every pool.
	Pools       []PoolWait `json:"pools"`
	Jobs        int        `json:"jobs"`
	WaitSeconds int64      `json:"wait_seconds"`
	// NoRunnerRecord counts jobs that started after waiting past
	// eligibility with no runner request on record (its runner pruned), so
	// that wait could not be split into startup and GitHub's delay.
	NoRunnerRecord   int  `json:"no_runner_record"`
	JobsTruncated    bool `json:"jobs_truncated"`
	RunnersTruncated bool `json:"runners_truncated"`
	PoolsCut         int  `json:"pools_cut"`
}

type QueueWaitFinding = Finding[QueueWaitEvidence]

var queueWaitCodes = []string{"jobs.unmatched", "host.throttled", "pool.no_capacity",
	"pool.queue_wait_high", "pool.runners_failing", "pool.no_eligible_host"}

// causeAdvice is how a cause reads in the summary and what its first step
// says. on is "on pool linux" (or "that no pool claimed"), pool is "pool
// linux" (or "a pool"), saved is what the cause cost there in words.
type causeAdvice struct {
	phrase  string
	setting string
	code    string
	text    func(on, pool, saved string) string
}

var causeAdvices = map[WaitCause]causeAdvice{
	CauseHeldForReview: {"held for a deployment review", "", "", func(on, _, saved string) string {
		return fmt.Sprintf("Jobs %s were held for a deployment review; that wait is GitHub's environment protection, not the fleet, and approving sooner would have saved up to %s.", on, saved)
	}},
	CauseUnmatched: {"waiting for a pool to claim them", "pool.labels", "jobs.unmatched", func(_, pool, saved string) string {
		return fmt.Sprintf("Make %s's labels match what the jobs ask for in runs-on; claiming them at once would have saved up to %s.", pool, saved)
	}},
	CausePoolCeiling: {"a pool at its maximum runners", "pool.max_runners", "", func(_, pool, saved string) string {
		return fmt.Sprintf("Raise the maximum runners of %s, which sat at its ceiling while jobs waited; room for more runners would have saved up to %s.", pool, saved)
	}},
	CauseThrottled: {"hosts throttled after sustained pressure", "host.capacity", "host.throttled", func(_, pool, saved string) string {
		return fmt.Sprintf("Look at the capacity of the hosts serving %s, which were throttled after sustained pressure; room on them would have saved up to %s.", pool, saved)
	}},
	CauseNoRoom: {"no host with room", "host.capacity", "pool.no_capacity", func(_, pool, saved string) string {
		return fmt.Sprintf("Add capacity for %s, on its hosts or with another host; room for its runners would have saved up to %s.", pool, saved)
	}},
	CauseRunnerStartup: {"runners starting", "pool.min_runners", "pool.queue_wait_high", func(_, pool, saved string) string {
		return fmt.Sprintf("Keep warm runners ready for %s with its minimum runners; runners ready in advance would have saved up to %s.", pool, saved)
	}},
	CauseGitHubDelay: {"GitHub handing jobs to runners that were ready", "", "", func(_, pool, saved string) string {
		return fmt.Sprintf("Runners for %s were registered and idle while GitHub handed them their jobs; that delay is GitHub's and no setting here changes it, though a faster hand-off would have saved up to %s.", pool, saved)
	}},
	CauseUnattributed: {"time the fleet records no cause for", "", "pool.no_eligible_host", func(on, pool, saved string) string {
		return fmt.Sprintf("The fleet records no cause for %s of the wait %s; a pool with no eligible host is one cause it does not record, so check that %s has a host that fits it.", saved, on, pool)
	}},
}

// A failed start sends the reader to the failures first: warm runners that
// also fail to start would not have saved anything.
func failingStartText(_, pool, saved string) string {
	return fmt.Sprintf("Runners for %s failed to start while jobs waited; fixing those starts, then keeping warm runners with its minimum runners, would have saved up to %s.", pool, saved)
}

const unattributedReason = "most of the wait has no cause the fleet records: a pool with no eligible host is one, and the fleet keeps it only as a current problem (pool.no_eligible_host), never minute by minute"

// segment is a stretch of a pool's period with one condition holding.
type segment struct {
	from, to time.Time
	cause    WaitCause
}

type poolTally struct {
	conditions   PoolConditions
	timeline     []segment
	jobs         int
	causes       map[WaitCause]time.Duration
	waits        []LongWait
	failedStarts bool
}

// QueueWait puts every second of every job's wait in the period down to one
// cause. It follows ZF-242's interval rule: the phases are disjoint, startup
// is carved out of the wait rather than added to it, and time no record
// explains stays unattributed.
func QueueWait(s QueueWaitSnapshot) (QueueWaitFinding, error) {
	f := QueueWaitFinding{Analysis: "queue_wait", Version: QueueWaitVersion, Since: s.Since, Until: s.Until,
		NextSteps: []NextStep{}, Codes: []string{}}
	if s.Since.IsZero() || !s.Since.Before(s.Until) {
		return f, errors.New("queue-wait snapshot needs a period with a start before its end")
	}
	tallies := map[string]*poolTally{}
	for _, p := range s.Pools {
		tallies[p.PoolID] = &poolTally{conditions: p, timeline: poolTimeline(p, s.Until)}
	}
	fleet := map[WaitCause]time.Duration{}
	e := QueueWaitEvidence{Jobs: len(s.Jobs), JobsTruncated: s.JobsTruncated, RunnersTruncated: s.RunnersTruncated}
	for _, j := range s.Jobs {
		t := tallies[j.PoolID]
		if t == nil {
			t = &poolTally{conditions: PoolConditions{PoolID: j.PoolID}}
			tallies[j.PoolID] = t
		}
		if t.causes == nil {
			t.causes = map[WaitCause]time.Duration{}
		}
		causes, noRecord := splitWait(j, s.Until, t.timeline)
		if noRecord {
			e.NoRunnerRecord++
		}
		var total time.Duration
		for c, d := range causes {
			t.causes[c] += d
			fleet[c] += d
			total += d
		}
		t.jobs++
		t.failedStarts = t.failedStarts || j.FirstStartFailureAt != nil
		if w := seconds(total); w > 0 {
			t.waits = append(t.waits, LongWait{JobID: j.ID, Repo: j.Repo, Workflow: j.Workflow, JobName: j.JobName,
				WaitSeconds: w, Cause: largest(causes)})
		}
	}

	var pools []PoolWait
	for _, t := range tallies {
		if t.jobs == 0 {
			continue
		}
		causes, total := shares(t.causes)
		slices.SortFunc(t.waits, func(a, b LongWait) int {
			return cmp.Or(cmp.Compare(b.WaitSeconds, a.WaitSeconds), strings.Compare(a.JobID, b.JobID))
		})
		pw := PoolWait{PoolID: t.conditions.PoolID, PoolName: t.conditions.PoolName, Deleted: t.conditions.Deleted,
			Jobs: t.jobs, WaitSeconds: total, Causes: causes, Longest: nonNil(t.waits[:min(len(t.waits), MaxLongestPerPool)])}
		if len(causes) > 0 {
			pw.Dominant = causes[0].Cause
		}
		pools = append(pools, pw)
	}
	slices.SortFunc(pools, func(a, b PoolWait) int {
		return cmp.Or(cmp.Compare(b.WaitSeconds, a.WaitSeconds), strings.Compare(a.PoolID, b.PoolID))
	})
	e.PoolsCut = max(0, len(pools)-MaxEvidencePools)
	e.Pools = nonNil(pools[:min(len(pools), MaxEvidencePools)])
	e.Fleet, e.WaitSeconds = shares(fleet)
	f.Evidence = e

	total := time.Duration(e.WaitSeconds) * time.Second
	switch {
	case e.Jobs == 0:
		f.Confidence, f.ConfidenceReason = Low, "no jobs were queued in this period"
		f.Summary = "No jobs were queued in this period."
		return f, nil
	case e.Jobs < 5 || e.WaitSeconds < 60:
		little := fmt.Sprintf("%s waited, %s in all: too little to break down", count(e.Jobs, "job"), Words(total))
		f.Confidence, f.ConfidenceReason = Low, "only "+little
		f.Summary = "Only " + little + "."
		return f, nil
	}

	top := e.Fleet[0]
	f.Dominant = string(top.Cause)
	rawShare := float64(top.Seconds) / float64(e.WaitSeconds)
	where := ""
	if t := worstPool(tallies, top.Cause); t != nil && t.conditions.PoolID != "" {
		where = ", most of it on " + poolLabel(t.conditions)
	}
	f.Summary = fmt.Sprintf("Jobs waited %s in all; %d%% of it was %s%s.", Words(total),
		int(math.Round(rawShare*100)), causeAdvices[top.Cause].phrase, where)

	if top.Cause == CauseUnattributed {
		f.Confidence, f.ConfidenceReason = Low, unattributedReason
	} else {
		var reasons []string
		if s.JobsTruncated {
			reasons = append(reasons, fmt.Sprintf("the period held more than %d jobs; the newest %d were read, so narrow since", MaxReadJobs, MaxReadJobs))
		}
		if s.RunnersTruncated {
			reasons = append(reasons, fmt.Sprintf("the period held more than %d runners; the newest %d were read, so narrow since", MaxReadJobs, MaxReadJobs))
		}
		if e.Jobs < 10 {
			reasons = append(reasons, fmt.Sprintf("only %s waited: too few to call one cause dominant", count(e.Jobs, "job")))
		}
		if rawShare < 0.5 {
			reasons = append(reasons, fmt.Sprintf("the largest cause, %s, took %d%% of the wait, not half",
				causeAdvices[top.Cause].phrase, int(math.Round(rawShare*100))))
		}
		if e.NoRunnerRecord*10 > e.Jobs {
			reasons = append(reasons, fmt.Sprintf("%d of %d jobs have no runner record, so their wait after eligibility could not be split into runner startup and GitHub's delay",
				e.NoRunnerRecord, e.Jobs))
		}
		f.Confidence = High
		if len(reasons) > 0 {
			f.Confidence, f.ConfidenceReason = Medium, strings.Join(reasons, "; ")
		}
	}

	// A step for each cause that took a tenth of the wait or more, the
	// largest first: the first is the one change that would have cut most.
	for _, c := range e.Fleet {
		if len(f.NextSteps) == 3 || float64(c.Seconds) < 0.1*float64(e.WaitSeconds) {
			break
		}
		f.NextSteps = append(f.NextSteps, waitStep(c.Cause, worstPool(tallies, c.Cause)))
	}
	for _, step := range f.NextSteps {
		if step.Code != "" && !slices.Contains(f.Codes, step.Code) {
			f.Codes = append(f.Codes, step.Code)
		}
	}
	return f, nil
}

// splitWait walks one job's wait phase by phase. Every stamp is clamped into
// the wait and no phase starts before the previous one ended, so stamps out
// of order (at-least-once deliveries, two clocks) give an empty phase rather
// than a negative one, and the phases always meet end to end.
func splitWait(j WaitJob, until time.Time, timeline []segment) (map[WaitCause]time.Duration, bool) {
	causes := map[WaitCause]time.Duration{}
	start, end := j.QueuedAt, until
	if j.StartedAt != nil && j.StartedAt.Before(until) {
		end = *j.StartedAt
	}
	if !end.After(start) {
		return causes, false
	}
	add := func(c WaitCause, d time.Duration) {
		if d > 0 {
			causes[c] += d
		}
	}
	// stamp is t clamped into the wait, or otherwise when there is none.
	stamp := func(t *time.Time, otherwise time.Time) time.Time {
		if t == nil {
			return otherwise
		}
		return later(start, earlier(end, *t))
	}
	cursor := start
	phase := func(to time.Time, c WaitCause) {
		to = later(cursor, to)
		add(c, to.Sub(cursor))
		cursor = to
	}
	if j.HeldAt != nil {
		phase(stamp(j.ApprovedAt, end), CauseHeldForReview)
	}
	if j.EligibleAt != nil {
		phase(stamp(j.EligibleAt, end), CauseUnmatched)
	}
	if j.RunnerRequestedAt == nil {
		// With no runner on record the wait after eligibility cannot be told
		// apart into startup and GitHub's delay; only the pool's own
		// conditions can still be named.
		attribute(cursor, end, timeline, j.FirstStartFailureAt, add)
		return causes, j.StartedAt != nil && end.After(cursor)
	}
	// A runner requested or registered before the job was eligible was
	// already there, so its phase is empty.
	requested := later(cursor, stamp(j.RunnerRequestedAt, end))
	attribute(cursor, requested, timeline, j.FirstStartFailureAt, add)
	cursor = requested
	phase(stamp(j.RunnerRegisteredAt, end), CauseRunnerStartup)
	phase(end, CauseGitHubDelay)
	return causes, false
}

// attribute splits [a, b) at every boundary of the pool's conditions and the
// job's first failed start, giving each piece the first cause that holds:
// the ceiling, then a failed start, then a throttled or plain lack of room.
func attribute(a, b time.Time, timeline []segment, failedAt *time.Time, add func(WaitCause, time.Duration)) {
	piece := func(from, to time.Time, c WaitCause) {
		if c == CausePoolCeiling || failedAt == nil || !failedAt.Before(to) {
			add(c, to.Sub(from))
			return
		}
		cut := later(from, *failedAt)
		add(c, cut.Sub(from))
		add(CauseRunnerStartup, to.Sub(cut))
	}
	i := sort.Search(len(timeline), func(i int) bool { return timeline[i].to.After(a) })
	for x := a; x.Before(b); {
		if i < len(timeline) && !timeline[i].from.After(x) {
			to := earlier(timeline[i].to, b)
			piece(x, to, timeline[i].cause)
			x = to
			i++
			continue
		}
		to := b
		if i < len(timeline) && timeline[i].from.Before(b) {
			to = timeline[i].from
		}
		piece(x, to, CauseUnattributed)
		x = to
	}
}

// poolTimeline flattens a pool's conditions into ordered, disjoint segments,
// each carrying the condition that wins there. Reading a job's wait is then
// a walk along it rather than a search of every minute for every job.
func poolTimeline(p PoolConditions, until time.Time) []segment {
	var noRoom []Interval
	for _, m := range p.NoRoom {
		noRoom = append(noRoom, Interval{From: m, To: m.Add(time.Minute)})
	}
	throttled := make([]Interval, 0, len(p.Throttled))
	for _, iv := range p.Throttled {
		if iv.To.IsZero() {
			iv.To = until
		}
		throttled = append(throttled, iv)
	}
	ceiling, room, throttle := merge(p.AtCeiling), merge(noRoom), merge(throttled)
	var bounds []time.Time
	for _, list := range [][]Interval{ceiling, room, throttle} {
		for _, iv := range list {
			bounds = append(bounds, iv.From, iv.To)
		}
	}
	slices.SortFunc(bounds, func(a, b time.Time) int { return a.Compare(b) })
	bounds = slices.CompactFunc(bounds, func(a, b time.Time) bool { return a.Equal(b) })
	var out []segment
	for k := 0; k+1 < len(bounds); k++ {
		x := bounds[k]
		var c WaitCause
		switch {
		case covers(ceiling, x):
			c = CausePoolCeiling
		case covers(room, x) && covers(throttle, x):
			c = CauseThrottled
		case covers(room, x):
			c = CauseNoRoom
		default:
			continue
		}
		if n := len(out); n > 0 && out[n-1].cause == c && out[n-1].to.Equal(x) {
			out[n-1].to = bounds[k+1]
			continue
		}
		out = append(out, segment{from: x, to: bounds[k+1], cause: c})
	}
	return out
}

// merge sorts intervals and joins those that overlap or touch, dropping any
// that are empty or reversed.
func merge(in []Interval) []Interval {
	var out []Interval
	for _, iv := range in {
		if iv.To.After(iv.From) {
			out = append(out, iv)
		}
	}
	slices.SortFunc(out, func(a, b Interval) int { return a.From.Compare(b.From) })
	merged := out[:0]
	for _, iv := range out {
		if n := len(merged); n > 0 && !iv.From.After(merged[n-1].To) {
			merged[n-1].To = later(merged[n-1].To, iv.To)
			continue
		}
		merged = append(merged, iv)
	}
	return merged
}

// covers reports whether x falls in one of a merged list's intervals.
func covers(list []Interval, x time.Time) bool {
	i := sort.Search(len(list), func(i int) bool { return list[i].To.After(x) })
	return i < len(list) && !list[i].From.After(x)
}

// shares turns durations into whole seconds and fractions, largest first.
// The total is the sum of the rounded parts, so the rows always add up.
func shares(d map[WaitCause]time.Duration) ([]CauseShare, int64) {
	out := []CauseShare{}
	var total int64
	for _, c := range waitCauses {
		if s := seconds(d[c]); s > 0 {
			out = append(out, CauseShare{Cause: c, Seconds: s})
			total += s
		}
	}
	slices.SortStableFunc(out, func(a, b CauseShare) int { return cmp.Compare(b.Seconds, a.Seconds) })
	for i := range out {
		out[i].Share = math.Round(float64(out[i].Seconds)/float64(total)*100) / 100
	}
	return out, total
}

func largest(d map[WaitCause]time.Duration) WaitCause {
	var best WaitCause
	for _, c := range waitCauses {
		if d[c] > 0 && (best == "" || d[c] > d[best]) {
			best = c
		}
	}
	return best
}

// worstPool is the pool that lost the most time to a cause.
func worstPool(tallies map[string]*poolTally, c WaitCause) *poolTally {
	var best *poolTally
	for _, t := range tallies {
		if t.causes[c] <= 0 {
			continue
		}
		if best == nil || t.causes[c] > best.causes[c] ||
			(t.causes[c] == best.causes[c] && t.conditions.PoolID < best.conditions.PoolID) {
			best = t
		}
	}
	return best
}

func waitStep(c WaitCause, t *poolTally) NextStep {
	a := causeAdvices[c]
	on, pool, saved := "that no pool claimed", "a pool", "0s"
	if t != nil {
		saved = Words(t.causes[c])
		if t.conditions.PoolID != "" {
			pool = poolLabel(t.conditions)
			on = "on " + pool
		}
	}
	text := a.text
	code := a.code
	if c == CauseRunnerStartup && t != nil && t.failedStarts {
		text, code = failingStartText, "pool.runners_failing"
	}
	return NextStep{Text: text(on, pool, saved), Setting: a.setting, Code: code}
}

// poolLabel names a pool by its name, or by its id once it has none; a
// deleted pool's wait still counts and is said to be deleted.
func poolLabel(p PoolConditions) string {
	name := p.PoolName
	if name == "" {
		name = p.PoolID
	}
	if p.Deleted {
		return "pool " + name + " (deleted)"
	}
	return "pool " + name
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func seconds(d time.Duration) int64 { return int64(math.Round(d.Seconds())) }

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func earlier(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
