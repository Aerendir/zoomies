package controller

import (
	"fmt"
	"math"

	"github.com/eyupio/zoomies/internal/store"
)

const experimentGrantBasis = "retained_job_granted_size_v1"

// ExperimentRecordedGrant is a controller stamp, not backend attestation. It
// records the guaranteed assignment size, not burst limits, usage or an image.
// No observation timestamp is invented from the job's execution timestamps.
type ExperimentRecordedGrant struct {
	Basis    string   `json:"basis"`
	Source   string   `json:"source"`
	CPUs     *float64 `json:"cpus"`
	MemoryMB *int64   `json:"memory_mb"`
}

func experimentRecordedGrant(j *store.Job) (*ExperimentRecordedGrant, error) {
	if j.GrantedCPUs < 0 || math.IsNaN(j.GrantedCPUs) || math.IsInf(j.GrantedCPUs, 0) || j.GrantedMemoryMB < 0 {
		return nil, fmt.Errorf("%w: recorded granted size is invalid", ErrExperimentEvidence)
	}
	if !knownExperimentGrantSource(j.GrantedSource) || (j.GrantedCPUs == 0 && j.GrantedMemoryMB == 0) {
		return nil, nil
	}
	g := &ExperimentRecordedGrant{Basis: experimentGrantBasis, Source: j.GrantedSource}
	if j.GrantedCPUs > 0 {
		v := j.GrantedCPUs
		g.CPUs = &v
	}
	if j.GrantedMemoryMB > 0 {
		v := j.GrantedMemoryMB
		g.MemoryMB = &v
	}
	return g, nil
}
func knownExperimentGrantSource(s string) bool {
	switch s {
	case store.AllocationFromPool, store.AllocationFromHost, store.AllocationReduced, store.AllocationHistory, store.AllocationFromProfile:
		return true
	}
	return false
}
func cloneExperimentGrant(g *ExperimentRecordedGrant) *ExperimentRecordedGrant {
	if g == nil {
		return nil
	}
	copy := *g
	if g.CPUs != nil {
		v := *g.CPUs
		copy.CPUs = &v
	}
	if g.MemoryMB != nil {
		v := *g.MemoryMB
		copy.MemoryMB = &v
	}
	return &copy
}

// Retained envelopes are replayed without reading current jobs or runners.
// Reject malformed provenance rather than keeping a valid-looking report over
// evidence that this client no longer understands.
func validateExperimentGrants(a ExperimentJoinedAttempt) error {
	remote := map[int64]bool{}
	for _, j := range a.Remote.Jobs {
		remote[j.ID] = true
	}
	seen := map[int64]bool{}
	for _, j := range a.MatchedJobs {
		if j.RecordedGrant == nil {
			continue
		}
		g := j.RecordedGrant
		if j.FleetJobID == "" || !remote[j.GitHubJobID] || seen[j.GitHubJobID] || g.Basis != experimentGrantBasis || !knownExperimentGrantSource(g.Source) || (g.CPUs == nil && g.MemoryMB == nil) || g.CPUs != nil && (*g.CPUs <= 0 || math.IsNaN(*g.CPUs) || math.IsInf(*g.CPUs, 0)) || g.MemoryMB != nil && *g.MemoryMB <= 0 {
			return fmt.Errorf("%w: recorded grant provenance is invalid", ErrExperimentEvidence)
		}
		seen[j.GitHubJobID] = true
	}
	return nil
}
