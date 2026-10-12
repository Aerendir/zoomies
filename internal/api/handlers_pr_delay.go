package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/github"
)

func (s *Server) handlePRDelay(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()["pull_number"]
	if len(values) != 1 {
		badRequestField(w, "pull_number", "supply one positive PR number")
		return
	}
	pull, err := strconv.Atoi(values[0])
	if err != nil || pull <= 0 {
		badRequestField(w, "pull_number", "pull_number must be a positive whole number")
		return
	}
	out, err := s.ctrl.PRDelayForJob(r.Context(), chiURLParam(r, "id"), pull)
	if err != nil {
		var admission *controller.PRDelayAdmissionError
		switch {
		case errors.As(err, &admission):
			// Round upwards so the header never invites a retry before the
			// sliding-window allowance has actually replenished.
			rateLimited(w, err.Error(), admission.RetryAfter.Truncate(time.Second)+time.Second)
		case errors.Is(err, github.ErrRateLimited):
			rateLimited(w, "The GitHub installation is holding reads for its rate limit. Try later.", time.Minute)
		case errors.Is(err, github.ErrForbidden):
			forbidden(w, "The GitHub installation cannot read PR metadata; check the App's Pull requests read permission.")
		case errors.Is(err, controller.ErrPRDelayEvidence):
			badRequest(w, err.Error())
		case errors.Is(err, controller.ErrPRDelayUnsupported):
			conflict(w, err.Error())
		case errors.Is(err, controller.ErrPRDelayJobHeadChanged), errors.Is(err, controller.ErrPRDelayTargetChanged), errors.Is(err, controller.ErrPRDelayClosed):
			conflict(w, err.Error())
		case errors.Is(err, context.DeadlineExceeded):
			conflict(w, "Collecting PR delay evidence exceeded its time limit. Try again later.")
		default:
			s.fail(w, r, "collecting PR delay evidence", err)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}
