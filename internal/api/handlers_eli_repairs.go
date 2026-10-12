package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/auth"
	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

func (s *Server) handleEliRepairs(w http.ResponseWriter, r *http.Request) {
	id := Identity(r.Context())
	rows, _, err := s.ctrl.Store().ListEliRepairsPage(r.Context(), store.EliRepairFilter{
		Scoped:  !id.Can(auth.ActionAssistantRead),
		OwnerID: id.UserID,
		Limit:   store.MaxEliRepairPage,
	})
	if err != nil {
		s.internal(w, r, "listing Eli repairs", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}
func (s *Server) handleEliRepairSettings(w http.ResponseWriter, r *http.Request) {
	identities, err := s.ctrl.Store().EliIdentities(r.Context())
	if err != nil {
		s.internal(w, r, "reading Eli account links", err)
		return
	}
	admin := Identity(r.Context()).Can(auth.ActionAssistantRead)
	visible := []store.EliIdentity{}
	for _, id := range identities {
		if admin || id.UserID == Identity(r.Context()).UserID {
			visible = append(visible, id)
		}
	}
	policies := []store.EliRepairPolicy{}
	if admin {
		policies, err = s.ctrl.Store().EliRepairPolicies(r.Context())
		if err != nil {
			s.internal(w, r, "reading Eli policies", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"identities": visible, "policies": policies})
}
func (s *Server) handleSetEliIdentity(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID         string `json:"user_id"`
		GitHubLogin    string `json:"github_login"`
		InstallationID string `json:"installation_id"`
		Repo           string `json:"repo"`
	}
	if !decode(w, r, &in) {
		return
	}
	user, err := s.ctrl.Store().GetUser(r.Context(), in.UserID)
	if err != nil || user.Disabled {
		unprocessable(w, "choose an active Zoomies user", []fieldError{{"user_id", "an active account is required"}})
		return
	}
	if in.GitHubLogin == "" {
		if err := s.ctrl.Store().DeleteEliIdentity(r.Context(), in.UserID); err != nil {
			s.fail(w, r, "removing the GitHub account link", err)
			return
		}
		s.auth.Auditor().Act(r.Context(), Identity(r.Context()), "assistant.github_unlink", "user", in.UserID, nil)
		noContent(w)
		return
	}
	raw, err := s.ctrl.ClientFor(r.Context(), in.InstallationID)
	if err != nil {
		s.fail(w, r, "opening the GitHub installation", err)
		return
	}
	client, ok := raw.(github.RepairClient)
	if !ok {
		conflict(w, "this installation does not support Eli PR repairs")
		return
	}
	actor, err := client.RepairActor(r.Context(), strings.TrimSpace(in.Repo), strings.TrimSpace(in.GitHubLogin))
	if err != nil || actor.ID <= 0 || !actor.CanWrite {
		unprocessable(w, "the GitHub account must have write permission on this repository", []fieldError{{"github_login", "verify the user's GitHub account and repository access before linking it"}})
		return
	}
	id := store.EliIdentity{UserID: user.ID, GitHubUserID: actor.ID, GitHubLogin: actor.Login}
	if err := s.ctrl.Store().SetEliIdentity(r.Context(), id); err != nil {
		s.fail(w, r, "linking the GitHub account", err)
		return
	}
	s.auth.Auditor().Act(r.Context(), Identity(r.Context()), "assistant.github_link", "user", user.ID, id)
	writeJSON(w, http.StatusOK, id)
}
func (s *Server) handleSetEliPolicy(w http.ResponseWriter, r *http.Request) {
	var p store.EliRepairPolicy
	if !decode(w, r, &p) {
		return
	}
	p.Repo = strings.TrimSpace(p.Repo)
	_, _, kind := github.SplitTarget(p.Repo)
	if kind != store.TargetRepo || p.DailyLimit < 1 || p.DailyLimit > 50 {
		unprocessable(w, "choose a repository and a daily repair limit from 1 to 50", nil)
		return
	}
	if p.Automatic && p.AskFirst {
		unprocessable(w, "Choose automatic repairs or ask first, not both.", nil)
		return
	}
	if p.Automatic || p.ProviderID != "" {
		provider, err := s.ctrl.Store().GetAssistantProvider(r.Context(), p.ProviderID)
		if err != nil || provider.OwnerID != "" || p.Enabled && !provider.Enabled {
			unprocessable(w, "choose an enabled installation provider for unattended repairs", []fieldError{{"provider_id", "a personal provider cannot pay for unattended repairs"}})
			return
		}
	}
	inst, err := s.ctrl.Store().FindInstallationByTarget(r.Context(), p.Repo)
	if err != nil || inst.ID != p.InstallationID {
		unprocessable(w, "choose the installation that serves this repository", nil)
		return
	}
	if err := s.ctrl.Store().SetEliRepairPolicy(r.Context(), p); err != nil {
		s.fail(w, r, "saving the repository repair policy", err)
		return
	}
	s.auth.Auditor().Act(r.Context(), Identity(r.Context()), "assistant.repair_policy", "repository", p.Repo, p)
	writeJSON(w, http.StatusOK, p)
}
func (s *Server) handleRequestEliRepair(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Repo        string `json:"repo"`
		PullNumber  int    `json:"pull_number"`
		Instruction string `json:"instruction"`
		HeadSHA     string `json:"head_sha"`
	}
	if !decode(w, r, &in) {
		return
	}
	owner := Identity(r.Context()).UserID
	if owner == "" {
		conflict(w, "sign in with a personal account to request a PR repair")
		return
	}
	ready := s.ctrl.EliReadiness(r.Context(), owner, in.Repo, in.PullNumber, 0)
	if !ready.Ready {
		conflict(w, ready.Blockers[0].Message)
		return
	}
	row, err := s.ctrl.RequestPinnedEliRepair(r.Context(), owner, in.Repo, in.PullNumber, in.Instruction, in.HeadSHA, s.isAssistantAdmin(r))
	switch {
	case errors.Is(err, controller.ErrAssistantSubscriptionRestricted):
		forbidden(w, err.Error())
	case errors.Is(err, store.ErrNotFound):
		notFound(w, "no enabled repair policy or linked GitHub account was found")
	case errors.Is(err, store.ErrConflict), errors.Is(err, controller.ErrAssistantNoModel):
		conflict(w, "The repair could not start. Refresh its readiness: the PR, access or allowance may have changed.")
	case err != nil:
		s.internal(w, r, "requesting the PR repair", err)
	default:
		s.auth.Auditor().Act(r.Context(), Identity(r.Context()), "assistant.repair_request", "eli_repair", row.ID, map[string]any{"repo": row.Repo, "pull_number": row.PullNumber})
		writeJSON(w, http.StatusAccepted, row)
	}
}

func (s *Server) handleEliRepairConsent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		GitHubUserID int64 `json:"github_user_id"`
		Enabled      bool  `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	owner := Identity(r.Context()).UserID
	if err := s.ctrl.Store().ConfirmEliIdentity(r.Context(), owner, in.GitHubUserID, in.Enabled); err != nil {
		s.fail(w, r, "confirming the personal GitHub account link", err)
		return
	}
	s.auth.Auditor().Act(r.Context(), Identity(r.Context()), "assistant.github_consent", "user", owner, in)
	noContent(w)
}

func (s *Server) handleEliReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	pull, _ := strconv.Atoi(r.URL.Query().Get("pull_number"))
	run, _ := strconv.ParseInt(r.URL.Query().Get("run_id"), 10, 64)
	writeJSON(w, http.StatusOK, s.ctrl.EliReadiness(ctx, Identity(ctx).UserID, r.URL.Query().Get("repo"), pull, run))
}
func (s *Server) handleEliOpportunities(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	rows, err := s.ctrl.EliOpportunities(ctx, Identity(ctx).UserID)
	if err != nil {
		s.fail(w, r, "reading repair offers", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}
func (s *Server) handleEliOpportunityAction(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &in) {
		return
	}
	row, err := s.ctrl.ActEliOpportunity(r.Context(), Identity(r.Context()).UserID, chiURLParam(r, "id"), in.Action, s.isAssistantAdmin(r))
	if err != nil {
		s.fail(w, r, "acting on repair offer; refresh the PR and repair setup", err)
		return
	}
	s.auth.Auditor().Act(r.Context(), Identity(r.Context()), "assistant.repair_offer_"+in.Action, "eli_opportunity", chiURLParam(r, "id"), nil)
	writeJSON(w, http.StatusOK, map[string]any{"repair": row})
}
func (s *Server) handleEliRepositories(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	raw, err := s.ctrl.ClientFor(ctx, r.URL.Query().Get("installation_id"))
	if err != nil {
		s.fail(w, r, "opening GitHub installation", err)
		return
	}
	repos, err := raw.ListRepositories(ctx, 0)
	if err != nil {
		s.fail(w, r, "discovering repositories", err)
		return
	}
	capped := len(repos) >= github.RepositoryDiscoveryLimit
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	names := []string{}
	for _, repo := range repos {
		if strings.Contains(strings.ToLower(repo.FullName), query) {
			names = append(names, repo.FullName)
		}
	}
	sort.Strings(names)
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	offset = max(0, min(offset, len(names)))
	end := min(offset+50, len(names))
	missing := []string{}
	info, err := raw.Probe(ctx)
	if err != nil {
		missing = append(missing, "Could not check GitHub App permissions.")
	} else {
		for _, permission := range []string{"contents", "pull_requests", "actions"} {
			v := info.Permissions[permission]
			if v != "write" && (permission != "actions" || v != "read") {
				missing = append(missing, permission)
			}
		}
		found := false
		for _, event := range info.Events {
			if event == "workflow_job" {
				found = true
			}
		}
		if !found {
			missing = append(missing, "workflow_job events")
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": names[offset:end], "total": len(names), "capped": capped, "missing_permissions": missing})
}
