package mcp

import (
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/eyupio/zoomies/internal/analysis"
)

// Independent REST whitelists keep private and future fields out of model input.
type comparisonWorkflowEvidence struct {
	Version   string                 `json:"version"`
	Baseline  *comparisonWorkflowArm `json:"baseline"`
	Candidate *comparisonWorkflowArm `json:"candidate"`
}
type comparisonWorkflowArm struct {
	SelectedAttempts *int                        `json:"selected_attempts"`
	RootSources      *int                        `json:"root_sources"`
	RowsOmitted      *bool                       `json:"rows_omitted"`
	Shown            int                         `json:"shown"`
	Attempts         []comparisonWorkflowAttempt `json:"attempts"`
}
type comparisonWorkflowAttempt struct {
	RunID              string                `json:"run_id"`
	Attempt            int                   `json:"attempt"`
	Event              *string               `json:"event"`
	WorkflowPathSHA256 *string               `json:"workflow_path_sha256"`
	RootSource         *comparisonRootSource `json:"root_source"`
}
type comparisonRootSource struct {
	Dependencies  *analysis.RootDependencies `json:"dependencies"`
	Basis         string                     `json:"basis"`
	CommitSHA     string                     `json:"commit_sha"`
	BlobSHA       string                     `json:"blob_sha"`
	ContentSHA256 string                     `json:"content_sha256"`
	Bytes         int                        `json:"bytes"`
}

func validateComparisonWorkflows(e *comparisonWorkflowEvidence, a, b analysis.ExperimentArm) bool {
	if e == nil {
		return true
	}
	if e.Version != "workflow-evidence-v1" || e.Baseline == nil || e.Candidate == nil || len(a.Attempts) > 100 || len(b.Attempts) > 100 {
		return false
	}
	seen := map[string]bool{}
	for i, side := range []*comparisonWorkflowArm{e.Baseline, e.Candidate} {
		arm := a
		if i == 1 {
			arm = b
		}
		if side.SelectedAttempts == nil || side.RootSources == nil || side.RowsOmitted == nil || side.Attempts == nil || len(side.Attempts) > 100 {
			return false
		}
		n, roots := *side.SelectedAttempts, *side.RootSources
		if n != len(arm.Attempts) || n < len(side.Attempts) || roots < 0 || roots > n || *side.RowsOmitted != (n > len(side.Attempts)) {
			return false
		}
		revisions := map[string]string{}
		for _, v := range arm.Attempts {
			revisions[strconv.FormatInt(v.RunID, 10)+":"+strconv.Itoa(v.Attempt)] = v.SourceRevision
		}
		shownRoots := 0
		dependencyRows := 0
		for _, row := range side.Attempts {
			key := row.RunID + ":" + strconv.Itoa(row.Attempt)
			revision, ok := revisions[key]
			if !comparisonDecimalID(row.RunID) || row.Attempt <= 0 || !ok || seen[key] {
				return false
			}
			seen[key] = true
			if row.Event != nil && !comparisonWorkflowEvent(*row.Event) {
				return false
			}
			if row.WorkflowPathSHA256 != nil && !comparisonHash(*row.WorkflowPathSHA256) {
				return false
			}
			if s := row.RootSource; s != nil {
				if row.Event == nil || *row.Event != "push" || row.WorkflowPathSHA256 == nil || s.Basis != "push_head_sha_root_file" || !comparisonWorkflowSHA(s.CommitSHA) || s.CommitSHA != revision || !comparisonWorkflowSHA(s.BlobSHA) || !comparisonHash(s.ContentSHA256) || s.Bytes < 1 || s.Bytes > 128*1024 {
					return false
				}
				if !analysis.ValidRootDependencies(s.Dependencies, s.ContentSHA256) {
					return false
				}
				if s.Dependencies != nil {
					dependencyRows += len(s.Dependencies.References)
				}
				if dependencyRows > analysis.RootDependencyRows {
					return false
				}
				shownRoots++
			}
		}
		if roots < shownRoots || roots-shownRoots > n-len(side.Attempts) {
			return false
		}
		// Shown describes the locally selected rows, never an upstream assertion.
		side.Shown = len(side.Attempts)
	}
	return true
}
func comparisonWorkflowSHA(s string) bool {
	_, err := hex.DecodeString(s)
	return len(s) == 40 && err == nil && s == strings.ToLower(s)
}
func comparisonWorkflowEvent(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}
