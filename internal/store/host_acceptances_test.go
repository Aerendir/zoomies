package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/hosttune"
)

var acceptanceNow = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func acceptanceHost(t *testing.T, s *Store, name, current string) *Host {
	t.Helper()
	h := &Host{Name: name, Capacity: 2, Backends: StringSlice{"docker"}}
	if err := s.CreateHost(context.Background(), h); err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	rep := &hosttune.Report{Results: []hosttune.Result{{ID: "docker.logs", Title: "Docker logs", Tier: hosttune.Safe, Status: hosttune.Warn, Current: current}}}
	if err := s.SetHostDoctor(context.Background(), h.ID, rep); err != nil {
		t.Fatalf("SetHostDoctor: %v", err)
	}
	return h
}

func acceptingFor(h *Host, check, current string, ends time.Time) *HostCheckAcceptance {
	return &HostCheckAcceptance{HostID: h.ID, CheckID: check, Current: current, Reason: "pinned on purpose", By: "usr_1", ByName: "Sam", ExpiresAt: ends}
}

// An acceptance has to outlive the report it is about: a heartbeat replaces the
// report wholesale every few seconds, and a decision that vanished with it
// would be asked for again at every beat.
func TestAnAcceptanceSurvivesTheReportBeingReplaced(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreAt(t, func() time.Time { return acceptanceNow })
	h := acceptanceHost(t, s, "vm-a", "none")
	a := acceptingFor(h, "docker.logs", "none", acceptanceNow.Add(24*time.Hour))
	if err := s.UpsertHostCheckAcceptance(ctx, a); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a.ID, "hca_") || !a.CreatedAt.Equal(acceptanceNow) {
		t.Fatalf("acceptance = %+v, want an hca_ id and the store's clock", a)
	}
	rep := &hosttune.Report{Results: []hosttune.Result{{ID: "docker.logs", Title: "Docker logs", Tier: hosttune.Safe, Status: hosttune.Warn, Current: "none"}}}
	if err := s.SetHostDoctor(ctx, h.ID, rep); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetHost(ctx, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	j := got.JudgedDoctor(acceptanceNow)
	if j == nil || j.Results[0].Accepted == nil || j.Results[0].Accepted.Reason != "pinned on purpose" {
		t.Fatalf("judged result = %+v, want it accepted", j)
	}
	if got.Doctor.Results[0].Accepted != nil {
		t.Error("judging wrote through to the stored report")
	}
}

