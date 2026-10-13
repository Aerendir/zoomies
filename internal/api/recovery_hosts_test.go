package api

import (
	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/store"
	"net/http"
	"testing"
)

func TestFleetAdministratorsCanRetireOfflineRecordsButCannotLiftRecovery(t *testing.T) {
	h := newHarness(t)
	host := h.host("carried-over")
	pool := h.pool(h.installation(), "history")
	runner := h.runner(pool, host, store.RunnerRemoved)
	if err := h.st.MarkTransferredHosts(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.fence(t, "complete transfer")
	path := "/api/v1/recovery/hosts/" + host.ID + "/retire"
	body := map[string]any{"confirm_host_id": host.ID}
	for _, role := range []store.Role{store.RoleViewer, store.RoleOperator, store.RoleAdmin} {
		u, _ := h.user(string(role), role)
		cookie := h.session(u)
		h.do(request{method: http.MethodPost, path: "/api/v1/recovery/unfence", cookie: cookie}).mustStatus(t, http.StatusForbidden, "ordinary actor cannot lift fence")
		if role != store.RoleAdmin {
			h.do(request{method: http.MethodPost, path: path, cookie: cookie, body: body}).mustStatus(t, http.StatusForbidden, "retirement needs administrator")
		}
	}
	admin, _ := h.user("retiring-admin", store.RoleAdmin)
	cookie := h.session(admin)
	platform, _ := h.user("recovery-platform", store.RolePlatform)
	h.do(request{method: http.MethodPost, path: "/api/v1/recovery/unfence", cookie: h.session(platform)}).mustStatus(t, http.StatusConflict, "unresolved host keeps fence")
	h.do(request{method: http.MethodPost, path: path, cookie: cookie, body: map[string]any{"confirm_host_id": "another-host"}}).mustStatus(t, http.StatusConflict, "host-specific confirmation")
	h.do(request{method: http.MethodDelete, path: "/api/v1/hosts/" + host.ID + "?force=true", cookie: cookie}).mustStatus(t, http.StatusConflict, "deletion cannot bypass recovery")
	h.do(request{method: http.MethodPost, path: path, cookie: cookie, body: body}).mustStatus(t, http.StatusOK, "offline retirement")
	if !h.ctrl.Fenced().Fenced {
		t.Fatal("retirement lifted fence")
	}
	if _, err := h.st.GetRunner(h.ctx, runner.ID); err != nil {
		t.Fatalf("runner history lost: %v", err)
	}
	rows, _, err := h.st.ListAudit(h.ctx, store.AuditFilter{Actions: []string{"host.retire"}}, store.Page{Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].ActorID != admin.ID || rows[0].TargetID != host.ID {
		t.Fatalf("retirement audit: %+v %v", rows, err)
	}
	if batch, err := h.ctrl.PollTasks(h.ctx, host.ID, 0); err != agent.ErrHostGone || batch != nil {
		t.Fatalf("retirement delivered a remote task: %+v %v", batch, err)
	}
	h.do(request{method: http.MethodPost, path: "/api/v1/recovery/unfence", cookie: h.session(platform)}).mustStatus(t, http.StatusOK, "platform can complete resolved recovery")
}

func TestRetiredAgentCredentialsCannotReachAnyAgentRoute(t *testing.T) {
	h := newHarness(t)
	id, token := h.agentToken("retiring-agent")
	if err := h.st.MarkTransferredHosts(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.fence(t, "transfer")
	admin, _ := h.user("retirer", store.RoleAdmin)
	h.do(request{method: http.MethodPost, path: "/api/v1/recovery/hosts/" + id + "/retire", cookie: h.session(admin), body: map[string]any{"confirm_host_id": id}}).mustStatus(t, http.StatusOK, "retirement")
	for _, route := range agentRoutesUnderTest() {
		h.do(request{method: route.method, path: route.path, token: token, body: route.body, rawBody: route.raw}).mustStatus(t, http.StatusUnauthorized, "retired credential on "+route.name)
	}
}

func TestRetirementAPIRefusesUnknownNewAndOnlineHosts(t *testing.T) {
	h := newHarness(t)
	id, _ := h.agentToken("online-carried")
	if err := h.st.MarkTransferredHosts(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.fence(t, "transfer")
	if err := h.st.Heartbeat(h.ctx, id, h.ctrl.Now()); err != nil {
		t.Fatal(err)
	}
	fresh := h.host("new-host")
	admin, _ := h.user("administrator", store.RoleAdmin)
	cookie := h.session(admin)
	for _, tc := range []struct {
		id     string
		status int
	}{{"missing", http.StatusNotFound}, {id, http.StatusConflict}, {fresh.ID, http.StatusConflict}} {
		h.do(request{method: http.MethodPost, path: "/api/v1/recovery/hosts/" + tc.id + "/retire", cookie: cookie, body: map[string]any{"confirm_host_id": tc.id}}).mustStatus(t, tc.status, "ineligible retirement")
	}
	if err := h.st.CompleteRecovery(h.ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.ctrl.LoadFence(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.do(request{method: http.MethodPost, path: "/api/v1/recovery/hosts/" + id + "/retire", cookie: cookie, body: map[string]any{"confirm_host_id": id}}).mustStatus(t, http.StatusConflict, "unfenced retirement")
}
