package events

import (
	"context"
	"testing"
)

// limits.event_subscribers is enforced here rather than by counting first and
// subscribing after, so that the ceiling holds for streams opened together.
// Closing one has to give its place back, or a browser that reconnects would
// find the instance permanently full of its own dead streams.
func TestSubscribeWithinRefusesAtTheCeilingAndFreesAPlaceOnClose(t *testing.T) {
	b := New()
	ctx := context.Background()

	first, ok := b.SubscribeWithin(ctx, SubscribeOptions{}, 2)
	if !ok {
		t.Fatal("the first subscription was refused under a ceiling of 2")
	}
	if _, ok := b.SubscribeWithin(ctx, SubscribeOptions{}, 2); !ok {
		t.Fatal("the second subscription was refused under a ceiling of 2")
	}
	if s, ok := b.SubscribeWithin(ctx, SubscribeOptions{}, 2); ok || s != nil {
		t.Fatalf("a third subscription was admitted under a ceiling of 2 (%d open)", b.Subscribers())
	}
	if n := b.Subscribers(); n != 2 {
		t.Fatalf("a refused subscription was still counted: %d open", n)
	}

	first.Close()
	if _, ok := b.SubscribeWithin(ctx, SubscribeOptions{}, 2); !ok {
		t.Fatal("closing a subscription did not free its place")
	}
	if _, ok := b.SubscribeWithin(ctx, SubscribeOptions{}, 0); !ok {
		t.Fatal("a ceiling of 0 refused a subscription; 0 is unlimited")
	}
}

// A public view's visitors are a class with a ceiling of their own. However
// many of them there are, the people who run the fleet keep every stream the
// ordinary ceiling allows them, and a visitor is refused only at the visitors'
// ceiling, never because staff are busy.
func TestAClassHasItsOwnCeilingAndNeverTakesTheOrdinaryOne(t *testing.T) {
	b := New()
	ctx := context.Background()
	visitor := SubscribeOptions{Class: "visitor", ClassLimit: 2}

	var visitors []*Subscription
	for i := range 2 {
		s, ok := b.SubscribeWithin(ctx, visitor, 1)
		if !ok {
			t.Fatalf("visitor %d was refused under a visitors' ceiling of 2", i+1)
		}
		visitors = append(visitors, s)
	}
	if _, ok := b.SubscribeWithin(ctx, visitor, 1); ok {
		t.Fatal("a third visitor was admitted under a visitors' ceiling of 2")
	}

	// Two visitors are open and the ordinary ceiling is 1: staff still get it.
	staff, ok := b.SubscribeWithin(ctx, SubscribeOptions{}, 1)
	if !ok {
		t.Fatal("visitors took the place the ordinary ceiling keeps for staff")
	}
	if _, ok := b.SubscribeWithin(ctx, SubscribeOptions{}, 1); ok {
		t.Fatal("a second ordinary subscription was admitted under a ceiling of 1")
	}
	if got := b.ClassSubscribers("visitor"); got != 2 {
		t.Errorf("ClassSubscribers(visitor) = %d; want 2", got)
	}
	if got := b.ClassSubscribers(""); got != 1 {
		t.Errorf("ClassSubscribers(\"\") = %d; want 1", got)
	}
	if got := b.Subscribers(); got != 3 {
		t.Errorf("Subscribers() = %d; want every open stream, 3", got)
	}

	// Staff leaving frees nothing for visitors, and a visitor leaving does.
	staff.Close()
	if _, ok := b.SubscribeWithin(ctx, visitor, 1); ok {
		t.Fatal("a staff stream closing made room under the visitors' ceiling")
	}
	visitors[0].Close()
	if _, ok := b.SubscribeWithin(ctx, visitor, 1); !ok {
		t.Fatal("a visitor leaving did not free a visitor's place")
	}
}
