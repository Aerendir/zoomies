package controller

import (
	"context"
	"time"

	"github.com/eyupio/zoomies/internal/auth"
	"github.com/eyupio/zoomies/internal/scheduler"
	"github.com/eyupio/zoomies/internal/store"
)

// autoAdjustCooldown is the least time between two automatic moves of one host.
// The recommendation follows the pools' runner size, which an operator is often
// in the middle of editing, and a capacity that followed every keystroke would
// make a host flap between sizes that no job ever ran at.
const autoAdjustCooldown = 5 * time.Minute

// AutoAdjustEffective is whether the controller keeps this host at its
// recommendation: the host's own answer where it has one, the fleet's otherwise.
func (c *Controller) AutoAdjustEffective(h *store.Host) bool {
	switch h.AutoAdjust {
	case store.AutoAdjustOn:
		return true
	case store.AutoAdjustOff:
		return false
	}
	return c.cfg().Scheduler.HostAutoAdjust
}

// HostRecommendation is what Adjust recommends for a host now, and whether it
// could be worked out at all. It is the same answer the automatic adjustment
// applies, so a person looking at the dialog and the controller moving the host
// are looking at one set of numbers.
func (c *Controller) HostRecommendation(ctx context.Context, h *store.Host) (scheduler.HostRecommendation, bool) {
	pools, err := c.fleetPools(ctx)
	if err != nil {
		return scheduler.HostRecommendation{}, false
	}
	return c.recommendHost(h, pools)
}

func (c *Controller) recommendHost(h *store.Host, pools []*store.Pool) (scheduler.HostRecommendation, bool) {
	r := c.cfg().Runners
	return scheduler.RecommendHost(h, scheduler.AskOf(pools, r.DefaultCPUs, r.DefaultMemoryMB))
}

// autoAdjustHosts is the housekeeping half of the setting: every host that is
// to be kept at its recommendation is checked against it, and moved when it has
// drifted. Each of these is a reason to leave a host alone, and each is said so
// in the code rather than discovered in a bug report:
//
//   - a capacity of 0 is a host an operator paused, and only a person un-pauses it;
//   - a host that has not said how big it is has nothing to recommend from;
//   - a host that is not heartbeating, is cordoned or retired is not one to be
//     rearranging, and its figures may be stale;
//   - a host moved lately is left to settle (autoAdjustCooldown);
//   - a move that would strand a pool is the same refusal a person gets.
func (c *Controller) autoAdjustHosts(ctx context.Context, now time.Time) {
	hosts, err := c.st.ListHosts(ctx)
	if err != nil {
		c.log.Warn("could not list hosts to keep them at their recommendation", "error", err)
		return
	}
	var pools []*store.Pool
	for _, h := range hosts {
		if IsDemoID(h.ID) || h.Capacity <= 0 || h.Cordoned || h.Retired || h.Incompatible || !h.Healthy(now) || !c.AutoAdjustEffective(h) {
			continue
		}
		if !h.AutoAdjustedAt.IsZero() && now.Sub(h.AutoAdjustedAt) < autoAdjustCooldown {
			continue
		}
		if pools == nil {
			if pools, err = c.fleetPools(ctx); err != nil {
				c.log.Warn("could not read the pools to keep hosts at their recommendation", "error", err)
				return
			}
		}
		rec, ok := c.recommendHost(h, pools)
		if !ok {
			continue
		}
		if h.Capacity == rec.Capacity && h.ReserveCPUs == rec.ReserveCPUs && h.ReserveMemoryMB == rec.ReserveMemoryMB && h.ReserveDiskMB == rec.ReserveDiskMB {
			continue
		}
		c.applyAutoAdjust(ctx, h, rec, now)
	}
}

func (c *Controller) applyAutoAdjust(ctx context.Context, h *store.Host, rec scheduler.HostRecommendation, now time.Time) {
	proposed := *h
	proposed.Capacity, proposed.ReserveCPUs, proposed.ReserveMemoryMB, proposed.ReserveDiskMB =
		rec.Capacity, rec.ReserveCPUs, rec.ReserveMemoryMB, rec.ReserveDiskMB
	stranded, err := c.HostStrandings(ctx, &proposed)
	if err != nil {
		c.log.Warn("could not check whether adjusting a host would strand a pool", "host", h.ID, "error", err)
		return
	}
	if len(stranded) > 0 {
		// Left as it is, and tried again after the cooldown: the pool may have
		// been edited, and nothing here is urgent enough to override the check
		// a person would be stopped by.
		c.log.Info("a host was not adjusted to its recommendation because that would leave a pool with nowhere to run",
			"host", h.ID, "name", h.Name, "pool", stranded[0].Pool)
		if err := c.st.MarkHostAutoAdjusted(ctx, h.ID, now); err != nil {
			c.log.Warn("could not record the attempt to adjust a host", "host", h.ID, "error", err)
		}
		return
	}
	before := *h
	changes := store.HostChanges{Capacity: &rec.Capacity, ReserveCPUs: &rec.ReserveCPUs, ReserveMemoryMB: &rec.ReserveMemoryMB, ReserveDiskMB: &rec.ReserveDiskMB}
	if err := c.st.PatchHost(ctx, h.ID, changes); err != nil {
		c.log.Warn("could not adjust a host to its recommendation", "host", h.ID, "error", err)
		return
	}
	if err := c.st.MarkHostAutoAdjusted(ctx, h.ID, now); err != nil {
		c.log.Warn("could not record that a host was adjusted", "host", h.ID, "error", err)
	}
	h.Capacity, h.ReserveCPUs, h.ReserveMemoryMB, h.ReserveDiskMB = rec.Capacity, rec.ReserveCPUs, rec.ReserveMemoryMB, rec.ReserveDiskMB
	h.AutoAdjustedAt = now
	c.log.Info("adjusted a host to its recommendation",
		"host", h.ID, "name", h.Name, "capacity", rec.Capacity, "was", before.Capacity, "limited_by", rec.LimitedBy)
	c.authsvc.Auditor().Act(ctx, auth.SystemIdentity(), "host.auto_adjust", "host", h.ID, map[string]any{
		"reason":            rec.Reason(),
		"capacity":          rec.Capacity,
		"was_capacity":      before.Capacity,
		"reserve_cpus":      rec.ReserveCPUs,
		"reserve_memory_mb": rec.ReserveMemoryMB,
		"reserve_disk_mb":   rec.ReserveDiskMB,
	})
	c.publishHost(h)
	// Capacity and the reserve decide where runners may be placed.
	c.Nudge()
}
