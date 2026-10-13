package analysis

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// A version is fixed before looking at a cohort; changing the sample rule
// after seeing its failures would let a caller choose the answer.
const ExperimentRuleVersion = "selected-runs-v1"
const MinExperimentSamples = 30
const MaxExperimentAttempts = 2000

type ExperimentAttempt struct {
	// Nil is missing evidence; zero is an observed zero. Consumption stays
	// attached to failed and excluded attempts rather than disappearing.
	LatencyMS       *int64    `json:"latency_ms"`
	ExecutionMS     *int64    `json:"execution_ms"`
	AllocatedIdleMS *int64    `json:"allocated_idle_ms"`
	RunID           int64     `json:"run_id"`
	Attempt         int       `json:"attempt"`
	ObservedAt      time.Time `json:"observed_at"`
	Outcome         string    `json:"outcome"`
	Cache           string    `json:"cache"`
	HostCondition   string    `json:"host_condition"`
}

type ExperimentArm struct {
	SourceRevision   string `json:"source_revision"`
	WorkflowRevision string `json:"workflow_revision"`
	InputsID         string `json:"inputs_id"`
	TestScopeID      string `json:"test_scope_id"`
	ConfigurationID  string `json:"configuration_id"`
	ImageID          string `json:"image_id"`
	ResourceID       string `json:"resource_id"`
	// The controller must verify this against its retained selection, including
	// failed attempts. The pure calculation cannot discover omitted runs.
	SelectionComplete bool                `json:"selection_complete"`
	Attempts          []ExperimentAttempt `json:"attempts"`
}

type ExperimentSnapshot struct {
	RuleVersion     string        `json:"rule_version"`
	Kind            string        `json:"kind"`
	Repository      string        `json:"repository"`
	InitiatingUser  string        `json:"initiating_user"`
	Hypothesis      string        `json:"hypothesis"`
	IntendedDelta   string        `json:"intended_delta"`
	EvidenceID      string        `json:"evidence_id"`
	RevisionDeltaID string        `json:"revision_delta_id"`
	Since           time.Time     `json:"since"`
	Until           time.Time     `json:"until"`
	Baseline        ExperimentArm `json:"baseline"`
	Candidate       ExperimentArm `json:"candidate"`
}

