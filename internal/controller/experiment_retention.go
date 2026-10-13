package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/eyupio/zoomies/internal/analysis"
	"github.com/eyupio/zoomies/internal/store"
)

type experimentRetainedEnvelope struct {
	Version   string                      `json:"version"`
	Record    ExperimentComparisonRecord  `json:"record"`
	Baseline  *ExperimentJoinedEvidence   `json:"baseline"`
	Candidate *ExperimentJoinedEvidence   `json:"candidate"`
	Report    *ExperimentComparisonReport `json:"report"`
}

type RetainedExperimentComparison struct {
	ID        string                      `json:"id"`
	SHA256    string                      `json:"sha256"`
	CreatedAt time.Time                   `json:"created_at"`
	Report    *ExperimentComparisonReport `json:"report"`
}

// RetainExperimentComparison requires an authenticated, authorised actor from
// the transport. Actor and evidence identity are never accepted from a record.
// Nothing succeeds until the complete envelope and audit are durably committed.
func (c *Controller) RetainExperimentComparison(ctx context.Context, actor *store.AuditEvent, record ExperimentComparisonRecord, a, b *ExperimentJoinedEvidence) (*RetainedExperimentComparison, error) {
	if actor == nil || actor.ActorID == "" {
		return nil, ErrExperimentEvidence
	}
	record.InitiatingUser = actor.ActorID
	record.EvidenceID = store.NewID(store.PrefixComparison)
	report, err := AssembleExperimentComparison(record, a, b)
	if err != nil {
		return nil, err
	}
	envelope := experimentRetainedEnvelope{Version: store.ComparisonEvidenceVersion, Record: record, Baseline: a, Candidate: b, Report: report}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	row := &store.ComparisonEvidence{ID: record.EvidenceID, InstallationID: a.InstallationID, Repository: record.Repository, InitiatingUser: actor.ActorID, Version: envelope.Version, Payload: payload}
	if err := c.st.PutComparisonEvidence(ctx, row, actor); err != nil {
		return nil, err
	}
	return &RetainedExperimentComparison{ID: row.ID, SHA256: row.SHA256, CreatedAt: row.CreatedAt, Report: report}, nil
}

// ReplayExperimentComparison reads only retained evidence. Its digest checks
// stored bytes; recomputation also checks the report against its recorded input.
// The transport must authorise the repository read before invoking this method.
func (c *Controller) ReplayExperimentComparison(ctx context.Context, installation, repo, id string) (*RetainedExperimentComparison, error) {
	row, err := c.st.GetComparisonEvidence(ctx, installation, repo, id)
	if err != nil {
		return nil, err
	}
	if row.Version != store.ComparisonEvidenceVersion {
		return nil, ErrExperimentEvidence
	}
	var envelope experimentRetainedEnvelope
	decoder := json.NewDecoder(bytes.NewReader(row.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, ErrExperimentEvidence
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, ErrExperimentEvidence
	}
	if envelope.Version != row.Version || envelope.Report == nil || envelope.Baseline == nil || envelope.Candidate == nil || envelope.Record.EvidenceID != row.ID || envelope.Record.Repository != row.Repository || envelope.Record.InitiatingUser != row.InitiatingUser || envelope.Baseline.InstallationID != row.InstallationID || envelope.Candidate.InstallationID != row.InstallationID || envelope.Report.Snapshot.RuleVersion != analysis.ExperimentRuleVersion {
		return nil, ErrExperimentEvidence
	}
	recomputed, err := AssembleExperimentComparison(envelope.Record, envelope.Baseline, envelope.Candidate)
	if err != nil {
		return nil, err
	}
	oldJSON, err := json.Marshal(envelope.Report)
	if err != nil {
		return nil, err
	}
	newJSON, err := json.Marshal(recomputed)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(oldJSON, newJSON) {
		return nil, ErrExperimentEvidence
	}
	return &RetainedExperimentComparison{ID: row.ID, SHA256: row.SHA256, CreatedAt: row.CreatedAt, Report: recomputed}, nil
}
