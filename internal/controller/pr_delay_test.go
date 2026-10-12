package controller

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/analysis"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

func prDelayInputFixture() PRDelayInput {
	start := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	started, done := start.Add(2*time.Minute), start.Add(3*time.Minute)
	return PRDelayInput{Repository: "acme/repo", InstallationID: "ins_one", WindowStart: start,
		ObservedAt: start.Add(10 * time.Minute), JobsCoverage: analysis.CoverageComplete,
		Evidence: &github.PRDelayEvidence{Number: 1, HeadSHA: "head", CurrentHeadSHA: "head",
			TargetBranch: "release", CurrentTargetBranch: "release", Open: true, StillOpen: true,
			ProtectionCoverage: "complete", ChecksCoverage: "complete",
			Protection: &github.KennelProtection{Required: []string{"private test name"}},
			Checks: []github.PRDelayCheck{{ID: 1, AppID: github.GitHubActionsAppID,
				Name: "private test name", WorkflowRunID: 20, WorkflowJobID: 30,
				HeadSHA: "head", Status: "completed", Conclusion: "success", CompletedAt: &done}}},
		Jobs: []PRDelayJobEvidence{{Job: &store.Job{ID: "job_one", GitHubJobID: 30, GitHubRunID: 20,
			Repo: "acme/repo", InstallationID: "ins_one", HeadSHA: "head", JobName: "private test name",
			RunAttempt: 1, QueuedAt: start, StartedAt: &started, CompletedAt: &done, Conclusion: "success"}}}}
}

func TestPRDelayAssemblyChecksNumericIdentityAndReturnsNoCheckNames(t *testing.T) {
	r, err := AnalysePRDelayEvidence(prDelayInputFixture())
	if err != nil {
		t.Fatal(err)
	}
	if r.State != "checks_passed" || len(r.Checks) != 1 || r.Checks[0].CheckID != "chk_001" ||
		*r.Checks[0].Timing.PreStartMS != 120000 || r.Checks[0].Timing.QueueMS != nil {
		t.Fatalf("got %+v", r)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "private test name") {
		t.Fatalf("exposed a check name: %s", b)
	}
}

