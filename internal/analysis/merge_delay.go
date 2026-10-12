// Package analysis computes evidence-backed findings from explicit snapshots.
// It does not fetch evidence or decide whether a caller may read it.
package analysis

import (
	"errors"
	"sort"
	"time"
)

const MergeDelayVersion = "merge-delay-v1"
const MaxRequiredChecks = 500
const MaxCheckObservations = 5000

// Coverage describes the reader's knowledge, not whether checks passed.
type Coverage string

const (
	CoverageComplete     Coverage = "complete"
	CoveragePartial      Coverage = "partial"
	CoverageNotPermitted Coverage = "not_permitted"
	CoverageNotAvailable Coverage = "not_available"
	CoverageHeldBack     Coverage = "held_back"
	CoverageFailed       Coverage = "failed"
)

// RequiredCheck uses opaque identities. The reader must resolve producer and
// target-branch rules before mapping observations; job names alone do not suffice.
type RequiredCheck struct {
	ID     string
	Mapped bool
}

// CheckObservation identifies one logical instance in a workflow run. Matrix
// members need distinct InstanceIDs; retries retain that identity and raise Attempt.
type CheckObservation struct {
	ID            string
	CheckID       string
	RunID         string
	InstanceID    string
	Attempt       int
	HeadSHA       string
	Conclusion    string
	QueuedAt      time.Time
	ProvisionedAt *time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
}

type MergeDelaySnapshot struct {
	HeadSHA        string
	CurrentHeadSHA string
	TargetBranch   string
	ObservedAt     time.Time
	// WindowStart is the start of this head's observation, not a job's start.
	WindowStart         time.Time
	ProtectionCoverage  Coverage
	ObservationCoverage Coverage
	Required            []RequiredCheck
	Observations        []CheckObservation
}

// Timing partitions a job's observable lifetime. Unknown phases remain nil.
// Startup is carved out of pre-start wait, never added on top of queue wait.
type CheckTiming struct {
	PreStartMS  *int64
	QueueMS     *int64
	StartupMS   *int64
	ExecutionMS *int64
}

type CheckDelay struct {
	CheckID           string
	State             string
	ObservationID     string
	CompletionDelayMS *int64
	Timing            CheckTiming
}

type MergeDelayResult struct {
	Version             string
	HeadSHA             string
	TargetBranch        string
	ObservedAt          time.Time
	WindowStart         time.Time
	ProtectionCoverage  Coverage
	ObservationCoverage Coverage
	State               string
	Checks              []CheckDelay
	// This is an observed completion order, not a dependency critical path or
	// a verdict on reviews, conflicts, merge queues or full merge readiness.
	LastFinishingCheckID string
	IgnoredOtherHeads    int
}

