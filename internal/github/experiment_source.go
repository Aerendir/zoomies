package github

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	gh "github.com/google/go-github/v88/github"
)

const ExperimentWorkflowSourceBytes = 128 * 1024

// ExperimentWorkflowSource describes only the root file read at a push commit.
// Its digest does not cover reusable workflows, actions, inputs or test scope.
type ExperimentWorkflowSource struct {
	Version       string `json:"version"`
	Basis         string `json:"basis"`
	CommitSHA     string `json:"commit_sha"`
	Path          string `json:"path"`
	BlobSHA       string `json:"blob_sha"`
	ContentSHA256 string `json:"content_sha256"`
	Bytes         int    `json:"bytes"`
}

func experimentSourcePath(e *ExperimentAttemptEvidence) string {
	if e == nil || e.Event != "push" || !validDelaySHA(e.HeadSHA) {
		return ""
	}
	p := e.WorkflowPath
	// Mutable @refs and ambiguous paths must not be guessed from HeadSHA.
	if strings.Contains(p, "@") || strings.ContainsAny(p, "\\\\?#%") || path.Clean(p) != p || path.Dir(p) != ".github/workflows" || !(strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml")) || !ValidExperimentAttemptContext(e.Event, p) {
		return ""
	}
	return p
}

func (c *appClient) readExperimentSource(ctx context.Context, owner, repo string, e *ExperimentAttemptEvidence) (*ExperimentWorkflowSource, error) {
	p := experimentSourcePath(e)
	if p == "" {
		return nil, nil
	}
	// GetContents renders JSON inline; no download_url or branch lookup is followed.
	f, directory, resp, err := c.asInstallation.Repositories.GetContents(ctx, owner, repo, p, &gh.RepositoryContentGetOptions{Ref: e.HeadSHA})
	if err != nil {
		return nil, c.fail("read comparison workflow source at its push commit", resp, err)
	}
	raw := ""
	if f != nil && f.Content != nil {
		raw = *f.Content
	}
	if f == nil || directory != nil || f.GetType() != "file" || f.GetPath() != p || f.GetEncoding() != "base64" || f.GetSize() <= 0 || f.GetSize() > ExperimentWorkflowSourceBytes || len(raw) > ExperimentWorkflowSourceBytes*4/3+4096 || f.GetSubmoduleGitURL() != "" || f.GetTarget() != "" {
		return nil, fmt.Errorf("github: comparison workflow source has invalid identity, type, size or encoding")
	}
	content, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(raw, "\n", ""))
	if err != nil || len(content) != f.GetSize() || !utf8.Valid(content) || contextGitBlobSHA(string(content)) != f.GetSHA() {
		return nil, fmt.Errorf("github: comparison workflow source failed content integrity validation")
	}
	digest := sha256.Sum256(content)
	return &ExperimentWorkflowSource{Version: "workflow-source-v1", Basis: "push_head_sha_root_file", CommitSHA: e.HeadSHA, Path: p, BlobSHA: f.GetSHA(), ContentSHA256: hex.EncodeToString(digest[:]), Bytes: len(content)}, nil
}

// ValidExperimentWorkflowSource checks the retained association and digest forms.
// Collection checks bytes; replay has only the retained digests, not source text.
func ValidExperimentWorkflowSource(e *ExperimentAttemptEvidence) bool {
	if e == nil {
		return false
	}
	s := e.WorkflowSource
	if s == nil {
		return true
	}
	validHash := func(v string, n int) bool {
		_, err := hex.DecodeString(v)
		return len(v) == n && err == nil && v == strings.ToLower(v)
	}
	return s.Version == "workflow-source-v1" && s.Basis == "push_head_sha_root_file" && experimentSourcePath(e) != "" && s.Path == e.WorkflowPath && s.CommitSHA == e.HeadSHA && s.Bytes > 0 && s.Bytes <= ExperimentWorkflowSourceBytes && validHash(s.BlobSHA, 40) && validHash(s.ContentSHA256, 64)
}
