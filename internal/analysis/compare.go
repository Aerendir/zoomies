package analysis

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

const CompareVersion = "compare-v1"

// MinCompareJobs is the fewest jobs a side needs before the comparison ranks
// anything: under it, a release that ran three slow jobs would be blamed for
// them. ConfidentCompareJobs is what a side needs for a change to be called
// with high confidence.
const (
	MinCompareJobs       = 30
	ConfidentCompareJobs = 100
)

// Spread is a measure's percentiles over a side's jobs. It mirrors
// store.JobPercentiles, field for field and name for name, so a figure in a
// finding and on the Usage page's table read the same.
type Spread struct {
	Samples int    `json:"samples"`
	P50MS   *int64 `json:"p50_ms"`
	P95MS   *int64 `json:"p95_ms"`
}

// JobFigures is what one side's jobs did. The caller leaves out the
// conclusions it names in CompareSnapshot.Excluded before counting, which is
// why there is no cancelled count: it would always read 0, and a 0 here
// would look like a measurement of none cancelled.
type JobFigures struct {
	Jobs        int    `json:"jobs"`
	Succeeded   int    `json:"succeeded"`
	Failed      int    `json:"failed"`
	FleetFailed int    `json:"fleet_failed"`
	OOMKilled   int    `json:"oom_killed"`
	Duration    Spread `json:"duration"`
	QueueWait   Spread `json:"queue_wait"`
	Startup     Spread `json:"startup"`
}

type CompareSide struct {
	// Label is a release such as v1.3.3, or a period such as "the 24 hours
	// before". A period's label reads after "in", so it is a noun phrase.
	Label string    `json:"label"`
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
	// Figures on a CountsOnly side hold Jobs and FleetFailed and nothing
	// else: the roll-up it was read from keeps no outcomes, no OOM kills and
	// no times, and a zero there is not a measurement.
	Figures    JobFigures `json:"figures"`
	CountsOnly bool       `json:"counts_only"`
	// JobsGoneBefore is set on a release side reaching past retention.jobs:
	// no roll-up records which release ran a job, so the jobs before it are
	// simply gone. A period side never has it: past retention.jobs it is
	// read from the roll-up instead (CountsOnly), so Compare refuses it.
	JobsGoneBefore *time.Time `json:"jobs_gone_before"`
	// MeanSlots is the fleet's slots on offer, averaged over the side's
	// sampled minutes; nil when no sample covers the side.
	MeanSlots *float64 `json:"mean_slots"`
	// NoRecordBefore is set on a CountsOnly side whose counts begin later
	// than the period asked for: the fleet keeps no counts before it (its
	// first record, or where the roll-up began). With Since not before Until
	// the side could not be read at all, which is not the same as no jobs
	// having run: where the oldest rows were pruned before the roll-up first
	// ran, jobs did run, and nobody can now say how many.
	NoRecordBefore *time.Time `json:"no_record_before"`
	// AskedUntil is where the period asked for ends. A CountsOnly side can
	// end before it, because the roll-up reads whole UTC days: the hours
	// from Until to AskedUntil are then the other side's or neither side's,
	// and only this says so once Until has moved. Zero when not known.
	AskedUntil time.Time `json:"asked_until"`
}

// unread is a side the fleet keeps no counts for at all.
func (s CompareSide) unread() bool { return s.NoRecordBefore != nil && !s.Since.Before(s.Until) }

type CompareSnapshot struct {
	// By is "version" or "period".
	By   string
	A, B CompareSide
	// Excluded names the conclusions the read left out (cancelled and
	// skipped). The analysis cannot see what it was not given, so it says
	// what its caller says it left out, and no more.
	Excluded []string
}

// A change's direction. not_enough is a measure the sample cannot judge,
// which is not the same as one that held still.
const (
	Regressed = "regressed"
	Improved  = "improved"
	Unchanged = "unchanged"
	NotEnough = "not_enough"
)

