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

func optimisationPreviewFixture(t *testing.T, s *Store) *OptimisationPreview {
	t.Helper()
	inst := &Installation{AppID: 1, InstallationID: 2, Target: "acme/repo", TargetType: TargetRepo}
	if err := s.CreateInstallation(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
	return &OptimisationPreview{ID: NewID(PrefixOptimisationPreview), InstallationID: inst.ID, Repository: inst.Target, InitiatingUser: "usr_actor", Version: OptimisationPreviewVersion, Payload: json.RawMessage(`{"version":"checkout-retained-v1","evidence":"sample"}`)}
}
func optimisationAudit() *AuditEvent {
	return &AuditEvent{ActorID: "usr_actor", ActorKind: "user", ActorName: "operator"}
}
func TestOptimisationPreviewIsImmutableScopedAndAudited(t *testing.T) {
	s := newTestStore(t)
	r := optimisationPreviewFixture(t, s)
	ctx := context.Background()
	if err := s.PutOptimisationPreview(ctx, r, optimisationAudit()); err != nil {
		t.Fatal(err)
	}
	first, err := s.GetOptimisationPreview(ctx, r.InstallationID, r.Repository, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PutOptimisationPreview(ctx, r, optimisationAudit()); err != nil {
		t.Fatal(err)
	}
	events, total, err := s.ListAudit(ctx, AuditFilter{TargetID: r.ID}, Page{Limit: 10})
	if err != nil || total != 1 || events[0].Action != "optimisation.preview_retained" {
		t.Fatalf("audit %+v total=%d err=%v", events, total, err)
	}
	r.Payload = json.RawMessage(`{"version":"checkout-retained-v1","evidence":"changed"}`)
	if err := s.PutOptimisationPreview(ctx, r, optimisationAudit()); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	after, err := s.GetOptimisationPreview(ctx, r.InstallationID, r.Repository, r.ID)
	if err != nil || string(after.Payload) != string(first.Payload) || !after.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("immutable record changed", err)
	}
	if _, err := s.GetOptimisationPreview(ctx, "other", r.Repository, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.GetOptimisationPreview(ctx, r.InstallationID, "other/repo", r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
func TestOptimisationPreviewBoundsAndExpiryDoNotRenewHistory(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	s := newTestStoreAt(t, func() time.Time { return now })
	r := optimisationPreviewFixture(t, s)
	ctx := context.Background()
	firstID := r.ID
	for i := 0; i < MaxPreviewsPerInstallation+1; i++ {
		r.ID = NewID(PrefixOptimisationPreview)
		if i == 0 {
			firstID = r.ID
		}
		if err := s.PutOptimisationPreview(ctx, r, optimisationAudit()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.GetOptimisationPreview(ctx, r.InstallationID, r.Repository, firstID); !errors.Is(err, ErrNotFound) {
		t.Fatal("oldest record not pruned", err)
	}
	now = now.Add(OptimisationPreviewRetention)
	if _, err := s.GetOptimisationPreview(ctx, r.InstallationID, r.Repository, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired record visible", err)
	}
	if err := s.PutOptimisationPreview(ctx, r, optimisationAudit()); !errors.Is(err, ErrNotFound) {
		t.Fatal("retry renewed expiry", err)
	}
}
func TestOptimisationPreviewRejectsInvalidPayloadScopeAndActor(t *testing.T) {
	s := newTestStore(t)
	original := optimisationPreviewFixture(t, s)
	for _, mode := range []string{"oversized", "null", "version", "scope", "actor"} {
		r := *original
		actor := optimisationAudit()
		switch mode {
		case "oversized":
			r.Payload = json.RawMessage(fmt.Sprintf(`{"version":"checkout-retained-v1","data":%q}`, strings.Repeat("x", MaxOptimisationPreviewBytes)))
		case "null":
			r.Payload = json.RawMessage(`null`)
		case "version":
			r.Version = "future"
		case "scope":
			r.Repository = "other/repo"
		case "actor":
			actor.ActorID = "other"
		}
		if err := s.PutOptimisationPreview(context.Background(), &r, actor); !errors.Is(err, ErrInvalidOptimisationPreview) {
			t.Fatalf("%s: %v", mode, err)
		}
	}
}

func TestOptimisationPreviewGlobalBoundAndInstallationDeletion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	var firstID, firstInstallation string
	last := &OptimisationPreview{}
	for i := 0; i < MaxRetainedPreviews+1; i++ {
		if i%MaxPreviewsPerInstallation == 0 {
			inst := &Installation{AppID: 1, InstallationID: int64(i + 10), Target: fmt.Sprintf("acme/repo%d", i), TargetType: TargetRepo}
			if err := s.CreateInstallation(ctx, inst); err != nil {
				t.Fatal(err)
			}
			last.InstallationID = inst.ID
			last.Repository = inst.Target
		}
		last.ID = NewID(PrefixOptimisationPreview)
		last.InitiatingUser = "usr_actor"
		last.Version = OptimisationPreviewVersion
		last.Payload = json.RawMessage(`{"version":"checkout-retained-v1"}`)
		if i == 0 {
			firstID = last.ID
			firstInstallation = last.InstallationID
		}
		if err := s.PutOptimisationPreview(ctx, last, optimisationAudit()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.GetOptimisationPreview(ctx, firstInstallation, "acme/repo0", firstID); !errors.Is(err, ErrNotFound) {
		t.Fatal("global cap failed", err)
	}
	if _, err := s.DeleteInstallation(ctx, last.InstallationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetOptimisationPreview(ctx, last.InstallationID, last.Repository, last.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted installation left evidence", err)
	}
}
func TestOptimisationPreviewDetectsCorruptionAndRollsBackAuditFailure(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	r := optimisationPreviewFixture(t, s)
	if _, err := s.exec(ctx, `CREATE TRIGGER reject_preview_audit BEFORE INSERT ON audit_events WHEN NEW.action = 'optimisation.preview_retained' BEGIN SELECT RAISE(FAIL,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.PutOptimisationPreview(ctx, r, optimisationAudit()); err == nil {
		t.Fatal("failed audit accepted")
	}
	if _, err := s.GetOptimisationPreview(ctx, r.InstallationID, r.Repository, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("unaudited row committed", err)
	}
	if _, err := s.exec(ctx, `DROP TRIGGER reject_preview_audit`); err != nil {
		t.Fatal(err)
	}
	if err := s.PutOptimisationPreview(ctx, r, optimisationAudit()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.exec(ctx, `UPDATE optimisation_previews SET payload = ? WHERE id = ?`, []byte(`{"version":"checkout-retained-v1","corrupt":true}`), r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetOptimisationPreview(ctx, r.InstallationID, r.Repository, r.ID); !errors.Is(err, ErrInvalidOptimisationPreview) {
		t.Fatal("corruption accepted", err)
	}
	if err := s.PutOptimisationPreview(ctx, r, optimisationAudit()); !errors.Is(err, ErrInvalidOptimisationPreview) {
		t.Fatal("corrupt retry accepted", err)
	}
}

func TestOptimisationPreviewSurvivesClosingAndReopeningTheDatabase(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/previews.db"
	s, err := Open(ctx, Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	r := optimisationPreviewFixture(t, s)
	if err := s.PutOptimisationPreview(ctx, r, optimisationAudit()); err != nil {
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
	stored, err := reopened.GetOptimisationPreview(ctx, r.InstallationID, r.Repository, r.ID)
	if err != nil || stored.SHA256 != r.SHA256 || string(stored.Payload) != string(r.Payload) {
		t.Fatalf("record changed on reopening: %+v, %v", stored, err)
	}
}

func TestOptimisationPreviewSurvivesAReadOnlyBackupWithItsAudit(t *testing.T) {
	ctx := context.Background()
	s, _ := onDiskStore(t)
	r := optimisationPreviewFixture(t, s)
	if err := s.PutOptimisationPreview(ctx, r, optimisationAudit()); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir() + "/preview-backup.db"
	if err := s.Backup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	copy, err := Open(ctx, Options{Path: dest, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { copy.Close() })
	got, err := copy.GetOptimisationPreview(ctx, r.InstallationID, r.Repository, r.ID)
	if err != nil || got.SHA256 != r.SHA256 || string(got.Payload) != string(r.Payload) {
		t.Fatal("backup changed private preview", err)
	}
	_, total, err := copy.ListAudit(ctx, AuditFilter{TargetID: r.ID}, Page{Limit: 10})
	if err != nil || total != 1 {
		t.Fatal("backup lost preview audit", err)
	}
}
