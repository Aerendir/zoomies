package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/store"
)

// Projection only: the next slice will call this after authenticating and
// correlating a live create result. No full Spec, credentials, environment,
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
