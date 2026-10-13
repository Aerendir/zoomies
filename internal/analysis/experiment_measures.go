package analysis

import (
	"fmt"
	"math"
	"slices"
)

const ExperimentMeasureVersion = "selected-measures-v1"
const ExperimentMaterialRatio = 1.10
const ExperimentFailurePercentagePoints = 5

type ExperimentDistribution struct {
	Samples int    `json:"samples"`
	Missing int    `json:"missing"`
	MinMS   *int64 `json:"min_ms"`
	P50MS   *int64 `json:"p50_ms"`
	P95MS   *int64 `json:"p95_ms"`
	MaxMS   *int64 `json:"max_ms"`
	// A partial sum would look like a smaller bill. Report no total until every
	// relevant attempt is measured, even when the measured subset is zero.
	TotalMS *int64 `json:"total_ms"`
}

type ExperimentMeasures struct {
	Latency       ExperimentDistribution `json:"successful_latency"`
	Execution     ExperimentDistribution `json:"all_attempt_execution"`
	AllocatedIdle ExperimentDistribution `json:"all_attempt_allocated_idle"`
	FailureRate   *float64               `json:"failure_rate"`
}

type ExperimentMeasurement struct {
	Version       string                `json:"version"`
	Eligibility   ExperimentEligibility `json:"eligibility"`
	Baseline      ExperimentMeasures    `json:"baseline"`
	Candidate     ExperimentMeasures    `json:"candidate"`
	LatencyChange string                `json:"latency_change"`
	// Consumption ranks the mean per selected attempt, not totals from cohorts
	// of different sizes. Total consumption and sample counts remain visible.
	ExecutionChange     string `json:"execution_mean_change"`
	AllocatedIdleChange string `json:"allocated_idle_mean_change"`
	FailureChange       string `json:"failure_rate_change"`
}

// MeasureExperiment reports descriptive figures from supplied evidence. It
// supplies no confidence interval, causal inference or cost attribution.
func MeasureExperiment(s ExperimentSnapshot) (ExperimentMeasurement, error) {
	eligibility, err := AssessExperiment(s)
	r := ExperimentMeasurement{Version: ExperimentMeasureVersion, Eligibility: eligibility, LatencyChange: NotEnough, ExecutionChange: NotEnough, AllocatedIdleChange: NotEnough, FailureChange: NotEnough}
	if err != nil {
		return r, err
	}
	r.Baseline, err = measureExperimentArm(s.Baseline, eligibility.Baseline)
	if err != nil {
		return r, err
	}
	r.Candidate, err = measureExperimentArm(s.Candidate, eligibility.Candidate)
	if err != nil {
		return r, err
	}
	if eligibility.State != "eligible" {
		return r, nil
	}
	a, b := r.Baseline, r.Candidate
	if a.Latency.Samples >= MinExperimentSamples && b.Latency.Samples >= MinExperimentSamples && a.Latency.Missing == 0 && b.Latency.Missing == 0 {
		r.LatencyChange = experimentDirection(float64(*a.Latency.P50MS), float64(*b.Latency.P50MS))
	}
	r.ExecutionChange = experimentConsumptionDirection(a.Execution, b.Execution)
	r.AllocatedIdleChange = experimentConsumptionDirection(a.AllocatedIdle, b.AllocatedIdle)
	if a.FailureRate != nil && b.FailureRate != nil {
		ac, bc := eligibility.Baseline, eligibility.Candidate
		// Integer cross-products preserve the exact five-point boundary.
		delta := int64(bc.Failed*ac.Completed-ac.Failed*bc.Completed) * 100
		bar := int64(ac.Completed*bc.Completed) * ExperimentFailurePercentagePoints
		switch {
		case delta >= bar:
			r.FailureChange = Regressed
		case delta <= -bar:
			r.FailureChange = Improved
		default:
			r.FailureChange = Unchanged
		}
	}
	return r, nil
}

func measureExperimentArm(arm ExperimentArm, counts ExperimentCounts) (ExperimentMeasures, error) {
	var latency, execution, idle []*int64
	for _, a := range arm.Attempts {
		for _, v := range []*int64{a.LatencyMS, a.ExecutionMS, a.AllocatedIdleMS} {
			if v != nil && *v < 0 {
				return ExperimentMeasures{}, fmt.Errorf("record non-negative attempt measurements")
			}
		}
		if a.Outcome == "success" {
			latency = append(latency, a.LatencyMS)
		}
		execution = append(execution, a.ExecutionMS)
		idle = append(idle, a.AllocatedIdleMS)
	}
	var r ExperimentMeasures
	var err error
	r.Latency, err = experimentDistribution(latency)
	if err != nil {
		return r, err
	}
	r.Execution, err = experimentDistribution(execution)
	if err != nil {
		return r, err
	}
	r.AllocatedIdle, err = experimentDistribution(idle)
	if err != nil {
		return r, err
	}
	if counts.Completed > 0 {
		rate := float64(counts.Failed) / float64(counts.Completed)
		r.FailureRate = &rate
	}
	return r, nil
}

func experimentDistribution(values []*int64) (ExperimentDistribution, error) {
	r := ExperimentDistribution{}
	observed := make([]int64, 0, len(values))
	var total int64
	for _, v := range values {
		if v == nil {
			r.Missing++
			continue
		}
		if *v > math.MaxInt64-total {
			return r, fmt.Errorf("attempt measurement total exceeds supported milliseconds")
		}
		total += *v
		observed = append(observed, *v)
	}
	r.Samples = len(observed)
	if len(observed) == 0 {
		return r, nil
	}
	slices.Sort(observed)
	copyValue := func(v int64) *int64 { return &v }
	r.MinMS = copyValue(observed[0])
	r.MaxMS = copyValue(observed[len(observed)-1])
	// Nearest rank is ceil(p*n)-1. Integer arithmetic fixes the convention for
	// even sample counts and avoids a library-dependent interpolation rule.
	r.P50MS = copyValue(observed[(50*len(observed)+99)/100-1])
	r.P95MS = copyValue(observed[(95*len(observed)+99)/100-1])
	if r.Missing == 0 {
		r.TotalMS = copyValue(total)
	}
	return r, nil
}

func experimentConsumptionDirection(a, b ExperimentDistribution) string {
	if a.TotalMS == nil || b.TotalMS == nil || a.Samples < MinExperimentSamples || b.Samples < MinExperimentSamples {
		return NotEnough
	}
	return experimentDirection(float64(*a.TotalMS)/float64(a.Samples), float64(*b.TotalMS)/float64(b.Samples))
}
func experimentDirection(a, b float64) string {
	switch {
	case b > a && (a == 0 || b/a >= ExperimentMaterialRatio):
		return Regressed
	case a > b && (b == 0 || a/b >= ExperimentMaterialRatio):
		return Improved
	default:
		return Unchanged
	}
}