// MergeDelay never guesses required checks when protection is unavailable.
// Its caller supplies bounded, authorised evidence and a clock snapshot.
func MergeDelay(s MergeDelaySnapshot) (MergeDelayResult, error) {
	r := MergeDelayResult{Version: MergeDelayVersion, HeadSHA: s.HeadSHA,
		TargetBranch: s.TargetBranch, ObservedAt: s.ObservedAt, WindowStart: s.WindowStart,
		ProtectionCoverage: s.ProtectionCoverage, ObservationCoverage: s.ObservationCoverage,
		Checks: []CheckDelay{}}
	if s.HeadSHA == "" || s.TargetBranch == "" || s.ObservedAt.IsZero() || s.WindowStart.IsZero() || s.WindowStart.After(s.ObservedAt) {
		return r, errors.New("merge-delay snapshot needs a head, target branch and ordered observation window")
	}
	if !validCoverage(s.ProtectionCoverage) || !validCoverage(s.ObservationCoverage) {
		return r, errors.New("merge-delay snapshot needs explicit coverage states")
	}
	if len(s.Required) > MaxRequiredChecks || len(s.Observations) > MaxCheckObservations {
		return r, errors.New("merge-delay snapshot exceeds the check or observation limit")
	}
	if s.CurrentHeadSHA == "" || s.CurrentHeadSHA != s.HeadSHA {
		r.State = "superseded"
		if s.CurrentHeadSHA == "" {
			r.State = "currentness_unknown"
		}
		return r, nil
	}
	seen := map[string]bool{}
	for _, c := range s.Required {
		if c.ID == "" || seen[c.ID] {
			return r, errors.New("required checks need unique opaque identities")
		}
		seen[c.ID] = true
	}
	byCheck := map[string][]CheckObservation{}
	for _, o := range s.Observations {
		if o.HeadSHA != s.HeadSHA {
			r.IgnoredOtherHeads++
			continue
		}
		if !seen[o.CheckID] {
			continue
		}
		if o.ID == "" || o.RunID == "" || o.InstanceID == "" || o.Attempt < 1 {
			return r, errors.New("mapped observations need an identity, run, instance and positive attempt")
		}
		byCheck[o.CheckID] = append(byCheck[o.CheckID], o)
	}
	complete := s.ProtectionCoverage == CoverageComplete && s.ObservationCoverage == CoverageComplete
	pending, failed := false, false
	var latest time.Time
	for _, c := range s.Required {
		d := CheckDelay{CheckID: c.ID, State: "missing"}
		if !c.Mapped {
			d.State = "unmapped"
		} else {
			// Two distinct runs producing one required identity are ambiguous.
			// Multiple latest matrix members are likewise not silently collapsed.
			selected := map[[2]string]CheckObservation{}
			ambiguous := false
			for _, o := range byCheck[c.ID] {
				key := [2]string{o.RunID, o.InstanceID}
				prev, exists := selected[key]
				if !exists || o.Attempt > prev.Attempt {
					selected[key] = o
				}
			}
			for _, o := range byCheck[c.ID] {
				prev := selected[[2]string{o.RunID, o.InstanceID}]
				if o.Attempt == prev.Attempt && o.ID != prev.ID {
					ambiguous = true
				}
			}
			if len(selected) > 1 || ambiguous {
				d.State = "ambiguous"
			} else {
				for _, o := range selected {
					d.ObservationID = o.ID
					if !validTimeline(o, s) {
						d.State = "invalid_timing"
						break
					}
					d.Timing = observationTiming(o)
					if o.CompletedAt == nil {
						d.State = "pending"
						break
					}
					d.CompletionDelayMS = milliseconds(o.CompletedAt.Sub(s.WindowStart))
					d.State = "failed"
					if o.Conclusion == "success" || o.Conclusion == "neutral" || o.Conclusion == "skipped" {
						d.State = "passed"
					}
					if o.Conclusion == "" {
						d.State = "unknown_outcome"
					}
					if o.CompletedAt.After(latest) || (o.CompletedAt.Equal(latest) && (r.LastFinishingCheckID == "" || c.ID < r.LastFinishingCheckID)) {
						latest = *o.CompletedAt
						r.LastFinishingCheckID = c.ID
					}
				}
			}
		}
		switch d.State {
		case "pending":
			pending = true
		case "failed":
			failed = true
		case "passed":
		default:
			complete = false
		}
		r.Checks = append(r.Checks, d)
	}
	sort.Slice(r.Checks, func(i, j int) bool { return r.Checks[i].CheckID < r.Checks[j].CheckID })
	switch {
	case !complete:
		r.State = "incomplete"
	case len(s.Required) == 0:
		r.State = "no_required_checks"
	case failed:
		r.State = "checks_failed"
	case pending:
		r.State = "checks_pending"
	default:
		r.State = "checks_passed"
	}
	return r, nil
}

func validCoverage(c Coverage) bool {
	switch c {
	case CoverageComplete, CoveragePartial, CoverageNotPermitted, CoverageNotAvailable, CoverageHeldBack, CoverageFailed:
		return true
	}
	return false
}

func validTimeline(o CheckObservation, s MergeDelaySnapshot) bool {
	if o.QueuedAt.IsZero() || o.QueuedAt.Before(s.WindowStart) || o.QueuedAt.After(s.ObservedAt) {
		return false
	}
	prev := o.QueuedAt
	for _, at := range []*time.Time{o.ProvisionedAt, o.StartedAt, o.CompletedAt} {
		if at == nil {
			continue
		}
		if at.Before(prev) || at.After(s.ObservedAt) {
			return false
		}
		prev = *at
	}
	// GitHub can complete a skipped check without ever starting its job.
	return o.CompletedAt == nil || o.StartedAt != nil || o.Conclusion == "skipped" || o.Conclusion == "neutral"
}

func milliseconds(d time.Duration) *int64 { v := d.Milliseconds(); return &v }

func observationTiming(o CheckObservation) CheckTiming {
	t := CheckTiming{}
	if o.StartedAt != nil {
		t.PreStartMS = milliseconds(o.StartedAt.Sub(o.QueuedAt))
		if o.ProvisionedAt != nil {
			t.QueueMS = milliseconds(o.ProvisionedAt.Sub(o.QueuedAt))
			t.StartupMS = milliseconds(o.StartedAt.Sub(*o.ProvisionedAt))
		}
	}
	if o.CompletedAt != nil && o.StartedAt != nil {
		t.ExecutionMS = milliseconds(o.CompletedAt.Sub(*o.StartedAt))
	}
	return t
}
