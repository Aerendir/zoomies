package api

import (
	"context"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// machineResourceID is the provider's own identifier for a test's machine,
// derived from its name because the store refuses two machines on one resource
// -- which is the constraint that stops two rows believing they own one VM.
func machineResourceID(name string) string {
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(name))
	return strconv.Itoa(100 + int(sum.Sum32()%9000))
}

// machineOn is a machine that has reached a state and, when a host is given,
// enrolled onto it -- which is what makes it a machine with work on it.
func (h *harness) machineOn(p *store.Provider, name string, state store.MachineState, host *store.Host) *store.Machine {
	h.t.Helper()
	m := &store.Machine{
		ProviderID:   p.ID,
		Name:         name,
		State:        state,
		ResourceZone: "zone-a",
		ResourceID:   machineResourceID(name),
		Capacity:     p.MachineCapacity,
		Labels:       store.StringMap{},
	}
	if host != nil {
		m.HostID = host.ID
	}
	if err := h.st.CreateMachine(h.ctx, m); err != nil {
		h.t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

// A machine exists because demand asked for one. One creation path means one
// accounting path, so there is no route that makes one: a hand-made machine
// would be supply the reconciler would then decide to delete.
func TestThereIsNoWayToCreateAMachineByHand(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)

	resp := h.do(request{method: http.MethodPost, path: "/api/v1/machines", cookie: cookie,
		body: map[string]any{"provider_id": "prv_anything"}})
	if resp.status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /machines answered %d, want 405: %s", resp.status, truncate(resp.body))
	}
}

// Deleting a machine destroys the VM its runners are running on, so the first
// call refuses and says what to do; the second, after a drain, is the short one.
func TestDeletingAMachineWithLiveRunnersIsRefusedUnlessForced(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)

	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	host := h.host("zoomies-mach-host")
	prov := h.provider("proxmox-lab")
	m := h.machineOn(prov, "zoomies-mach-busy", store.MachineReady, host)
	h.runner(pool, host, store.RunnerBusy)

	refused := h.do(request{method: http.MethodDelete, path: "/api/v1/machines/" + m.ID, cookie: cookie})
	refused.mustStatus(t, http.StatusConflict, "delete a machine with a busy runner")
	msg := refused.errorMessage(t)
	if !strings.Contains(msg, "1 runner") {
		t.Errorf("the refusal does not say how many runners are on it: %q", msg)
	}
	if !strings.Contains(msg, "force=true") {
		t.Errorf("the refusal does not say how to insist: %q", msg)
	}

	forced := h.do(request{method: http.MethodDelete, path: "/api/v1/machines/" + m.ID + "?force=true", cookie: cookie})
	forced.mustStatus(t, http.StatusOK, "force-delete a machine")
	var out machineResponse
	forced.into(t, &out)
	if out.State != store.MachineDeleting {
		t.Errorf("the machine is %s rather than deleting; a delete is finished when the resource is gone, not when this returns", out.State)
	}
}

// A quarantined machine's resource may belong to somebody else, which is
// exactly what quarantine means. Deleting it -- forced or not -- would destroy
// another fleet's machine, so the refusal points at release instead.
func TestAQuarantinedMachineIsNeverDeletableAndTheRefusalPointsAtRelease(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	m := h.machineOn(prov, "zoomies-mach-doubtful", store.MachineQuarantined, nil)

	for _, path := range []string{
		"/api/v1/machines/" + m.ID,
		"/api/v1/machines/" + m.ID + "?force=true",
	} {
		resp := h.do(request{method: http.MethodDelete, path: path, cookie: cookie})
		resp.mustStatus(t, http.StatusConflict, "delete a quarantined machine")
		msg := resp.errorMessage(t)
		if !strings.Contains(msg, "release") {
			t.Errorf("the refusal does not point at the escape hatch: %q", msg)
		}
		if !strings.Contains(msg, "zone-a/"+machineResourceID(m.Name)) {
			t.Errorf("the refusal does not name the resource to go and look at: %q", msg)
		}
	}
	row, err := h.st.GetMachine(h.ctx, m.ID)
	if err != nil {
		t.Fatalf("GetMachine: %v", err)
	}
	if row.State != store.MachineQuarantined {
		t.Errorf("the machine moved to %s despite the refusal", row.State)
	}
}

