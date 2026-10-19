package api

import (
	"net/http"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

func TestAnOperatorChoosesWhetherAHostIsKeptAtItsRecommendation(t *testing.T) {
	h := newHarness(t)
	host := h.host("vm-1")
	host.CPUs, host.MemoryMB, host.DiskTotalMB, host.DiskFreeMB = 36, 128*1024, 40*1024, 30*1024
	if err := h.st.UpdateHost(h.ctx, host); err != nil {
		t.Fatal(err)
	}
	operator, _ := h.user("operator", store.RoleOperator)
	cookie := h.session(operator)

	patch := func(body map[string]any) *response {
		return h.do(request{method: http.MethodPatch, path: "/api/v1/hosts/" + host.ID, cookie: cookie, body: body})
	}
	var view hostResponse
	set := patch(map[string]any{"auto_adjust": "on"})
	set.mustStatus(t, http.StatusOK, "turn it on")
	set.into(t, &view)
	if view.AutoAdjust != "on" || !view.AutoAdjustEffective {
		t.Fatalf("on: %+v", view)
	}
	// An edit that names something else leaves the choice alone.
	patch(map[string]any{"labels": map[string]string{"a": "b"}}).mustStatus(t, http.StatusOK, "other field")
	if got, _ := h.st.GetHost(h.ctx, host.ID); got.AutoAdjust != store.AutoAdjustOn {
		t.Fatal("an unrelated edit changed the choice")
	}
	patch(map[string]any{"auto_adjust": "off"}).into(t, &view)
	if view.AutoAdjust != "off" || view.AutoAdjustEffective {
		t.Fatalf("off: %+v", view)
	}
	patch(map[string]any{"auto_adjust": ""}).into(t, &view)
	if view.AutoAdjust != "" || view.AutoAdjustEffective {
		t.Fatalf("handed back to the fleet, whose default is off: %+v", view)
	}
	patch(map[string]any{"auto_adjust": "sometimes"}).mustStatus(t, http.StatusUnprocessableEntity, "a value that is not an answer")
}

func TestTheRecommendationRouteSaysWhatAdjustWouldSetAndWhyDiskLimitedIt(t *testing.T) {
	h := newHarness(t)
	host := h.host("vm-1")
	host.CPUs, host.MemoryMB, host.DiskTotalMB, host.DiskFreeMB = 36, 128*1024, 40*1024, 30*1024
	if err := h.st.UpdateHost(h.ctx, host); err != nil {
		t.Fatal(err)
	}
	viewer, _ := h.user("viewer", store.RoleViewer)
	var rec HostRecommendationView
	resp := h.do(request{method: http.MethodGet, path: "/api/v1/hosts/" + host.ID + "/recommendation", cookie: h.session(viewer)})
	resp.mustStatus(t, http.StatusOK, "recommendation")
	resp.into(t, &rec)
	if !rec.Available || rec.LimitedBy != "disk" || rec.Capacity < 1 || rec.Capacity >= 16 || rec.Reason == "" || rec.ReserveDiskMB == 0 {
		t.Fatalf("a 40 GB disk should limit the runners: %+v", rec)
	}
	assertShape(t, loadSpec(t), "HostRecommendation", resp.body)

	bare := h.host("vm-2")
	bare.CPUs, bare.MemoryMB, bare.DiskTotalMB = 0, 0, 0
	if err := h.st.UpdateHost(h.ctx, bare); err != nil {
		t.Fatal(err)
	}
	h.do(request{method: http.MethodGet, path: "/api/v1/hosts/" + bare.ID + "/recommendation", cookie: h.session(viewer)}).into(t, &rec)
	if rec.Available {
		t.Fatal("a host that has not said how big it is gets no recommendation")
	}
}
