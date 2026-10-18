package events

import (
	"context"
	"testing"
	"time"
)

// Every state change in the fleet is published here, from the middle of a
// reconcile pass, and the bus holds its lock while it offers the frame to each
// subscriber in turn. That time is time the pass is not scaling anything, and
// it grows with the number of browser tabs open. These benchmarks size Publish
// against the number of subscribers, and Subscribe, whose queue is large enough
// to be worth knowing the price of every time a tab connects.

// benchFrame stands in for the payload of a resource frame. The controller
// publishes the same JSON its GET route returns: a dozen scalar fields, a few
// times and a list of labels, about half a kilobyte on the wire.
type benchFrame struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	PoolID     string     `json:"pool_id"`
	PoolName   string     `json:"pool_name"`
	HostID     string     `json:"host_id"`
	State      string     `json:"state"`
	Backend    string     `json:"backend"`
	Image      string     `json:"image"`
	Labels     []string   `json:"labels"`
	CPUs       float64    `json:"cpus"`
	MemoryMB   int64      `json:"memory_mb"`
	Ephemeral  bool       `json:"ephemeral"`
	CreatedAt  time.Time  `json:"created_at"`
	LastIdleAt *time.Time `json:"last_idle_at,omitempty"`
	JobID      string     `json:"current_job_id,omitempty"`
}

func newBenchFrame() benchFrame {
	created := time.Date(2025, 3, 4, 12, 0, 0, 0, time.UTC)
	idle := created.Add(2 * time.Minute)
	return benchFrame{
		ID: "run_8f3k2m1q9x", Name: "zoomies-linux-x64-8f3k2m1q9x", PoolID: "pool_a1b2c3d4e5",
		PoolName: "linux-x64", HostID: "host_p9o8i7u6y5", State: "idle", Backend: "docker",
		Image:  "ghcr.io/eyupio/zoomies-runner:ubuntu-24.04",
		Labels: []string{"self-hosted", "linux", "x64", "zoomies"}, CPUs: 2, MemoryMB: 4096,
		Ephemeral: true, CreatedAt: created, LastIdleAt: &idle,
	}
}

func BenchmarkPublish(b *testing.B) {
	frame := newBenchFrame()
	for _, tc := range []struct {
		name        string
		subscribers int
		opts        SubscribeOptions
		// delivered says the frame reaches every subscriber. A filter that
		// turns it away still costs a test per subscriber and delivers nothing.
		delivered bool
	}{
		{name: "subscribers=0", delivered: true},
		{name: "subscribers=1", subscribers: 1, delivered: true},
		{name: "subscribers=10", subscribers: 10, delivered: true},
		{name: "subscribers=100", subscribers: 100, delivered: true},
		{
			name:        "subscribers=100,filtered_out",
			subscribers: 100,
			opts:        SubscribeOptions{Kinds: []Kind{KindPoolUpdated}, TopicPrefix: "pool:"},
		},
	} {
		b.Run(tc.name, func(b *testing.B) {
			bus := New()
			subs := make([]*Subscription, tc.subscribers)
			for i := range subs {
				subs[i] = bus.Subscribe(context.Background(), tc.opts)
			}
			b.Cleanup(func() {
				for _, s := range subs {
					s.Close()
				}
			})

			// The subscribers are emptied from the benchmark's own goroutine,
			// every half queue, so that none is ever cut off for falling
			// behind (which would end its feed and log a warning, and make
			// the numbers about something else). That puts the receive in the
			// timing, so the figure is the whole cost of delivering a frame
			// and not only the publisher's share, and it spares the numbers
			// the scheduler noise of a goroutine for each subscriber.
			drain := func() {
				for _, s := range subs {
					for len(s.C) > 0 {
						<-s.C
					}
				}
			}
			every := bus.buffer / 2

			want := 0
			if tc.delivered {
				want = 1
			}
			bus.Publish(KindRunnerUpdated, "runner:run_bench", frame)
			for i, s := range subs {
				if got := len(s.C); got != want {
					b.Fatalf("subscriber %d holds %d frames after one publish, want %d", i, got, want)
				}
			}
			drain()

			b.ReportAllocs()
			sent := 0
			for b.Loop() {
				bus.Publish(KindRunnerUpdated, "runner:run_bench", frame)
				if sent++; sent == every {
					sent = 0
					drain()
				}
			}
		})
	}
}

// BenchmarkSubscribe is a browser tab connecting and leaving. Each feed is a
// queue of 512 events, about 45 KB allocated and cleared whether or not
// anything ever fills it, so a controller restart that brings every open tab
// back in the same second is that cost times the tabs.
func BenchmarkSubscribe(b *testing.B) {
	ctx := context.Background()

	b.Run("no_replay", func(b *testing.B) {
		bus := New()
		b.ReportAllocs()
		for b.Loop() {
			bus.Subscribe(ctx, SubscribeOptions{}).Close()
		}
	})

	// A tab that reconnects with the last event it saw is handed everything
	// the ring has kept since, under the lock every publisher is waiting on.
	// This is the full ring, which is what a tab that was away for a few
	// seconds on a busy fleet is given.
	b.Run("full_ring_replay", func(b *testing.B) {
		bus := New()
		frame := newBenchFrame()
		for range bus.ringCap + 50 {
			bus.Publish(KindRunnerUpdated, "runner:run_bench", frame)
		}
		since := bus.LastID() - uint64(bus.ringCap)
		sub := bus.Subscribe(ctx, SubscribeOptions{Replay: since})
		if !sub.Complete || len(sub.C) != bus.ringCap {
			b.Fatalf("replay holds %d of %d ringed events, complete = %v", len(sub.C), bus.ringCap, sub.Complete)
		}
		sub.Close()

		b.ReportAllocs()
		for b.Loop() {
			bus.Subscribe(ctx, SubscribeOptions{Replay: since}).Close()
		}
	})
}
