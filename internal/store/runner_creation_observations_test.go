package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func creationObservationFixture(t *testing.T, s *Store) (string, *RunnerCreationObservationData) {
	t.Helper()
	ctx := context.Background()
	inst := &Installation{AppID: 1, InstallationID: 2, Target: "acme/repo", TargetType: TargetRepo}
	if err := s.CreateInstallation(ctx, inst); err != nil {
		t.Fatal(err)
	}
	pool := &Pool{Name: "observation", InstallationID: inst.ID, Backend: BackendDocker, DockerMode: DockerNone, MaxRunners: 1, Ephemeral: true}
	if err := s.CreatePool(ctx, pool); err != nil {
		t.Fatal(err)
	}
	host := &Host{Name: "observation-host", Capacity: 1}
	if err := s.CreateHost(ctx, host); err != nil {
		t.Fatal(err)
	}
	r := &Runner{Name: "observation-runner", PoolID: pool.ID, HostID: host.ID}
	if err := s.CreateRunner(ctx, r); err != nil {
		t.Fatal(err)
	}
	cpu := 2.0
	mem := int64(8192)
	return inst.ID, &RunnerCreationObservationData{Version: RunnerCreationObservationVersion, RunnerID: r.ID, HostID: host.ID, PoolID: pool.ID, TaskID: "task_one", IssuedAt: s.Now(), Issued: IssuedRunnerConfiguration{Backend: BackendDocker, CPUs: &cpu, MemoryMB: &mem, DockerMode: DockerNone, ResourceSource: AllocationFromPool}, Reported: ReportedRunnerCreation{DigestCoverage: "not_reported"}}
}