type Change struct {
	Measure string `json:"measure"`
	// Unit is "ms" for a time, "rate" for a fraction of the side's jobs.
	Unit      string   `json:"unit"`
	Direction string   `json:"direction"`
	From      *float64 `json:"from"`
	To        *float64 `json:"to"`
	// Ratio is how far the measure moved against its bar, whichever way: the
	// larger time over the smaller, or a rate's difference over five points.
	// It is nil when the measure could not be judged, and for a time that
	// moved from nothing, which has no finite ratio.
	Ratio *float64 `json:"ratio"`
}

type CompareEvidence struct {
	By string      `json:"by"`
	A  CompareSide `json:"a"`
	B  CompareSide `json:"b"`
	// Changes holds one row for every measure, in a fixed order, so a
	// reader finds a measure by name whether or not it moved.
	Changes                []Change `json:"changes"`
	Excluded               []string `json:"excluded"`
	PercentilesUnavailable bool     `json:"percentiles_unavailable"`
}

type CompareFinding = Finding[CompareEvidence]

var compareCodes = []string{"jobs.duration_regressed", "pool.queue_wait_high", "jobs.runner_lost",
	"jobs.workflow_failing", "jobs.oom_killed"}

// A rate moved when it differs by five points and the worse side has three
// events: one failure in thirty jobs is three points of noise, not a trend.
const (
	rateStep      = 0.05
	rateParts     = 20 // five points is one part in twenty
	minRateEvents = 3
)

type compareMeasure struct {
	name, words string
	family      string
	// spread and p95 pick a time measure; floor is the least it must move,
	// as well as a quarter, to count.
	spread func(JobFigures) Spread
	p95    bool
	floor  time.Duration
	// events picks a rate measure's count of jobs.
	events func(JobFigures) int
	// fromRollUp is a measure the usage roll-up also keeps, so a counts-only
	// side can still be compared on it.
	fromRollUp bool
}

var compareMeasures = []compareMeasure{
	{name: "duration_p50", words: "duration p50", family: "duration", spread: func(f JobFigures) Spread { return f.Duration }, floor: 30 * time.Second},
	{name: "duration_p95", words: "duration p95", family: "duration", spread: func(f JobFigures) Spread { return f.Duration }, p95: true, floor: 30 * time.Second},
	{name: "queue_wait_p50", words: "queue wait p50", family: "queue_wait", spread: func(f JobFigures) Spread { return f.QueueWait }, floor: 10 * time.Second},
	{name: "queue_wait_p95", words: "queue wait p95", family: "queue_wait", spread: func(f JobFigures) Spread { return f.QueueWait }, p95: true, floor: 10 * time.Second},
	{name: "startup_p50", words: "startup p50", family: "startup", spread: func(f JobFigures) Spread { return f.Startup }, floor: 10 * time.Second},
	{name: "startup_p95", words: "startup p95", family: "startup", spread: func(f JobFigures) Spread { return f.Startup }, p95: true, floor: 10 * time.Second},
	{name: "fleet_failure_rate", words: "fleet failure rate", family: "fleet_failure", events: func(f JobFigures) int { return f.FleetFailed }, fromRollUp: true},
	{name: "failure_rate", words: "failure rate", family: "failure", events: func(f JobFigures) int { return f.Failed }},
	{name: "oom_rate", words: "OOM kill rate", family: "oom", events: func(f JobFigures) int { return f.OOMKilled }},
}

// regressionSteps say what to read for each family of measure that got
// worse. under is "under v1.3.3" or "in the last 24 hours", moved is
// "duration p95 5m to 9m".
var regressionSteps = map[string]func(under, moved string) NextStep{
	"duration": func(under, moved string) NextStep {
		return NextStep{Code: "jobs.duration_regressed", Text: fmt.Sprintf(
			"Jobs ran longer %s (%s); group the slower jobs' runs by host to tell a slower workflow from a slower machine.", under, moved)}
	},
	"queue_wait": func(under, moved string) NextStep {
		return NextStep{Code: "pool.queue_wait_high", Text: fmt.Sprintf(
			"Jobs waited longer to start %s (%s); the queue-wait breakdown over the same period says which cause grew.", under, moved)}
	},
	"startup": func(under, moved string) NextStep {
		return NextStep{Code: "pool.queue_wait_high", Setting: "pool.min_runners", Text: fmt.Sprintf(
			"Runners took longer to start %s (%s); warm runners kept by a pool's minimum runners hide startup from its jobs.", under, moved)}
	},
	"fleet_failure": func(under, moved string) NextStep {
		return NextStep{Code: "jobs.runner_lost", Text: fmt.Sprintf(
			"The fleet lost more jobs %s (%s); the fault kinds of those jobs say whether their runners never started or stopped under them.", under, moved)}
	},
	"failure": func(under, moved string) NextStep {
		return NextStep{Code: "jobs.workflow_failing", Text: fmt.Sprintf(
			"More jobs failed %s (%s); a failure the fleet did not cause is the workflow's own, and the step each job failed at says what to fix.", under, moved)}
	},
	"oom": func(under, moved string) NextStep {
		return NextStep{Code: "jobs.oom_killed", Setting: "pool.resources.memory_mb", Text: fmt.Sprintf(
			"More jobs were killed for their memory limit %s (%s); the memory their pool gives them is what to raise.", under, moved)}
	},
}

