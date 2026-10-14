package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/eyupio/zoomies/internal/optimisation"
	"github.com/eyupio/zoomies/internal/prrepair"
	"github.com/eyupio/zoomies/internal/store"
)

var ErrOptimisationPreview = errors.New("optimisation preview evidence is unavailable, inconsistent or unsupported")

type optimisationRetainedSource struct {
	HeadSHA string `json:"head_sha"`
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	Content string `json:"content"`
}

func (s optimisationRetainedSource) source() optimisation.Source {
	return optimisation.Source{HeadSHA: s.HeadSHA, File: prrepair.File{Path: s.Path, Mode: s.Mode, Content: s.Content}}
}

type optimisationRetainedEnvelope struct {
	Version        string                     `json:"version"`
	ID             string                     `json:"id"`
	InstallationID string                     `json:"installation_id"`
	Repository     string                     `json:"repository"`
	ReviewedBy     string                     `json:"reviewed_by"`
	ModelFunding   string                     `json:"model_funding"`
	Source         optimisationRetainedSource `json:"source"`
	Timing         optimisation.Timing        `json:"timing"`
	History        optimisation.HistoryReview `json:"history"`
	Preview        optimisation.Preview       `json:"preview"`
}

type RetainedOptimisationPreview struct {
	ID             string
	InstallationID string
	Repository     string
	ReviewedBy     string
	SHA256         string
	CreatedAt      time.Time
	Currentness    string
	Preview        optimisation.Preview
}

// RetainOptimisationPreview is an internal adapter for already admitted source,
// timing and policy. It is not a collector and has no route. A future transport
// must independently authorise source-bearing history and obtain evidence; request
// booleans or client-submitted timing must never stand in for server admission.
func (c *Controller) RetainOptimisationPreview(ctx context.Context, actor *store.AuditEvent, installation, repo string, source optimisation.Source, timing optimisation.Timing, history optimisation.HistoryReview, policy optimisation.Policy) (*RetainedOptimisationPreview, error) {
	if actor == nil || actor.ActorID == "" || source.File.Partial || source.File.StartLine != 0 {
		return nil, ErrOptimisationPreview
	}
	preview, err := optimisation.PreviewCheckout(source, timing, history, policy)
	if err != nil {
		return nil, ErrOptimisationPreview
	}
	envelope := optimisationRetainedEnvelope{
		Version: store.OptimisationPreviewVersion, ID: store.NewID(store.PrefixOptimisationPreview),
		InstallationID: installation, Repository: repo, ReviewedBy: actor.ActorID,
		ModelFunding: "no_model_call",
		Source:       optimisationRetainedSource{HeadSHA: source.HeadSHA, Path: source.File.Path, Mode: source.File.Mode, Content: source.File.Content},
		Timing:       timing, History: history, Preview: preview,
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	row := &store.OptimisationPreview{ID: envelope.ID, InstallationID: installation, Repository: repo, InitiatingUser: actor.ActorID, Version: envelope.Version, Payload: payload}
	if err := c.st.PutOptimisationPreview(ctx, row, actor); err != nil {
		return nil, err
	}
	return retainedOptimisationView(row, preview), nil
}

// ReplayOptimisationPreview validates historical calculation, not current head
// or current publication policy. It never reads replacement source. Its private
// source-bearing result requires separate authorisation before any future route.
func (c *Controller) ReplayOptimisationPreview(ctx context.Context, installation, repo, id string) (*RetainedOptimisationPreview, error) {
	row, err := c.st.GetOptimisationPreview(ctx, installation, repo, id)
	if err != nil {
		return nil, err
	}
	var e optimisationRetainedEnvelope
	dec := json.NewDecoder(bytes.NewReader(row.Payload))
	dec.DisallowUnknownFields()
	if dec.Decode(&e) != nil || dec.Decode(new(any)) != io.EOF || row.Version != store.OptimisationPreviewVersion || e.Version != row.Version || e.ID != row.ID || e.InstallationID != row.InstallationID || e.Repository != row.Repository || e.ReviewedBy != row.InitiatingUser || e.ModelFunding != "no_model_call" {
		return nil, ErrOptimisationPreview
	}
	// Policy here permits recomputation only. Replay neither grants future edit
	// authority nor claims the historical preview remains current.
	preview, err := optimisation.PreviewCheckout(e.Source.source(), e.Timing, e.History, optimisation.Policy{SourceAllowed: true, WorkflowEditsAllowed: true})
	if err != nil {
		return nil, ErrOptimisationPreview
	}
	oldJSON, err := json.Marshal(e.Preview)
	if err != nil {
		return nil, err
	}
	newJSON, err := json.Marshal(preview)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(oldJSON, newJSON) {
		return nil, ErrOptimisationPreview
	}
	return retainedOptimisationView(row, preview), nil
}
func retainedOptimisationView(row *store.OptimisationPreview, preview optimisation.Preview) *RetainedOptimisationPreview {
	return &RetainedOptimisationPreview{ID: row.ID, InstallationID: row.InstallationID, Repository: row.Repository, ReviewedBy: row.InitiatingUser, SHA256: row.SHA256, CreatedAt: row.CreatedAt, Currentness: "not_checked", Preview: preview}
}
