package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

type EliReadiness struct {
	Ready          bool         `json:"ready"`
	Repo           string       `json:"repo"`
	PullNumber     int          `json:"pull_number"`
	HeadSHA        string       `json:"head_sha"`
	Provider       string       `json:"provider"`
	Model          string       `json:"model"`
	AllowWorkflows bool         `json:"allow_workflows"`
	Remaining      int          `json:"remaining"`
	ResetAt        *time.Time   `json:"reset_at,omitempty"`
	Blockers       []EliBlocker `json:"blockers"`
}
type EliBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (c *Controller) eliIdentity(ctx context.Context, user string) (*store.EliIdentity, error) {
	ids, err := c.st.EliIdentities(ctx)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if id.UserID == user && id.Confirmed {
			return &id, nil
		}
	}
	return nil, store.ErrNotFound
}
func (c *Controller) EliReadiness(ctx context.Context, user, repo string, pull int, run int64) EliReadiness {
	out := EliReadiness{Repo: repo, PullNumber: pull, Blockers: []EliBlocker{}}
	block := func(code, message string) { out.Blockers = append(out.Blockers, EliBlocker{code, message}) }
	id, err := c.eliIdentity(ctx, user)
	if err != nil {
		block("identity", "Link and confirm your GitHub account in repair setup.")
	}
	provider, err := c.personalChatProvider(ctx, "", user)
	if err != nil {
		block("provider", "Add an enabled personal model provider in Eli settings.")
	} else {
		out.Provider = provider.Name
		out.Model = provider.Model
	}
	inst, err := c.st.FindInstallationByTarget(ctx, repo)
	if err != nil {
		block("installation", "Connect the GitHub installation that serves this repository.")
		return out
	}
	if id == nil {
		return out
	}
	raw, err := c.ClientFor(ctx, inst.ID)
	if err != nil {
		block("github", "The GitHub installation is unavailable.")
		return out
	}
	client, ok := raw.(github.RepairClient)
	if !ok {
		block("github", "This installation cannot repair PRs.")
		return out
	}
	actor, err := client.RepairActor(ctx, repo, id.GitHubLogin)
	if err != nil || !actor.CanWrite || actor.ID != id.GitHubUserID {
		block("access", "Your linked GitHub account needs write access to this repository.")
		return out
	}
	policy, err := c.repairPolicy(ctx, repo, inst.ID)
	if err != nil {
		block("policy", "An administrator needs to enable repairs for this repository.")
	} else {
		out.AllowWorkflows = policy.AllowWorkflows
		used, personal, reset, e := c.st.EliRepairBudget(ctx, repo, user)
		if e != nil {
			block("budget_unavailable", "Repair allowance could not be checked. Try again.")
		} else {
			out.Remaining = max(0, min(policy.DailyLimit-used, store.EliPersonDailyLimit-personal))
			out.ResetAt = reset
			if out.Remaining == 0 {
				block("budget", "The repair allowance is exhausted. Wait until an attempt leaves the 24-hour window.")
			}
		}
	}
	var pr github.RepairPull
	if run > 0 {
		pr, err = client.RepairPullForRun(ctx, repo, run)
	} else if pull > 0 {
		pr, err = client.RepairPull(ctx, repo, pull)
	} else {
		block("pull", "Choose an open pull request or paste its GitHub URL.")
		return out
	}
	if err != nil {
		block("pull", "Choose an open PR in this repository at its current head; forks cannot be repaired.")
		return out
	}
	out.PullNumber = pr.Number
	out.HeadSHA = pr.HeadSHA
	info, err := raw.Probe(ctx)
	if err != nil {
		block("permissions", "GitHub App permissions could not be checked.")
	} else {
		for _, permission := range []string{"contents", "pull_requests", "actions"} {
			value := info.Permissions[permission]
			if value != "write" && (permission != "actions" || value != "read") {
				block("permissions", "Update the GitHub App permission: "+permission+map[bool]string{true: " read", false: " write"}[permission == "actions"])
			}
		}
	}
	out.Ready = len(out.Blockers) == 0
	return out
}

