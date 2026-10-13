package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/eyupio/zoomies/internal/store"
)

// Historical job links and immutable observations survive runner pruning. No
// current pool, host or backend settings may supply a missing observation.
func (c *Controller) experimentRunnerCreation(ctx context.Context, installation string, j *store.Job) (*store.RunnerCreationObservation, error) {
	if j.RunnerID == "" || j.PoolID == "" {
		return nil, nil
	}
	r, err := c.st.GetRunnerCreationObservation(ctx, installation, j.RunnerID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !experimentRunnerCreationMatches(installation, j.RunnerID, j.PoolID, j.HostID, r) {
		return nil, fmt.Errorf("%w: runner creation does not match the retained job", ErrExperimentEvidence)
	}
	return r, nil
}

func experimentRunnerCreationMatches(installation, runner, pool, host string, r *store.RunnerCreationObservation) bool {
	return store.ValidRetainedRunnerCreationObservation(r) && r.InstallationID == installation && runner != "" && pool != "" && r.Data.RunnerID == runner && r.Data.PoolID == pool && (host == "" || r.Data.HostID == host)
}

// Replay checks the original scoped digest and job bindings, not live rows or
// freshness. Agent reports remain claims and requested limits remain requests.
func validateExperimentRunnerCreations(installation string, a ExperimentJoinedAttempt, seenRunners map[string]bool) error {
	hasObservation := false
	for _, j := range a.MatchedJobs {
		hasObservation = hasObservation || j.RunnerCreation != nil
	}
	if !hasObservation {
		return nil
	}
	remote := map[int64]bool{}
	for _, j := range a.Remote.Jobs {
		remote[j.ID] = true
	}
	seenJobs := map[int64]bool{}
	seenFleet := map[string]bool{}
	for _, j := range a.MatchedJobs {
		if j.FleetJobID == "" || !remote[j.GitHubJobID] || seenJobs[j.GitHubJobID] || seenFleet[j.FleetJobID] {
			return fmt.Errorf("%w: runner creation job binding is invalid", ErrExperimentEvidence)
		}
		seenJobs[j.GitHubJobID] = true
		seenFleet[j.FleetJobID] = true
		if j.RunnerCreation == nil {
			continue
		}
		if seenRunners[j.RunnerID] || !experimentRunnerCreationMatches(installation, j.RunnerID, j.PoolID, j.HostID, j.RunnerCreation) {
			return fmt.Errorf("%w: runner creation provenance is invalid", ErrExperimentEvidence)
		}
		seenRunners[j.RunnerID] = true
	}
	return nil
}

func cloneExperimentRunnerCreation(r *store.RunnerCreationObservation) *store.RunnerCreationObservation {
	if r == nil {
		return nil
	}
	copy := *r
	if r.Data.Issued.CPUs != nil {
		v := *r.Data.Issued.CPUs
		copy.Data.Issued.CPUs = &v
	}
	if r.Data.Issued.MemoryMB != nil {
		v := *r.Data.Issued.MemoryMB
		copy.Data.Issued.MemoryMB = &v
	}
	if r.Data.Reported.ImageDigest != nil {
		v := *r.Data.Reported.ImageDigest
		copy.Data.Reported.ImageDigest = &v
	}
	if r.Data.Reported.CompletedAt != nil {
		v := *r.Data.Reported.CompletedAt
		copy.Data.Reported.CompletedAt = &v
	}
	if r.Data.Reported.ContainerStartedAt != nil {
		v := *r.Data.Reported.ContainerStartedAt
		copy.Data.Reported.ContainerStartedAt = &v
	}
	return &copy
}
