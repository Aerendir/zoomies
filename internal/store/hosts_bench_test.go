package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// ListHosts is one of the dozen reads that take a reconcile pass's snapshot, and
// it is three questions: the hosts, how many runners are live on each, and which
// checks an operator has accepted on them. A fifth of the fleet has accepted one,
// as a fleet that has been looked after does. The scenario is checked before
// anything is timed, because a fleet with no acceptances would time a read that
// has nothing to carry.
func BenchmarkListHosts(b *testing.B) {
	ctx := context.Background()
	for _, n := range []int{4, 20, 100} {
		b.Run(fmt.Sprintf("hosts=%d", n), func(b *testing.B) {
			s := newTestStore(b)
			accepted := 0
			for i := range n {
				h := &Host{Name: fmt.Sprintf("vm-%03d", i), Capacity: 4, Backends: StringSlice{"docker"}}
				if err := s.CreateHost(ctx, h); err != nil {
					b.Fatalf("CreateHost: %v", err)
				}
				if i%5 == 0 {
					a := &HostCheckAcceptance{HostID: h.ID, CheckID: "docker.logs", Current: "none", Reason: "pinned on purpose",
						By: "usr_1", ByName: "Sam", ExpiresAt: time.Now().Add(24 * time.Hour)}
					if err := s.UpsertHostCheckAcceptance(ctx, a); err != nil {
						b.Fatalf("UpsertHostCheckAcceptance: %v", err)
					}
					accepted++
				}
			}
			hosts, err := s.ListHosts(ctx)
			if err != nil {
				b.Fatalf("ListHosts: %v", err)
			}
			carried := 0
			for _, h := range hosts {
				carried += len(h.Acceptances)
			}
			if len(hosts) != n || carried != accepted {
				b.Fatalf("ListHosts returned %d hosts carrying %d acceptances; want %d carrying %d", len(hosts), carried, n, accepted)
			}

			b.ReportAllocs()
			for b.Loop() {
				if _, err := s.ListHosts(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