// Offers contain repository data, so even administrators must have current
// access through their linked GitHub account before seeing personal offers.
func (c *Controller) EliOpportunities(ctx context.Context, user string) ([]store.EliRepairOpportunity, error) {
	out := []store.EliRepairOpportunity{}
	id, err := c.eliIdentity(ctx, user)
	if errors.Is(err, store.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := c.st.EliRepairOpportunities(ctx, user)
	if err != nil {
		return nil, err
	}
	access := map[string]bool{}
	for _, o := range rows {
		key := o.InstallationID + ":" + o.Repo
		allowed, seen := access[key]
		if !seen {
			raw, err := c.ClientFor(ctx, o.InstallationID)
			if err == nil {
				if client, ok := raw.(github.RepairClient); ok {
					actor, e := client.RepairActor(ctx, o.Repo, id.GitHubLogin)
					allowed = e == nil && actor.ID == id.GitHubUserID && actor.CanWrite
				}
			}
			access[key] = allowed
		}
		if allowed {
			out = append(out, o)
		}
	}
	return out, nil
}
func (c *Controller) ActEliOpportunity(ctx context.Context, user, id, action string, admin bool) (*store.EliRepair, error) {
	rows, err := c.EliOpportunities(ctx, user)
	if err != nil {
		return nil, err
	}
	for _, o := range rows {
		if o.ID == id {
			if action == "dismiss" {
				return nil, c.st.DismissEliOpportunity(ctx, id, user)
			}
			if action != "accept" {
				return nil, store.ErrConflict
			}
			row, err := c.requestPinnedEliRepair(ctx, user, o.Repo, o.PullNumber, "", o.HeadSHA, "offer:"+o.ID, admin)
			if err != nil {
				return nil, err
			}
			if err = c.st.SetEliOpportunityState(ctx, id, "accepted"); err != nil {
				return nil, err
			}
			return row, nil
		}
	}
	return nil, store.ErrNotFound
}

// A bounded reconciliation catches jobs learnt through polling and failures
// that existed before setup. It runs independently of opening the UI.
func (c *Controller) reconcileEliOpportunities(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	policies, err := c.st.EliRepairPolicies(ctx)
	if err != nil {
		return
	}
	repos := []string{}
	byRepo := map[string]store.EliRepairPolicy{}
	for _, p := range policies {
		if p.Enabled && p.AskFirst {
			repos = append(repos, p.Repo)
			byRepo[p.Repo] = p
		}
	}
	if len(repos) == 0 {
		return
	}
	since := c.st.Now().Add(-24 * time.Hour)
	jobs, _, err := c.st.ListJobs(ctx, store.JobFilter{Repos: repos, Conclusions: []string{"failure", "timed_out"}, Since: &since}, store.Page{Limit: 50, Sort: "queued_at", Desc: true})
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, j := range jobs {
		key := fmt.Sprintf("%s:%d", j.Repo, j.GitHubRunID)
		if seen[key] || j.GitHubRunID == 0 || j.FaultKind != "" {
			continue
		}
		seen[key] = true
		p := byRepo[j.Repo]
		raw, err := c.ClientFor(ctx, p.InstallationID)
		if err != nil {
			continue
		}
		client, ok := raw.(github.RepairClient)
		if !ok {
			continue
		}
		pr, err := client.RepairPullForRun(ctx, j.Repo, j.GitHubRunID)
		if err != nil {
			continue
		}
		own, e := c.st.IsEliRepairCommit(ctx, j.Repo, pr.HeadSHA)
		if e != nil || own {
			continue
		}
		state, e := client.RepairChecks(ctx, j.Repo, pr.HeadSHA)
		if e != nil || state == "passed" {
			continue
		}
		_ = c.st.OfferEliRepair(ctx, store.EliRepairOpportunity{Repo: j.Repo, InstallationID: p.InstallationID, PullNumber: pr.Number, HeadSHA: pr.HeadSHA, RunID: j.GitHubRunID, Title: pr.Title})
	}
	// Dismissals remain tied to their original head, never to a future commit.
	rows, _ := c.st.EliRepairOpportunities(ctx, "")
	for _, o := range rows {
		raw, e := c.ClientFor(ctx, o.InstallationID)
		if e != nil {
			continue
		}
		client, ok := raw.(github.RepairClient)
		if !ok {
			continue
		}
		pr, e := client.RepairPull(ctx, o.Repo, o.PullNumber)
		if errors.Is(e, github.ErrRepairUnsupported) || e == nil && pr.HeadSHA != o.HeadSHA {
			_ = c.st.SetEliOpportunityState(ctx, o.ID, "superseded")
			continue
		}
		if e == nil {
			state, e := client.RepairChecks(ctx, o.Repo, o.HeadSHA)
			if e == nil && state == "passed" {
				_ = c.st.SetEliOpportunityState(ctx, o.ID, "superseded")
			}
		}
	}
}

func normaliseRepairRepo(repo string) string { return strings.TrimSpace(repo) }
