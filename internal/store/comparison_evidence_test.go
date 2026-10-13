package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func comparisonEvidenceFixture(t *testing.T, s *Store) *ComparisonEvidence {
	t.Helper()
	inst := &Installation{AppID: 1, InstallationID: 2, Target: "acme/repo", TargetType: TargetRepo}
	if err := s.CreateInstallation(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
	return &ComparisonEvidence{ID: NewID(PrefixComparison), InstallationID: inst.ID, Repository: inst.Target, InitiatingUser: "usr_actor", Version: ComparisonEvidenceVersion, Payload: json.RawMessage(`{"version":"observed-comparison-v1","evidence":"sample"}`)}
}
func comparisonAudit() *AuditEvent {
	return &AuditEvent{ActorID: "usr_actor", ActorKind: "user", ActorName: "operator"}
}
func TestComparisonEvidenceIsImmutableScopedAndAudited(t *testing.T) {
	s := newTestStore(t)
	r := comparisonEvidenceFixture(t, s)
	ctx := context.Background()
	if err := s.PutComparisonEvidence(ctx, r, comparisonAudit()); err != nil {
		t.Fatal(err)
	}
	first, err := s.GetComparisonEvidence(ctx, r.InstallationID, r.Repository, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PutComparisonEvidence(ctx, r, comparisonAudit()); err != nil {
		t.Fatal(err)
	}
	events, total, err := s.ListAudit(ctx, AuditFilter{TargetID: r.ID}, Page{Limit: 10})
	if err != nil || total != 1 || events[0].Action != "comparison.evidence_retained" {
		t.Fatalf("audit %+v total=%d err=%v", events, total, err)
	}
	r.Payload = json.RawMessage(`{"version":"observed-comparison-v1","evidence":"changed"}`)
	if err := s.PutComparisonEvidence(ctx, r, comparisonAudit()); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	after, err := s.GetComparisonEvidence(ctx, r.InstallationID, r.Repository, r.ID)
	if err != nil || string(after.Payload) != string(first.Payload) || !after.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("immutable record changed", err)
	}
	if _, err := s.GetComparisonEvidence(ctx, "other", r.Repository, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.GetComparisonEvidence(ctx, r.InstallationID, "other/repo", r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
func TestComparisonEvidenceBoundsAndExpiryDoNotRenewHistory(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	s := newTestStoreAt(t, func() time.Time { return now })
	r := comparisonEvidenceFixture(t, s)
	ctx := context.Background()
	firstID := r.ID
	for i := 0; i < MaxComparisonsPerInstallation+1; i++ {
		r.ID = NewID(PrefixComparison)
		if i == 0 {
			firstID = r.ID
		}
		if err := s.PutComparisonEvidence(ctx, r, comparisonAudit()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.GetComparisonEvidence(ctx, r.InstallationID, r.Repository, firstID); !errors.Is(err, ErrNotFound) {
		t.Fatal("oldest record not pruned", err)
	}
	now = now.Add(ComparisonEvidenceRetention)
	if _, err := s.GetComparisonEvidence(ctx, r.InstallationID, r.Repository, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired record visible", err)
	}
	if err := s.PutComparisonEvidence(ctx, r, comparisonAudit()); !errors.Is(err, ErrNotFound) {
		t.Fatal("retry renewed expiry", err)
	}
}
func TestComparisonEvidenceRejectsInvalidPayloadScopeAndActor(t *testing.T) {
	s := newTestStore(t)
	original := comparisonEvidenceFixture(t, s)
	for _, mode := range []string{"oversized", "null", "version", "scope", "actor"} {
		r := *original
		actor := comparisonAudit()
		switch mode {
		case "oversized":
			r.Payload = json.RawMessage(fmt.Sprintf(`{"version":"observed-comparison-v1","data":%q}`, strings.Repeat("x", MaxComparisonEvidenceBytes)))
		case "null":
			r.Payload = json.RawMessage(`null`)
		case "version":
			r.Version = "future"
		case "scope":
			r.Repository = "other/repo"
		case "actor":
			actor.ActorID = "other"
		}
		if err := s.PutComparisonEvidence(context.Background(), &r, actor); !errors.Is(err, ErrInvalidComparisonEvidence) {
			t.Fatalf("%s: %v", mode, err)
		}
	}
}

func TestComparisonEvidenceGlobalBoundAndInstallationDeletion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	var firstID, firstInstallation string
	last := &ComparisonEvidence{}
	for i := 0; i < MaxRetainedComparisons+1; i++ {
		if i%MaxComparisonsPerInstallation == 0 {
			inst := &Installation{AppID: 1, InstallationID: int64(i + 10), Target: fmt.Sprintf("acme/repo%d", i), TargetType: TargetRepo}
			if err := s.CreateInstallation(ctx, inst); err != nil {
				t.Fatal(err)
			}
			last.InstallationID = inst.ID
			last.Repository = inst.Target
		}
		last.ID = NewID(PrefixComparison)
		last.InitiatingUser = "usr_actor"
		last.Version = ComparisonEvidenceVersion
		last.Payload = json.RawMessage(`{"version":"observed-comparison-v1"}`)
		if i == 0 {
			firstID = last.ID
			firstInstallation = last.InstallationID
		}
		if err := s.PutComparisonEvidence(ctx, last, comparisonAudit()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.GetComparisonEvidence(ctx, firstInstallation, "acme/repo0", firstID); !errors.Is(err, ErrNotFound) {
		t.Fatal("global cap failed", err)
	}
	if _, err := s.DeleteInstallation(ctx, last.InstallationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetComparisonEvidence(ctx, last.InstallationID, last.Repository, last.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted installation left evidence", err)
	}
}
func TestComparisonEvidenceDetectsCorruptionAndRollsBackAuditFailure(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	r := comparisonEvidenceFixture(t, s)
	if _, err := s.exec(ctx, `CREATE TRIGGER reject_comparison_audit BEFORE INSERT ON audit_events WHEN NEW.action = 'comparison.evidence_retained' BEGIN SELECT RAISE(FAIL,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.PutComparisonEvidence(ctx, r, comparisonAudit()); err == nil {
		t.Fatal("failed audit accepted")
	}
	if _, err := s.GetComparisonEvidence(ctx, r.InstallationID, r.Repository, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("unaudited row committed", err)
	}
	if _, err := s.exec(ctx, `DROP TRIGGER reject_comparison_audit`); err != nil {
		t.Fatal(err)
	}
	if err := s.PutComparisonEvidence(ctx, r, comparisonAudit()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.exec(ctx, `UPDATE comparison_evidence SET payload = ? WHERE id = ?`, []byte(`{"version":"observed-comparison-v1","corrupt":true}`), r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetComparisonEvidence(ctx, r.InstallationID, r.Repository, r.ID); !errors.Is(err, ErrInvalidComparisonEvidence) {
		t.Fatal("corruption accepted", err)
	}
	if err := s.PutComparisonEvidence(ctx, r, comparisonAudit()); !errors.Is(err, ErrInvalidComparisonEvidence) {
		t.Fatal("corrupt retry accepted", err)
	}
}

func TestComparisonEvidenceSurvivesClosingAndReopeningTheDatabase(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/comparisons.db"
	s, err := Open(ctx, Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	r := comparisonEvidenceFixture(t, s)
	if err := s.PutComparisonEvidence(ctx, r, comparisonAudit()); err != nil {
		s.Close()
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	stored, err := reopened.GetComparisonEvidence(ctx, r.InstallationID, r.Repository, r.ID)
	if err != nil || stored.SHA256 != r.SHA256 || string(stored.Payload) != string(r.Payload) {
		t.Fatalf("record changed on reopening: %+v, %v", stored, err)
	}
}
