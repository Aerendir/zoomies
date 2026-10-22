package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAHostFollowsTheFleetUntilItIsToldOtherwiseAndKeepsWhenItWasMoved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	h := &Host{Name: "fido", Capacity: 4}
	if err := s.CreateHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetHost(ctx, h.ID)
	if err != nil || got.AutoAdjust != "" || !got.AutoAdjustedAt.IsZero() {
		t.Fatalf("a new host follows the fleet and has never been moved: %+v %v", got, err)
	}
	moved := time.Now().UTC().Truncate(time.Millisecond)
	if err := s.MarkHostAutoAdjusted(ctx, h.ID, moved); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHostAutoAdjust(ctx, h.ID, AutoAdjustOn); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetHost(ctx, h.ID)
	if got.AutoAdjust != AutoAdjustOn || !got.AutoAdjustedAt.Equal(moved) {
		t.Fatalf("changing the answer must not forget the last move: %+v", got)
	}
	if err := s.SetHostAutoAdjust(ctx, h.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetHost(ctx, h.ID); got.AutoAdjust != "" {
		t.Fatal("an empty answer hands the host back to the fleet")
	}
	if err := s.SetHostAutoAdjust(ctx, h.ID, "sometimes"); !errors.Is(err, ErrConflict) {
		t.Fatalf("an unknown answer must be refused: %v", err)
	}
	if err := s.SetHostAutoAdjust(ctx, "host_missing", AutoAdjustOn); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a host that does not exist: %v", err)
	}
	if _, err := s.DeleteHost(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
}
