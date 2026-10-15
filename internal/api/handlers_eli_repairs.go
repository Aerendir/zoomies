package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eyupio/zoomies/internal/auth"
	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

// maxEliRepairSearch is the longest repository filter the contract allows. No
// repository name is that long, so a longer search can only be a mistake, and
// naming it is kinder than showing an empty history.
const maxEliRepairSearch = 200

// maxBulkEliRepairs is the most repairs one clear may name. It is the size of
// the largest selection the selection endpoint hands out, so "select all" is
// always exactly one request. There is a ceiling at all because ClearEliRepairs
// runs its statements for every id under the store's single writer, and an
// unbounded list would hold every other write for as long as it took.
const maxBulkEliRepairs = store.MaxEliRepairSelection

// eliRepairFilter reads the history's filters from the query string and builds
// the scope from who is asking: an administrator reaches every repair, anyone
// else only their own. The list and the selection share it so a select-all
// takes exactly the rows the list shows.
//
// An unknown group or trigger is refused here, with the parameter's name,
// rather than passed on. The store reads a value it does not know as "no
// filter", which is right for a query builder and wrong for a person: a filter
// that quietly widens shows them the wrong history without saying so.
//
// When it returns false it has already answered 422. paged is for the list;
// a selection is the matches, not a page of them.
func eliRepairFilter(w http.ResponseWriter, r *http.Request, paged bool) (store.EliRepairFilter, bool) {
	q := r.URL.Query()
	rawRepo := q.Get("repo")
	f := store.EliRepairFilter{
		Group:   strings.TrimSpace(q.Get("group")),
		Repo:    strings.TrimSpace(rawRepo),
		Trigger: strings.TrimSpace(q.Get("trigger")),
	}
	f.OwnerID, f.Scoped = eliRepairScope(r, auth.ActionAssistantRead)

	var fields []fieldError
	if f.Group != "" && !slices.Contains(store.EliRepairGroups, f.Group) {
		fields = append(fields, fieldError{"group", fmt.Sprintf("%q is not a group of repairs; use %s", f.Group, orList(store.EliRepairGroups))})
	}
	if f.Trigger != "" && !slices.Contains(store.EliRepairTriggers, f.Trigger) {
		fields = append(fields, fieldError{"trigger", fmt.Sprintf("%q is not a way a repair begins; use %s", f.Trigger, orList(store.EliRepairTriggers))})
	}
	// The contract's maxLength is about what was sent, so it is the raw value that
	// is measured: counting the trimmed one would accept a value the OpenAPI text
	// says is too long, as long as enough of it was spaces.
	if n := utf8.RuneCountInString(rawRepo); n > maxEliRepairSearch {
		fields = append(fields, fieldError{"repo", fmt.Sprintf("a repository search is at most %d characters and this one is %d; shorten it", maxEliRepairSearch, n)})
	}
	if len(fields) > 0 {
		unprocessable(w, "the repair history could not be filtered as asked", fields)
		return f, false
	}

	if paged {
		p := parsePage(r)
		f.Limit, f.Offset = p.Limit, p.Offset
	}
	return f, true
}

// eliRepairScope says whose repairs the caller reaches: everyone's when they
// hold wide, otherwise only the repairs that belong to their own account. It is
// the one place the rule is written, so the list, the selection and the clear
// cannot drift apart.
//
// wide is what differs. Reading the history asks for assistant.read and
// clearing it asks for assistant.write, because a clear is a change and a scope
// on a resource implies reading it: a token narrowed to assistant:own can read
// everything its owner's role can, and must not be able to clear it too.
// assistant:own is "only the owner-scoped personal endpoints".
//
// A caller with no identity is scoped to nobody, never to everybody; the empty
// owner matches no repair.
func eliRepairScope(r *http.Request, wide auth.Action) (ownerID string, scoped bool) {
	id := Identity(r.Context())
	if id == nil {
		return "", true
	}
	return id.UserID, !id.Can(wide)
}

// orList renders "a, b or c" for a message that offers the values a parameter
// takes.
func orList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

func (s *Server) handleEliRepairs(w http.ResponseWriter, r *http.Request) {
	f, ok := eliRepairFilter(w, r, true)
	if !ok {
		return
	}
	rows, total, err := s.ctrl.Store().ListEliRepairsPage(r.Context(), f)
	if err != nil {
		s.internal(w, r, "listing Eli repairs", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "total": total})
}

