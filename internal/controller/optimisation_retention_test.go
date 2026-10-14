package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/optimisation"
	"github.com/eyupio/zoomies/internal/prrepair"
	"github.com/eyupio/zoomies/internal/store"
)

func optimisationRetentionFixture(t *testing.T) (*Controller, string, optimisation.Source, optimisation.Timing, optimisation.HistoryReview, optimisation.Policy, *store.AuditEvent) {
	t.Helper()
	s, err := store.Open(context.Background(), store.Options{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	inst := &store.Installation{AppID: 1, InstallationID: 2, Target: "acme/repo", TargetType: store.TargetRepo}
	if err := s.CreateInstallation(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
	file := prrepair.File{Path: ".github/workflows/private.yml", Mode: "100644", Content: "on: push\njobs:\n  private:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@" + strings.Repeat("b", 40) + "\n        with:\n          fetch-depth: 0\n      - run: npm test\n"}
	source := optimisation.Source{HeadSHA: strings.Repeat("a", 40), File: file}
	sum := sha256.Sum256([]byte(file.Content))
	digest := hex.EncodeToString(sum[:])
	ms, needs := int64(4200), false
	timing := optimisation.Timing{EvidenceID: "fixture", HeadSHA: source.HeadSHA, SourceSHA256: digest, RunID: "9223372036854775807", Attempt: 2, JobIndex: 0, StepIndex: 0, Outcome: "success", CheckoutMS: &ms}
	history := optimisation.HistoryReview{HeadSHA: source.HeadSHA, SourceSHA256: digest, Scope: "repository_and_downstream_consumers", NeedsHistory: &needs}
	return &Controller{st: s}, inst.ID, source, timing, history, optimisation.Policy{SourceAllowed: true, WorkflowEditsAllowed: true}, &store.AuditEvent{ActorID: "usr_operator", ActorKind: "user", ActorName: "operator"}
}
func TestRetainedOptimisationReplaysExactPrivateSourceWithoutGitHubOrCurrentPolicy(t *testing.T) {
	c, installation, source, timing, history, policy, actor := optimisationRetentionFixture(t)
	saved, err := c.RetainOptimisationPreview(context.Background(), actor, installation, "acme/repo", source, timing, history, policy)
	if err != nil {
		t.Fatal(err)
	}
	*timing.CheckoutMS = 9999
	*history.NeedsHistory = true
	source.File.Content = "changed"
	policy.WorkflowEditsAllowed = false
	actor.ActorID = "changed"
	replay, err := c.ReplayOptimisationPreview(context.Background(), installation, "acme/repo", saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(saved)
	b, _ := json.Marshal(replay)
	if string(a) != string(b) || replay.Preview.File.Mode != "100644" || replay.ReviewedBy != "usr_operator" || replay.Preview.Verification != "inconclusive" || replay.Currentness != "not_checked" || replay.Preview.RunID != "9223372036854775807" {
		t.Fatal("offline replay changed the preview or authority")
	}
	if _, err := c.ReplayOptimisationPreview(context.Background(), "other", "acme/repo", saved.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("cross-installation read", err)
	}
	if _, err := c.ReplayOptimisationPreview(context.Background(), installation, "other/repo", saved.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("cross-repository read", err)
	}
	events, total, err := c.st.ListAudit(context.Background(), store.AuditFilter{TargetID: saved.ID}, store.Page{Limit: 10})
	if err != nil || total != 1 || strings.Contains(events[0].Before+events[0].After, "fetch-depth") || events[0].Action != "optimisation.preview_retained" {
		t.Fatal("source in audit or missing atomic audit", err)
	}
}
func TestOptimisationReplayRefusesAlteredCalculationAndBindingsEvenWithAValidStorageDigest(t *testing.T) {
	c, installation, source, timing, history, policy, actor := optimisationRetentionFixture(t)
	saved, err := c.RetainOptimisationPreview(context.Background(), actor, installation, "acme/repo", source, timing, history, policy)
	if err != nil {
		t.Fatal(err)
	}
	original, err := c.st.GetOptimisationPreview(context.Background(), installation, "acme/repo", saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"preview", "history", "source", "installation", "repository", "actor", "funding", "future"} {
		t.Run(mode, func(t *testing.T) {
			var e optimisationRetainedEnvelope
			if err := json.Unmarshal(original.Payload, &e); err != nil {
				t.Fatal(err)
			}
			row := *original
			row.ID = store.NewID(store.PrefixOptimisationPreview)
			e.ID = row.ID
			switch mode {
			case "preview":
				e.Preview.Verification = "improved"
			case "history":
				*e.History.NeedsHistory = true
			case "source":
				e.Source.Mode = "120000"
			case "installation":
				e.InstallationID = "other"
			case "repository":
				e.Repository = "other/repo"
			case "actor":
				e.ReviewedBy = "other"
			case "funding":
				e.ModelFunding = "someone_else"
			}
			row.Payload, _ = json.Marshal(e)
			if mode == "future" {
				row.Payload = append(row.Payload[:len(row.Payload)-1], []byte(`,"future":true}`)...)
			}
			if err := c.st.PutOptimisationPreview(context.Background(), &row, actor); err != nil {
				t.Fatal(err)
			}
			if _, err := c.ReplayOptimisationPreview(context.Background(), installation, "acme/repo", row.ID); !errors.Is(err, ErrOptimisationPreview) {
				t.Fatal("altered evidence accepted", err)
			}
		})
	}
}
func TestOptimisationRetentionRefusesInvalidInputsBeforeWritingHistory(t *testing.T) {
	for _, mode := range []string{"actor", "history", "policy", "partial", "window", "scope"} {
		t.Run(mode, func(t *testing.T) {
			c, installation, source, timing, history, policy, actor := optimisationRetentionFixture(t)
			repo := "acme/repo"
			switch mode {
			case "actor":
				actor = nil
			case "history":
				history.NeedsHistory = nil
			case "policy":
				policy.WorkflowEditsAllowed = false
			case "partial":
				source.File.Partial = true
			case "window":
				source.File.StartLine = 1
			case "scope":
				repo = "other/repo"
			}
			if r, err := c.RetainOptimisationPreview(context.Background(), actor, installation, repo, source, timing, history, policy); err == nil || r != nil {
				t.Fatal("unsupported preview retained")
			}
			_, total, err := c.st.ListAudit(context.Background(), store.AuditFilter{Actions: []string{"optimisation.preview_retained"}}, store.Page{Limit: 10})
			if err != nil || total != 0 {
				t.Fatal("refusal wrote an audit", err)
			}
		})
	}
}
