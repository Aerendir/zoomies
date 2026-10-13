package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestCarriedOverHostsNeedFreshChecksOrExplicitRetirement(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	s := newTestStoreAt(t, func() time.Time { return now })
	offline := &Host{Name: "offline", TokenHash: "old-token", Capacity: 4}
	online := &Host{Name: "online", TokenHash: "online-token", Capacity: 4}
	for _, h := range []*Host{offline, online} {
		if err := s.CreateHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkTransferredHosts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckHostRecovery(ctx); !errors.Is(err, ErrConflict) {
		t.Fatalf("unresolved recovery: %v", err)
	}
	if err := s.RetireRecoveryHost(ctx, offline.ID, offline.ID, "actor"); !errors.Is(err, ErrConflict) {
		t.Fatalf("unfenced retirement: %v", err)
	}
	if err := s.SetRecoveryFence(ctx, true, "transfer"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, confirm string
		want        error
	}{
		{offline.ID, "", ErrConflict}, {offline.ID, online.ID, ErrConflict}, {"missing", "missing", ErrNotFound},
	} {
		if err := s.RetireRecoveryHost(ctx, tc.id, tc.confirm, "actor"); !errors.Is(err, tc.want) {
			t.Fatalf("retire %+v: %v", tc, err)
		}
	}
	if err := s.Heartbeat(ctx, online.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHostProtocol(ctx, online.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckHostRecovery(ctx); !errors.Is(err, ErrConflict) {
		t.Fatalf("incompatible host: %v", err)
	}
	if err := s.RetireRecoveryHost(ctx, online.ID, online.ID, "actor"); !errors.Is(err, ErrConflict) {
		t.Fatalf("online retirement: %v", err)
	}
	if err := s.RetireRecoveryHost(ctx, offline.ID, offline.ID, "actor"); err != nil {
		t.Fatal(err)
	}
	if err := s.RetireRecoveryHost(ctx, offline.ID, offline.ID, "actor"); !errors.Is(err, ErrConflict) {
		t.Fatalf("repeated retirement: %v", err)
	}
	retired, err := s.GetHost(ctx, offline.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.CountHosts(ctx); err != nil || n != 1 {
		t.Fatalf("retired host consumed enrolment limit: %d %v", n, err)
	}
	if retired.Free() != 0 {
		t.Fatal("retired host advertises free capacity")
	}
	if !retired.Retired || retired.Healthy(now) || retired.Available(now) || retired.TokenHash != "" {
		t.Fatalf("retired host can work: %+v", retired)
	}
	if _, err := s.FindHostByTokenHash(ctx, "old-token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retired credential accepted: %v", err)
	}
	if err := s.SetHostCordoned(ctx, offline.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Heartbeat(ctx, offline.ID, now); err != nil {
		t.Fatal(err)
	}
	retired, _ = s.GetHost(ctx, offline.ID)
	if retired.Healthy(now) || retired.Available(now) {
		t.Fatal("heartbeat or uncordon revived retirement")
	}
	if err := s.SetHostProtocol(ctx, online.ID, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckHostRecovery(ctx); err != nil {
		t.Fatal(err)
	}
	hosts, err := s.RecoveryHosts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hosts {
		if h.HostID == offline.ID && (h.State != "retired" || h.RetiredBy != "actor" || h.RetiredAt == nil) {
			t.Fatalf("retirement lost attribution: %+v", h)
		}
	}
	now = now.Add(HeartbeatTimeout)
	if err := s.CheckHostRecovery(ctx); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale verification: %v", err)
	}
	fresh := &Host{Name: "not-imported"}
	if err := s.CreateHost(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	now = now.Add(HeartbeatTimeout)
	if err := s.RetireRecoveryHost(ctx, fresh.ID, fresh.ID, "actor"); !errors.Is(err, ErrConflict) {
		t.Fatalf("new host retired: %v", err)
	}
}

func TestRetirementPreservesUsageSessionsAndAuditReferences(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	s := newTestStoreAt(t, func() time.Time { return now })
	_, pool, host := seedPool(t, s)
	pricedPool(t, s, pool, 0.36)
	runner := confirmed(t, s, &now, pool, host, "historical", now.Add(-2*time.Hour), now.Add(-time.Hour))
	if _, err := s.RollUpUsage(ctx, now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendAudit(ctx, &AuditEvent{ActorID: "historical-actor", Action: "host.join", TargetKind: "host", TargetID: host.ID}); err != nil {
		t.Fatal(err)
	}
	beforeDays := usageDays(t, s)
	beforeSessions := sessionsFor(t, s, runner.ID)
	if len(beforeDays) == 0 || len(beforeSessions) == 0 {
		t.Fatal("test has no historical usage")
	}
	if err := s.MarkTransferredHosts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRecoveryFence(ctx, true, "transfer"); err != nil {
		t.Fatal(err)
	}
	if err := s.RetireRecoveryHost(ctx, host.ID, host.ID, "retiring-actor"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeDays, usageDays(t, s)) || !reflect.DeepEqual(beforeSessions, sessionsFor(t, s, runner.ID)) {
		t.Fatal("retirement rewrote usage or sessions")
	}
	if _, err := s.GetRunner(ctx, runner.ID); err != nil {
		t.Fatal("retirement removed historical runner", err)
	}
	audits, _, err := s.ListAudit(ctx, AuditFilter{Actions: []string{"host.join"}}, Page{Limit: 10})
	if err != nil || len(audits) != 1 || audits[0].TargetID != host.ID {
		t.Fatalf("historical audit lost: %+v %v", audits, err)
	}
	if _, err := s.DeleteHost(ctx, host.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("retired history was deletable: %v", err)
	}
	if err := s.MarkTransferredHosts(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetHost(ctx, host.ID)
	if err != nil || !after.Retired {
		t.Fatalf("another import revived retirement: %+v %v", after, err)
	}
}

func TestCompletedRecoveryRestoresNormalHostLifecycleWithoutRevivingRetirement(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	s := newTestStoreAt(t, func() time.Time { return now })
	_, _, host := seedPool(t, s)
	host.TokenHash = "owned"
	if err := s.ReenrolRecoveryHost(ctx, host); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkTransferredHosts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRecoveryFence(ctx, true, "transfer"); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteRecovery(ctx); !errors.Is(err, ErrConflict) {
		t.Fatalf("unresolved transaction lifted fence: %v", err)
	}
	if f, err := s.RecoveryFenced(ctx); err != nil || !f.Fenced {
		t.Fatalf("failure changed fence: %+v %v", f, err)
	}
	if err := s.Heartbeat(ctx, host.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteRecovery(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetHost(ctx, host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.RecoveryRequired {
		t.Fatal("verified host permanently stuck in cutover lifecycle")
	}
	entries, err := s.RecoveryHosts(ctx)
	if err != nil || len(entries) != 1 || entries[0].State != "verified" || entries[0].VerifiedAt == nil {
		t.Fatalf("completion evidence missing: %+v %v", entries, err)
	}
	now = now.Add(HeartbeatTimeout)
	if err := s.SetRecoveryFence(ctx, true, "ordinary restore"); err != nil {
		t.Fatal(err)
	}
	if err := s.RetireRecoveryHost(ctx, host.ID, host.ID, "actor"); !errors.Is(err, ErrConflict) {
		t.Fatalf("completed cutover host eligible for retirement: %v", err)
	}
	if _, err := s.DeleteHost(ctx, host.ID); err != nil {
		t.Fatal("normal verified host lifecycle blocked", err)
	}
	if err := s.CompleteRecovery(ctx); err != nil {
		t.Fatal("completed host deletion reopened cutover obligation", err)
	}
}
