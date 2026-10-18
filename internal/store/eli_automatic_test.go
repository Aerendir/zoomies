package store

import (
	"context"
	"testing"
	"time"
)

// The allowance an administrator is shown has to be the one EnqueueEliRepair
// enforces, or the panel would say a repository has room when the next failure
// is refused. That one counts every attempt in the last 24 hours, by anyone, and
// a repair cleared from the history still counts.
func TestRepairUsageCountsTheLast24HoursPerRepositoryClearedOnesIncluded(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := s.Now()
	edge := now.Add(-23 * time.Hour)
	insertRepair(t, s, EliRepair{Repo: "acme/site", CreatedAt: now.Add(-25 * time.Hour)})
	insertRepair(t, s, EliRepair{Repo: "acme/site", CreatedAt: edge})
	cleared := insertRepair(t, s, EliRepair{Repo: "acme/site", CreatedAt: now.Add(-time.Hour)})
	insertRepair(t, s, EliRepair{Repo: "acme/other", Trigger: "automatic", CreatedAt: now.Add(-2 * time.Hour)})
	insertRepair(t, s, EliRepair{Repo: "acme/stale", CreatedAt: now.Add(-48 * time.Hour)})
	if _, err := s.ClearEliRepairs(ctx, []string{cleared}, "", false); err != nil {
		t.Fatal(err)
	}

	usage, err := s.EliRepairUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	site := usage["acme/site"]
	if site.Used != 2 {
		t.Errorf("acme/site used %d, want the 23-hour row and the cleared 1-hour row: 2", site.Used)
	}
	if len(site.Times) == 0 || site.Times[0].UnixMilli() != edge.UnixMilli() {
		t.Errorf("acme/site's oldest attempt in the window is %v, want %s", site.Times, edge)
	}
	if usage["acme/other"].Used != 1 {
		t.Errorf("acme/other used %d, want 1", usage["acme/other"].Used)
	}
	if _, ok := usage["acme/stale"]; ok {
		t.Error("a repository whose only attempt is outside the window has an entry")
	}
}

// What an administrator wants beside each repository is whether Eli is working
// there now and how the last attempt went. Requested repairs are not the
// automation's, and a repair the administrator cleared from the history is not
// offered back as the latest.
func TestAutomaticActivityCountsRunningAndFindsTheNewestVisibleRepair(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := s.Now()
	ago := func(minutes int) time.Time { return now.Add(-time.Duration(minutes) * time.Minute) }
	auto := func(state string, pull, created, updated int) string {
		return insertRepair(t, s, EliRepair{Repo: "acme/site", Trigger: "automatic", State: state, PullNumber: pull, CreatedAt: ago(created), UpdatedAt: ago(updated)})
	}
	auto("succeeded", 1, 60, 59)
	auto("queued", 2, 55, 55)
	auto("working", 3, 54, 54)
	auto("checking", 4, 53, 53)
	failed := auto("failed", 7, 40, 39)
	hidden := auto("succeeded", 8, 10, 9)
	insertRepair(t, s, EliRepair{Repo: "acme/site", Trigger: "mention", State: "working", PullNumber: 9, CreatedAt: ago(5)})
	insertRepair(t, s, EliRepair{Repo: "acme/quiet", Trigger: "mention", State: "working", PullNumber: 1, CreatedAt: ago(5)})
	insertRepair(t, s, EliRepair{Repo: "acme/older", Trigger: "automatic", State: "succeeded", PullNumber: 5, CreatedAt: ago(900), UpdatedAt: ago(899)})
	if _, err := s.ClearEliRepairs(ctx, []string{hidden}, "", false); err != nil {
		t.Fatal(err)
	}

	got, err := s.EliAutomaticActivity(ctx, []string{"acme/site", "acme/quiet", "acme/older"})
	if err != nil {
		t.Fatal(err)
	}
	site := got["acme/site"]
	if site.Running != 3 {
		t.Errorf("acme/site has %d running, want the queued, working and checking automatic ones: 3", site.Running)
	}
	if site.Last == nil || site.Last.ID != failed || site.Last.State != "failed" || site.Last.PullNumber != 7 ||
		site.Last.UpdatedAt.UnixMilli() != ago(39).UnixMilli() {
		t.Errorf("acme/site's latest is %+v, want the failed repair %s on #7 (the newer one was cleared)", site.Last, failed)
	}
	if _, ok := got["acme/quiet"]; ok {
		t.Error("a repository with only requested repairs has automatic activity")
	}
	// The newest-row scan has to keep going past the newer rows of other
	// repositories, or a quiet repository would read as having done nothing.
	older := got["acme/older"]
	if older.Last == nil || older.Last.PullNumber != 5 || older.Running != 0 {
		t.Errorf("acme/older's activity is %+v, want its old repair on #5 and nothing running", older)
	}
	if none, err := s.EliAutomaticActivity(ctx, nil); err != nil || len(none) != 0 {
		t.Errorf("asking about no repositories gave %v, %v", none, err)
	}
}
