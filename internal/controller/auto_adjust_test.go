package controller

import (
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/store"
)

// A rented machine with plenty of cores and memory and a small disk, set up for
// more runners than the disk holds: the host that filled its disk and failed jobs.
func autoAdjustHost(t *testing.T, h *harness, name string) *store.Host {
	t.Helper()
	host := h.host(name)
	host.CPUs, host.MemoryMB, host.DiskTotalMB, host.DiskFreeMB, host.Capacity = 36, 128*1024, 40*1024, 30*1024, 20
	if err := h.st.UpdateHost(h.ctx, host); err != nil {
		t.Fatal(err)
	}
	return host
}

func reload(t *testing.T, h *harness, id string) *store.Host {
	t.Helper()
	got, err := h.st.GetHost(h.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAHostThatIsKeptAtItsRecommendationIsMovedToItAndAuditedOnce(t *testing.T) {
	h := newHarness(t)
	host := autoAdjustHost(t, h, "proxmox-1")
	if err := h.st.SetHostAutoAdjust(h.ctx, host.ID, store.AutoAdjustOn); err != nil {
		t.Fatal(err)
	}
	h.c.autoAdjustHosts(h.ctx, h.c.Now())
	got := reload(t, h, host.ID)
	rec, ok := h.c.HostRecommendation(h.ctx, got)
	if !ok || got.Capacity != rec.Capacity || got.Capacity >= 20 || rec.LimitedBy != "disk" {
		t.Fatalf("the disk should have brought 20 runners down: capacity %d, %+v", got.Capacity, rec)
	}
	if got.ReserveDiskMB != rec.ReserveDiskMB || got.ReserveCPUs != rec.ReserveCPUs || got.ReserveMemoryMB != rec.ReserveMemoryMB {
		t.Fatalf("the reserve should have followed: %+v vs %+v", got, rec)
	}
	if got.AutoAdjustedAt.IsZero() {
		t.Fatal("the move must be remembered, or the cooldown has nothing to count from")
	}
	_, total, err := h.st.ListAudit(h.ctx, store.AuditFilter{Actions: []string{"host.auto_adjust"}}, store.Page{Limit: 10})
	if err != nil || total != 1 {
		t.Fatalf("one audited move, got %d: %v", total, err)
	}
	// Already at its recommendation: nothing more to do and nothing more to say.
	h.advance(10 * time.Minute)
	h.c.autoAdjustHosts(h.ctx, h.c.Now())
	if _, total, _ = h.st.ListAudit(h.ctx, store.AuditFilter{Actions: []string{"host.auto_adjust"}}, store.Page{Limit: 10}); total != 1 {
		t.Fatalf("a host at its recommendation was moved again: %d rows", total)
	}
}

func TestOnlyTheHostsThatAskForItAreMovedAndTheFleetDefaultIsOffUntilTurnedOn(t *testing.T) {
	h := newHarness(t)
	follows := autoAdjustHost(t, h, "follows")
	off := autoAdjustHost(t, h, "off")
	on := autoAdjustHost(t, h, "on")
	if err := h.st.SetHostAutoAdjust(h.ctx, off.ID, store.AutoAdjustOff); err != nil {
		t.Fatal(err)
	}
	if err := h.st.SetHostAutoAdjust(h.ctx, on.ID, store.AutoAdjustOn); err != nil {
		t.Fatal(err)
	}
	h.c.autoAdjustHosts(h.ctx, h.c.Now())
	if reload(t, h, follows.ID).Capacity != 20 || reload(t, h, off.ID).Capacity != 20 {
		t.Fatal("a fleet default of off, and a host set to off, must leave capacity as typed")
	}
	if reload(t, h, on.ID).Capacity == 20 {
		t.Fatal("a host set to on was not moved")
	}
	// The fleet's default reaches every host that follows it, and a host set to
	// off still keeps what was typed.
	h.c.UpdateConfig(func(c *config.Config) { c.Scheduler.HostAutoAdjust = true })
	h.c.autoAdjustHosts(h.ctx, h.c.Now())
	if reload(t, h, follows.ID).Capacity == 20 {
		t.Fatal("the fleet default did not reach a host that follows it")
	}
	if reload(t, h, off.ID).Capacity != 20 {
		t.Fatal("a host set to off was moved by the fleet default")
	}
}

func TestAutoAdjustNeverUnpausesNorTouchesAHostItCannotSeeOrWasToldToLeave(t *testing.T) {
	cases := map[string]func(*store.Host){
		"paused":     func(h *store.Host) { h.Capacity = 0 },
		"cordoned":   func(h *store.Host) { h.Cordoned = true },
		"silent":     func(h *store.Host) { h.LastHeartbeat = time.Now().Add(-time.Hour) },
		"unmeasured": func(h *store.Host) { h.CPUs, h.MemoryMB = 0, 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			host := autoAdjustHost(t, h, "box")
			mutate(host)
			if err := h.st.UpdateHost(h.ctx, host); err != nil {
				t.Fatal(err)
			}
			if err := h.st.SetHostAutoAdjust(h.ctx, host.ID, store.AutoAdjustOn); err != nil {
				t.Fatal(err)
			}
			before := reload(t, h, host.ID)
			h.c.autoAdjustHosts(h.ctx, h.c.Now())
			after := reload(t, h, host.ID)
			if after.Capacity != before.Capacity || after.ReserveDiskMB != before.ReserveDiskMB || !after.AutoAdjustedAt.IsZero() {
				t.Fatalf("%s: the host was adjusted: %+v -> %+v", name, before, after)
			}
		})
	}
}

func TestAHostIsLeftToSettleBetweenAutomaticMoves(t *testing.T) {
	h := newHarness(t)
	host := autoAdjustHost(t, h, "box")
	if err := h.st.SetHostAutoAdjust(h.ctx, host.ID, store.AutoAdjustOn); err != nil {
		t.Fatal(err)
	}
	h.c.autoAdjustHosts(h.ctx, h.c.Now())
	moved := reload(t, h, host.ID)
	// Somebody types a different capacity; inside the cooldown it is left, and
	// after it the recommendation is restored.
	typed := 3
	if err := h.st.PatchHost(h.ctx, host.ID, store.HostChanges{Capacity: &typed}); err != nil {
		t.Fatal(err)
	}
	h.advance(time.Minute)
	h.c.autoAdjustHosts(h.ctx, h.c.Now())
	if reload(t, h, host.ID).Capacity != typed {
		t.Fatal("a host was moved again inside the cooldown")
	}
	h.advance(autoAdjustCooldown)
	h.c.autoAdjustHosts(h.ctx, h.c.Now())
	if got := reload(t, h, host.ID); got.Capacity != moved.Capacity {
		t.Fatalf("after the cooldown the host should be back at %d, got %d", moved.Capacity, got.Capacity)
	}
}
