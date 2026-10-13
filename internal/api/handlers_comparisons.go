package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

type ComparisonCaptureRequest struct {
	InstallationID  string                                  `json:"installation_id"`
	Repository      string                                  `json:"repository"`
	Kind            string                                  `json:"kind"`
	Hypothesis      string                                  `json:"hypothesis"`
	IntendedDelta   string                                  `json:"intended_delta"`
	RevisionDeltaID string                                  `json:"revision_delta_id"`
	Since           time.Time                               `json:"since"`
	Until           time.Time                               `json:"until"`
	Baseline        []controller.ExperimentAttemptSelection `json:"baseline"`
	Candidate       []controller.ExperimentAttemptSelection `json:"candidate"`
}

func (s *Server) handleCaptureComparison(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := Identity(r.Context())
	if id == nil || id.ID == "" {
		unauthorized(w, "An identified caller is required to retain a comparison.")
		return
	}
	var input ComparisonCaptureRequest
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		badRequest(w, "Supply one comparison request within 32 KiB using only documented fields.")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		badRequest(w, "Supply one comparison request.")
		return
	}
	if len(input.InstallationID) > 128 || len(input.Repository) > 1024 || strings.TrimSpace(input.InstallationID) == "" || strings.TrimSpace(input.Repository) == "" {
		badRequest(w, "Supply a bounded installation ID and repository.")
		return
	}
	if !s.statsWindowFits(w, input.Since, input.Until, "comparison windows") {
		return
	}
	actor := &store.AuditEvent{ActorID: id.ID, ActorName: id.Name, ActorKind: id.Kind, IP: id.IP}
	record := controller.ExperimentComparisonRecord{Kind: input.Kind, Repository: input.Repository, Hypothesis: input.Hypothesis, IntendedDelta: input.IntendedDelta, RevisionDeltaID: input.RevisionDeltaID, Since: input.Since, Until: input.Until}
	out, err := s.ctrl.CaptureExperimentComparison(r.Context(), actor, record, input.InstallationID, input.Baseline, input.Candidate)
	if err != nil {
		s.comparisonError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
func (s *Server) handleGetComparison(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	q, parseErr := url.ParseQuery(r.URL.RawQuery)
	if parseErr != nil || len(q) != 2 || len(q["installation_id"]) != 1 || len(q["repo"]) != 1 || q.Get("installation_id") == "" || q.Get("repo") == "" || len(q.Get("installation_id")) > 128 || len(q.Get("repo")) > 1024 {
		badRequest(w, "Supply exactly one installation_id and repo, without other query arguments.")
		return
	}
	id := chiURLParam(r, "id")
	if !strings.HasPrefix(id, "cmp_") || len(id) > 128 {
		badRequest(w, "Supply a comparison ID.")
		return
	}
	out, err := s.ctrl.ReplayExperimentComparison(r.Context(), q.Get("installation_id"), q.Get("repo"), id)
	if err != nil {
		s.comparisonError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) comparisonError(w http.ResponseWriter, r *http.Request, err error) {
	var admission *controller.ExperimentAdmissionError
	switch {
	case errors.As(err, &admission):
		rateLimited(w, err.Error(), admission.RetryAfter.Truncate(time.Second)+time.Second)
	case errors.Is(err, github.ErrRateLimited):
		rateLimited(w, "The GitHub installation is holding diagnostic reads. Try later.", time.Minute)
	case errors.Is(err, github.ErrForbidden):
		forbidden(w, "The installation cannot read Actions metadata; check the App's Actions read permission.")
	case errors.Is(err, store.ErrNotFound):
		notFound(w, "The comparison or installation is unavailable; retained history may have expired or been evicted.")
	case errors.Is(err, github.ErrExperimentAttemptChanged):
		conflict(w, "The workflow attempt changed during collection. Retry manually.")
	case errors.Is(err, controller.ErrExperimentUnsupported), errors.Is(err, context.DeadlineExceeded):
		conflict(w, "Comparison collection is unsupported or exceeded its time limit. Try later.")
	case errors.Is(err, controller.ErrExperimentEvidence):
		badRequest(w, "Comparison evidence is invalid, inconsistent or uses an unsupported version.")
	case errors.Is(err, store.ErrInvalidComparisonEvidence):
		conflict(w, "Comparison evidence cannot be retained or verified within its bounds.")
	default:
		s.fail(w, r, "processing comparison evidence", err)
	}
}