func TestRetainedRunnerCreationDigestDoesNotDependOnLiveAgeOrRunnerRows(t *testing.T) {
	s := newTestStore(t)
	installation, data := creationObservationFixture(t, s)
	// JSON timestamps must retain their offset and sub-millisecond precision
	// when a decoded observation is copied into a comparison envelope.
	data.IssuedAt = time.Date(2026, 10, 13, 1, 2, 3, 123456789, time.FixedZone("fixture", 3600))
	r, err := s.PutRunnerCreationObservation(context.Background(), installation, data)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidRetainedRunnerCreationObservation(r) {
		t.Fatal("valid scoped digest rejected")
	}
	payload, _ := json.Marshal(r)
	var copy RunnerCreationObservation
	if err := json.Unmarshal(payload, &copy); err != nil || !ValidRetainedRunnerCreationObservation(&copy) {
		t.Fatal("JSON round trip changed canonical digest", err)
	}
	copy.ReceivedAt = copy.ReceivedAt.Add(-2 * RunnerCreationObservationRetention)
	if !ValidRetainedRunnerCreationObservation(&copy) {
		t.Fatal("replay applied live expiry")
	}
	copy.InstallationID = "ins_foreign"
	if ValidRetainedRunnerCreationObservation(&copy) {
		t.Fatal("digest did not bind installation")
	}
	copy = *r
	copy.Data.Issued.CPUs = new(float64)
	*copy.Data.Issued.CPUs = 8
	if ValidRetainedRunnerCreationObservation(&copy) {
		t.Fatal("altered request accepted")
	}
	if ValidRetainedRunnerCreationObservation(nil) {
		t.Fatal("missing evidence accepted")
	}
}
func TestRunnerCreationObservationIsImmutableScopedAndSurvivesRunnerPruning(t *testing.T) {
	s := newTestStore(t)
	inst, d := creationObservationFixture(t, s)
	ctx := context.Background()
	first, err := s.PutRunnerCreationObservation(ctx, inst, d)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRunner(ctx, d.RunnerID); err != nil {
		t.Fatal(err)
	}
	again, err := s.PutRunnerCreationObservation(ctx, inst, d)
	if err != nil || !again.ReceivedAt.Equal(first.ReceivedAt) || again.SHA256 != first.SHA256 {
		t.Fatalf("retry renewed or changed observation: %+v %v", again, err)
	}
	read, err := s.GetRunnerCreationObservation(ctx, inst, d.RunnerID)
	if err != nil || *read.Data.Issued.CPUs != 2 {
		t.Fatal("lost pruned-runner evidence", err)
	}
	*d.Issued.CPUs = 8
	if _, err := s.PutRunnerCreationObservation(ctx, inst, d); !errors.Is(err, ErrConflict) {
		t.Fatal("overwrote first observation", err)
	}
	if _, err := s.GetRunnerCreationObservation(ctx, "ins_other", d.RunnerID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-scope read", err)
	}
	if _, err := s.DeleteInstallation(ctx, inst); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRunnerCreationObservation(ctx, inst, d.RunnerID); !errors.Is(err, ErrNotFound) {
		t.Fatal("installation cleanup failed", err)
	}
}
func TestRunnerCreationObservationRefusesWrongOwnershipAndCorruptPayload(t *testing.T) {
	s := newTestStore(t)
	inst, d := creationObservationFixture(t, s)
	ctx := context.Background()
	original := d.HostID
	d.HostID = "host_other"
	if _, err := s.PutRunnerCreationObservation(ctx, inst, d); !errors.Is(err, ErrInvalidRunnerCreationObservation) {
		t.Fatal("cross-host write", err)
	}
	d.HostID = original
	record, err := s.PutRunnerCreationObservation(ctx, inst, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.exec(ctx, `UPDATE runner_creation_observations SET payload=? WHERE runner_id=?`, []byte(`{}`), d.RunnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRunnerCreationObservation(ctx, inst, d.RunnerID); !errors.Is(err, ErrInvalidRunnerCreationObservation) {
		t.Fatal("accepted corrupt hash", err)
	}
	payload, _ := json.Marshal(runnerCreationObservationEnvelope{InstallationID: inst, Data: *d})
	payload = append(payload[:len(payload)-1], []byte(`,"environment":{"TOKEN":"must-not-pass"}}`)...)
	sum := sha256.Sum256(payload)
	if _, err := s.exec(ctx, `UPDATE runner_creation_observations SET payload=?,sha256=? WHERE runner_id=?`, payload, hex.EncodeToString(sum[:]), record.Data.RunnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRunnerCreationObservation(ctx, inst, d.RunnerID); !errors.Is(err, ErrInvalidRunnerCreationObservation) {
		t.Fatal("accepted unreviewed fields", err)
	}
}
func TestRunnerCreationObservationHasBoundedAgeAndSequenceEviction(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	s, err := Open(ctx, Options{Path: ":memory:", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	inst, d := creationObservationFixture(t, s)
	if err := s.tx(ctx, func(tx *sql.Tx) error {
		for n := 0; n < MaxRunnerCreationObservationsPerInstallation; n++ {
			if _, err := tx.ExecContext(ctx, `INSERT INTO runner_creation_observations(runner_id,installation_id,sha256,payload,received_at) VALUES(?,?,?,?,?)`, fmt.Sprintf("run_seed_%d", n), inst, "seed", []byte(`{}`), ms(now)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutRunnerCreationObservation(ctx, inst, d); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.read.QueryRowContext(ctx, `SELECT count(*) FROM runner_creation_observations WHERE installation_id=?`, inst).Scan(&count); err != nil || count != MaxRunnerCreationObservationsPerInstallation {
		t.Fatalf("count=%d %v", count, err)
	}
	if _, err := s.GetRunnerCreationObservation(ctx, inst, d.RunnerID); err != nil {
		t.Fatal("evicted newest at frozen clock", err)
	}
	if _, err := s.GetRunnerCreationObservation(ctx, inst, "run_seed_0"); !errors.Is(err, ErrNotFound) {
		t.Fatal("kept oldest", err)
	}
	now = now.Add(RunnerCreationObservationRetention)
	if _, err := s.GetRunnerCreationObservation(ctx, inst, d.RunnerID); !errors.Is(err, ErrNotFound) {
		t.Fatal("kept expired read", err)
	}
	if _, err := s.PutRunnerCreationObservation(ctx, inst, d); !errors.Is(err, ErrNotFound) {
		t.Fatal("renewed expired retry", err)
	}
}
func TestRunnerCreationObservationGlobalPruningKeepsTheNewestAcrossInstallations(t *testing.T) {
	s := newTestStore(t)
	inst, d := creationObservationFixture(t, s)
	ctx := context.Background()
	other := &Installation{AppID: 2, InstallationID: 3, Target: "other/repo", TargetType: TargetRepo}
	if err := s.CreateInstallation(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err := s.tx(ctx, func(tx *sql.Tx) error {
		for n := 0; n < MaxRunnerCreationObservations; n++ {
			if _, err := tx.ExecContext(ctx, `INSERT INTO runner_creation_observations(runner_id,installation_id,sha256,payload,received_at) VALUES(?,?,?,?,?)`, fmt.Sprintf("run_global_%d", n), other.ID, "seed", []byte(`{}`), ms(s.Now())); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutRunnerCreationObservation(ctx, inst, d); err != nil {
		t.Fatal(err)
	}
	var count int
	s.read.QueryRowContext(ctx, `SELECT count(*) FROM runner_creation_observations`).Scan(&count)
	if count != MaxRunnerCreationObservations {
		t.Fatalf("global count=%d", count)
	}
	if _, err := s.GetRunnerCreationObservation(ctx, other.ID, "run_global_0"); !errors.Is(err, ErrNotFound) {
		t.Fatal("kept oldest global row", err)
	}
	if _, err := s.GetRunnerCreationObservation(ctx, inst, d.RunnerID); err != nil {
		t.Fatal("lost newest global row", err)
	}
}
