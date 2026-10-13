package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/analysis"
)

// demoSeededAt is off the minute, as the clock a demo is seeded on almost
// always is: a window of two hours then overlaps a hundred and twenty-one
// sampled minutes, and the seeded causes have to hold all the same.
var demoSeededAt = time.Date(2026, 10, 12, 14, 37, 23, 0, time.UTC)

// seededDemo is the demo fleet on a clock the test holds, with the window the
// analysis fixture was placed in.
func seededDemo(t *testing.T) (*harness, AnalysisWindow) {
	t.Helper()
	h := newHarnessAt(t, func() time.Time { return demoSeededAt })
	if err := h.c.SeedDemo(h.ctx); err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}
	return h, AnalysisWindow{Since: demoSeededAt.Add(-demoAnalysisFrom), Until: demoSeededAt.Add(-demoAnalysisTo)}
}

// The demo is where a person first sees an analysis, so each must lead with
// the cause seeded for it, and say it firmly: a demo that hedges, or names
// another cause, teaches that the analyses cannot be leaned on.
func TestEachSeededCauseIsNamedDominantAtHighConfidence(t *testing.T) {
	h, w := seededDemo(t)

	for _, tc := range []struct {
		pool string
		want analysis.WaitCause
	}{
		{demoPoolLinuxID, analysis.CauseRunnerStartup},
		{demoPoolArmID, analysis.CausePoolCeiling},
	} {
		f, err := h.c.AnalyseQueueWait(h.ctx, w, tc.pool)
		if err != nil {
			t.Fatalf("AnalyseQueueWait(%s): %v", tc.pool, err)
		}
		if f.Dominant != string(tc.want) || f.Confidence != analysis.High {
			t.Errorf("queue wait on %s leads with %q at %s (%s): %q; want %s at high",
				tc.pool, f.Dominant, f.Confidence, f.ConfidenceReason, f.Summary, tc.want)
		}
	}

	host := demoHostPrefix + "b"
	p, err := h.c.AnalyseHostPressure(h.ctx, w, host)
	if err != nil {
		t.Fatalf("AnalyseHostPressure(%s): %v", host, err)
	}
	if p.Dominant != analysis.HostPressured || p.Confidence != analysis.High {
		t.Errorf("host pressure on %s leads with %q at %s (%s): %q; want %s at high",
			host, p.Dominant, p.Confidence, p.ConfidenceReason, p.Summary, analysis.HostPressured)
	}

	s, err := h.c.AnalyseStartupFailures(h.ctx, w, "", "")
	if err != nil {
		t.Fatalf("AnalyseStartupFailures: %v", err)
	}
	if s.Dominant != "host:"+host || s.Confidence != analysis.High {
		t.Errorf("startup failures lead with %q at %s (%s): %q; want host:%s at high",
			s.Dominant, s.Confidence, s.ConfidenceReason, s.Summary, host)
	}
	// The image fault is said as Task 4 words it, and no more than that: a
	// pull that failed or an image that would not start.
	if want := "the runner image could not be pulled or would not start"; !strings.Contains(s.Summary, want) {
		t.Errorf("startup failures say %q; want the image fault's own words, %q", s.Summary, want)
	}
}

// Decision 7: over the last day the demo has a dozen jobs or so on each of
// its two releases, and the comparison says that is too few to rank rather
// than ranking them. The analysis fixture's jobs carry no release, so they
// cannot tip it.
func TestTheDemoComparisonSaysItsSampleIsSmall(t *testing.T) {
	h, _ := seededDemo(t)

	f, err := h.c.AnalyseCompare(h.ctx, CompareRequest{Since: demoSeededAt.Add(-24 * time.Hour), Until: demoSeededAt})
	if err != nil {
		t.Fatalf("AnalyseCompare: %v", err)
	}
	if f.Evidence.A.Label != "v1.3.2" || f.Evidence.B.Label != "v1.3.3" {
		t.Errorf("the comparison set %s against %s; want v1.3.2 against v1.3.3", f.Evidence.A.Label, f.Evidence.B.Label)
	}
	if f.Confidence != analysis.Low {
		t.Errorf("the comparison is at %s (%s); want low", f.Confidence, f.ConfidenceReason)
	}
	if want := "fewer than 30 on either side, so the two are not ranked"; !strings.Contains(f.Summary, want) {
		t.Errorf("the comparison says %q; want it to say %q", f.Summary, want)
	}
}