// judged is one measure's change with what the sentences need: key ranks it
// unrounded (infinite for a time that moved from nothing), and from and to
// are its figures in words.
type judged struct {
	measure  compareMeasure
	change   Change
	key      float64
	from, to string
}

// Compare says what got worse and what got better between two releases or
// two periods, and refuses to rank either when a side is too small to say.
// A is the earlier, B the one being judged against it.
func Compare(s CompareSnapshot) (CompareFinding, error) {
	f := CompareFinding{Analysis: "compare", Version: CompareVersion, NextSteps: []NextStep{}, Codes: []string{}}
	if s.By != "version" && s.By != "period" {
		return f, fmt.Errorf("compare snapshot is by %q; it is by version or by period", s.By)
	}
	for _, side := range []CompareSide{s.A, s.B} {
		if side.Label == "" {
			return f, errors.New("compare snapshot has a side with no label, which no sentence could name")
		}
		if !side.Since.IsZero() && !side.Until.IsZero() && side.Until.Before(side.Since) {
			return f, fmt.Errorf("compare side %s ends before it starts", side.Label)
		}
		if s.By == "period" && side.JobsGoneBefore != nil {
			return f, fmt.Errorf("compare side %s is a period cut short by retention.jobs; a period past it is read from the roll-up", side.Label)
		}
	}
	f.Since, f.Until = span(s.A, s.B)
	w := wording{by: s.By, a: s.A.Label, b: s.B.Label}
	na, nb := s.A.Figures.Jobs, s.B.Figures.Jobs
	small := na < MinCompareJobs || nb < MinCompareJobs

	var all []judged
	e := CompareEvidence{By: s.By, A: s.A, B: s.B, Excluded: nonNil(slices.Clone(s.Excluded)),
		PercentilesUnavailable: s.A.CountsOnly || s.B.CountsOnly}
	for _, m := range compareMeasures {
		j := judge(m, s.A, s.B, small)
		all = append(all, j)
		e.Changes = append(e.Changes, j.change)
	}
	e.Changes = nonNil(e.Changes)
	f.Evidence = e

	switch {
	case s.A.unread() && s.B.unread():
		f.Summary = fmt.Sprintf("The fleet keeps no counts from before %s, so neither %s nor %s could be read, and there is nothing to compare%s.",
			stamp(*s.B.NoRecordBefore), w.a, w.b, w.cutOff(s, s.B, true))
		f.ConfidenceReason = fmt.Sprintf("the fleet keeps no counts from before %s, so neither side could be read", stamp(*s.B.NoRecordBefore))
	case s.A.unread() || s.B.unread():
		gone, other, later := s.A, w.b, false
		if s.B.unread() {
			gone, other, later = s.B, w.a, true
		}
		f.ConfidenceReason = fmt.Sprintf("the fleet keeps no counts from before %s, so %s could not be read", stamp(*gone.NoRecordBefore), gone.Label)
		f.Summary = fmt.Sprintf("The fleet keeps no counts from before %s, so %s could not be read, and there is nothing to compare %s with%s.",
			stamp(*gone.NoRecordBefore), gone.Label, other, w.cutOff(s, gone, later))
	case na == 0 && nb == 0:
		f.Summary, f.ConfidenceReason = w.neitherRan(), "neither side ran a job"
	case na == 0:
		f.Summary, f.ConfidenceReason = w.oneRanNothing(w.a, w.b), w.ranNothing(w.a)
	case nb == 0:
		f.Summary, f.ConfidenceReason = w.oneRanNothing(w.b, w.a), w.ranNothing(w.b)
	case small:
		// Name the short side: a release hours old beside one that ran
		// hundreds of jobs is the commonest case, and "either side" would be
		// false of the one that ran hundreds.
		where := "on either side"
		if na >= MinCompareJobs {
			where = w.under(w.b)
		} else if nb >= MinCompareJobs {
			where = w.under(w.a)
		}
		ran := w.ran(na, nb) + fmt.Sprintf(": fewer than %d %s", MinCompareJobs, where)
		f.Summary, f.ConfidenceReason = ran+", so the two are not ranked.", ran
	}
	if small {
		f.Confidence = Low
		for _, c := range w.coverage(s) {
			f.ConfidenceReason += "; also, " + c
		}
		f.NextSteps = append(f.NextSteps, NextStep{Setting: "limits.job_stats_window", Text: fmt.Sprintf(
			"Compare again once each side holds at least %d jobs, or over a longer period, up to limits.job_stats_window.", MinCompareJobs)})
		return f, nil
	}

	regressed, improved := moved(all, Regressed), moved(all, Improved)
	switch {
	case len(regressed) > 0:
		f.Dominant, f.Summary = regressed[0].measure.name, w.lead(regressed, true)
	case len(improved) > 0:
		f.Dominant, f.Summary = improved[0].measure.name, w.lead(improved, false)
	default:
		f.Summary = w.alike(all)
	}
	f.Confidence = High
	if reasons := w.doubts(s); len(reasons) > 0 {
		// A retention reason carries a colon and commas of its own, so a bare
		// separator would hide where the next reason begins.
		f.Confidence, f.ConfidenceReason = Medium, strings.Join(reasons, "; also, ")
	}

	// One step for each family that got worse, the furthest first; then,
	// for a release, the one setting that keeps the fleet from taking the
	// next one while this is looked into.
	families := map[string]bool{}
	for _, j := range regressed {
		if families[j.measure.family] {
			continue
		}
		families[j.measure.family] = true
		f.NextSteps = append(f.NextSteps, regressionSteps[j.measure.family](w.under(w.b), j.measure.words+" "+j.from+" to "+j.to))
	}
	if len(regressed) > 0 && s.By == "version" {
		f.NextSteps = append(f.NextSteps, NextStep{Setting: "updates.mode", Text: "Hold automatic updates while the regression is looked into."})
	}
	for _, step := range f.NextSteps {
		if step.Code != "" && !slices.Contains(f.Codes, step.Code) {
			f.Codes = append(f.Codes, step.Code)
		}
	}
	return f, nil
}

