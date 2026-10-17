package controller

import "time"

const experimentRunnerViewVersion = "runner-evidence-v1"
const maxExperimentRunnerViewJobs = 100

// This response projection is separate from the stored report. Older envelopes
// still replay byte-for-byte reports; descriptive facts come only from their
// validated retained inputs, never a current runner or configuration lookup.
type ExperimentRunnerEvidence struct {
	Version   string                      `json:"version"`
	Baseline  ExperimentRunnerEvidenceArm `json:"baseline"`
	Candidate ExperimentRunnerEvidenceArm `json:"candidate"`
}
type ExperimentRunnerEvidenceArm struct {
	SelectedJobs       int                           `json:"selected_jobs"`
	MatchedJobs        int                           `json:"matched_jobs"`
	RecordedGrants     int                           `json:"recorded_grants"`
	RunnerObservations int                           `json:"runner_observations"`
	RowsOmitted        bool                          `json:"rows_omitted"`
	Jobs               []ExperimentRunnerEvidenceJob `json:"jobs"`
}
type ExperimentRunnerEvidenceJob struct {
	RunID         int64                            `json:"run_id,string"`
	Attempt       int                              `json:"attempt"`
	GitHubJobID   int64                            `json:"github_job_id,string"`
	FleetMatched  bool                             `json:"fleet_matched"`
	RecordedGrant *ExperimentRecordedGrantView     `json:"recorded_grant"`
	Observation   *ExperimentRunnerObservationView `json:"observation"`
}
type ExperimentRecordedGrantView struct {
	Basis    string   `json:"basis"`
	Source   string   `json:"source"`
	CPUs     *float64 `json:"cpus"`
	MemoryMB *int64   `json:"memory_mb"`
}

type ExperimentRunnerObservationView struct {
	SHA256                  string                      `json:"sha256"`
	IssuedAt                time.Time                   `json:"issued_at"`
	ReceivedAt              time.Time                   `json:"received_at"`
	Requested               ExperimentRunnerRequestView `json:"requested"`
	ImageDigest             *string                     `json:"image_digest"`
	DigestCoverage          string                      `json:"digest_coverage"`
	AgentCompletedAt        *time.Time                  `json:"agent_completed_at"`
	AgentContainerStartedAt *time.Time                  `json:"agent_container_started_at"`
}

// Keep an explicit whitelist even when the private store type gains fields.
// The response deliberately omits host, pool, runner, task and actor identities.
type ExperimentRunnerRequestView struct {
	Backend              string   `json:"backend"`
	ImageReferenceSHA256 string   `json:"image_reference_sha256"`
	CPUs                 *float64 `json:"cpus"`
	MemoryMB             *int64   `json:"memory_mb"`
	ResourceSource       string   `json:"resource_source"`
	DockerMode           string   `json:"docker_mode"`
	CPUTyped             bool     `json:"cpu_typed"`
	MemoryTyped          bool     `json:"memory_typed"`
}

func experimentRunnerView(a, b *ExperimentJoinedEvidence) *ExperimentRunnerEvidence {
	return &ExperimentRunnerEvidence{Version: experimentRunnerViewVersion, Baseline: experimentRunnerArmView(a), Candidate: experimentRunnerArmView(b)}
}
func experimentRunnerArmView(e *ExperimentJoinedEvidence) ExperimentRunnerEvidenceArm {
	out := ExperimentRunnerEvidenceArm{Jobs: []ExperimentRunnerEvidenceJob{}}
	for _, attempt := range e.Attempts {
		matched := map[int64]ExperimentMatchedJob{}
		for _, j := range attempt.MatchedJobs {
			matched[j.GitHubJobID] = j
		}
		for _, remote := range attempt.Remote.Jobs {
			out.SelectedJobs++
			j, ok := matched[remote.ID]
			if ok {
				out.MatchedJobs++
			}
			if j.RecordedGrant != nil {
				out.RecordedGrants++
			}
			if j.RunnerCreation != nil {
				out.RunnerObservations++
			}
			if len(out.Jobs) >= maxExperimentRunnerViewJobs {
				out.RowsOmitted = true
				continue
			}
			row := ExperimentRunnerEvidenceJob{RunID: attempt.Remote.RunID, Attempt: attempt.Remote.Attempt, GitHubJobID: remote.ID, FleetMatched: ok}
			if g := cloneExperimentGrant(j.RecordedGrant); g != nil {
				row.RecordedGrant = &ExperimentRecordedGrantView{Basis: g.Basis, Source: g.Source, CPUs: g.CPUs, MemoryMB: g.MemoryMB}
			}
			if o := cloneExperimentRunnerCreation(j.RunnerCreation); o != nil {
				d := o.Data
				row.Observation = &ExperimentRunnerObservationView{SHA256: o.SHA256, IssuedAt: d.IssuedAt, ReceivedAt: o.ReceivedAt, Requested: ExperimentRunnerRequestView{Backend: string(d.Issued.Backend), ImageReferenceSHA256: d.Issued.ImageReferenceSHA256, CPUs: d.Issued.CPUs, MemoryMB: d.Issued.MemoryMB, ResourceSource: d.Issued.ResourceSource, DockerMode: string(d.Issued.DockerMode), CPUTyped: d.Issued.CPUTyped, MemoryTyped: d.Issued.MemoryTyped}, ImageDigest: d.Reported.ImageDigest, DigestCoverage: d.Reported.DigestCoverage, AgentCompletedAt: d.Reported.CompletedAt, AgentContainerStartedAt: d.Reported.ContainerStartedAt}
			}
			out.Jobs = append(out.Jobs, row)
		}
	}
	return out
}
