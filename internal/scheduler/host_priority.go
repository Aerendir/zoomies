package scheduler

import (
	"fmt"
	"math"
	"strings"

	"github.com/eyupio/zoomies/internal/store"
)

// Host priority: how far ahead of the fleet's other hosts a host is offered a
// runner, worked out from the specs its agent last reported and optionally
// pinned by an operator.
//
// The computed score is absolute, not relative to the fleet: each axis is
// scaled against a fixed full mark, so a host's number moves only when its
// own specs do. Ranking against whatever else is enrolled would need every
// host rescored whenever one joined or left, and would make the number on a
// host card meaningless outside the moment it was rendered. A periodic rescore
// has nothing to save back either: what a host reports is the whole of what it
// is, so after an upgrade its next heartbeat arrives with the new figures and
// the score follows without anyone asking.
//
// Two hosts on the same score fall back to the placement order and, beyond
// that, spread by headroom as they always have.

// hostPriorityAxis is one figure the spec score climbs, with the full mark
// that says "as much of this as we expect anywhere" and the share of the
// whole score the axis moves. The weights sum to 100; a host at its full
// marks everywhere earns exactly 100, and the common ranges sit well inside
// them so ordinary differences still matter.
type hostPriorityAxis struct {
	name  string
	share int
	full  float64
	value func(h *store.Host) float64
	known func(h *store.Host) bool
}

var hostPriorityAxes = []hostPriorityAxis{
	{
		name:  "CPUs",
		share: 30,
		full:  64,
		value: func(h *store.Host) float64 { return float64(h.CPUs) },
		known: func(h *store.Host) bool { return h.CPUs > 0 },
	},
	{
		name:  "CPU clock",
		share: 20,
		full:  4000,
		value: func(h *store.Host) float64 { return float64(h.CPUMHz) },
		known: func(h *store.Host) bool { return h.CPUMHz > 0 },
	},
	{
		name:  "memory",
		share: 25,
		full:  256 * 1024,
		value: func(h *store.Host) float64 { return float64(h.MemoryMB) },
		known: func(h *store.Host) bool { return h.MemoryMB > 0 },
	},
	{
		name:  "memory speed",
		share: 5,
		full:  6400,
		value: func(h *store.Host) float64 { return float64(h.MemoryMTs) },
		known: func(h *store.Host) bool { return h.MemoryMTs > 0 },
	},
	{
		name:  "disk",
		share: 10,
		full:  2 * 1024 * 1024,
		value: func(h *store.Host) float64 { return float64(h.DiskTotalMB) },
		known: func(h *store.Host) bool { return h.DiskTotalMB > 0 },
	},
	{
		// Storage speed is the one axis the kernel says plainly: a spinning
		// disk never reaches what a NAND device starts at. It names "ssd" or
		// "hdd" rather than a rate because no rate a probe can afford to
		// measure is one worth ranking on.
		name:  "disk speed",
		share: 10,
		full:  1,
		value: func(h *store.Host) float64 { return h.SpecDiskSpeed() },
		known: func(h *store.Host) bool { return h.DiskKind != "" },
	},
}

// HostSpecPriority rounds a host's specs to the 0..100 priority the
// placement prefers hosts by. An axis the host cannot measure -- an agent
// too old to report its CPU clock, a device that will not answer for its
// disk -- is left out and the score is taken over the axes it can, so
// unmeasured never reads as slowest; a host that reports nothing at all
// scores 0 and is placed by headroom as it always was.
func HostSpecPriority(h *store.Host) int {
	if h == nil {
		return 0
	}
	value, weight := 0.0, 0.0
	for _, axis := range hostPriorityAxes {
		if !axis.known(h) {
			continue
		}
		weight += float64(axis.share)
		value += float64(axis.share) * math.Min(axis.value(h)/axis.full, 1)
	}
	if weight == 0 {
		return 0
	}
	return int(math.Round(value / weight * 100))
}

type prioritySource string

const (
	// PrioritySourceComputed is the spec score, on every host with an agent
	// that reports any figure.
	PrioritySourceComputed prioritySource = "computed"
	// PrioritySourceOverride is the operator's own pin, standing in for the
	// spec score.
	PrioritySourceOverride prioritySource = "overridden"
)

// Priority is where a host stands in placement: the score placement prefers
// hosts by and the source that set it.
type Priority struct {
	// Score is the 0..100 figure placement compares.
	Score int
	// Source is "computed" while the spec score stands, "overridden" once an
	// operator has pinned the host's own.
	Source prioritySource
}

// HostPriority is the priority placement acts on: the operator's override
// where one is set, the spec score otherwise. The scheduler, the hosts page
// and the event stream all read this through the same function, so a card
// cannot disagree with a decision made against it.
func HostPriority(h *store.Host) Priority {
	if h == nil {
		return Priority{}
	}
	if h.PriorityOverride != nil {
		return Priority{Score: *h.PriorityOverride, Source: PrioritySourceOverride}
	}
	return Priority{Score: HostSpecPriority(h), Source: PrioritySourceComputed}
}

// HostPriorityReason is the sentence the host's card shows beside its
// priority: what the number came from, spec by spec, and what an override
// replaced.
func HostPriorityReason(h *store.Host) string {
	if h == nil {
		return ""
	}
	var parts []string
	if h.CPUs > 0 {
		parts = append(parts, fmt.Sprintf("%d CPUs", h.CPUs))
	}
	if h.CPUMHz > 0 {
		parts = append(parts, fmt.Sprintf("%.1f GHz of clock", float64(h.CPUMHz)/1000.0))
	}
	if h.MemoryMB > 0 {
		parts = append(parts, fmt.Sprintf("%.0f GB of memory", float64(h.MemoryMB)/1024.0))
	}
	if h.MemoryMTs > 0 {
		parts = append(parts, fmt.Sprintf("%d MT/s of memory speed", h.MemoryMTs))
	}
	if h.DiskTotalMB > 0 {
		parts = append(parts, fmt.Sprintf("%.0f GB of disk", float64(h.DiskTotalMB)/1024.0))
	}
	if h.DiskKind != "" {
		parts = append(parts, fmt.Sprintf("a %s work directory", h.DiskKind))
	}
	var out string
	if len(parts) == 0 {
		out = "scored from the specs this host has reported, none of which include a size yet"
	} else {
		out = fmt.Sprintf("scored from %s", strings.Join(parts, ", "))
	}
	if h.PriorityOverride != nil {
		return fmt.Sprintf("%s, ahead of the %d that would stand without it", out, HostSpecPriority(h))
	}
	return out
}
