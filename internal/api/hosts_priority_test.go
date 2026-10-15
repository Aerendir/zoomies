package api

import (
	"net/http"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// specdHost is a host whose agent has measured the size and the speed
// figures placement ranks it by, as h.host's bare row will not carry them.
func specdHost(t *testing.T, h *harness, name string) *store.Host {
	t.Helper()
	host := h.host(name)
	host.CPUs, host.MemoryMB, host.CPUMHz, host.MemoryMTs, host.DiskKind = 8, 16*1024, 3600, 4800, "ssd"
	if err := h.st.UpdateHost(h.ctx, host); err != nil {
		t.Fatalf("UpdateHost: %v", err)
	}
	return host
}

// The priority is the one number that says where a host stands in placement,
// so the view carries it and says what set it. Nothing reported is 0, and the
// reason sentence names the specs it came from.
func TestAHostsViewCarriesItsPriority(t *testing.T) {
	h := newHarness(t)
	host := h.host("vm-1")
	if err := h.st.UpdateHost(h.ctx, host); err != nil {
		t.Fatalf("UpdateHost: %v", err)
	}
	operator, _ := h.user("operator", store.RoleOperator)

	view := h.do(request{method: http.MethodGet, path: "/api/v1/hosts/" + host.ID, cookie: h.session(operator)})
	view.mustStatus(t, http.StatusOK, "read a host that reports nothing")
	var got hostResponse
	view.into(t, &got)
	if got.Priority != 0 || got.PrioritySource != "computed" {
		t.Errorf("a host with no specs reads priority %d from %q; want the computed 0: %+v", got.Priority, got.PrioritySource, got)
	}

	specpet := specdHost(t, h, "vm-2")
	view = h.do(request{method: http.MethodGet, path: "/api/v1/hosts/" + specpet.ID, cookie: h.session(operator)})
	view.mustStatus(t, http.StatusOK, "read a host with specs")
	view.into(t, &got)
	if got.Priority == 0 || got.PrioritySource != "computed" {
		t.Errorf("a host with specs reads priority %d from %q, want the specs score: %+v", got.Priority, got.PrioritySource, got)
	}
	if got.PriorityReason == "" {
		t.Errorf("the priority block does not say what it was scored from")
	}
}

// The pin is the operator's override: a PATCH sets it, the card says it was
// set, and an override of null hands the ranking back to the specs.
func TestAnOperatorPinsAHostsPriorityAndClearsIt(t *testing.T) {
	h := newHarness(t)
	host := specdHost(t, h, "vm-1")
	operator, _ := h.user("operator", store.RoleOperator)

	pinOrder := h.do(request{method: http.MethodPatch, path: "/api/v1/hosts/" + host.ID,
		cookie: h.session(operator), body: map[string]any{"priority": map[string]any{"override": 90}}})
	pinOrder.mustStatus(t, http.StatusOK, "pin the host")
	var got hostResponse
	pinOrder.into(t, &got)
	if got.Priority != 90 || got.PrioritySource != "overridden" {
		t.Fatalf("after the pin the card reads %d from %q, want the operator's 90", got.Priority, got.PrioritySource)
	}
	stored, err := h.st.GetHost(h.ctx, host.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if stored.PriorityOverride == nil || *stored.PriorityOverride != 90 {
		t.Fatalf("the stored pin is %+v, want 90", stored.PriorityOverride)
	}
	if row := lastAudit(t, h, "host.update"); row == nil || row.TargetID != host.ID {
		t.Errorf("the audit row for the pin is %+v", row)
	}

	clear := h.do(request{method: http.MethodPatch, path: "/api/v1/hosts/" + host.ID,
		cookie: h.session(operator), body: map[string]any{"priority": map[string]any{"override": nil}}})
	clear.mustStatus(t, http.StatusOK, "clear the pin")
	clear.into(t, &got)
	if got.PrioritySource != "computed" {
		t.Fatalf("after the clear the card still reads %q, want the spec score", got.PrioritySource)
	}
	stored, err = h.st.GetHost(h.ctx, host.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if stored.PriorityOverride != nil {
		t.Fatalf("the stored pin survives a clear: %+v", stored.PriorityOverride)
	}
}

// A pin out of range is refused with the field named, and a field left out
// changes nothing.
func TestAPinOffTheScaleIsRefusedAndAQuietPatchChangesNothing(t *testing.T) {
	h := newHarness(t)
	host := specdHost(t, h, "vm-1")
	operator, _ := h.user("operator", store.RoleOperator)

	off := h.do(request{method: http.MethodPatch, path: "/api/v1/hosts/" + host.ID,
		cookie: h.session(operator), body: map[string]any{"priority": map[string]any{"override": 101}}})
	off.mustStatus(t, http.StatusUnprocessableEntity, "a pin off the scale")

	quiet := h.do(request{method: http.MethodPatch, path: "/api/v1/hosts/" + host.ID,
		cookie: h.session(operator), body: map[string]any{}})
	quiet.mustStatus(t, http.StatusOK, "a patch that names no priority")
	stored, err := h.st.GetHost(h.ctx, host.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if stored.PriorityOverride != nil {
		t.Fatalf("a patch that named no priority moved the pin: %+v", stored.PriorityOverride)
	}
}

// Re-check settles the card now. The specs are re-measured by every heartbeat,
// so the endpoint's only act is the frame and the pass; what it answers is the
// host with the priority, and the audit row says who asked.
func TestRecheckingAHostsPriorityAnswersItAndAuditsThePress(t *testing.T) {
	h := newHarness(t)
	host := specdHost(t, h, "vm-1")
	operator, _ := h.user("operator", store.RoleOperator)

	resp := h.do(request{method: http.MethodPost, path: "/api/v1/hosts/" + host.ID + "/priority-check",
		cookie: h.session(operator)})
	resp.mustStatus(t, http.StatusOK, "re-check the priority")
	var got hostResponse
	resp.into(t, &got)
	if got.Priority == 0 {
		t.Fatalf("the re-check answered a priority of 0")
	}
	if row := lastAudit(t, h, "host.priority_checked"); row == nil || row.TargetID != host.ID {
		t.Errorf("no host.priority_checked audit row was written: %+v", row)
	}

	viewer, _ := h.user("viewer", store.RoleViewer)
	if r := h.do(request{method: http.MethodPost, path: "/api/v1/hosts/" + host.ID + "/priority-check", cookie: h.session(viewer)}); r.status != http.StatusForbidden {
		t.Errorf("a viewer re-checked the priority: %d", r.status)
	}
	if r := h.do(request{method: http.MethodPost, path: "/api/v1/hosts/host_missing/priority-check", cookie: h.session(operator)}); r.status != http.StatusNotFound {
		t.Errorf("a stale id read %d, want 404", r.status)
	}
}