// judge decides one measure. Every figure higher is worse, so the direction
// is the sign of B against A.
func judge(m compareMeasure, a, b CompareSide, small bool) judged {
	j := judged{measure: m, change: Change{Measure: m.name, Unit: "ms", Direction: NotEnough}}
	// A counts-only side's zeros are not measurements: compared, a failure
	// rate read from the roll-up would always look like an improvement.
	usable := !small && (m.fromRollUp || (!a.CountsOnly && !b.CountsOnly))
	if m.events == nil {
		va, sa := timeFigure(m, a)
		vb, sb := timeFigure(m, b)
		j.change.From, j.change.To = msFloat(va), msFloat(vb)
		if !usable || va == nil || vb == nil || sa < MinCompareJobs || sb < MinCompareJobs {
			return j
		}
		lo, hi := min(*va, *vb), max(*va, *vb)
		j.from, j.to = Words(time.Duration(*va)*time.Millisecond), Words(time.Duration(*vb)*time.Millisecond)
		switch {
		case lo > 0:
			j.key = float64(hi) / float64(lo)
			j.change.Ratio = round(j.key, 100)
		case hi > 0:
			j.key = math.Inf(1)
		default:
			j.key = 1
			j.change.Ratio = round(1, 1)
		}
		// Integer arithmetic, so a move of exactly a quarter is one.
		if hi*100 >= lo*125 && hi-lo >= m.floor.Milliseconds() {
			j.change.Direction = direction(*va, *vb)
		} else {
			j.change.Direction = Unchanged
		}
		return j
	}

	j.change.Unit = "rate"
	ea, eb := m.events(a.Figures), m.events(b.Figures)
	na, nb := int64(a.Figures.Jobs), int64(b.Figures.Jobs)
	if na > 0 && (m.fromRollUp || !a.CountsOnly) {
		j.change.From = round(float64(ea)/float64(na), 10000)
	}
	if nb > 0 && (m.fromRollUp || !b.CountsOnly) {
		j.change.To = round(float64(eb)/float64(nb), 10000)
	}
	if !usable || na == 0 || nb == 0 {
		return j
	}
	j.from, j.to = percent(ea, na), percent(eb, nb)
	// The rates' difference times na*nb, kept in integers so a move of
	// exactly five points is one, which a float division would not promise.
	cross := int64(eb)*na - int64(ea)*nb
	j.key = float64(abs(cross)) / (rateStep * float64(na*nb))
	j.change.Ratio = round(j.key, 100)
	worse := ea
	if cross > 0 {
		worse = eb
	}
	if cross != 0 && abs(cross)*rateParts >= na*nb && worse >= minRateEvents {
		j.change.Direction = direction(int64(ea)*nb, int64(eb)*na)
	} else {
		j.change.Direction = Unchanged
	}
	return j
}

