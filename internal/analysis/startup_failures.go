package analysis

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const StartupFailuresVersion = "startup-failures-v1"

// MaxStartupGroups bounds the groups a finding lists, so a fleet whose every
// host is failing still fits a reader's window; the counts keep the rest.
const MaxStartupGroups = 20

// RepeatedFailures is the fewest failures that make a host or a pool one that
// keeps failing: one or two failed starts are what a busy fleet always has.
const RepeatedFailures = 3

// The outcomes a start can have. Anything else, or nothing, is unknown.
const (
	StartStarted = "started"
	StartFailed  = "failed"
	// StartUnknown is a start whose end is not on record: a runner still
	// starting when the period ended, one removed before it registered with
	// no fault recorded, or one known only from a session that never
	// registered (a session that did is started). An outcome this package
	// does not know is counted the same way.
	StartUnknown = "unknown"
)

// Where a failed start stopped, in the order a start goes through them.
const (
	StageBeforeContainer    = "before_container"
	StageBeforeRegistration = "before_registration"
	StageAfterRegistration  = "after_registration"
)

var startStages = []string{StageBeforeContainer, StageBeforeRegistration, StageAfterRegistration}

var stageWords = map[string]string{
	StageBeforeContainer:    "before the container started",
	StageBeforeRegistration: "after the container started but before the runner registered",
	StageAfterRegistration:  "after the runner registered",
}

// What a group's failures are put down to, in the order they are tried: a
// fault the failed runner recorded, then an incident its host still reports,
// then a throttle, whose only tie to a failure is when it happened.
const (
	ExplainImage             = "image"
	ExplainImagePullIncident = "image_pull_incident"
	ExplainRuntimeIncident   = "runtime_incident"
	ExplainThrottle          = "throttle"
)

// The incidents a host keeps, as store.HostIncidents names them.
const (
	IncidentRuntime   = "runtime"
	IncidentImagePull = "image_pull"
)

const (
	ScopeHost = "host"
	ScopePool = "pool"
)

// The fault kinds the explanations read, as the store spells them (pinned by
// a test, because this package may not import the store). A failing runtime
// or a host under pressure records a backend fault or none; a failure that
// recorded another kind, such as GitHub refusing a registration, has that
// kind's cause and is never put down to the host's machinery.
const (
	faultImage       = "image"
	faultBackend     = "backend"
	faultBackendBusy = "backend_busy"
)

// StartAttempt is one runner the fleet asked a host to start.
type StartAttempt struct {
	RunnerID, PoolID, PoolName, HostID, HostName string
	RequestedAt                                  time.Time
	ContainerStartedAt, RegisteredAt, EndedAt    *time.Time
	// Outcome is StartStarted, StartFailed or StartUnknown.
	Outcome   string
	FaultKind string
	// Message is the failure as the host reported it: written by a host, so
	// whoever shows it to a model treats it as untrusted.
	Message string
}

// HostIncident is a runtime or image-pull incident a host still reports. A
// host keeps them only while they are current, so one that has cleared is not
// known here; PoolID is the pool whose image would not pull.
type HostIncident struct {
	HostID, Kind, PoolID string
	Since                time.Time
}

// HostInterval is a stretch a host was throttled. A zero To is a throttle
// still on at Until.
type HostInterval struct {
	HostID string
	Interval
}

type StartupSnapshot struct {
	Since, Until time.Time
	// Attempts are the starts asked for in the period; one asked for outside
	// it is left out, so a start belongs to one period only.
	Attempts  []StartAttempt
	Throttles []HostInterval
	Incidents []HostIncident
	// Truncated says the read stopped at MaxReadJobs runners.
	Truncated bool
	// HeartbeatInterval is the agents' agent.heartbeat_interval. A runtime
	// incident is first heard of on the beat after the start that found the
	// runtime down, so a failure up to one interval before its since may be
	// what raised it, and one further back may not. Zero reaches back not
	// at all.
	HeartbeatInterval time.Duration
}

