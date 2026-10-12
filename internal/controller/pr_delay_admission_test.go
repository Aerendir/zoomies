package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/github"
)

func TestPRDelayAdmissionIsSharedByInstallationAndAllowsOnlyOneFlight(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	c := &Controller{clock: func() time.Time { return now }}
	release, err := c.admitPRDelay("ins_one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.admitPRDelay("ins_one"); !errors.Is(err, ErrPRDelayAdmission) {
		t.Fatal(err)
	}
	other, err := c.admitPRDelay("ins_two")
	if err != nil {
		t.Fatal(err)
	}
	other()
	release()
	for i := 1; i < prDelayReadsPerHour; i++ {
		release, err := c.admitPRDelay("ins_one")
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if _, err := c.admitPRDelay("ins_one"); !errors.Is(err, ErrPRDelayAdmission) {
		t.Fatal(err)
	}
	now = now.Add(time.Hour + time.Second)
	release, err = c.admitPRDelay("ins_one")
	if err != nil {
		t.Fatal(err)
	}
	release()
}

type prDelayQuotaFixture struct {
	*prDelayReaderFixture
	quota *github.RateLimit
}

func (f *prDelayQuotaFixture) RateLimit(_ context.Context) (*github.RateLimit, error) {
	return f.quota, nil
}

func TestPRDelayCollectionReservesQuotaBeforeReadingEvidence(t *testing.T) {
	h, inst, job, f := prDelayCollectorFixture(t)
	limited := &prDelayQuotaFixture{prDelayReaderFixture: f, quota: &github.RateLimit{Limit: 100, Remaining: 60, ResetAt: h.c.Now().Add(time.Hour)}}
	h.c.clients.entries[inst.ID] = &clientEntry{client: limited, updatedAt: inst.UpdatedAt.Truncate(time.Millisecond)}
	if _, err := h.c.PRDelayForJob(h.ctx, job.ID, 1); !errors.Is(err, ErrPRDelayAdmission) {
		t.Fatal(err)
	}
	if f.calls != 0 {
		t.Fatalf("spent %d evidence reads below the reserve", f.calls)
	}
	limited.quota.Remaining = 100
	if _, err := h.c.PRDelayForJob(h.ctx, job.ID, 1); err != nil {
		t.Fatal(err)
	}
}
