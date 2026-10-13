package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"time"
)

const RunnerCreationObservationVersion = "runner-create-result-v1"
const MaxRunnerCreationObservationBytes = 4096
const MaxRunnerCreationObservations = 10000
const MaxRunnerCreationObservationsPerInstallation = 2000
const RunnerCreationObservationRetention = 30 * 24 * time.Hour

var ErrInvalidRunnerCreationObservation = errors.New("invalid runner creation observation")

// IssuedRunnerConfiguration is a whitelist of requested fields, never a full
// backend specification. It does not prove limits were applied. The image
// reference is fingerprinted rather than retaining a possibly credentialed URI.
type IssuedRunnerConfiguration struct {
	Backend              BackendKind `json:"backend"`
	ImageReferenceSHA256 string      `json:"image_reference_sha256"`
	CPUs                 *float64    `json:"cpus"`
	MemoryMB             *int64      `json:"memory_mb"`
	ResourceSource       string      `json:"resource_source"`
	DockerMode           DockerMode  `json:"docker_mode"`
	CPUTyped             bool        `json:"cpu_typed"`
	MemoryTyped          bool        `json:"memory_typed"`
}
type ReportedRunnerCreation struct {
	ImageDigest        *string    `json:"image_digest"`
	DigestCoverage     string     `json:"digest_coverage"`
	CompletedAt        *time.Time `json:"completed_at"`
	ContainerStartedAt *time.Time `json:"container_started_at"`
}
type RunnerCreationObservationData struct {
	Version  string                    `json:"version"`
	RunnerID string                    `json:"runner_id"`
	HostID   string                    `json:"host_id"`
	PoolID   string                    `json:"pool_id"`
	TaskID   string                    `json:"task_id"`
	IssuedAt time.Time                 `json:"issued_at"`
	Issued   IssuedRunnerConfiguration `json:"issued"`
	Reported ReportedRunnerCreation    `json:"reported"`
}
type RunnerCreationObservation struct {
	InstallationID string                        `json:"installation_id"`
	SHA256         string                        `json:"sha256"`
	ReceivedAt     time.Time                     `json:"received_at"`
	Data           RunnerCreationObservationData `json:"data"`
}

func ValidRunnerCreationObservation(d *RunnerCreationObservationData) bool {
	if d == nil || d.Version != RunnerCreationObservationVersion || d.IssuedAt.IsZero() {
		return false
	}
	for _, id := range []string{d.RunnerID, d.HostID, d.PoolID, d.TaskID} {
		if id == "" || len(id) > 128 || strings.ContainsAny(id, " /\\\r\n\t\x00") {
			return false
		}
	}
	switch d.Issued.Backend {
	case BackendDocker, BackendPodman, BackendProcess:
	default:
		return false
	}
	switch d.Issued.DockerMode {
	case "", DockerNone, DockerHostSocket, DockerDinD:
	default:
		return false
	}
	switch d.Issued.ResourceSource {
	case "", AllocationFromPool, AllocationFromHost, AllocationFromProfile, AllocationReduced, AllocationHistory:
	default:
		return false
	}
	if d.Issued.ImageReferenceSHA256 != "" && !observationHash(d.Issued.ImageReferenceSHA256) {
		return false
	}
	if d.Issued.CPUs != nil && (*d.Issued.CPUs <= 0 || math.IsNaN(*d.Issued.CPUs) || math.IsInf(*d.Issued.CPUs, 0)) || d.Issued.MemoryMB != nil && *d.Issued.MemoryMB <= 0 {
		return false
	}
	for _, at := range []*time.Time{d.Reported.CompletedAt, d.Reported.ContainerStartedAt} {
		if at != nil && at.IsZero() {
			return false
		}
	}
	switch d.Reported.DigestCoverage {
	case "reported":
		if d.Reported.ImageDigest == nil || !strings.HasPrefix(*d.Reported.ImageDigest, "sha256:") || !observationHash(strings.TrimPrefix(*d.Reported.ImageDigest, "sha256:")) {
			return false
		}
	case "not_reported", "unsupported":
		if d.Reported.ImageDigest != nil {
			return false
		}
	default:
		return false
	}
	return true
}
func observationHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && s == strings.ToLower(s)
}

type runnerCreationObservationEnvelope struct {
	InstallationID string                        `json:"installation_id"`
	Data           RunnerCreationObservationData `json:"data"`
}