func timeFigure(m compareMeasure, s CompareSide) (*int64, int) {
	if s.CountsOnly {
		return nil, 0
	}
	sp := m.spread(s.Figures)
	if m.p95 {
		return sp.P95MS, sp.Samples
	}
	return sp.P50MS, sp.Samples
}

func direction(from, to int64) string {
	if to > from {
		return Regressed
	}
	return Improved
}

// moved is the changes in one direction, furthest first; a tie keeps the
// measures' own order, so the same snapshot always leads the same way.
func moved(all []judged, dir string) []judged {
	var out []judged
	for _, j := range all {
		if j.change.Direction == dir {
			out = append(out, j)
		}
	}
	slices.SortStableFunc(out, func(x, y judged) int { return cmp.Compare(y.key, x.key) })
	return out
}

// span is the time both sides cover. A side that ran nothing may carry no
// times at all, and a zero time is not a start.
func span(a, b CompareSide) (time.Time, time.Time) {
	var since, until time.Time
	for _, s := range []CompareSide{a, b} {
		if !s.Since.IsZero() && (since.IsZero() || s.Since.Before(since)) {
			since = s.Since
		}
		if s.Until.After(until) {
			until = s.Until
		}
	}
	return since, until
}

// wording is how a comparison reads. A release is the subject of its
// sentence ("v1.3.3 is worse than v1.3.2"); a period is where jobs ran
// ("jobs did worse in the last 24 hours"), because "the last 24 hours is
// worse" is not English.
type wording struct{ by, a, b string }

func (w wording) period() bool { return w.by == "period" }

// under is the phrase that places something on one side.
func (w wording) under(label string) string {
	if w.period() {
		return "in " + label
	}
	return "under " + label
}

func (w wording) ran(na, nb int) string {
	if w.period() {
		return fmt.Sprintf("%s ran in %s and %d in %s", count(na, "job"), w.a, nb, w.b)
	}
	return fmt.Sprintf("%s ran %s and %s ran %d in this period", w.a, count(na, "job"), w.b, nb)
}

func (w wording) ranNothing(label string) string {
	if w.period() {
		return "no jobs ran in " + label
	}
	return label + " ran no jobs in this period"
}

func (w wording) oneRanNothing(empty, other string) string {
	if w.period() {
		return fmt.Sprintf("No jobs ran in %s, so there is nothing to compare %s with.", empty, other)
	}
	return fmt.Sprintf("%s ran no jobs in this period, so there is nothing to compare %s with.", empty, other)
}