// StartupGroup is one host or one pool and how its starts went. Every time
// in it is when something happened: a failure when it failed (its request
// when that is not on record), a start when it registered.
type StartupGroup struct {
	Scope string `json:"scope"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	// PoolID is set on a host's group that counts one pool's starts alone:
	// the host a pool's failures all fell on while the pool started on
	// others, which is that host's trouble and not the pool's.
	PoolID   string `json:"pool_id"`
	Starts   int    `json:"starts"`
	Failures int    `json:"failures"`
	// Unknown counts its starts with no recorded outcome, in neither Starts
	// nor Failures.
	Unknown int `json:"unknown"`
	// FailingSince is the first failure after the last start that
	// registered before its latest failure, or its first failure when no
	// start registered before that.
	FailingSince time.Time `json:"failing_since"`
	// RecoveredAt is the first start that registered after its latest
	// failure; nil while it is still failing.
	RecoveredAt *time.Time `json:"recovered_at"`
	LastAt      time.Time  `json:"last_at"`
	// Stage and FaultKind are the most common among its failures.
	Stage     string `json:"stage"`
	FaultKind string `json:"fault_kind"`
	// Explanation is image, image_pull_incident, runtime_incident, throttle
	// or "", and Explained how many of its failures it covers.
	Explanation  string `json:"explanation"`
	Explained    int    `json:"explained"`
	LastRunnerID string `json:"last_runner_id"`
	// LastMessage is written by a host, not the fleet, and is untrusted.
	LastMessage string `json:"last_message"`
}

type StartupEvidence struct {
	// Groups are the hosts and pools that keep failing, most failures
	// first, cut to MaxStartupGroups; GroupsCut says how many were left out.
	// When none does but one failed three times or more, it holds that one,
	// the group the summary quotes.
	Groups    []StartupGroup `json:"groups"`
	GroupsCut int            `json:"groups_cut"`
	// Starts counts the starts in the period with an outcome, Failures those
	// that failed, and Unknown those with none.
	Starts    int  `json:"starts"`
	Failures  int  `json:"failures"`
	Unknown   int  `json:"unknown"`
	Truncated bool `json:"truncated"`
}

type StartupFailuresFinding = Finding[StartupEvidence]

var startupFailuresCodes = []string{"host.image_pull_failed", "host.runtime_recovering", "host.throttled", "pool.runners_failing"}

// The fewest starts over which "nothing keeps failing" is said firmly.
const confidentStarts = 10

// at is when a start's outcome happened, the clock every order here is on.
func (a StartAttempt) at() time.Time {
	switch {
	case a.Outcome == StartStarted && a.RegisteredAt != nil:
		return *a.RegisteredAt
	case a.Outcome == StartFailed && a.EndedAt != nil:
		return *a.EndedAt
	}
	return a.RequestedAt
}

func (a StartAttempt) stage() string {
	switch {
	case a.ContainerStartedAt == nil:
		return StageBeforeContainer
	case a.RegisteredAt == nil:
		return StageBeforeRegistration
	}
	return StageAfterRegistration
}

// startRow is a group with what its sentences need beside the evidence.
type startRow struct {
	StartupGroup
	repeats bool
	stageN  int
	faultN  int
	covered []StartAttempt
	since   time.Time // the earliest incident behind the explanation
}

// startupRead is the snapshot as the rows are judged against it.
type startupRead struct {
	known, unknown []StartAttempt
	poolNames      map[string]string
	hostNames      map[string]string
	incidents      []HostIncident
	throttles      []HostInterval
	since, until   time.Time
	heartbeat      time.Duration
}

// StartupFailures names the host or pool that keeps failing to start runners,
// since when, the stage it fails at, and whether a fault, an incident or a
// throttle explains it; or says that none does.
func StartupFailures(s StartupSnapshot) (StartupFailuresFinding, error) {
	f := StartupFailuresFinding{Analysis: "startup_failures", Version: StartupFailuresVersion, Since: s.Since, Until: s.Until,
		NextSteps: []NextStep{}, Codes: []string{}}
	if s.Since.IsZero() || !s.Since.Before(s.Until) {
		return f, errors.New("startup failures snapshot needs a period with a start before its end")
	}
	r := startupRead{poolNames: map[string]string{}, hostNames: map[string]string{}, incidents: s.Incidents,
		throttles: s.Throttles, since: s.Since, until: s.Until, heartbeat: s.HeartbeatInterval}
	e := StartupEvidence{Groups: []StartupGroup{}, Truncated: s.Truncated}
	for _, a := range s.Attempts {
		if a.RequestedAt.Before(s.Since) || !a.RequestedAt.Before(s.Until) {
			continue
		}
		if a.PoolName != "" {
			r.poolNames[a.PoolID] = a.PoolName
		}
		if a.HostName != "" {
			r.hostNames[a.HostID] = a.HostName
		}
		switch a.Outcome {
		case StartStarted, StartFailed:
			r.known = append(r.known, a)
			e.Starts++
			if a.Outcome == StartFailed {
				e.Failures++
			}
		default:
			// An outcome this package does not know is no outcome: counting
			// it as a failure would invent one.
			e.Unknown++
			r.unknown = append(r.unknown, a)
		}
	}
	slices.SortFunc(r.known, func(a, b StartAttempt) int {
		return cmp.Or(a.at().Compare(b.at()), a.RequestedAt.Compare(b.RequestedAt), strings.Compare(a.RunnerID, b.RunnerID))
	})

	// One row a host and one a pool. A start with no host yet still failed
	// for its pool.
	var rows []startRow
	seen := map[string]bool{}
	for _, a := range r.known {
		for _, k := range []struct{ scope, id string }{{ScopeHost, a.HostID}, {ScopePool, a.PoolID}} {
			if k.id == "" || seen[k.scope+":"+k.id] {
				continue
			}
			seen[k.scope+":"+k.id] = true
			rows = append(rows, r.row(k.scope, k.id, ""))
		}
	}
	byFailures := func(a, b startRow) int {
		// Most failures first; at a tie a host before a pool, the narrower
		// place to look; then by name, so a snapshot always reads one way.
		return cmp.Or(b.Failures-a.Failures, cmp.Compare(scopeRank(a.Scope), scopeRank(b.Scope)),
			strings.Compare(a.Name, b.Name), strings.Compare(a.ID, b.ID), strings.Compare(a.PoolID, b.PoolID))
	}
	slices.SortFunc(rows, byFailures)
	var repeating []startRow
	for _, row := range rows {
		if row.repeats {
			repeating = append(repeating, row)
		}
	}

	if len(repeating) > 0 {
		if host, ok := r.oneHostOfMany(repeating[0]); ok {
			// The pool fails only on this host and starts on others, so the
			// host is where to look. Its own row does not repeat, or it would
			// lead the pool: its other pools start there. So it is counted on
			// this pool alone.
			repeating = append(repeating, r.row(ScopeHost, host, repeating[0].ID))
			slices.SortFunc(repeating, byFailures)
		}
	}
	e.GroupsCut = max(0, len(repeating)-MaxStartupGroups)
	for _, row := range repeating[:min(len(repeating), MaxStartupGroups)] {
		e.Groups = append(e.Groups, row.StartupGroup)
	}
	f.Evidence = e

	unknownTail := ""
	if e.Unknown > 0 {
		unknownTail = fmt.Sprintf("; %d more %s no recorded outcome", e.Unknown, hasHave(e.Unknown))
	}
	var reasons []string
	switch {
	case e.Starts == 0 && e.Unknown == 0:
		f.Confidence, f.ConfidenceReason = Low, "no runners were asked to start in this period"
		f.Summary = "No runners were asked to start in this period."
		return f, nil
	case e.Starts == 0:
		f.Confidence, f.ConfidenceReason = Low, "no start in this period has a recorded outcome"
		f.Summary = fmt.Sprintf("None of the %s asked to start in this period has a recorded outcome.", count(e.Unknown, "runner"))
		if e.Unknown == 1 {
			f.Summary = "The one runner asked to start in this period has no recorded outcome."
		}
		return f, nil
	case len(repeating) > 0:
		top := repeating[0]
		f.Dominant = top.Scope + ":" + top.ID
		f.Summary = r.repeatSentence(top)
		f.NextSteps = r.steps(top)
		switch top.Explanation {
		case ExplainThrottle:
			reasons = append(reasons, "only the timing ties the failures to the throttle")
		case "":
			reasons = append(reasons, "no image fault, open incident or throttle explains most of the failures, and an incident that has already cleared is not kept")
		}
		if top.Explanation != "" && top.Explained*4 < top.Failures*3 {
			reasons = append(reasons, fmt.Sprintf("only %d of its %d failures %s, so the rest may have another cause",
				top.Explained, top.Failures, coveredWords[top.Explanation]))
		}
	case e.Failures == 0:
		f.Summary = fmt.Sprintf("No start failed in this period: %s started%s.", count(e.Starts, "runner"), unknownTail)
	case len(rows) == 0 || rows[0].Failures < RepeatedFailures:
		f.Summary = fmt.Sprintf("No host or pool failed to start runners more than twice in this period: %d of %d starts failed%s.",
			e.Failures, e.Starts, unknownTail)
	default:
		// Some host or pool failed three times or more, but under half its
		// starts: "more than twice" would be untrue of it. The sentence says
		// both halves of the rule, because a host that failed its only two
		// starts did fail half of them. The group it quotes goes in the
		// evidence, so whatever shows the sentence can show the row.
		worst := rows[0]
		f.Summary = fmt.Sprintf("No host or pool failed three or more times and at least half its starts in this period: %d of %d starts failed, the most on one being %d of %d on %s %s%s.",
			e.Failures, e.Starts, worst.Failures, worst.Starts, worst.Scope, worst.Name, unknownTail)
		f.Evidence.Groups = append(f.Evidence.Groups, worst.StartupGroup)
	}
	if f.Dominant == "" && e.Starts < confidentStarts {
		reasons = append(reasons, fmt.Sprintf("only %s in this period: too few to say that nothing keeps failing", count(e.Starts, "start")))
	}
	if e.Unknown*10 > e.Starts+e.Unknown {
		reasons = append(reasons, fmt.Sprintf("%d of the %d runners asked to start have no recorded outcome (still starting when the period ended, removed before registering with nothing recorded, or known only from a session that never registered, or with an outcome this analysis does not know), so some may have failed",
			e.Unknown, e.Starts+e.Unknown))
	}
	if s.Truncated {
		reasons = append(reasons, fmt.Sprintf("the period held more than %d runners; the newest %d were read, so narrow since", MaxReadJobs, MaxReadJobs))
	}
	f.Confidence = High
	if len(reasons) > 0 {
		// The truncation reason carries a semicolon of its own, so a bare
		// one between reasons would hide where the next begins.
		f.Confidence, f.ConfidenceReason = Medium, strings.Join(reasons, "; also, ")
	}
	for _, step := range f.NextSteps {
		if step.Code != "" && !slices.Contains(f.Codes, step.Code) {
			f.Codes = append(f.Codes, step.Code)
		}
	}
	return f, nil
}

func scopeRank(scope string) int {
	if scope == ScopeHost {
		return 0
	}
	return 1
}

// row judges one host or pool; a host's row given a pool counts that pool's
// starts on it alone.
func (r startupRead) row(scope, id, poolID string) startRow {
	in := func(a StartAttempt) bool {
		if scope == ScopePool {
			return a.PoolID == id
		}
		return a.HostID == id && (poolID == "" || a.PoolID == poolID)
	}
	row := startRow{StartupGroup: StartupGroup{Scope: scope, ID: id, PoolID: poolID}}
	names := r.hostNames
	if scope == ScopePool {
		names = r.poolNames
	}
	row.Name = cmp.Or(names[id], id)
	for _, a := range r.unknown {
		if in(a) {
			row.Unknown++
		}
	}
	var attempts, failures []StartAttempt
	for _, a := range r.known {
		if in(a) {
			attempts = append(attempts, a)
			if a.Outcome == StartFailed {
				failures = append(failures, a)
			}
		}
	}
	row.Starts, row.Failures = len(attempts), len(failures)
	row.repeats = row.Failures >= RepeatedFailures && row.Failures*2 >= row.Starts
	if row.Failures == 0 {
		return row
	}

	// The run of failures that is the latest: those after the last start
	// that registered before the latest failure.
	lastFail := len(attempts) - 1
	for attempts[lastFail].Outcome != StartFailed {
		lastFail--
	}
	from := 0
	for i := lastFail - 1; i >= 0; i-- {
		if attempts[i].Outcome == StartStarted {
			from = i + 1
			break
		}
	}
	for _, a := range attempts[from:] {
		if a.Outcome == StartFailed {
			row.FailingSince = a.at()
			break
		}
	}
	for _, a := range attempts[lastFail+1:] {
		if a.Outcome == StartStarted {
			at := a.at()
			row.RecoveredAt = &at
			break
		}
	}
	last := failures[len(failures)-1]
	row.LastAt, row.LastRunnerID, row.LastMessage = last.at(), last.RunnerID, last.Message

	row.Stage, row.stageN = mostCommon(failures, func(a StartAttempt) string { return a.stage() },
		func(x, y string) int { return slices.Index(startStages, x) - slices.Index(startStages, y) })
	// A fault kind recorded is said before none.
	row.FaultKind, row.faultN = mostCommon(failures, func(a StartAttempt) string { return a.FaultKind },
		func(x, y string) int { return cmp.Or(emptyLast(x)-emptyLast(y), strings.Compare(x, y)) })

	runtimeCovers := func(a StartAttempt) (bool, time.Time) { return r.runtimeCovers(a, row.FailingSince) }
	for _, ex := range []struct {
		name   string
		covers func(StartAttempt) (bool, time.Time)
		atHalf bool
	}{
		{ExplainImage, func(a StartAttempt) (bool, time.Time) { return a.FaultKind == faultImage, time.Time{} }, false},
		{ExplainImagePullIncident, r.imagePullCovers, false},
		{ExplainRuntimeIncident, runtimeCovers, false},
		// The throttle holds at half because nothing firmer is claimed of
		// it: the finding says only the timing ties it, at medium.
		{ExplainThrottle, r.throttleCovers, true},
	} {
		var covered []StartAttempt
		var since time.Time
		for _, a := range failures {
			if ok, t := ex.covers(a); ok {
				covered = append(covered, a)
				if !t.IsZero() && (since.IsZero() || t.Before(since)) {
					since = t
				}
			}
		}
		holds := len(covered)*2 > row.Failures || (ex.atHalf && len(covered)*2 == row.Failures)
		if ex.name == ExplainThrottle && holds {
			// A host throttled all period has every failure inside a
			// throttle by chance, so the failures must fall inside one
			// more often than time does on their hosts: more covered than
			// the sum of each failure's host's throttled share. In
			// milliseconds, so the comparison is exact.
			var expected int64
			for _, a := range failures {
				expected += r.throttledMS(a.HostID)
			}
			holds = int64(len(covered))*r.until.Sub(r.since).Milliseconds() > expected
		}
		if holds {
			row.Explanation, row.Explained, row.covered, row.since = ex.name, len(covered), covered, since
			break
		}
	}
	return row
}

// hostMachinery is a fault a failing runtime or a host under pressure
// leaves: a backend one, or none recorded.
func hostMachinery(kind string) bool {
	return kind == "" || kind == faultBackend || kind == faultBackendBusy
}

// imagePullCovers needs no time: the incident's since is when the controller
// heard of the failed pull, which is after the start that raised it.
func (r startupRead) imagePullCovers(a StartAttempt) (bool, time.Time) {
	if a.FaultKind != "" && a.FaultKind != faultImage {
		return false, time.Time{}
	}
	for _, inc := range r.incidents {
		if inc.Kind == IncidentImagePull && inc.HostID == a.HostID && inc.PoolID == a.PoolID {
			return true, inc.Since
		}
	}
	return false, time.Time{}
}

// runtimeCovers takes a failure at or after the incident's since. It also
// takes one before it by no more than a heartbeat interval, in the group's
// current run, with no start registered on its host in between: the
// controller hears of the incident on the beat after the start that found
// the runtime down, so that failure is its trigger. A failure further back
// came while the runtime was, for all anything says, working. Of several
// incidents that cover it, the earliest is the one it came under, so the
// order they are listed in cannot change the answer.
func (r startupRead) runtimeCovers(a StartAttempt, failingSince time.Time) (bool, time.Time) {
	if !hostMachinery(a.FaultKind) {
		return false, time.Time{}
	}
	at := a.at()
	var since time.Time
	for _, inc := range r.incidents {
		if inc.Kind != IncidentRuntime || inc.HostID != a.HostID || (!since.IsZero() && !inc.Since.Before(since)) {
			continue
		}
		if inc.Since.After(at) && !r.triggered(a, inc.Since, failingSince) {
			continue
		}
		since = inc.Since
	}
	return !since.IsZero(), since
}

// triggered says whether a failure before an incident's since could be the
// one that raised it.
func (r startupRead) triggered(a StartAttempt, since, failingSince time.Time) bool {
	at := a.at()
	if since.Sub(at) > r.heartbeat || at.Before(failingSince) {
		return false
	}
	for _, b := range r.known {
		if b.Outcome == StartStarted && b.HostID == a.HostID && b.at().After(at) && !b.at().After(since) {
			return false
		}
	}
	return true
}

func (r startupRead) throttleCovers(a StartAttempt) (bool, time.Time) {
	if !hostMachinery(a.FaultKind) {
		return false, time.Time{}
	}
	at := a.at()
	for _, th := range r.throttles {
		if th.HostID == a.HostID && !at.Before(th.From) && (th.To.IsZero() || at.Before(th.To)) {
			return true, time.Time{}
		}
	}
	return false, time.Time{}
}

// throttledMS is how long a host was throttled within the period.
func (r startupRead) throttledMS(host string) int64 {
	var list []Interval
	for _, th := range r.throttles {
		if th.HostID == host {
			iv := th.Interval
			if iv.To.IsZero() || iv.To.After(r.until) {
				iv.To = r.until
			}
			iv.From = later(iv.From, r.since)
			list = append(list, iv)
		}
	}
	var ms int64
	for _, iv := range merge(list) {
		ms += iv.To.Sub(iv.From).Milliseconds()
	}
	return ms
}

// oneHostOfMany is the host a pool's failures all fell on, when the pool's
// runners started on another host in the period.
func (r startupRead) oneHostOfMany(row startRow) (string, bool) {
	if row.Scope != ScopePool {
		return "", false
	}
	host := ""
	for _, a := range r.known {
		if a.PoolID == row.ID && a.Outcome == StartFailed {
			if a.HostID == "" || (host != "" && a.HostID != host) {
				return "", false
			}
			host = a.HostID
		}
	}
	return host, host != "" && r.startedElsewhere(host, []string{row.ID})
}

// startedElsewhere says whether any of pools started on a host but this one.
func (r startupRead) startedElsewhere(host string, pools []string) bool {
	for _, a := range r.known {
		if a.Outcome == StartStarted && a.HostID != "" && a.HostID != host && slices.Contains(pools, a.PoolID) {
			return true
		}
	}
	return false
}

// failedPools are the pools a row's failures were of.
func (r startupRead) failedPools(row startRow) []string {
	var out []string
	for _, a := range r.known {
		if a.Outcome == StartFailed && a.HostID == row.ID && (row.PoolID == "" || a.PoolID == row.PoolID) && !slices.Contains(out, a.PoolID) {
			out = append(out, a.PoolID)
		}
	}
	return out
}

func (r startupRead) repeatSentence(g startRow) string {
	var b strings.Builder
	switch {
	case g.PoolID != "":
		fmt.Fprintf(&b, "Host %s failed to start %d of %d runners of pool %s in this period", g.Name, g.Failures, g.Starts, r.poolName(g.PoolID))
	case g.Scope == ScopeHost:
		fmt.Fprintf(&b, "Host %s failed to start %d of %d runners in this period", g.Name, g.Failures, g.Starts)
	default:
		fmt.Fprintf(&b, "Pool %s failed to start %d of %d runners in this period", g.Name, g.Failures, g.Starts)
	}
	if g.Unknown > 0 {
		fmt.Fprintf(&b, " (%d more %s no recorded outcome)", g.Unknown, hasHave(g.Unknown))
	}
	if g.PoolID != "" {
		b.WriteString(", a pool whose runners started on other hosts")
	}
	if g.RecoveredAt != nil {
		fmt.Fprintf(&b, ", failing from %s until a start registered at %s", clock(g.FailingSince), clock(*g.RecoveredAt))
	} else {
		fmt.Fprintf(&b, ", failing since %s", clock(g.FailingSince))
	}
	b.WriteString(", ")
	if g.stageN < g.Failures {
		fmt.Fprintf(&b, "%d of them ", g.stageN)
	}
	b.WriteString(stageWords[g.Stage])
	they := ofThem(g.Explained, g.Failures, "they")
	hosts, verb := r.coveredHosts(g)
	switch g.Explanation {
	case ExplainImage:
		if g.Explained < g.Failures {
			fmt.Fprintf(&b, ": %d of them because the runner image could not be pulled or would not start", g.Explained)
		} else {
			// The image fault is a pull that failed or an image that would
			// not start, and the sentence claims no more than that.
			b.WriteString(": the runner image could not be pulled or would not start")
		}
	case ExplainImagePullIncident:
		fmt.Fprintf(&b, ": %s came on %s, which %s been unable to pull pool %s's image since %s",
			they, hosts, verb("has", "have"), r.poolName(g.covered[0].PoolID), clock(g.since))
	case ExplainRuntimeIncident:
		fmt.Fprintf(&b, ": %s came while the container runtime on %s was failing, as it has been since %s", they, hosts, clock(g.since))
	case ExplainThrottle:
		fmt.Fprintf(&b, ": %s came while %s %s throttled", they, hosts, verb("was", "were"))
	default:
		// Most, because that is what each explanation was held to: half of
		// them can be image faults and none explain the group.
		b.WriteString(", and no image fault, open incident or throttle explains most of them")
		if g.FaultKind != "" {
			fmt.Fprintf(&b, ": %s recorded the fault %s", ofThem(g.faultN, g.Failures, "all of them"), g.FaultKind)
		}
	}
	b.WriteString(".")
	return b.String()
}

// coveredWords finish "only 2 of its 3 failures ...".
var coveredWords = map[string]string{
	ExplainImage:             "recorded the image fault",
	ExplainImagePullIncident: "came on a host still unable to pull the pool's image",
	ExplainRuntimeIncident:   "came while the container runtime was failing",
	ExplainThrottle:          "came while the host was throttled",
}

func (r startupRead) steps(g startRow) []NextStep {
	var out []NextStep
	hosts, verb := r.coveredHosts(g)
	switch g.Explanation {
	case ExplainImage, ExplainImagePullIncident:
		text := fmt.Sprintf("Check that pool %s's image names a tag that exists, and that its hosts can reach the registry and are logged in to it if the image is private.", g.Name)
		if g.Scope == ScopeHost {
			pool, _ := mostCommon(g.covered, func(a StartAttempt) string { return a.PoolID }, strings.Compare)
			text = fmt.Sprintf("Check that %s can reach the registry of pool %s's image and is logged in to it if the image is private, and that the image's tag exists.",
				g.Name, r.poolName(pool))
		}
		out = append(out, NextStep{Setting: "pool.image", Code: "host.image_pull_failed", Text: text})
	case ExplainRuntimeIncident:
		out = append(out, NextStep{Setting: "host.capacity", Code: "host.runtime_recovering", Text: fmt.Sprintf(
			"Get the container runtime on %s answering again: %s, and if %s running but slow, lower %s capacity so it carries less.", hosts,
			verb("the host holds new starts until it recovers", "the hosts hold new starts until they recover"),
			verb("the runtime is", "a runtime is"), verb("the host's", "that host's"))})
	case ExplainThrottle:
		out = append(out, NextStep{Setting: "host.capacity", Code: "host.throttled", Text: fmt.Sprintf(
			"Lower the capacity of %s so %s: the failures came while %s throttled after sustained pressure, though only the timing ties them to it.", hosts,
			verb("its runners fit the machine", "their runners fit the machines"), verb("it was", "they were"))})
	default:
		out = append(out, NextStep{Code: "pool.runners_failing", Text: fmt.Sprintf(
			"Read what runner %s, the latest to fail on %s %s, reported when it failed: no image fault, open incident or throttle explains most of these failures, so its own message is the best lead.",
			g.LastRunnerID, g.Scope, g.Name)})
	}
	switch {
	case g.PoolID != "":
		// Its other pools start on it, so cordoning it would cost them.
		pool := r.poolName(g.PoolID)
		out = append(out, NextStep{Setting: "pool.host_selector", Text: fmt.Sprintf(
			"Keep pool %s off %s with its host selector while %s is looked at: the pool's runners started on other hosts in this period.", pool, g.Name, g.Name)})
	case g.Scope == ScopeHost && r.startedElsewhere(g.ID, r.failedPools(g)):
		out = append(out, NextStep{Setting: "host.cordoned", Text: fmt.Sprintf(
			"Cordon %s while it is looked at: runners of the pools it failed for started on other hosts in this period, so new ones would start there.", g.Name)})
	}
	return out
}

func (r startupRead) poolName(id string) string { return cmp.Or(r.poolNames[id], id) }

// coveredHosts names the hosts behind an explanation, the one with most of
// the covered failures first, and gives the verb that agrees with them.
func (r startupRead) coveredHosts(g startRow) (string, func(one, many string) string) {
	n := map[string]int{}
	var hosts []string
	for _, a := range g.covered {
		if n[a.HostID] == 0 {
			hosts = append(hosts, a.HostID)
		}
		n[a.HostID]++
	}
	slices.SortFunc(hosts, func(a, b string) int {
		return cmp.Or(n[b]-n[a], strings.Compare(cmp.Or(r.hostNames[a], a), cmp.Or(r.hostNames[b], b)))
	})
	verb := func(one, many string) string {
		if len(hosts) > 1 {
			return many
		}
		return one
	}
	switch len(hosts) {
	case 0:
		return "", verb
	case 1:
		return cmp.Or(r.hostNames[hosts[0]], hosts[0]), verb
	}
	return fmt.Sprintf("%s and %s", cmp.Or(r.hostNames[hosts[0]], hosts[0]), count(len(hosts)-1, "more host")), verb
}

// mostCommon is the commonest key among attempts and how many have it; a tie
// goes to the key first in order.
func mostCommon(attempts []StartAttempt, key func(StartAttempt) string, order func(a, b string) int) (string, int) {
	n := map[string]int{}
	for _, a := range attempts {
		n[key(a)]++
	}
	best, bestN := "", -1
	for k, c := range n {
		if c > bestN || (c == bestN && order(k, best) < 0) {
			best, bestN = k, c
		}
	}
	return best, bestN
}

// ofThem is "they" (or all) when n is every one, "2 of them" otherwise.
func ofThem(n, total int, all string) string {
	if n == total {
		return all
	}
	return fmt.Sprintf("%d of them", n)
}

func emptyLast(s string) int {
	if s == "" {
		return 1
	}
	return 0
}

func hasHave(n int) string {
	if n == 1 {
		return "has"
	}
	return "have"
}

// clock is a time as a summary says it: to the minute, in UTC, so every
// reader of one finding reads one clock.
func clock(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04") + " UTC"
}
