package controller

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/eyupio/zoomies/internal/analysis"
)

// Workflow evidence is a response-only whitelist over validated retained inputs.
// It never enriches an old record with current files or changes report identity.
type ExperimentWorkflowEvidence struct {
	Version   string                        `json:"version"`
	Baseline  ExperimentWorkflowEvidenceArm `json:"baseline"`
	Candidate ExperimentWorkflowEvidenceArm `json:"candidate"`
}
type ExperimentWorkflowEvidenceArm struct {
	SelectedAttempts int                                 `json:"selected_attempts"`
	RootSources      int                                 `json:"root_sources"`
	RowsOmitted      bool                                `json:"rows_omitted"`
	Attempts         []ExperimentWorkflowEvidenceAttempt `json:"attempts"`
}
type ExperimentWorkflowEvidenceAttempt struct {
	RunID              int64                     `json:"run_id,string"`
	Attempt            int                       `json:"attempt"`
	Event              *string                   `json:"event"`
	WorkflowPathSHA256 *string                   `json:"workflow_path_sha256"`
	RootSource         *ExperimentRootSourceView `json:"root_source"`
}
type ExperimentRootSourceView struct {
	Dependencies  *analysis.RootDependencies `json:"dependencies"`
	Basis         string                     `json:"basis"`
	CommitSHA     string                     `json:"commit_sha"`
	BlobSHA       string                     `json:"blob_sha"`
	ContentSHA256 string                     `json:"content_sha256"`
	Bytes         int                        `json:"bytes"`
}

func experimentWorkflowView(a, b *ExperimentJoinedEvidence) *ExperimentWorkflowEvidence {
	return &ExperimentWorkflowEvidence{Version: "workflow-evidence-v1", Baseline: experimentWorkflowArmView(a), Candidate: experimentWorkflowArmView(b)}
}
func experimentWorkflowArmView(e *ExperimentJoinedEvidence) ExperimentWorkflowEvidenceArm {
	remaining := analysis.RootDependencyRows
	out := ExperimentWorkflowEvidenceArm{SelectedAttempts: len(e.Attempts), Attempts: []ExperimentWorkflowEvidenceAttempt{}}
	for _, joined := range e.Attempts {
		r := joined.Remote
		if r.WorkflowSource != nil {
			out.RootSources++
		}
		if len(out.Attempts) >= 100 {
			out.RowsOmitted = true
			continue
		}
		row := ExperimentWorkflowEvidenceAttempt{RunID: r.RunID, Attempt: r.Attempt}
		if r.Event != "" {
			v := r.Event
			row.Event = &v
		}
		if r.WorkflowPath != "" {
			digest := sha256.Sum256([]byte(r.WorkflowPath))
			v := hex.EncodeToString(digest[:])
			row.WorkflowPathSHA256 = &v
		}
		if s := r.WorkflowSource; s != nil {
			row.RootSource = &ExperimentRootSourceView{Dependencies: experimentDependencyView(s.Dependencies, &remaining), Basis: s.Basis, CommitSHA: s.CommitSHA, BlobSHA: s.BlobSHA, ContentSHA256: s.ContentSHA256, Bytes: s.Bytes}
		}
		out.Attempts = append(out.Attempts, row)
	}
	return out
}