func (w wording) neitherRan() string {
	if w.period() {
		return fmt.Sprintf("No jobs ran in either %s or %s, so there is nothing to compare.", w.a, w.b)
	}
	return fmt.Sprintf("Neither %s nor %s ran a job in this period, so there is nothing to compare.", w.a, w.b)
}

func (w wording) lead(list []judged, worse bool) string {
	top := list[0]
	more := ""
	if len(list) > 1 {
		more = fmt.Sprintf(", and on %d more", len(list)-1)
	}
	how := "better"
	if worse {
		how = "worse"
	}
	if w.period() {
		return fmt.Sprintf("Jobs did %s in %s than in %s on %s (%s to %s)%s.", how, w.b, w.a, top.measure.words, top.from, top.to, more)
	}
	return fmt.Sprintf("%s is %s than %s on %s (%s to %s)%s.", w.b, how, w.a, top.measure.words, top.from, top.to, more)
}

// alike says nothing moved, and how many measures could not be judged, so
// "ran alike" never claims a measure nobody saw.
func (w wording) alike(all []judged) string {
	head := fmt.Sprintf("%s and %s ran alike", w.b, w.a)
	if w.period() {
		head = fmt.Sprintf("Jobs ran alike in %s and %s", w.a, w.b)
	}
	unjudged := 0
	for _, j := range all {
		if j.change.Direction == NotEnough {
			unjudged++
		}
	}
	if unjudged > 0 {
		return fmt.Sprintf("%s: no measure moved past its threshold, though %d of %d could not be compared.", head, unjudged, len(all))
	}
	return head + ": no measure moved past its threshold."
}

// doubts are the reasons a comparison that ranks is still not high
// confidence, each naming the side and the figure it is about.
func (w wording) doubts(s CompareSnapshot) []string {
	var out []string
	if s.A.Figures.Jobs < ConfidentCompareJobs || s.B.Figures.Jobs < ConfidentCompareJobs {
		out = append(out, w.ran(s.A.Figures.Jobs, s.B.Figures.Jobs)+
			fmt.Sprintf(": fewer than %d on a side, so a change named here is less certain", ConfidentCompareJobs))
	}
	for _, side := range []CompareSide{s.A, s.B} {
		if side.CountsOnly {
			out = append(out, side.Label+" reaches past retention.jobs, so it was read from the daily usage roll-up: its percentiles are unavailable, "+
				"it is compared on fleet failure rate alone, and the roll-up counts every job on the day it was first seen, cancelled and unfinished ones included, "+
				"so its counts are defined differently from the job rows'")
		}
	}
	out = append(out, w.coverage(s)...)
	for _, side := range []CompareSide{s.A, s.B} {
		if side.JobsGoneBefore != nil {
			out = append(out, fmt.Sprintf("%s's jobs from before %s are gone (retention.jobs keeps them for a set time, and raising it keeps more next time), so %s's figures cover only the jobs still kept",
				side.Label, side.JobsGoneBefore.UTC().Format("2006-01-02 15:04 UTC"), side.Label))
		}
	}
	switch {
	case s.A.MeanSlots == nil && s.B.MeanSlots == nil:
		out = append(out, "how many slots the fleet offered is not known for either side, because no host samples cover them (retention.samples decides how long they are kept), so queue waits may not be like for like")
	case s.A.MeanSlots == nil || s.B.MeanSlots == nil:
		unknown := s.A.Label
		if s.B.MeanSlots == nil {
			unknown = s.B.Label
		}
		out = append(out, fmt.Sprintf("how many slots the fleet offered %s is not known, because no host samples cover it (retention.samples decides how long they are kept), so queue waits may not be like for like", w.under(unknown)))
	default:
		lo, hi := min(*s.A.MeanSlots, *s.B.MeanSlots), max(*s.A.MeanSlots, *s.B.MeanSlots)
		if hi > lo*1.25 {
			out = append(out, fmt.Sprintf("the fleet offered %s slots %s and %s %s, so queue waits are not like for like",
				slotWords(*s.A.MeanSlots), w.under(s.A.Label), slotWords(*s.B.MeanSlots), w.under(s.B.Label)))
		}
	}
	return out
}