// Every read of a host must carry its acceptances, or one listing would count a
// warning the next one hides and the pill would flicker.
func TestEveryHostReadCarriesItsAcceptances(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreAt(t, func() time.Time { return acceptanceNow })
	a := acceptanceHost(t, s, "vm-a", "none")
	b := acceptanceHost(t, s, "vm-b", "none")
	if err := s.UpsertHostCheckAcceptance(ctx, acceptingFor(a, "docker.logs", "none", acceptanceNow.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	byID, _ := s.GetHost(ctx, a.ID)
	byName, _ := s.GetHostByName(ctx, "vm-a")
	all, err := s.ListHosts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(byID.Acceptances) != 1 || len(byName.Acceptances) != 1 {
		t.Fatalf("GetHost %d, GetHostByName %d acceptances, want 1 each", len(byID.Acceptances), len(byName.Acceptances))
	}
	for _, h := range all {
		want := 0
		if h.ID == a.ID {
			want = 1
		}
		if len(h.Acceptances) != want {
			t.Errorf("ListHosts: %s carries %d acceptances, want %d", h.Name, len(h.Acceptances), want)
		}
	}
	_ = b
}

func TestAcceptingAgainRenewsTheRowAndKeepsItsID(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreAt(t, func() time.Time { return acceptanceNow })
	h := acceptanceHost(t, s, "vm-a", "none")
	first := acceptingFor(h, "docker.logs", "none", acceptanceNow.Add(time.Hour))
	if err := s.UpsertHostCheckAcceptance(ctx, first); err != nil {
		t.Fatal(err)
	}
	again := acceptingFor(h, "docker.logs", "other", acceptanceNow.Add(48*time.Hour))
	again.Reason, again.ByName = "second reason", "Grace"
	if err := s.UpsertHostCheckAcceptance(ctx, again); err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Errorf("renewed acceptance got id %s, want %s kept", again.ID, first.ID)
	}
	got, _ := s.GetHost(ctx, h.ID)
	if len(got.Acceptances) != 1 || got.Acceptances[0].Current != "other" || got.Acceptances[0].ByName != "Grace" {
		t.Fatalf("acceptances = %+v, want the one renewed row", got.Acceptances)
	}
}

// A host cannot be made to hold an unbounded list, and the bound is counted
// where the write happens so two requests cannot both squeeze under it.
func TestAHostHoldsAtMostSixtyFourAcceptancesButMayRenewOne(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreAt(t, func() time.Time { return acceptanceNow })
	h := acceptanceHost(t, s, "vm-a", "none")
	for i := 0; i < MaxHostCheckAcceptances; i++ {
		if err := s.UpsertHostCheckAcceptance(ctx, acceptingFor(h, fmt.Sprintf("c.%d", i), "x", acceptanceNow.Add(time.Hour))); err != nil {
			t.Fatalf("acceptance %d: %v", i, err)
		}
	}
	if err := s.UpsertHostCheckAcceptance(ctx, acceptingFor(h, "c.extra", "x", acceptanceNow.Add(time.Hour))); !errors.Is(err, ErrTooManyHostAcceptances) {
		t.Fatalf("the 65th acceptance = %v, want ErrTooManyHostAcceptances", err)
	}
	if err := s.UpsertHostCheckAcceptance(ctx, acceptingFor(h, "c.0", "y", acceptanceNow.Add(2*time.Hour))); err != nil {
		t.Fatalf("renewing one at the cap: %v", err)
	}
	other := acceptanceHost(t, s, "vm-b", "none")
	if err := s.UpsertHostCheckAcceptance(ctx, acceptingFor(other, "c.0", "x", acceptanceNow.Add(time.Hour))); err != nil {
		t.Fatalf("another host's first acceptance: %v", err)
	}
}

func TestAnAcceptanceMustEndInTheFuture(t *testing.T) {
	s := newTestStoreAt(t, func() time.Time { return acceptanceNow })
	h := acceptanceHost(t, s, "vm-a", "none")
	if err := s.UpsertHostCheckAcceptance(context.Background(), acceptingFor(h, "docker.logs", "none", acceptanceNow)); err == nil {
		t.Fatal("an acceptance ending now was kept")
	}
}

// The three ways an acceptance stops covering a check, judged through the host
// so the clock, the stored value and the report are the ones a request sees.
func TestAnAcceptanceEndsWhenItsDatePassesOrTheValueChanges(t *testing.T) {
	ctx := context.Background()
	now := acceptanceNow
	s := newTestStoreAt(t, func() time.Time { return now })
	h := acceptanceHost(t, s, "vm-a", "none")
	if err := s.UpsertHostCheckAcceptance(ctx, acceptingFor(h, "docker.logs", "none", now.Add(24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetHost(ctx, h.ID)
	if r := got.JudgedDoctor(now.Add(time.Hour)).Results[0]; r.Accepted == nil {
		t.Fatalf("inside its term the result is %+v, want accepted", r)
	}
	if r := got.JudgedDoctor(now.Add(25 * time.Hour)).Results[0]; r.Accepted != nil || r.Ended == nil || r.Ended.Why != hosttune.EndedExpired {
		t.Fatalf("after its date the result is %+v, want ended as expired", r)
	}
	if err := s.SetHostDoctor(ctx, h.ID, &hosttune.Report{Results: []hosttune.Result{{ID: "docker.logs", Tier: hosttune.Safe, Status: hosttune.Warn, Current: "json-file"}}}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetHost(ctx, h.ID)
	if r := got.JudgedDoctor(now.Add(time.Hour)).Results[0]; r.Accepted != nil || r.Ended == nil || r.Ended.Why != hosttune.EndedChanged || r.Ended.Was != "none" {
		t.Fatalf("after the value changed the result is %+v, want ended as changed from %q", r, "none")
	}
}

func TestRevokingAnAcceptanceRemovesItAndAMissingOneIsNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreAt(t, func() time.Time { return acceptanceNow })
	h := acceptanceHost(t, s, "vm-a", "none")
	if err := s.UpsertHostCheckAcceptance(ctx, acceptingFor(h, "docker.logs", "none", acceptanceNow.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHostCheckAcceptance(ctx, h.ID, "docker.logs"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHostCheckAcceptance(ctx, h.ID, "docker.logs"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoking twice = %v, want ErrNotFound", err)
	}
	got, _ := s.GetHost(ctx, h.ID)
	if len(got.Acceptances) != 0 {
		t.Fatalf("acceptances after revoke = %+v", got.Acceptances)
	}
}

// Forgetting a host must not leave decisions about a machine that no longer
// exists, nor let a new host of the same name inherit them.
func TestDeletingAHostDeletesItsAcceptances(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreAt(t, func() time.Time { return acceptanceNow })
	h := acceptanceHost(t, s, "vm-a", "none")
	if err := s.UpsertHostCheckAcceptance(ctx, acceptingFor(h, "docker.logs", "none", acceptanceNow.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteHost(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.read.QueryRowContext(ctx, `SELECT COUNT(*) FROM host_check_acceptances`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d acceptances survived their host (%v)", n, err)
	}
}

func TestAHostWithoutAReportHasNothingToJudge(t *testing.T) {
	h := &Host{}
	if h.JudgedDoctor(acceptanceNow) != nil {
		t.Error("a host with no report produced one")
	}
	c := &Host{Doctor: HostDoctor{&hosttune.Report{Container: true, Results: []hosttune.Result{{ID: "docker.logs", Tier: hosttune.Safe, Status: hosttune.Warn, Current: "none"}}}},
		Acceptances: []HostCheckAcceptance{{CheckID: "docker.logs", Current: "none", ExpiresAt: acceptanceNow.Add(time.Hour)}}}
	if r := c.JudgedDoctor(acceptanceNow); r == nil || r.Results[0].Accepted != nil {
		t.Errorf("a container report was judged: %+v", r)
	}
}

// A host's acceptances come back in the order of their checks whichever statement
// read them, and a read of some of the hosts carries theirs and nobody else's:
// one host is read by its ID, several by reading every host's and keeping the
// ones that were asked about, so the keeping is what these hold.
func TestAcceptancesAreFilledInCheckOrderForTheHostsAskedAboutAndNoOthers(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreAt(t, func() time.Time { return acceptanceNow })
	a := acceptanceHost(t, s, "vm-a", "none")
	b := acceptanceHost(t, s, "vm-b", "none")
	c := acceptanceHost(t, s, "vm-c", "none")
	for _, h := range []*Host{a, b, c} {
		// Written in an order that is not the order of the checks.
		for _, check := range []string{"kernel.vm", "docker.logs", "docker.cgroup"} {
			if err := s.UpsertHostCheckAcceptance(ctx, acceptingFor(h, check, "none", acceptanceNow.Add(time.Hour))); err != nil {
				t.Fatal(err)
			}
		}
	}
	want := []string{"docker.cgroup", "docker.logs", "kernel.vm"}

	cases := map[string][]*Host{
		"one host":            {a},
		"two of three hosts":  {a, b},
		"all three":           {a, b, c},
		"the same host twice": {b, b},
		"one host and a copy": {a, {ID: a.ID}},
	}
	for name, asked := range cases {
		t.Run(name, func(t *testing.T) {
			// Fresh structs, as a read makes them: nothing carries acceptances yet.
			hosts := make([]*Host, len(asked))
			for i, h := range asked {
				hosts[i] = &Host{ID: h.ID, Name: h.Name}
			}
			if err := s.fillHostAcceptances(ctx, hosts); err != nil {
				t.Fatalf("fillHostAcceptances: %v", err)
			}
			for _, h := range hosts {
				var got []string
				for _, acc := range h.Acceptances {
					if acc.HostID != h.ID {
						t.Errorf("host %s carries an acceptance of host %s", h.ID, acc.HostID)
					}
					got = append(got, acc.CheckID)
				}
				if !slices.Equal(got, want) {
					t.Errorf("host %s carries the checks %v; want %v", h.ID, got, want)
				}
			}
		})
	}

	// And a host that is asked about with none to its name has none.
	d := acceptanceHost(t, s, "vm-d", "none")
	hosts := []*Host{{ID: d.ID}, {ID: a.ID}}
	if err := s.fillHostAcceptances(ctx, hosts); err != nil {
		t.Fatal(err)
	}
	if len(hosts[0].Acceptances) != 0 || len(hosts[1].Acceptances) != 3 {
		t.Errorf("a host with none carries %d and one with three carries %d", len(hosts[0].Acceptances), len(hosts[1].Acceptances))
	}
}
