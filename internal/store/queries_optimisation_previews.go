package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const OptimisationPreviewVersion = "checkout-retained-v1"
const MaxOptimisationPreviewBytes = 256 << 10
const MaxPreviewsPerInstallation = 20
const MaxRetainedPreviews = 100
const OptimisationPreviewRetention = 14 * 24 * time.Hour

var ErrInvalidOptimisationPreview = errors.New("invalid retained optimisation preview")

type OptimisationPreview struct {
	ID             string          `json:"id"`
	InstallationID string          `json:"installation_id"`
	Repository     string          `json:"repository"`
	InitiatingUser string          `json:"initiating_user"`
	Version        string          `json:"version"`
	SHA256         string          `json:"sha256"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAt      time.Time       `json:"created_at"`
}

// PutOptimisationPreview accepts an already authorised actor. The row and audit
// are one transaction; an existing ID cannot overwrite evidence or renew its lifetime.
func (s *Store) PutOptimisationPreview(ctx context.Context, r *OptimisationPreview, actor *AuditEvent) error {
	if r == nil || actor == nil || !strings.HasPrefix(r.ID, PrefixOptimisationPreview+"_") || len(r.ID) > 128 || strings.ContainsAny(r.ID, " /\r\n\x00") || r.InstallationID == "" || len(r.InstallationID) > 128 || r.InitiatingUser == "" || len(r.InitiatingUser) > 128 || actor.ActorID != r.InitiatingUser || strings.TrimSpace(actor.ActorKind) == "" || len(actor.ActorKind) > 64 || len(actor.ActorName) > 200 || len(actor.IP) > 128 || r.Version != OptimisationPreviewVersion || len(r.Payload) == 0 || len(r.Payload) > MaxOptimisationPreviewBytes || !utf8.Valid(r.Payload) {
		return ErrInvalidOptimisationPreview
	}
	owner, name, ok := strings.Cut(r.Repository, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") || len(r.Repository) > 240 || !utf8.ValidString(r.Repository) || strings.ContainsAny(r.Repository, " \t\r\n\x00\\?#%@") {
		return ErrInvalidOptimisationPreview
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(r.Payload, &envelope) != nil || envelope == nil {
		return ErrInvalidOptimisationPreview
	}
	var version string
	if json.Unmarshal(envelope["version"], &version) != nil || version != r.Version {
		return ErrInvalidOptimisationPreview
	}
	sum := sha256.Sum256(r.Payload)
	digest := hex.EncodeToString(sum[:])
	now := s.Now().UTC().Truncate(time.Millisecond)
	created := now
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var target, kind string
		if err := tx.QueryRowContext(ctx, `SELECT target,target_type FROM installations WHERE id = ?`, r.InstallationID).Scan(&target, &kind); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidOptimisationPreview
			}
			return err
		}
		if !(kind == string(TargetRepo) && target == r.Repository || kind == string(TargetOrg) && target == owner) {
			return ErrInvalidOptimisationPreview
		}
		var installation, repo, user, oldVersion, oldHash string
		var at int64
		var oldPayload []byte
		err := tx.QueryRowContext(ctx, `SELECT installation_id,repository,initiating_user,version,sha256,created_at,payload FROM optimisation_previews WHERE id = ?`, r.ID).Scan(&installation, &repo, &user, &oldVersion, &oldHash, &at, &oldPayload)
		if err == nil {
			if installation != r.InstallationID || repo != r.Repository || user != r.InitiatingUser || oldVersion != r.Version || oldHash != digest {
				return ErrConflict
			}
			if sha256.Sum256(oldPayload) != sum {
				return ErrInvalidOptimisationPreview
			}
			if at <= ms(now.Add(-OptimisationPreviewRetention)) {
				return ErrNotFound
			}
			created = time.UnixMilli(at).UTC()
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO optimisation_previews(id,installation_id,repository,initiating_user,version,sha256,payload,created_at) VALUES(?,?,?,?,?,?,?,?)`, r.ID, r.InstallationID, r.Repository, r.InitiatingUser, r.Version, digest, []byte(r.Payload), ms(now)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events (`+auditCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, NewID(PrefixAudit), actor.ActorID, actor.ActorName, actor.ActorKind, "optimisation.preview_retained", "optimisation_preview", r.ID, "", "", actor.IP, ms(now)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM optimisation_previews WHERE created_at <= ?`, ms(now.Add(-OptimisationPreviewRetention))); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM optimisation_previews WHERE installation_id = ? AND id NOT IN (SELECT id FROM optimisation_previews WHERE installation_id = ? ORDER BY sequence DESC LIMIT ?)`, r.InstallationID, r.InstallationID, MaxPreviewsPerInstallation); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM optimisation_previews WHERE id NOT IN (SELECT id FROM optimisation_previews ORDER BY sequence DESC LIMIT ?)`, MaxRetainedPreviews)
		return err
	})
	if err == nil {
		r.SHA256 = digest
		r.CreatedAt = created
	}
	return err
}

// A scoped miss includes expired or evicted evidence. It must not be rendered
// as an empty preview. The hash detects corruption; it is not an authorisation.
func (s *Store) GetOptimisationPreview(ctx context.Context, installation, repo, id string) (*OptimisationPreview, error) {
	r := &OptimisationPreview{}
	var payload []byte
	var created int64
	err := s.read.QueryRowContext(ctx, `SELECT id,installation_id,repository,initiating_user,version,sha256,payload,created_at FROM optimisation_previews WHERE installation_id = ? AND repository = ? AND id = ? AND created_at > ?`, installation, repo, id, ms(s.Now().Add(-OptimisationPreviewRetention))).Scan(&r.ID, &r.InstallationID, &r.Repository, &r.InitiatingUser, &r.Version, &r.SHA256, &payload, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(payload)
	if len(payload) > MaxOptimisationPreviewBytes || hex.EncodeToString(sum[:]) != r.SHA256 {
		return nil, ErrInvalidOptimisationPreview
	}
	r.Payload = payload
	r.CreatedAt = time.UnixMilli(created).UTC()
	return r, nil
}