func stamp(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") }

// cutOff is what an unread side's summary adds when the fleet's counts begin
// inside the hours it asked for, after the midnight its read stopped at:
// those hours are on record, and "could not be read" alone would say they
// were lost. An earlier side's are the later side's when its counts begin
// there; otherwise they are neither side's.
func (w wording) cutOff(s CompareSnapshot, gone CompareSide, later bool) string {
	first, end := *gone.NoRecordBefore, gone.AskedUntil
	if !first.Before(end) {
		return ""
	}
	which, went := "earlier", "on neither side"
	switch {
	case later:
		which = "later"
	case !s.B.Since.After(first):
		went = "in " + w.b
	}
	return fmt.Sprintf("; the roll-up keeps whole UTC days, so the jobs queued from %s to %s, the %s period's last hours, are counted %s",
		stamp(first), stamp(end), which, went)
}

// coverage is what a side read from the roll-up leaves out, or leaves to the
// other side, each with its cause; the dates alone would be the only other
// place it showed. An unread side's summary says its own.
//
// A side whose counts begin later than asked says the fleet keeps none
// before. Between the sides, hours can be in neither count for one of two
// reasons: the later side's counts begin late, and nothing before them is
// on record; or the roll-up keeps whole UTC days, and the earlier side ends
// at the last midnight it could read whole. The same whole days can start
// the later side at its first midnight, before the period asked for, so the
// earlier side's last hours are counted in it, and end it at its last, so
// the hours after are counted on neither side.
func (w wording) coverage(s CompareSnapshot) []string {
	if s.B.unread() {
		return nil
	}
	late := func(side CompareSide) string {
		return fmt.Sprintf("the fleet keeps no counts from before %s, so %s begins there, later than the period asked for",
			stamp(*side.NoRecordBefore), side.Label)
	}
	var out []string
	// An unread earlier side's summary already says what became of its
	// hours; the later side's own end is still this function's to name.
	if !s.A.unread() {
		if s.A.NoRecordBefore != nil {
			out = append(out, late(s.A))
		}
		between := !s.A.Until.IsZero() && s.A.Until.Before(s.B.Since)
		switch {
		case between && s.B.NoRecordBefore != nil:
			out = append(out, fmt.Sprintf("the fleet keeps no counts from before %s, so the hours from %s to %s, between the two periods, are counted on neither side",
				stamp(*s.B.NoRecordBefore), stamp(s.A.Until), stamp(s.B.Since)))
		case s.B.NoRecordBefore != nil:
			out = append(out, late(s.B))
		case between && s.A.CountsOnly:
			out = append(out, fmt.Sprintf("the roll-up keeps whole UTC days, so the jobs queued from %s to %s, between the two periods, are counted on neither side",
				stamp(s.A.Until), stamp(s.B.Since)))
		}
		// The later side read from its first midnight begins before the
		// period asked for, so the earlier side's last hours are its own.
		if s.B.CountsOnly && s.B.Since.Before(s.A.AskedUntil) {
			out = append(out, fmt.Sprintf("the roll-up keeps whole UTC days, so %s begins earlier than the period asked for, and the jobs queued from %s to %s are counted in it, not in %s",
				s.B.Label, stamp(s.B.Since), stamp(s.A.AskedUntil), s.A.Label))
		}
	}
	// The later side ended at the last midnight the roll-up read whole, and
	// nothing comes after it to count the rest.
	if s.B.CountsOnly && s.B.Until.Before(s.B.AskedUntil) {
		out = append(out, fmt.Sprintf("the roll-up keeps whole UTC days, so %s ends earlier than the period asked for, and the jobs queued from %s to %s, after its last whole day, are counted on neither side",
			s.B.Label, stamp(s.B.Until), stamp(s.B.AskedUntil)))
	}
	return out
}

func slotWords(n float64) string {
	return strconv.FormatFloat(math.Round(n*10)/10, 'f', -1, 64)
}

// percent is a rate as a whole percentage, the way the Usage table shows it.
func percent(events int, jobs int64) string {
	return fmt.Sprintf("%d%%", int(math.Round(float64(events)*100/float64(jobs))))
}

func msFloat(v *int64) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}

func round(v, places float64) *float64 {
	r := math.Round(v*places) / places
	return &r
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