// handleEliRepairSelection answers GET /api/v1/assistant/repairs/selection: the
// ids a select-all takes, for the same filters as the list.
func (s *Server) handleEliRepairSelection(w http.ResponseWriter, r *http.Request) {
	f, ok := eliRepairFilter(w, r, false)
	if !ok {
		return
	}
	ids, total, err := s.ctrl.Store().EliRepairSelection(r.Context(), f)
	if err != nil {
		s.internal(w, r, "selecting Eli repairs", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ids": ids, "total": total, "truncated": total > len(ids)})
}

type bulkEliRepairRequest struct {
	Action string   `json:"action"`
	IDs    []string `json:"ids"`
}

// handleBulkEliRepairs answers POST /api/v1/assistant/repairs/bulk.
//
// It answers 200 with a result for each id, in the order asked, as the machines'
// bulk delete does: a person clearing a page of repairs will have one that Eli
// picked up a moment ago, and "something failed" is not an answer they can act
// on. The store decides each id. The scope it applies is the list's rule with a
// stricter test for reaching everyone's repairs (assistant.write, not
// assistant.read; see eliRepairScope), so a person cannot clear, or learn the
// existence of, a repair that is not theirs unless they are an administrator
// whose credential may write.
func (s *Server) handleBulkEliRepairs(w http.ResponseWriter, r *http.Request) {
	var req bulkEliRepairRequest
	if !decode(w, r, &req) {
		return
	}

	var fields []fieldError
	switch req.Action {
	case "clear":
	case "":
		fields = append(fields, fieldError{"action", "say what to do with these repairs: clear"})
	default:
		fields = append(fields, fieldError{"action", fmt.Sprintf("%q is not an action; use clear", req.Action)})
	}
	switch {
	case len(req.IDs) == 0:
		fields = append(fields, fieldError{"ids", "no repairs were selected"})
	case len(req.IDs) > maxBulkEliRepairs:
		fields = append(fields, fieldError{"ids", fmt.Sprintf("%d repairs is more than the %d this endpoint accepts at once; clear them in batches",
			len(req.IDs), maxBulkEliRepairs)})
	}
	if len(fields) > 0 {
		unprocessable(w, "this bulk action could not be carried out", fields)
		return
	}

	ctx := r.Context()
	owner, scoped := eliRepairScope(r, auth.ActionAssistantWrite)
	results, err := s.ctrl.Store().ClearEliRepairs(ctx, req.IDs, owner, scoped)
	if err != nil {
		s.internal(w, r, "clearing Eli repairs from the history", err)
		return
	}

	// One row for the request, after the store has answered, naming what this
	// request cleared and nothing about what the repairs said. "OK" is not
	// enough: a repeat, or an administrator clearing what its owner already
	// cleared, is OK and changed nothing, and a row saying they cleared it would
	// put their name on someone else's action. A request that changed nothing
	// leaves no row.
	cleared := []string{}
	for _, res := range results {
		if res.Changed {
			cleared = append(cleared, res.ID)
		}
	}
	if len(cleared) > 0 {
		s.auth.Auditor().Act(ctx, Identity(ctx), "assistant.repair_clear", "eli_repair", "", map[string]any{"count": len(cleared), "ids": cleared})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
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
		// Relink is the confirmation the held-link refusal asks for: move the
		// GitHub account's link from the Zoomies account that holds it to the
		// one being verified. An instance move or a re-created account is how a
		// link ends up held by the account that is being replaced, and without
		// the flag the refusal cannot be turned into a new link by accident.
		Relink bool `json:"relink"`
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
	if in.Relink {
		holder, err := s.ctrl.Store().RelinkEliIdentity(r.Context(), id)
		if err != nil {
			s.fail(w, r, "moving the GitHub account link", err)
			return
		}
		after := map[string]any{"github_user_id": id.GitHubUserID, "github_login": id.GitHubLogin}
		if holder != "" {
			after["moved_from"] = holder
		}
		s.auth.Auditor().Act(r.Context(), Identity(r.Context()), "assistant.github_link", "user", user.ID, after)
		writeJSON(w, http.StatusOK, id)
		return
	}
	if err := s.ctrl.Store().SetEliIdentity(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrEliIdentityHeld) {
			conflictHeld(w, s.eliHeldMessage(r.Context(), actor.ID), s.eliHolderID(r.Context(), actor.ID))
			return
		}
		s.fail(w, r, "linking the GitHub account", err)
		return
	}
	s.auth.Auditor().Act(r.Context(), Identity(r.Context()), "assistant.github_link", "user", user.ID, id)
	writeJSON(w, http.StatusOK, id)
}

// eliHeldMessage and eliHolderID say whose link the verification found, for
// the person to act on: hold or move. The holder's name is the sentence;
// "another Zoomies user" would send the administrator looking through every
// account for the row. The holder travels in the refusal's detail, which is
// what the moved link's audit row and the page's own answer have in common.
func (s *Server) eliHeldMessage(ctx context.Context, githubUserID int64) string {
	name := s.eliHolderName(ctx, githubUserID)
	if name == "" {
		return "This GitHub account is already linked to another Zoomies user. Moving the link removes their confirmed link, and they would have to allow it again."
	}
	return fmt.Sprintf("This GitHub account is already linked to %s. Moving the link removes that account's confirmed link, and they would have to allow it again.", name)
}

// eliHolderID is the Zoomies user the GitHub account's link sits on now, or
// "" when the links could not be read: the page falls back to the message
// and the user does the looking.
func (s *Server) eliHolderID(ctx context.Context, githubUserID int64) string {
	identities, err := s.ctrl.Store().EliIdentities(ctx)
	if err != nil {
		return ""
	}
	for _, id := range identities {
		if id.GitHubUserID == githubUserID {
			return id.UserID
		}
	}
	return ""
}

// eliHolderName is the holding account's username, resolved for the message.
func (s *Server) eliHolderName(ctx context.Context, githubUserID int64) string {
	holder := s.eliHolderID(ctx, githubUserID)
	if holder == "" {
		return ""
	}
	user, err := s.ctrl.Store().GetUser(ctx, holder)
	if err != nil {
		return ""
	}
	return user.Username
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
