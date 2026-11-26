package scheduler

import (
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// The spec score is what turns "more CPU and faster CPU, more memory and
// faster memory, and more disk and faster disk" into one number a host card
// can show and the placement compares.

func TestAHostReportingNoSpecsScoresZeroAndIsPlacedAsItAlwaysWas(t *testing.T) {
	h := testHost("quiet", 4, 0)
	if got := HostSpecPriority(h); got != 0 {
		t.Errorf("HostSpecPriority = %d, want 0 for a host that reports no specs", got)
	}
	if p := HostPriority(h); p.Source != PrioritySourceComputed || p.Score != 0 {
		t.Errorf("HostPriority = %+v, want the computed 0", p)
	}
}

func TestTheSpecScoreClimbsWithEveryAxisItCanMeasure(t *testing.T) {
	base := func() *store.Host {
		h := bigHost("h")
		h.CPUMHz, h.MemoryMTs, h.DiskKind = 4800, 6400, "ssd"
		return h
	}
	best := base()
	if got := HostSpecPriority(best); got < 30 || got > 90 {
		t.Fatalf("a well-specced host scored %d, want mid-band, not pinned to either end", got)
	}
	for _, tc := range []struct {
		name string
		busy func(h *store.Host)
	}{
		{"fewer CPUs loses", func(h *store.Host) { h.CPUs, h.MemoryMB = 4, 4*1024 }},
		{"a slower clock loses", func(h *store.Host) { h.CPUMHz = 640 }},
		{"less memory loses", func(h *store.Host) { h.MemoryMB = 4 * 1024 }},
		{"a slower memory loses", func(h *store.Host) { h.MemoryMTs = 480 }},
		{"less disk loses", func(h *store.Host) { h.DiskTotalMB = 100 * 1024 }},
		{"a slower disk loses", func(h *store.Host) { h.DiskKind = "hdd" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := tc.busy == nil
			_ = other
			lesser := base()
			tc.busy(lesser)
			if got := HostSpecPriority(lesser); got >= HostSpecPriority(best) {
				t.Errorf("%s scored %d, not below the full host's %d",
					tc.name, HostSpecPriority(lesser), HostSpecPriority(best))
			}
		})
	}
	// Every figure at or past its full mark is exactly 100, so the scale is
	// absolute and two fleets' numbers can be compared.
	full := base()
	full.CPUs = 64
	full.MemoryMB = 256 * 1024
	full.DiskTotalMB = 2 * 1024 * 1024
	if got := HostSpecPriority(full); got != 100 {
		t.Errorf("a full-mark host scored %d, want 100", got)
	}
}

func TestAnUnmeasuredAxisIsLeftOutRatherThanScoredLowest(t *testing.T) {
	// A host that says nothing about its clock is not a slow host: the score
	// is taken over the axis it does measure, so an eight-core host scores
	// only on the CPUs it states. Read as a zero instead, the clock would
	// have pulled the same host down to 8, and an agent too old to measure
	// one figure must not rank behind agents that can.
	h := &store.Host{ID: "h", Capacity: 4, CPUs: 8}
	// Reading the shape out: the score for 8 CPUs stated alone is the CPU
	// axis's own share, rounded, and the exact number matters, so pin it.
	if got := HostSpecPriority(h); got != 13 {
		t.Errorf("HostSpecPriority = %d, want 13 (a quarter of the CPU axis)", got)
	}
	// With the clock stated a second axis joins the average: 8 CPUs and a
	// 3.2 GHz mark-high clock raise the host's own number, on its own scale.
	if got := HostSpecPriority(&store.Host{CPUs: 8, CPUMHz: 3200}); got != 40 {
		t.Errorf("with a clock stated too, the score = %d, want 40", got)
	}
}

func TestPlacementSendsRunnersToTheHigherScoringHost(t *testing.T) {
	p := profilePool("ci", 0, 0)
	for _, order := range []string{"", HostOrderHeadroom, HostOrderLargestStandard, HostOrderBestFit} {
		big, small := bigHost("a-big"), smallHost("b-small")
		hs := newHostSet([]*store.Host{big, small}, []*store.Pool{p}, nil, now)
		hs.order = order
		// The big host fills first under whichever order, because the spec
		// score ranks it ahead before the order is ever consulted. Once the
		// profile leaves it no room, the small host takes what remains.
		got := hs.place(p, 8)
		if len(got) == 0 || got[0] != "a-big" {
			t.Errorf("order %q placed %v, want the big host first", order, got)
		}
		for i := 1; i < len(got); i++ {
			if got[i] == "a-big" && got[i-1] == "b-small" {
				t.Errorf("order %q placed %v: the big host was offered work after the small one had already been filled", order, got)
			}
		}
	}
}

func TestAnOverridePinsAHostAheadOfTheSpecScore(t *testing.T) {
	p := profilePool("ci", 0, 0)
	big, small := bigHost("a-big"), smallHost("b-small")
	pin := 100
	small.PriorityOverride = &pin
	if p := HostPriority(small); p.Source != PrioritySourceOverride || p.Score != 100 {
		t.Fatalf("HostPriority = %+v, want the operator's 100", p)
	}
	hs := newHostSet([]*store.Host{big, small}, []*store.Pool{p}, nil, now)
	if got := hs.place(p, 4); len(got) == 0 || got[0] != "b-small" {
		t.Fatalf("placed %v, want the pinned small host first", got)
	}
}

// A pool that keeps warm runners only starts them where the fleet keeps its
// work: the minimum is not different machinery, it is fewer runners, and the
// same spec score ranks which hosts they stand up on.
func TestWarmRunnerMinimumFollowsTheSpecScore(t *testing.T) {
	p := profilePool("ci", 0, 0)
	p.MinRunners = 4
	big, small := bigHost("a-big"), smallHost("b-small")
	s := snap([]*store.Pool{p}, nil, nil, []*store.Host{big, small})
	pp := only(t, Decide(s))
	creates := actionsOf(pp.Actions, ActionCreate)
	// The big machine holds three standard runners, so three of the four warm
	// ones stack there; the fourth is the small host's, which keeps its place
	// until the fleet has filled where it ranks higher.
	if len(creates) != 4 {
		t.Fatalf("creates = %d (%s), want the pool minimum of 4", len(creates), pp.Reason)
	}
	for i, c := range creates {
		want := "a-big"
		if i == 3 {
			want = "b-small"
		}
		if c.HostID != want {
			t.Fatalf("warm create %d went to %q, want %q", i+1, c.HostID, want)
		}
	}
}