// PutRunnerCreationObservation requires a trusted result handler. It verifies
// live ownership, not caller authorisation. Identical retries never renew age.
// Only the first retained observation can occupy a runner's immutable slot.
func (s *Store) PutRunnerCreationObservation(ctx context.Context, installation string, d *RunnerCreationObservationData) (*RunnerCreationObservation, error) {
	if installation == "" || len(installation) > 128 || !ValidRunnerCreationObservation(d) {
		return nil, ErrInvalidRunnerCreationObservation
	}
	payload, err := json.Marshal(runnerCreationObservationEnvelope{InstallationID: installation, Data: *d})
	if err != nil || len(payload) > MaxRunnerCreationObservationBytes {
		return nil, ErrInvalidRunnerCreationObservation
	}
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	now := s.Now().UTC().Truncate(time.Millisecond)
	received := now
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var oldScope, oldHash string
		var oldPayload []byte
		var at int64
		err := tx.QueryRowContext(ctx, `SELECT installation_id,sha256,payload,received_at FROM runner_creation_observations WHERE runner_id=?`, d.RunnerID).Scan(&oldScope, &oldHash, &oldPayload, &at)
		if err == nil {
			if oldScope != installation || oldHash != digest {
				return ErrConflict
			}
			if !bytes.Equal(oldPayload, payload) {
				return ErrInvalidRunnerCreationObservation
			}
			if at <= ms(now.Add(-RunnerCreationObservationRetention)) {
				return ErrNotFound
			}
			received = time.UnixMilli(at).UTC()
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var pool, host, scope string
		if err := tx.QueryRowContext(ctx, `SELECT r.pool_id,r.host_id,p.installation_id FROM runners r JOIN pools p ON p.id=r.pool_id WHERE r.id=?`, d.RunnerID).Scan(&pool, &host, &scope); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidRunnerCreationObservation
			}
			return err
		}
		if pool != d.PoolID || host != d.HostID || scope != installation {
			return ErrInvalidRunnerCreationObservation
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO runner_creation_observations(runner_id,installation_id,sha256,payload,received_at) VALUES(?,?,?,?,?)`, d.RunnerID, installation, digest, payload, ms(now)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM runner_creation_observations WHERE received_at<=?`, ms(now.Add(-RunnerCreationObservationRetention))); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM runner_creation_observations WHERE installation_id=? AND sequence NOT IN (SELECT sequence FROM runner_creation_observations WHERE installation_id=? ORDER BY sequence DESC LIMIT ?)`, installation, installation, MaxRunnerCreationObservationsPerInstallation); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM runner_creation_observations WHERE sequence NOT IN (SELECT sequence FROM runner_creation_observations ORDER BY sequence DESC LIMIT ?)`, MaxRunnerCreationObservations)
		return err
	})
	if err != nil {
		return nil, err
	}
	var copy runnerCreationObservationEnvelope
	json.Unmarshal(payload, &copy)
	return &RunnerCreationObservation{InstallationID: installation, SHA256: digest, ReceivedAt: received, Data: copy.Data}, nil
}
func (s *Store) GetRunnerCreationObservation(ctx context.Context, installation, runner string) (*RunnerCreationObservation, error) {
	if installation == "" || runner == "" || len(installation) > 128 || len(runner) > 128 {
		return nil, ErrInvalidRunnerCreationObservation
	}
	r := &RunnerCreationObservation{InstallationID: installation}
	var payload []byte
	var at int64
	err := s.read.QueryRowContext(ctx, `SELECT sha256,payload,received_at FROM runner_creation_observations WHERE installation_id=? AND runner_id=? AND received_at>?`, installation, runner, ms(s.Now().Add(-RunnerCreationObservationRetention))).Scan(&r.SHA256, &payload, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(payload)
	if len(payload) > MaxRunnerCreationObservationBytes || hex.EncodeToString(sum[:]) != r.SHA256 {
		return nil, ErrInvalidRunnerCreationObservation
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var envelope runnerCreationObservationEnvelope
	if dec.Decode(&envelope) != nil || envelope.InstallationID != installation || !ValidRunnerCreationObservation(&envelope.Data) || envelope.Data.RunnerID != runner {
		return nil, ErrInvalidRunnerCreationObservation
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return nil, ErrInvalidRunnerCreationObservation
	}
	r.Data = envelope.Data
	r.ReceivedAt = time.UnixMilli(at).UTC()
	return r, nil
}
