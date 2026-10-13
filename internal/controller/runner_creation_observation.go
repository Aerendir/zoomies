package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/backend"
	"github.com/eyupio/zoomies/internal/store"
)

// Projection after authenticating and correlating a live create result. No full Spec, credentials, environment,
// workload handle, names, errors, logs or current pool settings enter the record.
func buildRunnerCreationObservation(host string, task agent.Task, res agent.TaskResult) (*store.RunnerCreationObservationData, error) {
	if task.Kind != agent.TaskCreateRunner || !res.OK || res.NotStarted {
		return nil, nil
	}
	if task.ID != res.TaskID || task.RunnerID != res.RunnerID || task.Spec == nil || task.Spec.RunnerID != task.RunnerID || task.Spec.PoolID != task.PoolID || (res.Kind != "" && res.Kind != task.Kind) {
		return nil, fmt.Errorf("%w: create result identity does not match the issued task", store.ErrInvalidRunnerCreationObservation)
	}
	spec := task.Spec
	d := &store.RunnerCreationObservationData{Version: store.RunnerCreationObservationVersion, HostID: host, RunnerID: task.RunnerID, PoolID: task.PoolID, TaskID: task.ID, IssuedAt: task.IssuedAt, Issued: store.IssuedRunnerConfiguration{Backend: task.Backend, DockerMode: spec.DockerMode, CPUTyped: spec.CPUTyped, MemoryTyped: spec.MemoryTyped}, Reported: store.ReportedRunnerCreation{DigestCoverage: "not_reported"}}
	if len(spec.Image) > 4096 || math.IsNaN(spec.Resources.CPUs) || math.IsInf(spec.Resources.CPUs, 0) || spec.Resources.CPUs < 0 || spec.Resources.MemoryMB < 0 {
		return nil, store.ErrInvalidRunnerCreationObservation
	}
	if spec.Image != "" {
		sum := sha256.Sum256([]byte(spec.Image))
		d.Issued.ImageReferenceSHA256 = hex.EncodeToString(sum[:])
	}
	if spec.Resources.CPUs > 0 {
		v := spec.Resources.CPUs
		d.Issued.CPUs = &v
	}
	if spec.Resources.MemoryMB > 0 {
		v := spec.Resources.MemoryMB
		d.Issued.MemoryMB = &v
	}
	if knownExperimentGrantSource(spec.ResourcesSource) {
		d.Issued.ResourceSource = spec.ResourcesSource
	}
	copyTime := func(t *time.Time) *time.Time {
		if t == nil || t.IsZero() {
			return nil
		}
		v := *t
		return &v
	}
	d.Reported.CompletedAt = copyTime(&res.CompletedAt)
	d.Reported.ContainerStartedAt = copyTime(res.ContainerStartedAt)
	if res.Digest != "" {
		d.Reported.DigestCoverage = "unsupported"
		if strings.HasPrefix(res.Digest, "sha256:") && len(res.Digest) == 71 {
			if _, err := hex.DecodeString(res.Digest[7:]); err == nil {
				v := strings.ToLower(res.Digest)
				d.Reported.ImageDigest = &v
				d.Reported.DigestCoverage = "reported"
			}
		}
	}
	if !store.ValidRunnerCreationObservation(d) {
		return nil, store.ErrInvalidRunnerCreationObservation
	}
	return d, nil
}

// The embedded agent shares task pointers with the controller. Freeze only the
// fields this evidence permits, before handing the task to either transport.
func freezeRunnerCreationTask(t agent.Task) agent.Task {
	if t.Kind != agent.TaskCreateRunner || t.Spec == nil {
		return agent.Task{}
	}
	spec := t.Spec
	return agent.Task{ID: t.ID, Kind: t.Kind, RunnerID: t.RunnerID, PoolID: t.PoolID, Backend: t.Backend, IssuedAt: t.IssuedAt, Spec: &backend.Spec{RunnerID: spec.RunnerID, PoolID: spec.PoolID, Image: spec.Image, Resources: store.Resources{CPUs: spec.Resources.CPUs, MemoryMB: spec.Resources.MemoryMB}, ResourcesSource: spec.ResourcesSource, DockerMode: spec.DockerMode, CPUTyped: spec.CPUTyped, MemoryTyped: spec.MemoryTyped}}
}

// Evidence collection must not trap a live runner or block cleanup. Missing
// observations remain missing; no fallback to current configuration is allowed.
// Lifecycle writes retain their existing error/lease retry behaviour.
func (c *Controller) recordRunnerCreationResult(ctx context.Context, host string, r *store.Runner, res agent.TaskResult) {
	if !res.OK || res.NotStarted || r.HostID != host {
		return
	}
	task, known := c.queues.get(host).creationTaskSnapshot(res.TaskID)
	if !known || task.RunnerID != r.ID || task.PoolID != r.PoolID {
		return
	}
	data, err := buildRunnerCreationObservation(host, task, res)
	if err != nil {
		c.log.Warn("runner creation observation unavailable", "runner", r.ID, "task", res.TaskID, "reason", "invalid_projection")
		return
	}
	if data == nil {
		return
	}
	// Scope is current ownership at receipt, not proof of installation ownership
	// at issue. Store checks it again atomically with the first observation write.
	pool, err := c.st.GetPool(ctx, r.PoolID)
	if err != nil || pool.InstallationID == "" {
		c.log.Warn("runner creation observation unavailable", "runner", r.ID, "task", res.TaskID, "reason", "scope_unavailable")
		return
	}
	if _, err := c.st.PutRunnerCreationObservation(ctx, pool.InstallationID, data); err != nil {
		reason := "storage_failure"
		switch {
		case errors.Is(err, store.ErrConflict):
			reason = "immutable_conflict"
		case errors.Is(err, store.ErrNotFound):
			reason = "expired_observation"
		case errors.Is(err, store.ErrInvalidRunnerCreationObservation):
			reason = "invalid_observation"
		}
		c.log.Warn("runner creation observation unavailable", "runner", r.ID, "task", res.TaskID, "reason", reason)
	}
}
