package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// An instance move or a re-created account leaves a GitHub account's link on
// the Zoomies user it used to belong to. The verification of the new link is
// refused with the holder named, and the move that answers the refusal is its
// own press, audited: a durable identity relation does not get rewritten
// because a form was submitted twice.
func TestAHeldGitHubIdentityIsNamedAndOnlyThenMoved(t *testing.T) {
	h := newHarness(t)
	holder, holderCookie := h.user("previous", store.RoleAdmin)
	h.st.SetEliIdentity(h.ctx, store.EliIdentity{UserID: holder.ID, GitHubUserID: 42, GitHubLogin: "octo"})
	target, targetCookie := h.user("fresh", store.RoleAdmin)
	inst := h.installation()
	h.gh.SetRepairActor("octo", 42, true)

	verify := func(extra map[string]any) map[string]any {
		body := map[string]any{
			"user_id":         target.ID,
			"github_login":    "octo",
			"repo":            inst.Target + "/repo",
			"installation_id": inst.ID,
		}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	resp := h.do(request{method: http.MethodPut, path: "/api/v1/assistant/repairs/identity", cookie: targetCookie, body: verify(nil)})
	resp.mustStatus(t, http.StatusConflict, "the held link is refused with the holder named")
	for _, want := range []string{"assistant.identity_held", `"detail":"` + holder.ID + `"`, "previous"} {
		if !strings.Contains(string(resp.body), want) {
			t.Fatalf("the refusal does not carry %s: %s", want, string(resp.body))
		}
	}
	// Nothing moved yet: the press the page reads as "move" is its own.
	rows, _ := h.st.EliIdentities(h.ctx)
	if len(rows) != 1 || rows[0].UserID != holder.ID {
		t.Fatalf("a refusal moved the link: %+v", rows)
	}

	h.do(request{method: http.MethodPut, path: "/api/v1/assistant/repairs/identity", cookie: targetCookie, body: verify(map[string]any{"relink": true})}).
		mustStatus(t, http.StatusOK, "the link moves")
	rows, _ = h.st.EliIdentities(h.ctx)
	if len(rows) != 1 || rows[0].UserID != target.ID || rows[0].Confirmed {
		t.Fatalf("the moved link: %+v", rows)
	}

	// The move is one row, naming what the holder lost.
	moved, _, _ := h.st.ListAudit(h.ctx, store.AuditFilter{Actions: []string{"assistant.github_link"}}, store.Page{Limit: 5})
	if len(moved) != 1 || !strings.Contains(moved[0].After, `"moved_from":"`+holder.ID+`"`) {
		t.Fatalf("the move's row: %+v %v", moved, moved[0].After)
	}
	_ = holderCookie
}
