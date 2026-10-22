package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The keys JobStats can group by. Each is also the name of the filter that
// takes the same value back, so a row of the answer can be opened as a listing.
const (
	GroupByControllerVersion = "controller_version"
	GroupByDay               = "day"
	GroupByHost              = "host"
	GroupByPool              = "pool"
	GroupByJobName           = "job_name"
	GroupBySize              = "size"
)

// JobStatsGroupKeys lists the accepted group_by values, in the order the
// documentation names them.
var JobStatsGroupKeys = []string{GroupByControllerVersion, GroupByDay, GroupByHost, GroupByPool, GroupByJobName, GroupBySize}

// MaxJobStatsGroups caps the groups one answer carries. Grouping by job name
// across a busy fleet can name thousands; an answer that large is not one a
// person or a model reads, so the rest are dropped and the answer says so.
const MaxJobStatsGroups = 500

// sizeSQL names the size of the runner a job ran on the way an operator would
// say it -- "3 CPU / 8 GB" -- and "unknown" for a job nothing recorded one for:
// one from before sizes were recorded, or on a runner created with no limits.
// It is the group of the size key and the value of the size filter, so a row of
// an answer can be opened as the listing it counted.
const sizeSQL = `CASE WHEN granted_source = '' THEN '` + UnknownVersion + `' ELSE
	CASE WHEN granted_cpus > 0 THEN printf('%g CPU', granted_cpus) ELSE 'no CPU limit' END || ' / ' ||
	CASE WHEN granted_memory_mb <= 0 THEN 'no memory limit'
		WHEN granted_memory_mb % 1024 = 0 THEN printf('%d GB', granted_memory_mb / 1024)
		ELSE printf('%d MB', granted_memory_mb) END
	END`

// groupExpr is the SQL that names a job's group for one key. The five that can
// be absent say "unknown" rather than leaving a NULL group, which most
// clients would drop or render as nothing.
var groupExpr = map[string]string{
	GroupBySize:              sizeSQL,
	GroupByControllerVersion: `COALESCE(controller_version, '` + UnknownVersion + `')`,
	GroupByDay:               `strftime('%Y-%m-%d', queued_at / 1000, 'unixepoch')`,
	GroupByHost:              `COALESCE(host_id, '` + UnknownVersion + `')`,
	GroupByPool:              `COALESCE(NULLIF(pool_id, ''), '` + UnknownVersion + `')`,
	GroupByJobName:           `job_name`,
}

// JobPercentiles is one measurement's spread over a group. Samples is how many
// jobs it was computed from, and the two figures are nil when that is none:
// zero milliseconds is an answer, and "nothing to measure" is not.
type JobPercentiles struct {
	Samples int    `json:"samples"`
	P50MS   *int64 `json:"p50_ms"`
	P95MS   *int64 `json:"p95_ms"`
}