func TestPRDelayAssemblyDoesNotBorrowEvidenceFromAnotherJobOrInstallation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*PRDelayInput)
	}{
		{"other installation", func(in *PRDelayInput) { in.Jobs[0].Job.InstallationID = "ins_other" }},
		{"other repository", func(in *PRDelayInput) { in.Jobs[0].Job.Repo = "other/repo" }},
		{"other head", func(in *PRDelayInput) { in.Jobs[0].Job.HeadSHA = "old" }},
		{"other run", func(in *PRDelayInput) { in.Jobs[0].Job.GitHubRunID = 21 }},
		{"other job", func(in *PRDelayInput) { in.Jobs[0].Job.GitHubJobID = 31 }},
		{"other name", func(in *PRDelayInput) { in.Jobs[0].Job.JobName = "unrelated" }},
		{"other producer", func(in *PRDelayInput) { in.Evidence.Checks[0].AppID = 99 }},
		{"unverified link", func(in *PRDelayInput) { in.Evidence.Checks[0].WorkflowJobID = 0 }},
		{"duplicate record", func(in *PRDelayInput) { in.Jobs = append(in.Jobs, in.Jobs[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := prDelayInputFixture()
			tc.change(&in)
			r, err := AnalysePRDelayEvidence(in)
			if err != nil || r.State != "incomplete" || r.Checks[0].State != "missing" {
				t.Fatalf("got %+v, %v", r, err)
			}
		})
	}
}

func TestPRDelayAssemblyKeepsNonActionsRequirementsAndPartialReadsVisible(t *testing.T) {
	in := prDelayInputFixture()
	in.Evidence.Protection.Unpinned = 1
	r, err := AnalysePRDelayEvidence(in)
	if err != nil || r.State != "incomplete" || len(r.Checks) != 2 || r.Checks[1].State != "unmapped" {
		t.Fatalf("got %+v, %v", r, err)
	}
	in = prDelayInputFixture()
	in.JobsCoverage = analysis.CoveragePartial
	r, err = AnalysePRDelayEvidence(in)
	if err != nil || r.State != "incomplete" || r.ObservationCoverage != analysis.CoveragePartial {
		t.Fatalf("got %+v, %v", r, err)
	}
	in = prDelayInputFixture()
	in.Evidence.Protection = nil
	r, err = AnalysePRDelayEvidence(in)
	if err != nil || r.State != "incomplete" || r.ProtectionCoverage != analysis.CoverageFailed {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestPRDelayAssemblyDoesNotPresentRacingJobAndCheckResultsAsAPass(t *testing.T) {
	for _, change := range []func(*PRDelayInput){
		func(in *PRDelayInput) { in.Jobs[0].Job.Conclusion = "failure" },
		func(in *PRDelayInput) { in.Evidence.Checks[0].Status = "in_progress" },
		func(in *PRDelayInput) { in.Evidence.Checks[0].Conclusion = "unknown" },
		func(in *PRDelayInput) { in.Jobs[0].Job.CompletedAt = nil },
	} {
		in := prDelayInputFixture()
		change(&in)
		r, err := AnalysePRDelayEvidence(in)
		if err != nil || r.State != "incomplete" || r.Checks[0].State == "passed" {
			t.Fatalf("got %+v, %v", r, err)
		}
	}
}

func TestPRDelayAssemblyPreservesKnownStartupWithoutCountingItTwice(t *testing.T) {
	in := prDelayInputFixture()
	provisioned := in.WindowStart.Add(time.Minute)
	in.Jobs[0].ProvisionedAt = &provisioned
	r, err := AnalysePRDelayEvidence(in)
	if err != nil {
		t.Fatal(err)
	}
	timing := r.Checks[0].Timing
	if *timing.QueueMS != 60000 || *timing.StartupMS != 60000 || *timing.PreStartMS != 120000 {
		t.Fatalf("got %+v", timing)
	}
}

func TestPRDelayAssemblyIncludesCheckReportingDelayButPreservesJobExecution(t *testing.T) {
	in := prDelayInputFixture()
	completed := in.WindowStart.Add(4 * time.Minute)
	in.Evidence.Checks[0].CompletedAt = &completed
	r, err := AnalysePRDelayEvidence(in)
	if err != nil || *r.Checks[0].CompletionDelayMS != 240000 || *r.Checks[0].Timing.ExecutionMS != 60000 {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestPRDelayAssemblyRefusesRetargetedAndClosedPRs(t *testing.T) {
	in := prDelayInputFixture()
	in.Evidence.CurrentTargetBranch = "main"
	if _, err := AnalysePRDelayEvidence(in); !errors.Is(err, ErrPRDelayTargetChanged) {
		t.Fatal(err)
	}
	in = prDelayInputFixture()
	in.Evidence.StillOpen = false
	if _, err := AnalysePRDelayEvidence(in); !errors.Is(err, ErrPRDelayClosed) {
		t.Fatal(err)
	}
	in = prDelayInputFixture()
	in.Evidence.CurrentHeadSHA = ""
	r, err := AnalysePRDelayEvidence(in)
	if err != nil || r.State != "currentness_unknown" {
		t.Fatalf("got %+v, %v", r, err)
	}
	in = prDelayInputFixture()
	in.Evidence.CurrentHeadSHA = "new"
	r, err = AnalysePRDelayEvidence(in)
	if err != nil || r.State != "superseded" {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestPRDelayAssemblyRefusesUnboundedOrMalformedRequirements(t *testing.T) {
	for _, change := range []func(*PRDelayInput){
		func(in *PRDelayInput) { in.InstallationID = "" },
		func(in *PRDelayInput) { in.Evidence.Protection.Unpinned = -1 },
		func(in *PRDelayInput) { in.Evidence.Protection.Unpinned = analysis.MaxRequiredChecks },
		func(in *PRDelayInput) { in.Evidence.Protection.Required = []string{"same", "same"} },
		func(in *PRDelayInput) { in.Jobs = make([]PRDelayJobEvidence, analysis.MaxCheckObservations+1) },
	} {
		in := prDelayInputFixture()
		change(&in)
		if _, err := AnalysePRDelayEvidence(in); !errors.Is(err, ErrPRDelayEvidence) {
			t.Fatal(err)
		}
	}
}
