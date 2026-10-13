package controller

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/backend"
	"github.com/eyupio/zoomies/internal/store"
)

func creationProjectionFixture() (agent.Task, agent.TaskResult) {
	at := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	started := at.Add(time.Minute)
	task := agent.Task{ID: "task_one", Kind: agent.TaskCreateRunner, RunnerID: "run_one", PoolID: "pool_one", Backend: store.BackendDocker, IssuedAt: at, Spec: &backend.Spec{RunnerID: "run_one", PoolID: "pool_one", Image: "registry.example/runner:latest", Resources: store.Resources{CPUs: 2, MemoryMB: 8192}, DockerMode: store.DockerDinD, Credentials: backend.Credentials{JITConfig: "private-jit-token", RegistrationToken: "private-registration-token", URL: "private-registration-url"}, Env: map[string]string{"TOKEN": "private-env-token"}}}
	res := agent.TaskResult{TaskID: task.ID, Kind: task.Kind, RunnerID: task.RunnerID, OK: true, Digest: "sha256:" + strings.Repeat("a", 64), CompletedAt: at.Add(2 * time.Minute), ContainerStartedAt: &started, Error: "private-error", Handle: "private-handle"}
	return task, res
}
func TestRunnerCreationProjectionSeparatesIssuedFromReportedAndExcludesSensitiveData(t *testing.T) {
	task, res := creationProjectionFixture()
	d, err := buildRunnerCreationObservation("host_one", task, res)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(d)
	for _, secret := range []string{"private-jit-token", "private-registration-token", "private-registration-url", "private-env-token", "private-error", "private-handle", task.Spec.Image, "credentials", "environment", "applied_cpus"} {
		if strings.Contains(string(payload), secret) {
			t.Errorf("retained forbidden field %q", secret)
		}
	}
	if d.Issued.ImageReferenceSHA256 == "" || *d.Issued.CPUs != 2 || d.Reported.ImageDigest == nil || *d.Reported.ImageDigest != res.Digest || !d.IssuedAt.Equal(task.IssuedAt) {
		t.Fatal("lost provenance")
	}
	*res.ContainerStartedAt = res.ContainerStartedAt.Add(time.Hour)
	task.Spec.Resources.CPUs = 99
	if *d.Issued.CPUs != 2 || d.Reported.ContainerStartedAt.Equal(*res.ContainerStartedAt) {
		t.Fatal("projection aliases source")
	}
}
func TestRunnerCreationProjectionKeepsLegacyAndUnsupportedReportedFactsUnknown(t *testing.T) {
	for _, digest := range []string{"", "legacy-token"} {
		task, res := creationProjectionFixture()
		task.Spec.Resources = store.Resources{}
		res.Digest = digest
		res.CompletedAt = time.Time{}
		res.ContainerStartedAt = nil
		d, err := buildRunnerCreationObservation("host_one", task, res)
		if err != nil || d.Issued.CPUs != nil || d.Issued.MemoryMB != nil || d.Reported.ImageDigest != nil || d.Reported.CompletedAt != nil || d.Reported.ContainerStartedAt != nil {
			t.Fatalf("invented legacy facts %+v %v", d, err)
		}
		want := "not_reported"
		if digest != "" {
			want = "unsupported"
		}
		if d.Reported.DigestCoverage != want {
			t.Fatal("lost unknown digest basis")
		}
	}
}
func TestRunnerCreationProjectionRefusesMismatchedIdentityAndInvalidRequests(t *testing.T) {
	for _, mode := range []string{"task", "runner", "pool", "kind", "nan", "negative", "issued_at"} {
		task, res := creationProjectionFixture()
		switch mode {
		case "task":
			res.TaskID = "other"
		case "runner":
			res.RunnerID = "other"
		case "pool":
			task.Spec.PoolID = "other"
		case "kind":
			res.Kind = agent.TaskStopRunner
		case "nan":
			task.Spec.Resources.CPUs = math.NaN()
		case "negative":
			task.Spec.Resources.MemoryMB = -1
		case "issued_at":
			task.IssuedAt = time.Time{}
		}
		if _, err := buildRunnerCreationObservation("host_one", task, res); !errors.Is(err, store.ErrInvalidRunnerCreationObservation) {
			t.Errorf("accepted %s: %v", mode, err)
		}
	}
	task, res := creationProjectionFixture()
	res.OK = false
	if d, err := buildRunnerCreationObservation("host_one", task, res); d != nil || err != nil {
		t.Fatal("recorded failed result")
	}
}