// JobStatsGroup is the completed jobs sharing one value of each group_by key.
type JobStatsGroup struct {
	// Keys maps each group_by key to this group's value. Empty when the
	// request grouped by nothing and the answer is a single total.
	Keys map[string]string `json:"keys"`
	// Count is every completed job in the group; the next four split it.
	// Failed includes fleet_failed, as it does on the Overview: a job the
	// fleet lost is a failed job whoever's fault it was.
	Count       int `json:"count"`
	Succeeded   int `json:"succeeded"`
	Failed      int `json:"failed"`
	Cancelled   int `json:"cancelled"`
	FleetFailed int `json:"fleet_failed"`
	// FleetFailedByKind splits FleetFailed by the fault category the runner
	// reported. A failure recorded before categories existed is
	// "unclassified".
	FleetFailedByKind map[string]int `json:"fleet_failed_by_kind"`
	// FleetFailureRate is FleetFailed over Count, so releases with different
	// volumes can be compared on one column.
	FleetFailureRate float64        `json:"fleet_failure_rate"`
	Duration         JobPercentiles `json:"duration"`
	QueueWait        JobPercentiles `json:"queue_wait"`
	Startup          JobPercentiles `json:"startup"`
	// PeakCPUs and PeakMemoryMB are the most any job in the group was
	// measured using, nil when none was measured. OOMKilled counts the jobs
	// the kernel killed something in for its memory limit.
	PeakCPUs     *float64 `json:"peak_cpus"`
	PeakMemoryMB *int64   `json:"peak_memory_mb"`
	OOMKilled    int      `json:"oom_killed"`
	// MeasuredMemory is how many of the group's jobs have a measured peak. A job too
	// short to be sampled has none, so it is fewer than Count, and a figure from three
	// readings is not the same evidence as one from three hundred. Not in the API: the
	// peaks above are what it exposes.
	MeasuredMemory int `json:"-"`
	// PeakRunnerMemoryMB and PeakDaemonMemoryMB are the most each container of a
	// docker-in-docker pair was measured using in any job of the group, and
	// MeasuredHalves how many jobs have them. Not in the API: they are what the
	// minimum advice sizes each half by, and the peaks above are what it exposes.
	PeakRunnerMemoryMB int64 `json:"-"`
	PeakDaemonMemoryMB int64 `json:"-"`
	MeasuredHalves     int   `json:"-"`
}

// JobStatsResult is JobStats' answer.
type JobStatsResult struct {
	Groups []JobStatsGroup
	// Truncated is true when more than MaxJobStatsGroups groups matched and
	// the rest were left out.
	Truncated bool
}

// JobMeasures chooses which of the figures of a JobStatsGroup are worked out. Each
// is a statement of its own over the same window, and the dearest are the three
// that find a percentile, so a caller that reads one figure should not pay for
// the rest: the problems list reads the queue wait of the pools, the duration of
// the job names and the peak memory of a pool, each by itself, on every pass a
// browser is watching.
type JobMeasures uint8

const (
	// MeasureCounts is Count, Succeeded, Failed, Cancelled, FleetFailed and
	// FleetFailureRate.
	MeasureCounts JobMeasures = 1 << iota
	// MeasureFaultKinds is FleetFailedByKind.
	MeasureFaultKinds
	// MeasurePeaks is PeakCPUs, PeakMemoryMB, OOMKilled, MeasuredMemory and the
	// three figures about the halves of a docker-in-docker pair.
	MeasurePeaks
	// MeasureDuration, MeasureQueueWait and MeasureStartup are the percentiles of
	// the same names.
	MeasureDuration
	MeasureQueueWait
	MeasureStartup

	// MeasureAll is every figure, which is what JobStats gives.
	MeasureAll = MeasureCounts | MeasureFaultKinds | MeasurePeaks | MeasureDuration | MeasureQueueWait | MeasureStartup
)

// JobStats aggregates the completed jobs f selects, grouped by up to two keys,
// with every figure worked out.
func (s *Store) JobStats(ctx context.Context, f JobFilter, groupBy []string) (*JobStatsResult, error) {
	return s.JobStatsOf(ctx, f, groupBy, MeasureAll)
}

