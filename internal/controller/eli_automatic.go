package controller

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// EliAutomaticRepository is one repository that repairs its own failed checks,
// with what an administrator needs to trust that or stop it: who pays, whether
// they can, what has been spent of the allowance and what the automation did
// last. It is an administrator's view; automatic repairs have no owner, so an
// ordinary user cannot read any of them.
type EliAutomaticRepository struct {
	Repo           string `json:"repo"`
	InstallationID string `json:"installation_id"`
	ProviderID     string `json:"provider_id"`
	// ProviderName and Model are empty when the policy has no provider, which is
	// what deleting a provider leaves behind.
	ProviderName string `json:"provider_name"`
	Model        string `json:"model"`
	// Problem is present only when automatic repairs here cannot run, in a
	// sentence that says what to do about it.
	Problem        string `json:"problem,omitempty"`
	AllowWorkflows bool   `json:"allow_workflows"`
	DailyLimit     int    `json:"daily_limit"`
	// Used is every attempt on the repository in the last 24 hours, by anyone and
	// by any trigger: the figure the daily limit is held against.
	Used int `json:"used"`
	// NextFreeAt is when the oldest of those attempts leaves the window, which is
	// when a full allowance has a place again.
	NextFreeAt *time.Time              `json:"next_free_at,omitempty"`
	Running    int                     `json:"running"`
	Last       *store.EliAutomaticLast `json:"last,omitempty"`
}

// EliAutomaticRepairs lists the repositories whose policy is enabled and
// Automatic, the ones with a problem first and then by name without regard to
// case. A provider that cannot pay is reported here for the same reasons the
// worker refuses it, so the administrator reads it before the next failure is
// spent on it rather than after.
func (c *Controller) EliAutomaticRepairs(ctx context.Context) ([]EliAutomaticRepository, error) {
	policies, err := c.st.EliRepairPolicies(ctx)
	if err != nil {
		return nil, err
	}
	var automatic []store.EliRepairPolicy
	var repos []string
	for _, p := range policies {
		if p.Enabled && p.Automatic {
			automatic = append(automatic, p)
			repos = append(repos, p.Repo)
		}
	}
	out := []EliAutomaticRepository{}
	if len(automatic) == 0 {
		return out, nil
	}
	usage, err := c.st.EliRepairUsage(ctx)
	if err != nil {
		return nil, err
	}
	activity, err := c.st.EliAutomaticActivity(ctx, repos)
	if err != nil {
		return nil, err
	}
	for _, p := range automatic {
		entry := EliAutomaticRepository{
			Repo: p.Repo, InstallationID: p.InstallationID, ProviderID: p.ProviderID,
			AllowWorkflows: p.AllowWorkflows, DailyLimit: p.DailyLimit,
			Running: activity[p.Repo].Running, Last: activity[p.Repo].Last,
		}
		if used, ok := usage[p.Repo]; ok && used.Used > 0 {
			entry.Used = used.Used
			free := used.Oldest.Add(24 * time.Hour)
			entry.NextFreeAt = &free
		}
		if p.ProviderID == "" {
			entry.Problem = "No installation provider is set for this repository, so automatic repairs cannot start. Choose one in setup."
		} else {
			provider, err := c.st.GetAssistantProvider(ctx, p.ProviderID)
			switch {
			case errors.Is(err, store.ErrNotFound):
				entry.Problem = "The installation provider for this repository is missing, so automatic repairs cannot start. Choose one in setup."
			case err != nil:
				return nil, err
			default:
				entry.ProviderName, entry.Model = provider.Name, provider.Model
				switch {
				case provider.OwnerID != "":
					entry.Problem = "The provider chosen for this repository belongs to a person, and automatic repairs need an installation provider. Choose one in setup."
				case !provider.Enabled:
					entry.Problem = fmt.Sprintf("The installation provider %q is disabled, so automatic repairs cannot start. Enable it or choose another in setup.", provider.Name)
				}
			}
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if broken := out[i].Problem != ""; broken != (out[j].Problem != "") {
			return broken
		}
		a, b := strings.ToLower(out[i].Repo), strings.ToLower(out[j].Repo)
		if a != b {
			return a < b
		}
		return out[i].Repo < out[j].Repo
	})
	return out, nil
}