type ExperimentCounts struct {
	Selected   int `json:"selected"`
	Completed  int `json:"completed"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	Cancelled  int `json:"cancelled"`
	Superseded int `json:"superseded"`
	Unknown    int `json:"unknown"`
}

type ExperimentEligibility struct {
	RuleVersion    string           `json:"rule_version"`
	State          string           `json:"state"`
	Reasons        []string         `json:"reasons"`
	MinimumSamples int              `json:"minimum_samples"`
	Baseline       ExperimentCounts `json:"baseline"`
	Candidate      ExperimentCounts `json:"candidate"`
}

// AssessExperiment classifies supplied historical evidence. Eligibility is
// permission to compare figures, not proof of causation, improvement or safety.
// The caller retains the snapshot beside this result for reproducibility.
func AssessExperiment(s ExperimentSnapshot) (ExperimentEligibility, error) {
	r := ExperimentEligibility{RuleVersion: ExperimentRuleVersion, State: "eligible", Reasons: []string{}, MinimumSamples: MinExperimentSamples}
	if s.RuleVersion != ExperimentRuleVersion {
		return r, fmt.Errorf("use comparison rule %s", ExperimentRuleVersion)
	}
	if s.Kind != "runner" && s.Kind != "workflow" {
		return r, fmt.Errorf("choose runner or workflow comparison")
	}
	for _, value := range []string{s.Repository, s.InitiatingUser, s.Hypothesis, s.IntendedDelta, s.EvidenceID} {
		if strings.TrimSpace(value) == "" || len(value) > 4096 {
			return r, fmt.Errorf("retain the repository, initiating user, hypothesis, intended delta and evidence identifier within 4096 bytes each")
		}
	}
	if s.Since.IsZero() || !s.Until.After(s.Since) {
		return r, fmt.Errorf("provide an increasing observation window")
	}
	reasons := map[string]bool{}
	if s.Baseline.InputsID != s.Candidate.InputsID {
		reasons["inputs_mismatch"] = true
	}
	if s.Baseline.TestScopeID != s.Candidate.TestScopeID {
		reasons["test_scope_mismatch"] = true
	}
	if s.Kind == "runner" {
		if s.Baseline.SourceRevision != s.Candidate.SourceRevision {
			reasons["source_mismatch"] = true
		}
		if s.Baseline.WorkflowRevision != s.Candidate.WorkflowRevision {
			reasons["workflow_mismatch"] = true
		}
	} else if strings.TrimSpace(s.RevisionDeltaID) == "" {
		reasons["revision_delta_missing"] = true
	}
	if len(s.RevisionDeltaID) > 4096 {
		return r, fmt.Errorf("retain a revision delta identifier within 4096 bytes")
	}
	// Identity includes attempt number: a retry consumes resources and is not
	// erased by the success of a later attempt of the same run.
	seen := map[[2]int64]bool{}
	caches := [2]string{}
	for index, arm := range []ExperimentArm{s.Baseline, s.Candidate} {
		for _, value := range []string{arm.SourceRevision, arm.WorkflowRevision, arm.InputsID, arm.TestScopeID, arm.ConfigurationID, arm.ImageID, arm.ResourceID} {
			if strings.TrimSpace(value) == "" || len(value) > 4096 {
				return r, fmt.Errorf("retain each arm's revision, workload, configuration, image and resource identifiers within 4096 bytes")
			}
		}
		if len(arm.Attempts) > MaxExperimentAttempts {
			return r, fmt.Errorf("select at most %d attempts per arm", MaxExperimentAttempts)
		}
		if !arm.SelectionComplete {
			reasons["selection_incomplete"] = true
		}
		counts := ExperimentCounts{Selected: len(arm.Attempts)}
		for _, a := range arm.Attempts {
			key := [2]int64{a.RunID, int64(a.Attempt)}
			if a.RunID <= 0 || a.Attempt <= 0 || seen[key] {
				return r, fmt.Errorf("select each positive run and attempt identity once across both arms")
			}
			seen[key] = true
			if a.ObservedAt.Before(s.Since) || !a.ObservedAt.Before(s.Until) {
				return r, fmt.Errorf("select attempts inside the observation window")
			}
			switch a.Outcome {
			case "success":
				counts.Succeeded++
				counts.Completed++
			case "failure":
				counts.Failed++
				counts.Completed++
			case "cancelled":
				counts.Cancelled++
			case "superseded":
				counts.Superseded++
			case "pending", "unknown":
				counts.Unknown++
				reasons["outcome_unknown"] = true
			default:
				return r, fmt.Errorf("use a documented attempt outcome")
			}
			// Excluded attempts still need retained identities and outcomes, but their
			// cache and host state cannot establish the completed cohort's eligibility.
			if a.Outcome != "success" && a.Outcome != "failure" {
				continue
			}
			switch a.Cache {
			case "cold", "warm":
				if caches[index] == "" {
					caches[index] = a.Cache
				} else if caches[index] != a.Cache {
					reasons["cache_mismatch"] = true
				}
			case "", "unknown":
				reasons["cache_unknown"] = true
			default:
				return r, fmt.Errorf("use cold, warm or unknown cache evidence")
			}
			switch a.HostCondition {
			case "quiet":
			case "busy":
				reasons["host_pressure"] = true
			case "", "unknown":
				reasons["host_unknown"] = true
			default:
				return r, fmt.Errorf("use quiet, busy or unknown host evidence")
			}
		}
		if counts.Completed < MinExperimentSamples {
			reasons["insufficient_samples"] = true
		}
		if index == 0 {
			r.Baseline = counts
		} else {
			r.Candidate = counts
		}
	}
	if caches[0] != "" && caches[1] != "" && caches[0] != caches[1] {
		reasons["cache_mismatch"] = true
	}
	for code := range reasons {
		r.Reasons = append(r.Reasons, code)
	}
	slices.Sort(r.Reasons)
	if len(r.Reasons) > 0 {
		r.State = "inconclusive"
	}
	return r, nil
}