// JobStatsOf is JobStats for the figures in want. The groups are the same
// whatever is asked for, with the same keys, in the same order, cut off at the
// same place, and a figure that was not asked for holds its zero value (nil for
// FleetFailedByKind and the peaks), which reads as "none", not as "unknown": a
// caller reads only what it asked for.
//
// Everything is computed in SQL, on the window the caller bounded: the counts
// with GROUP BY, and each percentile from the group's samples in order, which
// SQLite sorts and joins into one string (group_concat with an ORDER BY of its
// own), with the two positions worked out beside it. SQLite has no percentile
// aggregate. The window functions this replaced counted a group and numbered its
// samples in passes that sorted the window three times, and took a third to a half
// longer at ten thousand jobs; Go is left with a pass over the text to the two
// samples, and holds a group's samples as text, about seven bytes each, for as
// long as that takes.
//
// The position is the one the Overview's percentiles use, index round((n-1)*q)
// into the sorted samples, so a figure here and the same one on the Overview
// cannot differ by a rounding rule.
//
// Duration leaves out cancelled and skipped jobs, which stop when somebody
// says so rather than when the work is done, and would drag a release's median
// towards however impatient its users were. Queue wait keeps them: a job
// cancelled after it started still waited that long. Startup is the runner's
// container start less its creation, so it exists only for jobs whose runner
// row is still kept (retention.runners) and is measured from the same two
// stamps as the Overview's startup figures.
func (s *Store) JobStatsOf(ctx context.Context, f JobFilter, groupBy []string, want JobMeasures) (*JobStatsResult, error) {
	if len(groupBy) > 2 {
		return nil, fmt.Errorf("store: at most two group_by keys, got %d", len(groupBy))
	}
	keys := [2]string{`''`, `''`}
	for i, g := range groupBy {
		e, ok := groupExpr[g]
		if !ok {
			return nil, fmt.Errorf("store: %q is not a job statistics key", g)
		}
		keys[i] = e
	}

	where, args := jobWhere(f, s.Now())
	scope := `state = 'completed'`
	if where != "" {
		scope += ` AND ` + strings.TrimPrefix(where, "WHERE ")
	}
	base := `WITH base AS (SELECT ` + keys[0] + ` AS k1, ` + keys[1] + ` AS k2, queued_at AS q,
			conclusion, runner_fault, fault_kind,
			NULLIF(peak_cpus, 0) AS pc, NULLIF(peak_memory_mb, 0) AS pm, oom_killed AS oom,
			peak_runner_memory_mb AS prm, peak_daemon_memory_mb AS pdm,
			CASE WHEN started_at IS NOT NULL THEN MAX(started_at - queued_at, 0) END AS wait,
			CASE WHEN started_at IS NOT NULL AND completed_at IS NOT NULL AND conclusion NOT IN ('cancelled','skipped')
				THEN MAX(completed_at - started_at, 0) END AS dur,
			(SELECT r.container_started_at - r.created_at FROM runners r
				WHERE r.id = jobs.runner_id AND r.container_started_at >= r.created_at) AS startup
		FROM jobs WHERE ` + scope + `) `

	type gk struct{ a, b string }
	groups := map[gk]*JobStatsGroup{}
	first := map[gk]int64{}
	keyMap := func(k gk) map[string]string {
		m := map[string]string{}
		if len(groupBy) > 0 {
			m[groupBy[0]] = k.a
		}
		if len(groupBy) > 1 {
			m[groupBy[1]] = k.b
		}
		return m
	}

	byKind := func() map[string]int {
		if want&MeasureFaultKinds == 0 {
			return nil
		}
		return map[string]int{}
	}

	// The percentiles. col is one of the three literals below, never caller input.
	type figure struct {
		col  string
		is   JobMeasures
		into func(*JobStatsGroup) *JobPercentiles
		// common is true for the figures that nearly every job has. A job without
		// one still goes through a statement over every job.
		common bool
	}
	figures := []figure{
		{"dur", MeasureDuration, func(g *JobStatsGroup) *JobPercentiles { return &g.Duration }, true},
		{"wait", MeasureQueueWait, func(g *JobStatsGroup) *JobPercentiles { return &g.QueueWait }, true},
		{"startup", MeasureStartup, func(g *JobStatsGroup) *JobPercentiles { return &g.Startup }, false},
	}
	spread := func(g *JobStatsGroup, f *figure, n, at50, at95 int, samples string) error {
		if n == 0 {
			return nil
		}
		p50, p95, err := samplesAt(samples, at50, at95)
		if err != nil {
			return err
		}
		*f.into(g) = JobPercentiles{Samples: n, P50MS: &p50, P95MS: &p95}
		return nil
	}
	// lone is the figure asked for when it is the only one and a common one. The
	// statement that names the groups groups every job, so it gathers the samples
	// as well, and the groups are named for nothing extra: asking again for the
	// samples would be a second pass over the window to find what the first has.
	var lone *figure
	for i := range figures {
		if f := &figures[i]; f.common && want == f.is {
			lone = f
		}
	}

	// The groups come from this statement whatever is asked for, so which groups
	// an answer has, their order and where it is cut short do not depend on the
	// figures. Counting is most of what it costs, so when the counts are not wanted
	// it only names the groups.
	if want&MeasureCounts != 0 {
		rows, err := s.texts.query(ctx, base+`SELECT k1, k2, COUNT(*), MIN(q),
			SUM(CASE WHEN conclusion = 'success' AND runner_fault = '' THEN 1 ELSE 0 END),
			SUM(CASE WHEN `+failedJobSQL()+` THEN 1 ELSE 0 END),
			SUM(CASE WHEN conclusion IN ('cancelled','skipped') AND runner_fault = '' THEN 1 ELSE 0 END),
			SUM(CASE WHEN `+fleetFailedJobSQL()+` THEN 1 ELSE 0 END)
			FROM base GROUP BY k1, k2`, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var k gk
			var g JobStatsGroup
			var firstQueued int64
			if err := rows.Scan(&k.a, &k.b, &g.Count, &firstQueued, &g.Succeeded, &g.Failed, &g.Cancelled, &g.FleetFailed); err != nil {
				return nil, err
			}
			g.Keys, g.FleetFailedByKind = keyMap(k), byKind()
			if g.Count > 0 {
				g.FleetFailureRate = float64(g.FleetFailed) / float64(g.Count)
			}
			groups[k], first[k] = &g, firstQueued
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	} else {
		gather := ""
		if lone != nil {
			gather = ", " + spreadColumns(lone.col)
		}
		rows, err := s.texts.query(ctx, base+`SELECT k1, k2, MIN(q)`+gather+` FROM base GROUP BY k1, k2`, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var k gk
			var firstQueued int64
			var n, at50, at95 int
			var samples string
			dest := []any{&k.a, &k.b, &firstQueued}
			if lone != nil {
				dest = append(dest, &n, &at50, &at95, &samples)
			}
			if err := rows.Scan(dest...); err != nil {
				return nil, err
			}
			g := &JobStatsGroup{Keys: keyMap(k), FleetFailedByKind: byKind()}
			if lone != nil {
				if err := spread(g, lone, n, at50, at95, samples); err != nil {
					return nil, err
				}
			}
			groups[k], first[k] = g, firstQueued
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if len(groups) == 0 {
		// The other statements only fill in groups that exist.
		return &JobStatsResult{}, nil
	}

	if want&MeasureFaultKinds != 0 {
		kinds, err := s.texts.query(ctx, base+`SELECT k1, k2, COALESCE(NULLIF(fault_kind, ''), 'unclassified'), COUNT(*)
			FROM base WHERE `+fleetFailedJobSQL()+` GROUP BY 1, 2, 3`, args...)
		if err != nil {
			return nil, err
		}
		defer kinds.Close()
		for kinds.Next() {
			var k gk
			var kind string
			var n int
			if err := kinds.Scan(&k.a, &k.b, &kind, &n); err != nil {
				return nil, err
			}
			if g := groups[k]; g != nil {
				g.FleetFailedByKind[kind] = n
			}
		}
		if err := kinds.Err(); err != nil {
			return nil, err
		}
	}

	if want&MeasurePeaks != 0 {
		peaks, err := s.texts.query(ctx, base+`SELECT k1, k2, MAX(pc), MAX(pm), SUM(oom), COUNT(pm), MAX(prm), MAX(pdm),
			SUM(CASE WHEN prm > 0 AND pdm > 0 THEN 1 ELSE 0 END) FROM base GROUP BY k1, k2`, args...)
		if err != nil {
			return nil, err
		}
		defer peaks.Close()
		for peaks.Next() {
			var k gk
			var pc sql.NullFloat64
			var pm sql.NullInt64
			var oom, measured, halves int
			var prm, pdm int64
			if err := peaks.Scan(&k.a, &k.b, &pc, &pm, &oom, &measured, &prm, &pdm, &halves); err != nil {
				return nil, err
			}
			if g := groups[k]; g != nil {
				if pc.Valid {
					g.PeakCPUs = &pc.Float64
				}
				if pm.Valid {
					g.PeakMemoryMB = &pm.Int64
				}
				g.OOMKilled = oom
				g.MeasuredMemory = measured
				g.PeakRunnerMemoryMB, g.PeakDaemonMemoryMB, g.MeasuredHalves = prm, pdm, halves
			}
		}
		if err := peaks.Err(); err != nil {
			return nil, err
		}
	}

	for i := range figures {
		m := &figures[i]
		if want&m.is == 0 || m == lone {
			continue
		}
		pr, err := s.texts.query(ctx, base+`SELECT k1, k2, `+spreadColumns(m.col)+`
			FROM base WHERE `+m.col+` IS NOT NULL GROUP BY k1, k2`, args...)
		if err != nil {
			return nil, err
		}
		for pr.Next() {
			var k gk
			var n, at50, at95 int
			var samples string
			if err := pr.Scan(&k.a, &k.b, &n, &at50, &at95, &samples); err != nil {
				pr.Close()
				return nil, err
			}
			if g := groups[k]; g != nil {
				if err := spread(g, m, n, at50, at95, samples); err != nil {
					pr.Close()
					return nil, err
				}
			}
		}
		err = pr.Err()
		pr.Close()
		if err != nil {
			return nil, err
		}
	}

	keyed := make([]gk, 0, len(groups))
	for k := range groups {
		keyed = append(keyed, k)
	}
	// Releases read in the order they were first seen running, because "v1.10"
	// sorts before "v1.9"; every other key reads in name order.
	chronological := len(groupBy) > 0 && groupBy[0] == GroupByControllerVersion
	sort.Slice(keyed, func(i, j int) bool {
		a, b := keyed[i], keyed[j]
		if chronological && first[a] != first[b] {
			return first[a] < first[b]
		}
		if a.a != b.a {
			return a.a < b.a
		}
		return a.b < b.b
	})
	out := &JobStatsResult{}
	for _, k := range keyed {
		if len(out.Groups) == MaxJobStatsGroups {
			out.Truncated = true
			break
		}
		out.Groups = append(out.Groups, *groups[k])
	}
	return out, nil
}

// spreadColumns are the columns that give a percentile of col: how many samples a
// group has, the index of the two that are wanted, which is worked out here so that
// there is one spelling of the rounding rule, and the samples in order.
func spreadColumns(col string) string {
	return `COUNT(` + col + `), CAST((COUNT(` + col + `) - 1) * 0.5 + 0.5 AS INTEGER), CAST((COUNT(` + col + `) - 1) * 0.95 + 0.5 AS INTEGER),
		COALESCE(group_concat(` + col + `, ',' ORDER BY ` + col + `), '')`
}

// samplesAt reads the samples at indexes i and j, i <= j, from the comma-separated
// integers of a group, which SQLite has put in order. It looks at no other sample
// than the two, so what it costs is a pass over the text and not a parse of it.
func samplesAt(samples string, i, j int) (atI, atJ int64, err error) {
	for at := 0; at <= j; at++ {
		item, rest, more := strings.Cut(samples, ",")
		if at == i || at == j {
			v, err := strconv.ParseInt(item, 10, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("store: sample %d of a job statistic is not a whole number of milliseconds: %w", at, err)
			}
			if at == i {
				atI = v
			}
			if at == j {
				atJ = v
			}
		}
		if !more && at < j {
			return 0, 0, fmt.Errorf("store: a job statistic has %d samples, and was asked for sample %d", at+1, j)
		}
		samples = rest
	}
	return atI, atJ, nil
}
