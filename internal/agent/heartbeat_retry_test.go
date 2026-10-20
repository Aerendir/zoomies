package agent

import (
	"context"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/store"
)

type heartbeatScript struct {
	*fakeTransport
	beat func(context.Context) (*HeartbeatResponse, error)
}

func (s *heartbeatScript) Heartbeat(ctx context.Context, _ HeartbeatRequest) (*HeartbeatResponse, error) {
	return s.beat(ctx)
}

// A ticker retries at ninety seconds, not at 45 + 25 + 45: it keeps ticking
// while the failed POST is in flight. Ninety is still too late, because a
// host is healthy only while its silence is strictly below that boundary.
func TestAFailedHeartbeatIsRetriedBeforeTheHostBecomesUnhealthy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		start := time.Now()
		var attempts []time.Duration
		tr := &heartbeatScript{fakeTransport: newFakeTransport()}
		tr.beat = func(context.Context) (*HeartbeatResponse, error) {
			attempts = append(attempts, time.Since(start))
			switch len(attempts) {
			case 2:
				time.Sleep(defaultPingAfter + defaultPingTimeout)
				return nil, ErrRetryable
			case 4:
				return nil, ErrRetryable
			case 5:
				cancel()
			}
			return &HeartbeatResponse{OK: true}, nil
		}
		a, _, _, _ := newAgent(t, 1, func(o *Options) {
			o.Transport = tr
			o.HeartbeatInterval = config.MaxQuietHeartbeatInterval
		})
		if err := a.heartbeatLoop(ctx); err != nil {
			t.Fatal(err)
		}
		want := []time.Duration{0, 45 * time.Second, 71 * time.Second, 116 * time.Second, 117 * time.Second}
		if !reflect.DeepEqual(attempts, want) {
			t.Fatalf("heartbeat attempts = %v, want %v: a recovered link needs a prompt retry and success resets the backoff", attempts, want)
		}
		host := store.Host{LastHeartbeat: start}
		if !host.Healthy(start.Add(attempts[2])) {
			t.Fatal("the retry arrived after the host had become unhealthy")
		}
		if host.Healthy(start.Add(store.HeartbeatTimeout)) {
			t.Fatal("the timeout boundary is healthy; this scenario no longer demonstrates the old ticker's failure")
		}
	})
}

// A controller that really is down must not receive a tight retry loop. Fast
// recovery is for the first failure, and continued failures earn more time.
func TestHeartbeatFailuresBackOffWithoutWaitingLongerThanTheNormalInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		start := time.Now()
		var attempts []time.Duration
		tr := &heartbeatScript{fakeTransport: newFakeTransport()}
		tr.beat = func(context.Context) (*HeartbeatResponse, error) {
			attempts = append(attempts, time.Since(start))
			if len(attempts) == 9 {
				cancel()
			}
			return nil, ErrRetryable
		}
		a, _, _, _ := newAgent(t, 1, func(o *Options) {
			o.Transport = tr
			o.HeartbeatInterval = 10 * time.Second
		})
		if err := a.heartbeatLoop(ctx); err != nil {
			t.Fatal(err)
		}
		want := []time.Duration{0, time.Second, 3 * time.Second, 7 * time.Second, 15 * time.Second, 25 * time.Second, 35 * time.Second, 45 * time.Second, 55 * time.Second}
		if !reflect.DeepEqual(attempts, want) {
			t.Fatalf("heartbeat attempts = %v, want %v: retries must back off and stay within the configured interval", attempts, want)
		}
	})
}

func TestStoppingTheAgentInterruptsAHeartbeatRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		tr := &heartbeatScript{fakeTransport: newFakeTransport()}
		tr.beat = func(context.Context) (*HeartbeatResponse, error) {
			calls++
			return nil, ErrRetryable
		}
		a, _, _, _ := newAgent(t, 1, func(o *Options) { o.Transport = tr })
		done := make(chan error, 1)
		go func() { done <- a.heartbeatLoop(ctx) }()
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("the heartbeat retry did not stop when the agent stopped")
		}
		if calls != 1 {
			t.Fatalf("heartbeats = %d, want one before cancellation", calls)
		}
	})
}
