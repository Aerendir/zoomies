package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

// CheckoutSelectionBody names one exact run, attempt, GitHub job and the job and
// step positions in the workflow file. The two GitHub identities travel as
// decimal strings so a browser cannot round them.
type CheckoutSelectionBody struct {
	RunID       string `json:"run_id"`
	Attempt     int    `json:"attempt"`
	GitHubJobID string `json:"github_job_id"`
	JobIndex    int    `json:"job_index"`
	StepIndex   int    `json:"step_index"`
}

type CheckoutInspectionRequest struct {
	InstallationID string                `json:"installation_id"`
	Repository     string                `json:"repository"`
	Selection      CheckoutSelectionBody `json:"selection"`
}

// CheckoutPreviewRequest carries the operator's declaration. The scope of that
// declaration is fixed by the server; the request supplies only the head and
// digest of the source the operator was shown, so a file that moved since is
// refused rather than approved by accident.
type CheckoutPreviewRequest struct {
	InstallationID  string                `json:"installation_id"`
	Repository      string                `json:"repository"`
	Selection       CheckoutSelectionBody `json:"selection"`
	HeadSHA         string                `json:"head_sha"`
	SourceSHA256    string                `json:"source_sha256"`
	HistoryReviewed bool                  `json:"history_reviewed"`
}

type CheckoutInspectionView struct {
	InstallationID string                `json:"installation_id"`
	Repository     string                `json:"repository"`
	Selection      CheckoutSelectionBody `json:"selection"`
	HeadSHA        string                `json:"head_sha"`
	Path           string                `json:"path"`
	Mode           string                `json:"mode"`
	Content        string                `json:"content"`
	SourceSHA256   string                `json:"source_sha256"`
	StepNumber     int64                 `json:"step_number"`
	CheckoutMS     int64                 `json:"checkout_ms"`
	EvidenceID     string                `json:"evidence_id"`
	Basis          string                `json:"basis"`
	InspectedAt    time.Time             `json:"inspected_at"`
}

type CheckoutCollectionView struct {
	Version       string `json:"version"`
	Basis         string `json:"basis"`
	StepNumber    int64  `json:"step_number"`
	BlobSHA       string `json:"blob_sha"`
	SourceFunding string `json:"source_funding"`
}

type CheckoutPreviewDetail struct {
	Version      string `json:"version"`
	Kind         string `json:"kind"`
	State        string `json:"state"`
	HeadSHA      string `json:"head_sha"`
	SourceSHA256 string `json:"source_sha256"`
	AfterSHA256  string `json:"after_sha256"`
	EvidenceID   string `json:"evidence_id"`
	RunID        string `json:"run_id"`
	Attempt      int    `json:"attempt"`
	JobIndex     int    `json:"job_index"`
	StepIndex    int    `json:"step_index"`
	Line         int    `json:"line"`
	CheckoutMS   int64  `json:"checkout_ms"`
	Path         string `json:"path"`
	Before       string `json:"before"`
	After        string `json:"after"`
	Verification string `json:"verification"`
	Explanation  string `json:"explanation"`
}

// CheckoutPreviewView leaves the retained file out on purpose: the workflow
// source is shown once, at inspection, and the retained record answers with the
// one line it would change.
type CheckoutPreviewView struct {
	ID             string                  `json:"id"`
	InstallationID string                  `json:"installation_id"`
	Repository     string                  `json:"repository"`
	ReviewedBy     string                  `json:"reviewed_by"`
	SHA256         string                  `json:"sha256"`
	CreatedAt      time.Time               `json:"created_at"`
	Currentness    string                  `json:"currentness"`
	Collection     *CheckoutCollectionView `json:"collection"`
	Preview        CheckoutPreviewDetail   `json:"preview"`
}

func (b CheckoutSelectionBody) parse() (github.CheckoutSelection, bool) {
	run, err1 := strconv.ParseInt(b.RunID, 10, 64)
	job, err2 := strconv.ParseInt(b.GitHubJobID, 10, 64)
	sel := github.CheckoutSelection{RunID: run, Attempt: b.Attempt, GitHubJobID: job, JobIndex: b.JobIndex, StepIndex: b.StepIndex}
	return sel, err1 == nil && err2 == nil && strconv.FormatInt(run, 10) == b.RunID && strconv.FormatInt(job, 10) == b.GitHubJobID && github.ValidCheckoutSelection(sel)
}

func selectionBody(s github.CheckoutSelection) CheckoutSelectionBody {
	return CheckoutSelectionBody{RunID: strconv.FormatInt(s.RunID, 10), Attempt: s.Attempt, GitHubJobID: strconv.FormatInt(s.GitHubJobID, 10), JobIndex: s.JobIndex, StepIndex: s.StepIndex}
}

func checkoutPreviewView(p *controller.RetainedOptimisationPreview) CheckoutPreviewView {
	v := CheckoutPreviewView{
		ID: p.ID, InstallationID: p.InstallationID, Repository: p.Repository, ReviewedBy: p.ReviewedBy, SHA256: p.SHA256,
		CreatedAt: p.CreatedAt, Currentness: p.Currentness,
		Preview: CheckoutPreviewDetail{
			Version: p.Preview.Version, Kind: p.Preview.Kind, State: p.Preview.State, HeadSHA: p.Preview.HeadSHA,
			SourceSHA256: p.Preview.SourceSHA256, AfterSHA256: p.Preview.AfterSHA256, EvidenceID: p.Preview.EvidenceID,
			RunID: p.Preview.RunID, Attempt: p.Preview.Attempt, JobIndex: p.Preview.JobIndex, StepIndex: p.Preview.StepIndex,
			Line: p.Preview.Line, CheckoutMS: p.Preview.CheckoutMS, Path: p.Preview.Edit.Path, Before: p.Preview.Edit.Before,
			After: p.Preview.Edit.After, Verification: p.Preview.Verification, Explanation: p.Preview.Explanation,
		},
	}
	if c := p.Collection; c != nil {
		v.Collection = &CheckoutCollectionView{Version: c.Version, Basis: c.Basis, StepNumber: c.StepNumber, BlobSHA: c.BlobSHA, SourceFunding: c.SourceFunding}
	}
	return v
}

