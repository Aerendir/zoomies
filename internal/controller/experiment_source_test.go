package controller

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

func experimentSourceFixture(e *github.ExperimentAttemptEvidence) {
	e.Event = "push"
	e.WorkflowPath = ".github/workflows/private-root.yml"
	e.WorkflowSource = &github.ExperimentWorkflowSource{Version: "workflow-source-v1", Basis: "push_head_sha_root_file", CommitSHA: e.HeadSHA, Path: e.WorkflowPath, BlobSHA: strings.Repeat("b", 40), ContentSHA256: strings.Repeat("c", 64), Bytes: 100}
}

func TestRootWorkflowDigestsRetainPrivatelyWithoutPromotingWorkloadEquivalence(t *testing.T) {
	c, record, a, b := retainedComparisonFixture(t)
	old, err := AssembleExperimentComparison(record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	experimentSourceFixture(a.Attempts[0].Remote)
	experimentSourceFixture(b.Attempts[0].Remote)
	b.Attempts[0].Remote.WorkflowSource.ContentSHA256 = strings.Repeat("d", 64)
	current, err := AssembleExperimentComparison(record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(old)
	after, _ := json.Marshal(current)
	if string(before) != string(after) {
		t.Fatal("root source changed eligibility, revisions or measurements")
	}
	saved, err := c.RetainExperimentComparison(context.Background(), &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user"}, record, a, b)
	if err != nil {
		t.Fatal(err)
	}
	row, err := c.st.GetComparisonEvidence(context.Background(), a.InstallationID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var retained experimentRetainedEnvelope
	if err := json.Unmarshal(row.Payload, &retained); err != nil {
		t.Fatal(err)
	}
	if retained.Candidate.Attempts[0].Remote.WorkflowSource.ContentSHA256 != strings.Repeat("d", 64) {
		t.Fatal("private digest not retained")
	}
	a.Attempts[0].Remote.WorkflowSource.Bytes = 999
	replay, err := c.ReplayExperimentComparison(context.Background(), a.InstallationID, record.Repository, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ = json.Marshal(saved)
	after, _ = json.Marshal(replay)
	if string(before) != string(after) || strings.Contains(string(after), "private-root") || strings.Contains(string(after), "content_sha256") {
		t.Fatal("private source leaked or mutable input changed replay")
	}
}

func TestWorkflowSourceBindingIsValidatedForAlternateReaders(t *testing.T) {
	for _, mode := range []string{"commit", "path", "version", "basis", "blob", "digest", "size", "event", "mutable_ref"} {
		t.Run(mode, func(t *testing.T) {
			record, a, b := experimentSnapshotFixture()
			e := b.Attempts[0].Remote
			experimentSourceFixture(e)
			switch mode {
			case "commit":
				e.WorkflowSource.CommitSHA = strings.Repeat("b", 40)
			case "path":
				e.WorkflowSource.Path = ".github/workflows/other.yml"
			case "version":
				e.WorkflowSource.Version = "future"
			case "basis":
				e.WorkflowSource.Basis = "branch"
			case "blob":
				e.WorkflowSource.BlobSHA = "unknown"
			case "digest":
				e.WorkflowSource.ContentSHA256 = "unknown"
			case "size":
				e.WorkflowSource.Bytes = github.ExperimentWorkflowSourceBytes + 1
			case "event":
				e.Event = "pull_request_target"
			case "mutable_ref":
				e.WorkflowPath += "@main"
				e.WorkflowSource.Path = e.WorkflowPath
			}
			if _, err := AssembleExperimentComparison(record, a, b); !errors.Is(err, ErrExperimentEvidence) {
				t.Fatalf("invalid binding accepted: %v", err)
			}
		})
	}
}

func TestComparisonQuotaReservesTheAdditionalRootSourceRead(t *testing.T) {
	h, inst, f := experimentJoinFixture(t)
	f.quota = &github.RateLimit{Limit: 100, Remaining: 57}
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}}); !errors.Is(err, ErrExperimentAdmission) || f.calls != 0 {
		t.Fatalf("underfunded source budget admitted: %v", err)
	}
	f.quota.Remaining = 58
	if _, err := h.c.CollectExperimentEvidence(h.ctx, inst.ID, inst.Target, []ExperimentAttemptSelection{{10, 2}}); err != nil || f.calls != 1 {
		t.Fatalf("funded collection refused: %v", err)
	}
}