// Release forgets a row and touches nothing, which cannot be undone and cannot
// be checked afterwards -- so the machine's name has to be typed, and the audit
// row keeps the provider's identifiers the database is about to stop holding.
func TestReleasingAMachineNeedsItsNameAndThenForgetsIt(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	m := h.machineOn(prov, "zoomies-mach-doubtful", store.MachineQuarantined, nil)

	wrong := h.do(request{method: http.MethodPost, path: "/api/v1/machines/" + m.ID + "/release", cookie: cookie,
		body: map[string]any{"name": "zoomies-mach-something-else"}})
	wrong.mustStatus(t, http.StatusUnprocessableEntity, "release with the wrong name")
	if msg := wrong.errorMessage(t); !strings.Contains(msg, m.Name) {
		t.Errorf("the refusal does not say what to type: %q", msg)
	}
	if _, err := h.st.GetMachine(h.ctx, m.ID); err != nil {
		t.Fatalf("the machine was forgotten despite the wrong name: %v", err)
	}

	ok := h.do(request{method: http.MethodPost, path: "/api/v1/machines/" + m.ID + "/release", cookie: cookie,
		body: map[string]any{"name": m.Name}})
	ok.mustStatus(t, http.StatusNoContent, "release a machine")
	if _, err := h.st.GetMachine(h.ctx, m.ID); err == nil {
		t.Fatal("the machine is still there after being released")
	}

	audit := h.do(request{method: http.MethodGet, path: "/api/v1/audit?limit=50", cookie: cookie})
	audit.mustStatus(t, http.StatusOK, "audit")
	body := string(audit.body)
	if !strings.Contains(body, "machine.release") {
		t.Error("releasing a machine was not audited")
	}
	if !strings.Contains(body, machineResourceID(m.Name)) {
		t.Error("the audit row does not keep the resource identifier, which is now the only record of it")
	}
}