func decodeOptimisationBody(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		badRequest(w, "Supply one request within 8 KiB using only documented fields.")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		badRequest(w, "Supply one request.")
		return false
	}
	return true
}

func validScope(installation, repo string) bool {
	return strings.TrimSpace(installation) != "" && strings.TrimSpace(repo) != "" && len(installation) <= 128 && len(repo) <= 1024
}

func (s *Server) handleInspectCheckout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := Identity(r.Context())
	if id == nil || id.ID == "" {
		unauthorized(w, "An identified caller is required to read workflow source.")
		return
	}
	var in CheckoutInspectionRequest
	if !decodeOptimisationBody(w, r, &in) {
		return
	}
	sel, ok := in.Selection.parse()
	if !validScope(in.InstallationID, in.Repository) || !ok {
		badRequest(w, "Supply an installation, a repository and a positive decimal run ID, attempt, GitHub job ID, job index and step index.")
		return
	}
	out, err := s.ctrl.InspectCheckoutEvidence(r.Context(), in.InstallationID, in.Repository, sel)
	if err != nil {
		s.optimisationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CheckoutInspectionView{
		InstallationID: in.InstallationID, Repository: out.Repository, Selection: selectionBody(out.Selection), HeadSHA: out.HeadSHA,
		Path: out.Path, Mode: out.Mode, Content: out.Content, SourceSHA256: out.SourceSHA256, StepNumber: out.StepNumber,
		CheckoutMS: out.CheckoutMS, EvidenceID: out.EvidenceID, Basis: out.Basis, InspectedAt: out.InspectedAt,
	})
}

func (s *Server) handleCaptureCheckoutPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := Identity(r.Context())
	if id == nil || id.ID == "" {
		unauthorized(w, "An identified caller is required to retain a preview.")
		return
	}
	var in CheckoutPreviewRequest
	if !decodeOptimisationBody(w, r, &in) {
		return
	}
	sel, ok := in.Selection.parse()
	if !validScope(in.InstallationID, in.Repository) || !ok {
		badRequest(w, "Supply an installation, a repository and a positive decimal run ID, attempt, GitHub job ID, job index and step index.")
		return
	}
	if !in.HistoryReviewed {
		badRequest(w, "Confirm that you reviewed the source and that neither the repository's tooling nor its downstream consumers need full history.")
		return
	}
	actor := &store.AuditEvent{ActorID: id.ID, ActorName: id.Name, ActorKind: id.Kind, IP: id.IP}
	review := controller.OptimisationHistoryReview(in.HeadSHA, in.SourceSHA256)
	out, err := s.ctrl.CaptureOptimisationPreview(r.Context(), actor, in.InstallationID, in.Repository, sel, review, s.ctrl.OptimisationPolicy)
	if err != nil {
		s.optimisationError(w, r, err)
		return
	}
	writeCreated(w, r, out.ID, checkoutPreviewView(out))
}

func (s *Server) handleGetCheckoutPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	q, parseErr := url.ParseQuery(r.URL.RawQuery)
	if parseErr != nil || len(q) != 2 || len(q["installation_id"]) != 1 || len(q["repo"]) != 1 || !validScope(q.Get("installation_id"), q.Get("repo")) {
		badRequest(w, "Supply exactly one installation_id and repo, without other query arguments.")
		return
	}
	id := chiURLParam(r, "id")
	if !strings.HasPrefix(id, store.PrefixOptimisationPreview+"_") || len(id) > 128 {
		badRequest(w, "Supply a preview ID.")
		return
	}
	out, err := s.ctrl.ReplayOptimisationPreview(r.Context(), q.Get("installation_id"), q.Get("repo"), id)
	if err != nil {
		s.optimisationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, checkoutPreviewView(out))
}

func (s *Server) optimisationError(w http.ResponseWriter, r *http.Request, err error) {
	var admission *controller.PRDelayAdmissionError
	switch {
	case errors.Is(err, controller.ErrOptimisationPolicy):
		forbidden(w, "Reviewed workflow optimisations are off. An administrator turns on optimisation.source_reads and optimisation.workflow_edits in Settings.")
	case errors.As(err, &admission):
		rateLimited(w, err.Error(), admission.RetryAfter.Truncate(time.Second)+time.Second)
	case errors.Is(err, github.ErrRateLimited):
		rateLimited(w, "The GitHub installation is holding diagnostic reads. Try again later.", time.Minute)
	case errors.Is(err, github.ErrForbidden):
		forbidden(w, "The installation cannot read this repository's contents or Actions metadata; check the App's Contents and Actions read permissions.")
	case errors.Is(err, store.ErrNotFound):
		notFound(w, "The preview or installation is unavailable; a preview is kept for fourteen days and the oldest are evicted first.")
	case errors.Is(err, controller.ErrOptimisationPreview):
		conflict(w, "This step cannot be previewed. It needs a successful run on a push commit, a root workflow file with literal job and step names, a commit-pinned actions/checkout with fetch-depth: 0, and a source that has not changed since you reviewed it.")
	case errors.Is(err, context.DeadlineExceeded):
		conflict(w, "Reading the workflow exceeded its time limit. Try again later.")
	default:
		s.fail(w, r, "processing a checkout preview", err)
	}
}
