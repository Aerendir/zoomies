package controller

import (
	"errors"
	"time"

	"github.com/eyupio/zoomies/internal/auth"
)

const prDelayReadsPerHour = 20

// Two PR reads, classic protection, five rules pages and five check pages.
const prDelayMaximumRequests = 13

var ErrPRDelayAdmission = errors.New("PR delay diagnostic reads are temporarily limited")

type PRDelayAdmissionError struct {
	RetryAfter time.Duration
}

func (e *PRDelayAdmissionError) Error() string { return ErrPRDelayAdmission.Error() }
func (e *PRDelayAdmissionError) Unwrap() error { return ErrPRDelayAdmission }

// Admission is shared by PR-delay and workflow comparison diagnostics.
// It is per installation, not per person: several viewers share the
// same GitHub quota. One in-flight read also prevents overlapping snapshots
// from spending that quota before any of them sees its updated remainder.
func (c *Controller) admitPRDelay(installation string) (func(), error) {
	c.prDelayMu.Lock()
	defer c.prDelayMu.Unlock()
	if c.prDelayReads == nil {
		c.prDelayReads = auth.NewRateLimiter(prDelayReadsPerHour, time.Hour, c.Now)
		c.prDelayInFlight = map[string]bool{}
	}
	if c.prDelayInFlight[installation] {
		return nil, &PRDelayAdmissionError{RetryAfter: time.Second}
	}
	if !c.prDelayReads.Allow(installation) {
		return nil, &PRDelayAdmissionError{RetryAfter: max(time.Second, c.prDelayReads.RetryAfter(installation))}
	}
	c.prDelayInFlight[installation] = true
	return func() {
		c.prDelayMu.Lock()
		delete(c.prDelayInFlight, installation)
		c.prDelayMu.Unlock()
	}, nil
}