// A draining machine keeps the work it has and accepts no more. Without the
// cordon the scheduler would go on placing runners onto a host whose VM is
// about to be destroyed, and it would never empty.
func TestDrainingAMachineCordonsItsHost(t *testing.T) {
	h := newHarness(t)
	operator, _ := h.user("operator", store.RoleOperator)
	cookie := h.session(operator)
	host := h.host("zoomies-mach-host")
	prov := h.provider("proxmox-lab")
	m := h.machineOn(prov, "zoomies-mach-leaving", store.MachineReady, host)

	resp := h.do(request{method: http.MethodPost, path: "/api/v1/machines/" + m.ID + "/drain", cookie: cookie})
	resp.mustStatus(t, http.StatusOK, "drain a machine")
	var out machineResponse
	resp.into(t, &out)
	if out.State != store.MachineDraining {
		t.Errorf("the machine is %s rather than draining", out.State)
	}

	got, err := h.st.GetHost(h.ctx, host.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if !got.Cordoned {
		t.Error("the host was not cordoned, so the scheduler would keep placing runners on a machine that is leaving")
	}
}

// Every other road into a machine's deletion cordons its host first, because
// the scheduler's snapshot carries hosts and never machines: an idle machine
// with a queued job matching its pools stays placeable for the whole minutes-long
// delete, and a runner placed in that window dies with the VM.
func TestDeletingAMachineCordonsItsHost(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	host := h.host("zoomies-mach-host")
	prov := h.provider("proxmox-lab")
	m := h.machineOn(prov, "zoomies-mach-idle", store.MachineReady, host)

	resp := h.do(request{method: http.MethodDelete, path: "/api/v1/machines/" + m.ID, cookie: cookie})
	resp.mustStatus(t, http.StatusOK, "delete an idle machine")

	got, err := h.st.GetHost(h.ctx, host.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if !got.Cordoned {
		t.Error("the host of a machine being deleted was not cordoned, so the scheduler could place a runner on a VM that is about to be destroyed")
	}
}

// A refused delete must have no side effect: cordoning before the refusal would
// take a busy machine out of service by way of a request that said no.
func TestARefusedMachineDeleteLeavesItsHostUncordoned(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	host := h.host("zoomies-mach-host")
	prov := h.provider("proxmox-lab")
	m := h.machineOn(prov, "zoomies-mach-busy", store.MachineReady, host)
	h.runner(pool, host, store.RunnerBusy)

	resp := h.do(request{method: http.MethodDelete, path: "/api/v1/machines/" + m.ID, cookie: cookie})
	resp.mustStatus(t, http.StatusConflict, "delete a machine with a busy runner")

	got, err := h.st.GetHost(h.ctx, host.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if got.Cordoned {
		t.Error("a refused delete cordoned the host")
	}
}

// A drain the state machine refuses -- only a ready machine can drain -- used to
// cordon the host first and then answer 409, so a request that reported an error
// still took a failed machine's enrolled host out of service, with no audit row.
func TestARefusedMachineDrainLeavesItsHostUncordoned(t *testing.T) {
	h := newHarness(t)
	operator, _ := h.user("operator", store.RoleOperator)
	cookie := h.session(operator)
	host := h.host("zoomies-mach-host")
	prov := h.provider("proxmox-lab")
	m := h.machineOn(prov, "zoomies-mach-broken", store.MachineFailed, host)

	resp := h.do(request{method: http.MethodPost, path: "/api/v1/machines/" + m.ID + "/drain", cookie: cookie})
	resp.mustStatus(t, http.StatusConflict, "drain a failed machine")

	got, err := h.st.GetHost(h.ctx, host.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if got.Cordoned {
		t.Error("a refused drain cordoned the host")
	}
}

// A machine's host is not an ordinary host: the VM outlives the row, and an
// operator who deleted it here would go on paying for a machine nothing is
// tracking. The refusal says which act removes the VM as well.
func TestDeletingAHostThatIsAMachineSaysToDeleteTheMachineInstead(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	host := h.host("zoomies-mach-host")
	prov := h.provider("proxmox-lab")
	h.machineOn(prov, "zoomies-mach-serving", store.MachineReady, host)

	refused := h.do(request{method: http.MethodDelete, path: "/api/v1/hosts/" + host.ID, cookie: cookie})
	refused.mustStatus(t, http.StatusConflict, "delete a host that is a machine")
	msg := refused.errorMessage(t)
	for _, want := range []string{host.Name, prov.Name, "force=true"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal does not mention %q: %q", want, msg)
		}
	}

	// Forcing still works; what it does to the VM is the next test's business.
	forced := h.do(request{method: http.MethodDelete, path: "/api/v1/hosts/" + host.ID + "?force=true", cookie: cookie})
	forced.mustStatus(t, http.StatusNoContent, "force-delete a host that is a machine")
}

// The refusal above promises that ?force=true forgets the host and leaves the VM
// running. The machine row's host_id is ON DELETE SET NULL, and the machine loop
// reads a Ready machine with no host as one to release, so a bare host delete
// drained and destroyed the VM -- and any job still on it -- within two passes.
// Forcing has to forget the machine row as well, and audit it, because after
// that the audit row is the only record of the resource.
func TestForceDeletingAMachinesHostLeavesTheVMRunningAndForgetsTheRow(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	host := h.host("zoomies-mach-host")
	prov := h.provider("proxmox-lab")
	m := h.machineOn(prov, "zoomies-mach-serving", store.MachineReady, host)

	forced := h.do(request{method: http.MethodDelete, path: "/api/v1/hosts/" + host.ID + "?force=true", cookie: cookie})
	forced.mustStatus(t, http.StatusNoContent, "force-delete a host that is a machine")

	if row, err := h.st.GetMachine(h.ctx, m.ID); err == nil {
		t.Fatalf("the machine is still recorded as %s, so the machine loop would release the VM the operator was promised would be left running", row.State)
	}

	audit := h.do(request{method: http.MethodGet, path: "/api/v1/audit?limit=50", cookie: cookie})
	audit.mustStatus(t, http.StatusOK, "audit")
	body := string(audit.body)
	if !strings.Contains(body, "machine.release") {
		t.Error("forgetting the machine was not audited")
	}
	if !strings.Contains(body, machineResourceID(m.Name)) {
		t.Error("the audit row does not keep the resource identifier, which is now the only record of it")
	}
}

// A machine already being deleted is the one case where forgetting the row would
// be the destructive act: the delete is under way and the VM is going anyway, so
// dropping the row would abandon a half-finished delete and leak the resource.
func TestForceDeletingTheHostOfAMachineThatIsAlreadyDeletingLeavesTheDeleteRunning(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	host := h.host("zoomies-mach-host")
	prov := h.provider("proxmox-lab")
	m := h.machineOn(prov, "zoomies-mach-leaving", store.MachineDeleting, host)

	forced := h.do(request{method: http.MethodDelete, path: "/api/v1/hosts/" + host.ID + "?force=true", cookie: cookie})
	forced.mustStatus(t, http.StatusNoContent, "force-delete the host of a deleting machine")

	row, err := h.st.GetMachine(h.ctx, m.ID)
	if err != nil {
		t.Fatalf("the deleting machine was forgotten, abandoning its delete: %v", err)
	}
	if row.State != store.MachineDeleting {
		t.Errorf("the machine is %s, want it left deleting", row.State)
	}
}

// An ordinary host is not a machine, and the check for one must not turn every
// host deletion into a refusal.
func TestDeletingAnOrdinaryHostIsUnaffectedByTheMachineCheck(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	host := h.host("a-machine-somebody-else-made")

	resp := h.do(request{method: http.MethodDelete, path: "/api/v1/hosts/" + host.ID, cookie: cookie})
	resp.mustStatus(t, http.StatusNoContent, "delete an ordinary host")
}

// The machines list is what the Hosts page's band and the provider page both
// read, so its filters have to narrow rather than merely decorate -- and a
// state nobody has heard of is the caller's mistake, named.
func TestTheMachinesListIsFilteredByProviderAndState(t *testing.T) {
	h := newHarness(t)
	viewer, _ := h.user("viewer", store.RoleViewer)
	cookie := h.session(viewer)
	first := h.provider("proxmox-lab")
	second := h.provider("proxmox-dr")
	h.machineOn(first, "zoomies-mach-one", store.MachineReady, nil)
	h.machineOn(first, "zoomies-mach-two", store.MachinePlanned, nil)
	h.machineOn(second, "zoomies-mach-three", store.MachineReady, nil)

	byProvider := h.do(request{method: http.MethodGet, path: "/api/v1/machines?provider=" + first.ID, cookie: cookie})
	byProvider.mustStatus(t, http.StatusOK, "machines by provider")
	var page struct {
		Items []machineResponse `json:"items"`
		Total int               `json:"total"`
	}
	byProvider.into(t, &page)
	if page.Total != 2 {
		t.Errorf("filtering by provider returned %d machines, want 2", page.Total)
	}
	for _, m := range page.Items {
		if m.ProviderID != first.ID {
			t.Errorf("machine %s belongs to %s", m.Name, m.ProviderID)
		}
		if m.ProviderName != first.Name {
			t.Errorf("machine %s does not carry its provider's name, so the grid would show an identifier", m.Name)
		}
	}

	byState := h.do(request{method: http.MethodGet, path: "/api/v1/machines?state=ready", cookie: cookie})
	byState.mustStatus(t, http.StatusOK, "machines by state")
	byState.into(t, &page)
	if page.Total != 2 {
		t.Errorf("filtering by state returned %d machines, want 2", page.Total)
	}

	bad := h.do(request{method: http.MethodGet, path: "/api/v1/machines?state=melted", cookie: cookie})
	bad.mustStatus(t, http.StatusBadRequest, "an unknown machine state")
	if msg := bad.errorMessage(t); !strings.Contains(msg, "quarantined") {
		t.Errorf("the refusal does not list the states there are: %q", msg)
	}
}

// The event stream is how a page follows a machine, and a frame carrying a
// store row rather than the view would repaint it without its provider's name,
// its timeline or whether it is safe to delete.
func TestAMachineFrameIsTheShapeTheRouteReturns(t *testing.T) {
	h := newHarness(t)
	operator, _ := h.user("operator", store.RoleOperator)
	cookie := h.session(operator)
	prov := h.provider("proxmox-lab")
	m := h.machineOn(prov, "zoomies-mach-watched", store.MachineReady, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frames, resp := h.openStream(t, ctx, "/api/v1/events", cookie, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("event stream status %d", resp.StatusCode)
	}

	drained := h.do(request{method: http.MethodPost, path: "/api/v1/machines/" + m.ID + "/drain", cookie: cookie})
	drained.mustStatus(t, http.StatusOK, "drain a machine")

	frame := await(t, frames, "a machine frame", func(f sseFrame) bool {
		return f.event == "machine.updated"
	})
	for _, want := range []string{`"provider_name":"proxmox-lab"`, `"timeline"`, `"safe_to_delete"`} {
		if !strings.Contains(frame.data, want) {
			t.Errorf("the machine frame is not the GET shape; it is missing %s: %s", want, frame.data)
		}
	}
	if strings.Contains(frame.data, "owner_fingerprint") {
		t.Error("the frame carries the ownership fingerprint, which is the mark a delete is checked against")
	}
}

// machineWithNoVM is a machine that never got as far as a resource, which is
// what a create that failed before the hypervisor was asked leaves behind.
func (h *harness) machineWithNoVM(p *store.Provider, name string, state store.MachineState) *store.Machine {
	h.t.Helper()
	m := &store.Machine{
		ProviderID: p.ID,
		Name:       name,
		State:      state,
		Capacity:   p.MachineCapacity,
		Labels:     store.StringMap{},
	}
	if err := h.st.CreateMachine(h.ctx, m); err != nil {
		h.t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

// bulkMachineAnswer is what POST /machines/bulk answers, read without the
// handler's own types so that these tests pin the JSON a client sees rather than
// the Go names behind it.
type bulkMachineAnswer struct {
	Results []struct {
		ID    string `json:"id"`
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	} `json:"results"`
}

// bulkDeleteMachines asks to delete several machines in one request.
func (h *harness) bulkDeleteMachines(cookie string, ids []string, force bool) *response {
	h.t.Helper()
	return h.do(request{method: http.MethodPost, path: "/api/v1/machines/bulk", cookie: cookie,
		body: map[string]any{"action": "delete", "ids": ids, "force": force}})
}

// Clearing out a pile of failed machines is the reason this route exists, and a
// pile is never uniform: some of it can go, some of it is busy, some of it is
// somebody else's. One status for the whole request would leave the operator
// not knowing which, so each machine gets its own answer, in the order asked.
func TestBulkDeletingMachinesAnswersForEachMachineSeparately(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	prov := h.provider("proxmox-lab")

	failed := h.machineWithNoVM(prov, "zoomies-mach-failed", store.MachineFailed)
	idleHost := h.host("zoomies-mach-idle-host")
	idle := h.machineOn(prov, "zoomies-mach-idle", store.MachineReady, idleHost)
	busyHost := h.host("zoomies-mach-busy-host")
	busy := h.machineOn(prov, "zoomies-mach-busy", store.MachineReady, busyHost)
	h.runner(pool, busyHost, store.RunnerBusy)
	doubtful := h.machineOn(prov, "zoomies-mach-doubtful", store.MachineQuarantined, nil)

	ids := []string{failed.ID, idle.ID, busy.ID, doubtful.ID, "mach_nope"}
	resp := h.bulkDeleteMachines(cookie, ids, false)
	resp.mustStatus(t, http.StatusOK, "bulk delete machines")
	var out bulkMachineAnswer
	resp.into(t, &out)
	if len(out.Results) != len(ids) {
		t.Fatalf("got %d results for %d machines: %s", len(out.Results), len(ids), truncate(resp.body))
	}
	for i, want := range ids {
		if out.Results[i].ID != want {
			t.Errorf("result %d is for %s, want %s: the answers must come back in the order asked", i, out.Results[i].ID, want)
		}
	}

	if !out.Results[0].OK || !out.Results[1].OK {
		t.Errorf("the failed and the idle machine were not deleted: %+v", out.Results[:2])
	}
	if got := out.Results[2]; got.OK || !strings.Contains(got.Error, "1 runner") || !strings.Contains(got.Error, "force") {
		t.Errorf("the busy machine's refusal does not say how many runners are on it and how to insist: %+v", got)
	}
	// ?force=true is how the single delete is told to insist; in a bulk request
	// it is a field of the body, and advice in the wrong syntax is worse than none.
	if got := out.Results[2]; strings.Contains(got.Error, "?force=true") {
		t.Errorf("the busy machine's refusal tells a bulk caller to use a query string: %q", got.Error)
	}
	if got := out.Results[3]; got.OK || !strings.Contains(got.Error, "release") {
		t.Errorf("the quarantined machine's refusal does not point at release: %+v", got)
	}
	if got := out.Results[4]; got.OK || got.Error == "" {
		t.Errorf("a machine that does not exist reported success: %+v", got)
	}

	for _, c := range []struct {
		m    *store.Machine
		want store.MachineState
	}{
		{failed, store.MachineDeleting},
		{idle, store.MachineDeleting},
		{busy, store.MachineReady},
		{doubtful, store.MachineQuarantined},
	} {
		row, err := h.st.GetMachine(h.ctx, c.m.ID)
		if err != nil {
			t.Fatalf("GetMachine(%s): %v", c.m.Name, err)
		}
		if row.State != c.want {
			t.Errorf("%s is %s, want %s", c.m.Name, row.State, c.want)
		}
	}
}

// A machine with runners on it loses their work when its VM is destroyed, so
// the bulk route says no to it unless it is told to insist, exactly as the
// single delete does, and the refusal costs nothing: the host stays in service.
func TestBulkDeletingMachinesLeavesTheBusyOnesAloneUnlessForced(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	prov := h.provider("proxmox-lab")
	host := h.host("zoomies-mach-busy-host")
	busy := h.machineOn(prov, "zoomies-mach-busy", store.MachineReady, host)
	h.runner(pool, host, store.RunnerBusy)

	refused := h.bulkDeleteMachines(cookie, []string{busy.ID}, false)
	refused.mustStatus(t, http.StatusOK, "bulk delete a busy machine")
	var out bulkMachineAnswer
	refused.into(t, &out)
	if len(out.Results) != 1 || out.Results[0].OK {
		t.Fatalf("a machine with a busy runner was deleted without being forced: %s", truncate(refused.body))
	}
	if got, err := h.st.GetHost(h.ctx, host.ID); err != nil || got.Cordoned {
		t.Errorf("a refused delete took the host out of service (err %v)", err)
	}

	forced := h.bulkDeleteMachines(cookie, []string{busy.ID}, true)
	forced.mustStatus(t, http.StatusOK, "bulk force-delete a busy machine")
	forced.into(t, &out)
	if len(out.Results) != 1 || !out.Results[0].OK {
		t.Fatalf("forcing did not delete the busy machine: %s", truncate(forced.body))
	}
	row, err := h.st.GetMachine(h.ctx, busy.ID)
	if err != nil {
		t.Fatalf("GetMachine: %v", err)
	}
	if row.State != store.MachineDeleting {
		t.Errorf("the forced machine is %s rather than deleting", row.State)
	}
}

// Quarantine means nothing could prove the resource is ours, so deleting it
// might destroy somebody else's machine. Selecting it in a crowd and insisting
// does not change that.
func TestBulkDeletingMachinesNeverDeletesAQuarantinedOneEvenWhenForced(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	doubtful := h.machineOn(prov, "zoomies-mach-doubtful", store.MachineQuarantined, nil)

	resp := h.bulkDeleteMachines(cookie, []string{doubtful.ID}, true)
	resp.mustStatus(t, http.StatusOK, "bulk force-delete a quarantined machine")
	var out bulkMachineAnswer
	resp.into(t, &out)
	if len(out.Results) != 1 || out.Results[0].OK {
		t.Fatalf("a quarantined machine was deleted: %s", truncate(resp.body))
	}
	row, err := h.st.GetMachine(h.ctx, doubtful.ID)
	if err != nil {
		t.Fatalf("GetMachine: %v", err)
	}
	if row.State != store.MachineQuarantined {
		t.Errorf("the machine moved to %s despite the refusal", row.State)
	}
}

// A machine that is already gone, or was never made, has nothing left to
// delete. Each says so in words an operator can act on, rather than naming a
// state transition the state machine happened to refuse.
func TestBulkDeletingMachinesSaysWhyThereIsNothingToDelete(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	gone := h.machineOn(prov, "zoomies-mach-gone", store.MachineDeleted, nil)
	planned := h.machineWithNoVM(prov, "zoomies-mach-planned", store.MachinePlanned)

	resp := h.bulkDeleteMachines(cookie, []string{gone.ID, planned.ID}, false)
	resp.mustStatus(t, http.StatusOK, "bulk delete machines with nothing to delete")
	var out bulkMachineAnswer
	resp.into(t, &out)
	if len(out.Results) != 2 {
		t.Fatalf("got %d results, want 2: %s", len(out.Results), truncate(resp.body))
	}
	if got := out.Results[0]; got.OK || !strings.Contains(got.Error, "already deleted") {
		t.Errorf("a deleted machine's refusal does not say it is already deleted: %+v", got)
	}
	if got := out.Results[1]; got.OK || !strings.Contains(got.Error, "only planned") {
		t.Errorf("a planned machine's refusal does not say nothing exists yet: %+v", got)
	}
	for _, got := range out.Results {
		if strings.Contains(got.Error, "->") {
			t.Errorf("the refusal names a state transition rather than saying what to do: %q", got.Error)
		}
	}
}

// Every other road into a machine's deletion cordons its host first, because
// the scheduler places onto hosts and never sees machines. The bulk route is one
// more road, and it must cordon exactly the hosts of the machines it deleted.
func TestBulkDeletingMachinesCordonsOnlyTheHostsOfTheMachinesItDeletes(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	prov := h.provider("proxmox-lab")
	idleHost := h.host("zoomies-mach-idle-host")
	idle := h.machineOn(prov, "zoomies-mach-idle", store.MachineReady, idleHost)
	busyHost := h.host("zoomies-mach-busy-host")
	busy := h.machineOn(prov, "zoomies-mach-busy", store.MachineReady, busyHost)
	h.runner(pool, busyHost, store.RunnerBusy)

	h.bulkDeleteMachines(cookie, []string{idle.ID, busy.ID}, false).mustStatus(t, http.StatusOK, "bulk delete machines")

	for _, c := range []struct {
		host     *store.Host
		cordoned bool
	}{{idleHost, true}, {busyHost, false}} {
		got, err := h.st.GetHost(h.ctx, c.host.ID)
		if err != nil {
			t.Fatalf("GetHost(%s): %v", c.host.Name, err)
		}
		if got.Cordoned != c.cordoned {
			t.Errorf("host %s cordoned = %v, want %v", c.host.Name, got.Cordoned, c.cordoned)
		}
	}
}

// Destroying machines is an administrator's act. An operator may drain one, and
// the bulk route is no way round that.
func TestBulkDeletingMachinesIsForAdministratorsOnly(t *testing.T) {
	h := newHarness(t)
	prov := h.provider("proxmox-lab")
	failed := h.machineWithNoVM(prov, "zoomies-mach-failed", store.MachineFailed)

	for _, role := range []store.Role{store.RoleViewer, store.RoleOperator} {
		_, cookie := h.user(string(role), role)
		resp := h.bulkDeleteMachines(cookie, []string{failed.ID}, false)
		resp.mustStatus(t, http.StatusForbidden, "bulk delete machines as a "+string(role))
	}
	row, err := h.st.GetMachine(h.ctx, failed.ID)
	if err != nil {
		t.Fatalf("GetMachine: %v", err)
	}
	if row.State != store.MachineFailed {
		t.Errorf("the machine is %s after requests that were refused", row.State)
	}
}

// A request that names no machine, or too many, is the caller's mistake and is
// named as one. The ceiling is the same as the runners' so that a script written
// for one works for the other.
func TestBulkDeletingMachinesNeedsBetweenOneAndFiveHundredMachines(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)

	none := h.bulkDeleteMachines(cookie, []string{}, false)
	none.mustStatus(t, http.StatusUnprocessableEntity, "bulk delete no machines")
	if !strings.Contains(none.errorMessage(t), "could not be carried out") {
		t.Errorf("unexpected message: %q", none.errorMessage(t))
	}

	many := make([]string, 501)
	for i := range many {
		many[i] = "mach_" + strconv.Itoa(i)
	}
	tooMany := h.bulkDeleteMachines(cookie, many, false)
	tooMany.mustStatus(t, http.StatusUnprocessableEntity, "bulk delete 501 machines")
	if !strings.Contains(string(tooMany.body), "500") {
		t.Errorf("the refusal does not say what the ceiling is: %s", truncate(tooMany.body))
	}
}

// delete is the only thing this route does, and it is asked for by name so that
// a later action cannot be mistaken for it by a client that forgot to say.
func TestBulkMachineActionMustBeDelete(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	failed := h.machineWithNoVM(prov, "zoomies-mach-failed", store.MachineFailed)

	for _, action := range []string{"", "drain", "explode"} {
		resp := h.do(request{method: http.MethodPost, path: "/api/v1/machines/bulk", cookie: cookie,
			body: map[string]any{"action": action, "ids": []string{failed.ID}}})
		resp.mustStatus(t, http.StatusUnprocessableEntity, "bulk machines with the action "+strconv.Quote(action))
		if !strings.Contains(string(resp.body), `"action"`) {
			t.Errorf("the refusal does not name the field at fault for %q: %s", action, truncate(resp.body))
		}
	}
	row, err := h.st.GetMachine(h.ctx, failed.ID)
	if err != nil {
		t.Fatalf("GetMachine: %v", err)
	}
	if row.State != store.MachineFailed {
		t.Errorf("a refused request still moved the machine to %s", row.State)
	}
}

// Deleting a machine destroys a VM, so each one that goes is on the record under
// the same action name a single delete uses, marked as part of a bulk request.
// A machine that was refused changed nothing and has no row.
func TestBulkDeletingMachinesIsAuditedForEachMachineItDeleted(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	failed := h.machineOn(prov, "zoomies-mach-failed", store.MachineFailed, nil)
	doubtful := h.machineOn(prov, "zoomies-mach-doubtful", store.MachineQuarantined, nil)

	h.bulkDeleteMachines(cookie, []string{failed.ID, doubtful.ID}, false).mustStatus(t, http.StatusOK, "bulk delete machines")

	audit := h.do(request{method: http.MethodGet, path: "/api/v1/audit?action=machine.delete&limit=50", cookie: cookie})
	audit.mustStatus(t, http.StatusOK, "audit")
	var rows page[store.AuditEvent]
	audit.into(t, &rows)
	if len(rows.Items) != 1 {
		t.Fatalf("got %d machine.delete rows, want 1 (the refused machine changed nothing): %s", len(rows.Items), truncate(audit.body))
	}
	row := rows.Items[0]
	if row.TargetID != failed.ID {
		t.Errorf("the audit row is about %s, want %s", row.TargetID, failed.ID)
	}
	for _, want := range []string{`"bulk":true`, machineResourceID(failed.Name)} {
		if !strings.Contains(row.After, want) {
			t.Errorf("the audit row does not carry %s: %s", want, row.After)
		}
	}
}

// Another browser has the machines list open, and it learns about a delete the
// way it learns about every change: a machine.updated frame in the shape of the
// GET. The bulk route has to announce each machine, or the second admin's page
// goes on showing machines that are already being destroyed.
func TestBulkDeletingMachinesAnnouncesEachMachineOnTheEventStream(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	first := h.machineWithNoVM(prov, "zoomies-mach-first", store.MachineFailed)
	second := h.machineWithNoVM(prov, "zoomies-mach-second", store.MachineFailed)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frames, resp := h.openStream(t, ctx, "/api/v1/events", cookie, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("event stream status %d", resp.StatusCode)
	}

	h.bulkDeleteMachines(cookie, []string{first.ID, second.ID}, false).mustStatus(t, http.StatusOK, "bulk delete machines")

	// Read until both have been announced, in whatever order: the route owes the
	// stream a frame for each machine, not a particular sequence of them.
	machines := []*store.Machine{first, second}
	seen := map[string]string{}
	await(t, frames, "a frame for each machine", func(f sseFrame) bool {
		if f.event != "machine.updated" {
			return false
		}
		for _, m := range machines {
			if strings.Contains(f.data, `"id":"`+m.ID+`"`) {
				seen[m.ID] = f.data
			}
		}
		return len(seen) == len(machines)
	})
	for _, m := range machines {
		for _, want := range []string{`"state":"deleting"`, `"provider_name":"proxmox-lab"`, `"safe_to_delete"`} {
			if !strings.Contains(seen[m.ID], want) {
				t.Errorf("the frame for %s is not the GET shape; it is missing %s: %s", m.Name, want, seen[m.ID])
			}
		}
	}
}

// A delete that finds nothing to do should say so in words, not in the name of
// a transition the state machine refused: the single route shares the rules, so
// it gains the same sentence.
func TestDeletingAMachineThatIsAlreadyGoneSaysSo(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	gone := h.machineOn(prov, "zoomies-mach-gone", store.MachineDeleted, nil)

	resp := h.do(request{method: http.MethodDelete, path: "/api/v1/machines/" + gone.ID, cookie: cookie})
	resp.mustStatus(t, http.StatusConflict, "delete a machine that is already deleted")
	if msg := resp.errorMessage(t); !strings.Contains(msg, "already deleted") {
		t.Errorf("the refusal does not say the machine is already deleted: %q", msg)
	}
}
