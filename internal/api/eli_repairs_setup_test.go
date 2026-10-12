package api

import (
	"encoding/json"
	"github.com/eyupio/zoomies/internal/store"
	"testing"
)

func TestEliManualPolicyCanBeSavedWithoutAnInstallationProvider(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	_, admin := h.user("repair-admin", store.RoleAdmin)
	body := map[string]any{"repo": inst.Target + "/repo", "installation_id": inst.ID, "provider_id": "", "enabled": true, "automatic": false, "ask_first": true, "allow_workflows": false, "daily_limit": 5}
	h.do(request{method: "PUT", path: "/api/v1/assistant/repairs/policy", cookie: admin, body: body}).mustStatus(t, 200, "manual-only setup needs no installation payer")
	body["automatic"] = true
	body["ask_first"] = false
	h.do(request{method: "PUT", path: "/api/v1/assistant/repairs/policy", cookie: admin, body: body}).mustStatus(t, 422, "automatic repairs need an installation payer")
	policies, err := h.st.EliRepairPolicies(h.ctx)
	if err != nil || len(policies) != 1 || !policies[0].AskFirst {
		t.Fatalf("policy %+v %v", policies, err)
	}
}
func TestEliReadinessExplainsMissingSetupWithoutChargingAnAttempt(t *testing.T) {
	h := newHarness(t)
	_, viewer := h.user("repair-viewer", store.RoleViewer)
	response := h.do(request{method: "GET", path: "/api/v1/assistant/repairs/readiness?repo=acme/repo&pull_number=1", cookie: viewer})
	response.mustStatus(t, 200, "readiness is a diagnosis")
	var result struct {
		Ready    bool `json:"ready"`
		Blockers []struct {
			Code string `json:"code"`
		} `json:"blockers"`
	}
	if err := json.Unmarshal(response.body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Ready || len(result.Blockers) < 2 {
		t.Fatalf("readiness %+v", result)
	}
	rows, _, err := h.st.ListEliRepairsPage(h.ctx, store.EliRepairFilter{Limit: store.MaxEliRepairPage})
	if err != nil || len(rows) != 0 {
		t.Fatal("readiness spent an attempt")
	}
}
