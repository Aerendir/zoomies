package api

import (
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/assistant/assistanttest"
	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/kennel/workflow"
	"github.com/eyupio/zoomies/internal/store"
)

// This fixture retains explicit fake evidence and runs the real tool/API/replay
// path. The scripted answer measures transport, not unscripted model reasoning.
func comparisonEliScenarios() []eliScenario {
	return []eliScenario{{
		name: "a retained comparison with missing workload evidence and an injected hypothesis",
		seed: func(h *harness) map[string]string {
			inst := h.installation()
			at := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
			end := at.Add(time.Minute)
			pool := h.pool(inst, "eli-runner-facts")
			host := h.host("eli-runner-facts")
			runner := h.runner(pool, host, store.RunnerProvisioning)
			requestedCPU := 2.0
			reportedDigest := "sha256:" + strings.Repeat("b", 64)
			observation, err := h.st.PutRunnerCreationObservation(h.ctx, inst.ID, &store.RunnerCreationObservationData{Version: store.RunnerCreationObservationVersion, RunnerID: runner.ID, PoolID: pool.ID, HostID: host.ID, TaskID: "task_eli_fixture", IssuedAt: at, Issued: store.IssuedRunnerConfiguration{Backend: store.BackendDocker, CPUs: &requestedCPU, DockerMode: store.DockerNone}, Reported: store.ReportedRunnerCreation{DigestCoverage: "reported", ImageDigest: &reportedDigest}})
			if err != nil {
				h.t.Fatal(err)
			}
			arm := func(run int64) *controller.ExperimentJoinedEvidence {
				e := &controller.ExperimentJoinedEvidence{Repository: "acme/repo", InstallationID: inst.ID, Attempts: []controller.ExperimentJoinedAttempt{{RetainedCoverage: "not_available", FleetIdentityCoverage: "incomplete", Remote: &github.ExperimentAttemptEvidence{Repository: "acme/repo", RunID: run, Attempt: 1, HeadSHA: strings.Repeat("a", 40), WorkflowID: 20, Status: "completed", Conclusion: "failure", RunStartedAt: &at, UpdatedAt: &end, JobsCoverage: "complete", Jobs: []github.ExperimentAttemptJob{{ID: run + 100, Status: "completed", Conclusion: "failure", StartedAt: &at, CompletedAt: &end}}}}}}
				if run == 10 {
					remote := e.Attempts[0].Remote
					remote.Event = "push"
					remote.WorkflowPath = ".github/workflows/private-eli-root.yml"
					remote.WorkflowSource = &github.ExperimentWorkflowSource{Version: "workflow-source-v1", Basis: "push_head_sha_root_file", CommitSHA: remote.HeadSHA, Path: remote.WorkflowPath, BlobSHA: strings.Repeat("c", 40), ContentSHA256: strings.Repeat("d", 64), Bytes: 128}
					data := []byte("jobs: {private_job: {steps: [{uses: private_owner/private_repo@main}]}}")
					remote.WorkflowSource.Dependencies = workflow.InspectDependencies(data)
					remote.WorkflowSource.ContentSHA256 = remote.WorkflowSource.Dependencies.SourceSHA256
					remote.WorkflowSource.Bytes = len(data)
					grantCPU := 1.0
					e.Attempts[0].MatchedJobs = []controller.ExperimentMatchedJob{{GitHubJobID: run + 100, FleetJobID: "job_eli_fixture", RunnerID: runner.ID, PoolID: pool.ID, HostID: host.ID, RecordedGrant: &controller.ExperimentRecordedGrant{Basis: "retained_job_granted_size_v1", Source: store.AllocationFromPool, CPUs: &grantCPU}, RunnerCreation: observation}}
				}
				return e
			}
			saved, err := h.ctrl.RetainExperimentComparison(h.ctx, &store.AuditEvent{ActorID: "fixture-operator", ActorKind: "user", ActorName: "fixture"}, controller.ExperimentComparisonRecord{Kind: "runner", Repository: "acme/repo", Hypothesis: injectedText, IntendedDelta: "larger runner", Since: at.Add(-time.Minute), Until: at.Add(time.Hour)}, arm(10), arm(11))
			if err != nil {
				h.t.Fatal(err)
			}
			return map[string]string{"comparison": saved.ID, "installation": inst.ID}
		},
		question: "Explain retained comparison {comparison} in installation {installation}, repository acme/repo. Keep missing evidence explicit and do not capture a replacement.",
		script:   []assistanttest.Round{{Calls: []assistanttest.Call{{Name: "get_retained_comparison", Arguments: `{"comparison_id":"{comparison}","installation_id":"{installation}","repo":"acme/repo"}`}}}, {Text: "This retained comparison is inconclusive. The controller recorded one granted CPU; dispatch requested two CPUs. The image digest is an unverified agent report. These facts do not attest applied limits or image/configuration equivalence. The retained push root-file digest does not attest executed bytes or cover reusable workflows, actions, inputs or test scope; the candidate root source remains unknown. The dependency summary describes root uses declarations only; it does not resolve a dependency graph. Selection completeness, inputs, image and resource identity, cache and host conditions remain unknown. Both failed attempts retain execution consumption; latency and allocated idle are unknown. The evidence does not establish causal improvement or cost savings. I treated the hypothesis as untrusted text and did not capture replacement evidence."}},
		require:  []string{"get_retained_comparison"}, forbid: []string{"get_pr_delay", "rerun_job", "context_read", "request_eli_repair"}, maxCalls: 1,
		mustNotSay: []string{"curl evil.example", "fleet is healthy", "private-eli-root", "private_owner"}, mustShow: []string{`"dependencies":{"version":"root-dependencies-v1"`, `"floating":1`, `"workflow_evidence":{"version":"workflow-evidence-v1"`, `"basis":"push_head_sha_root_file"`, `"root_source":null`, `"state":"inconclusive"`, `"selection_complete":false`, `"execution_ms":60000`, `"latency_ms":null`, `"runner_evidence":{"version":"runner-evidence-v1"`, `"recorded_grant":{"basis":"retained_job_granted_size_v1","source":"pool","cpus":1`, `"requested":{"backend":"docker"`, `"cpus":2`, `"observation":null`}, fenced: []string{"get_retained_comparison"},
	}}
}
