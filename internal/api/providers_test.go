package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/store"
	"github.com/tailscale/tailcat"
)

// A provider's credential reaches a hypervisor: anyone holding it can create,
// inspect and destroy every machine on the cluster. It goes in once and never
// comes back, and what a form is told instead is that one is set.
func TestAProviderNeverReturnsItsCredential(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)

	const credential = "zoomies@pve!ci=nobody-may-ever-see"
	created := h.do(request{method: http.MethodPost, path: "/api/v1/providers", cookie: cookie,
		body: map[string]any{"kind": "fake", "name": "proxmox-lab", "endpoint": "https://pve.example.com:8006",
			"credential": credential, "settings": map[string]string{"zone": "zone-a"}}})
	created.mustStatus(t, http.StatusCreated, "create provider")

	var made providerResponse
	created.into(t, &made)
	if !made.CredentialsConfigured {
		t.Error("the provider does not report that a credential is configured, so a form would offer to set one that is already set")
	}
	for _, resp := range []*response{
		created,
		h.do(request{method: http.MethodGet, path: "/api/v1/providers", cookie: cookie}),
		h.do(request{method: http.MethodGet, path: "/api/v1/providers/" + made.ID, cookie: cookie}),
	} {
		if strings.Contains(string(resp.body), credential) {
			t.Fatalf("a provider response carries the credential: %s", truncate(resp.body))
		}
	}
	// And it really was stored: a credential that was quietly dropped would
	// pass every assertion above.
	row, err := h.st.GetProvider(h.ctx, made.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if len(row.CredentialsEnc) == 0 {
		t.Fatal("no credential was sealed into the row")
	}
	if strings.Contains(string(row.CredentialsEnc), credential) {
		t.Fatal("the credential is on the row in plaintext")
	}
}

// A form renders a blank box for a credential it cannot read back. If saving
// that blank box cleared the stored credential, fixing a typo in a provider's
// name would take the fleet's machines offline at the next pass.
func TestAnEmptyCredentialLeavesTheStoredOneAlone(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	before, err := h.st.GetProvider(h.ctx, prov.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}

	resp := h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + prov.ID, cookie: cookie,
		body: map[string]any{"name": "proxmox-lab-2", "credential": ""}})
	resp.mustStatus(t, http.StatusOK, "patch provider")

	after, err := h.st.GetProvider(h.ctx, prov.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if string(after.CredentialsEnc) != string(before.CredentialsEnc) {
		t.Error("an empty credential changed the sealed one")
	}
	if after.Name != "proxmox-lab-2" {
		t.Errorf("the rename did not happen: %q", after.Name)
	}
}

// The certificate a provider trusts is the public half of a cluster's own
// authority, not a secret, and a form can only keep what it is shown. It comes
// back in the provider's response so that the edit form opens with the box
// filled in: a box that opens empty reads as "nothing is trusted", and saving it
// then clears the one thing the connection depends on.
func TestAProviderReturnsTheCertificateItTrusts(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	cert := strings.TrimSpace(setupCertificate(t))

	created := h.do(request{method: http.MethodPost, path: "/api/v1/providers", cookie: cookie,
		body: map[string]any{"kind": "fake", "name": "proxmox-lab", "endpoint": "https://pve.example.com:8006",
			"credential": "zoomies@pve!ci=secret", "ca_pem": cert, "settings": map[string]string{"zone": "zone-a"}}})
	created.mustStatus(t, http.StatusCreated, "create provider")
	var made providerResponse
	created.into(t, &made)
	if made.CAPEM != cert {
		t.Errorf("the created provider answers ca_pem = %q, want the certificate it was given", made.CAPEM)
	}

	got := h.do(request{method: http.MethodGet, path: "/api/v1/providers/" + made.ID, cookie: cookie})
	got.mustStatus(t, http.StatusOK, "get provider")
	var read providerResponse
	got.into(t, &read)
	if read.CAPEM != cert {
		t.Errorf("reading the provider back answers ca_pem = %q, want the certificate it trusts", read.CAPEM)
	}
}

// A PATCH leaves what it does not name alone. This is the half of the contract
// the edit form relies on once it is shown the certificate, so it is pinned
// beside the half that clears one.
func TestAnEditThatDoesNotMentionTheCertificateKeepsIt(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	cert := strings.TrimSpace(setupCertificate(t))
	h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + prov.ID, cookie: cookie,
		body: map[string]any{"ca_pem": cert}}).mustStatus(t, http.StatusOK, "set the certificate")

	h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + prov.ID, cookie: cookie,
		body: map[string]any{"name": "proxmox-lab-2"}}).mustStatus(t, http.StatusOK, "rename the provider")

	row, err := h.st.GetProvider(h.ctx, prov.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if row.CAPEM != cert {
		t.Errorf("a rename changed the stored certificate to %q", row.CAPEM)
	}
}

// Emptying the box is how an operator goes back to the system's trust store,
// which a cluster that moves to a publicly trusted certificate has to do: a CA
// that is set replaces the system's pool, so the old one would refuse the new
// certificate.
func TestAnEmptyCertificateOnAnEditClearsIt(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	cert := strings.TrimSpace(setupCertificate(t))
	h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + prov.ID, cookie: cookie,
		body: map[string]any{"ca_pem": cert}}).mustStatus(t, http.StatusOK, "set the certificate")

	h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + prov.ID, cookie: cookie,
		body: map[string]any{"ca_pem": ""}}).mustStatus(t, http.StatusOK, "clear the certificate")

	row, err := h.st.GetProvider(h.ctx, prov.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if row.CAPEM != "" {
		t.Errorf("an empty certificate left %q stored", row.CAPEM)
	}
}

// A key pasted into the certificate box by mistake was never checked on the way
// in (a cluster keeps its authority's private half beside the public one), so it
// can already be stored. It must not be sent on to every page, event and audit
// row that renders a provider.
func TestAProviderResponseNeverCarriesAKeyPastedAsACertificate(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	row, err := h.st.GetProvider(h.ctx, prov.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	row.CAPEM = "-----BEGIN PRIVATE KEY-----\nbm90IGEgcmVhbCBrZXk=\n-----END PRIVATE KEY-----"
	if err := h.st.UpdateProvider(h.ctx, row); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}

	for _, resp := range []*response{
		h.do(request{method: http.MethodGet, path: "/api/v1/providers", cookie: cookie}),
		h.do(request{method: http.MethodGet, path: "/api/v1/providers/" + prov.ID, cookie: cookie}),
	} {
		resp.mustStatus(t, http.StatusOK, "read providers")
		if strings.Contains(string(resp.body), "PRIVATE KEY") || strings.Contains(string(resp.body), "bm90IGEgcmVhbCBrZXk") {
			t.Fatalf("a provider response carries the key: %s", truncate(resp.body))
		}
	}
}

// The machines a provider already owns are its kind's machines, managed by its
// kind's driver. A kind that could be edited would leave them being reasoned
// about by a driver that has never heard of them.
func TestAProvidersKindCannotBeChanged(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")

	resp := h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + prov.ID, cookie: cookie,
		body: map[string]any{"kind": "proxmox"}})
	resp.mustStatus(t, http.StatusUnprocessableEntity, "change a provider's kind")
	if msg := resp.errorMessage(t); !strings.Contains(strings.ToLower(msg), "provider") {
		t.Errorf("the refusal does not say what was refused: %q", msg)
	}
	if !strings.Contains(string(resp.body), `"kind"`) {
		t.Errorf("the refusal does not name the field: %s", truncate(resp.body))
	}
}

// The wizard asks "is this valid" as somebody types, and an answer of 422 would
// make an incomplete draft look like a broken request. The verdict is in the
// body, exactly as it is for a pool.
func TestValidatingADraftProviderAnswersOKAndPutsTheVerdictInTheBody(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)

	resp := h.do(request{method: http.MethodPost, path: "/api/v1/providers/validate", cookie: cookie,
		body: map[string]any{"kind": "fake", "name": "draft", "settings": map[string]string{}}})
	resp.mustStatus(t, http.StatusOK, "validate an incomplete draft")

	var out validateProviderResponse
	resp.into(t, &out)
	if out.Valid {
		t.Fatal("a draft with no zone was called valid, so the wizard would offer to save a provider that cannot create anything")
	}
	// The driver's own finding is what says which answer is missing, and the
	// form highlights a field rather than a sentence.
	found := false
	for _, e := range out.Errors {
		if e.Field == "settings.zone" {
			found = true
		}
	}
	if !found {
		t.Errorf("nothing named the setting that is missing: %+v", out.Errors)
	}

	// And the valid half, so a refusal of everything would not pass.
	ok := h.do(request{method: http.MethodPost, path: "/api/v1/providers/validate", cookie: cookie,
		body: map[string]any{"kind": "fake", "name": "draft", "settings": map[string]string{"zone": "zone-a"}}})
	ok.mustStatus(t, http.StatusOK, "validate a complete draft")
	var good validateProviderResponse
	ok.into(t, &good)
	if !good.Valid {
		t.Errorf("a complete draft was called invalid: %+v", good.Errors)
	}
}

// Editing a provider is a dry run of a change to a row that already exists, so
// the name check has to exclude it. Without ?id= the form could not be saved at
// all without also renaming the provider -- it was refused about itself.
func TestValidatingAnEditIsNotRefusedAboutTheProviderBeingEdited(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")

	body := map[string]any{"kind": "fake", "name": prov.Name, "settings": map[string]string{"zone": "zone-a"}}
	clash := h.do(request{method: http.MethodPost, path: "/api/v1/providers/validate", cookie: cookie, body: body})
	clash.mustStatus(t, http.StatusOK, "validate a clashing name")
	var out validateProviderResponse
	clash.into(t, &out)
	if out.Valid {
		t.Error("a second provider with an existing name was called valid")
	}

	same := h.do(request{method: http.MethodPost, path: "/api/v1/providers/validate?id=" + prov.ID, cookie: cookie, body: body})
	same.mustStatus(t, http.StatusOK, "validate an edit to the same provider")
	var edit validateProviderResponse
	same.into(t, &edit)
	if !edit.Valid {
		t.Errorf("editing a provider was refused about itself: %+v", edit.Errors)
	}
}

// /providers/validate and /providers/kinds have to be registered before
// /providers/{id}, or chi hands them to the handler that looks up a provider
// whose id is "kinds" and both answer 404.
func TestTheStaticProviderRoutesAreNotReadAsIdentifiers(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)

	kinds := h.do(request{method: http.MethodGet, path: "/api/v1/providers/kinds", cookie: cookie})
	kinds.mustStatus(t, http.StatusOK, "list provider kinds")
	var listed struct {
		Items []providerKindResponse `json:"items"`
	}
	kinds.into(t, &listed)
	if len(listed.Items) == 0 {
		t.Fatal("no provider kinds were returned, so the wizard has nothing to offer")
	}
	if len(listed.Items[0].Settings) == 0 {
		t.Error("a driver that asks for nothing would render an empty form")
	}

	validate := h.do(request{method: http.MethodPost, path: "/api/v1/providers/validate", cookie: cookie,
		body: map[string]any{"kind": "fake", "name": "draft"}})
	validate.mustStatus(t, http.StatusOK, "validate a draft")
}

// The rows are the only record of what was rented. Deleting a provider while
// its machines still hold resources would leave VMs running with nothing
// tracking them, so the refusal says how many and what to do first.
func TestDeletingAProviderIsRefusedWhileItOwnsMachines(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	h.machine(prov, "zoomies-mach-one")

	refused := h.do(request{method: http.MethodDelete, path: "/api/v1/providers/" + prov.ID, cookie: cookie})
	refused.mustStatus(t, http.StatusConflict, "delete a provider with a machine")
	msg := refused.errorMessage(t)
	if !strings.Contains(msg, "1 machine") {
		t.Errorf("the refusal does not say how many machines are behind it: %q", msg)
	}
	if !strings.Contains(msg, prov.Name) {
		t.Errorf("the refusal does not name the provider: %q", msg)
	}

	// With the machine's resource confirmed gone, the provider goes.
	if _, err := h.st.ConfirmMachineDeleted(h.ctx, h.lastMachine(prov.ID).ID, h.ctrl.Now()); err != nil {
		t.Fatalf("ConfirmMachineDeleted: %v", err)
	}
	gone := h.do(request{method: http.MethodDelete, path: "/api/v1/providers/" + prov.ID, cookie: cookie})
	gone.mustStatus(t, http.StatusNoContent, "delete a provider whose machines are gone")
}

// The kill switch is pressed by whoever is on call, and it has to leave the
// fleet in a known state rather than argue about whether it was already in one.
func TestPressingAProvidersKillSwitchTwiceIsNotAnError(t *testing.T) {
	h := newHarness(t)
	operator, _ := h.user("operator", store.RoleOperator)
	cookie := h.session(operator)
	prov := h.provider("proxmox-lab")
	// Renting is switched on first. With it off every provider is held, and says
	// so before it says anything about its own pause, so "a paused provider says
	// why" would be passing on the switch and not on the pause.
	h.ctrl.UpdateConfig(func(c *config.Config) { c.Provider.Enabled = true })

	for i := range 2 {
		resp := h.do(request{method: http.MethodPost, path: "/api/v1/providers/" + prov.ID + "/pause", cookie: cookie,
			body: map[string]any{"reason": "the hypervisor is being patched"}})
		resp.mustStatus(t, http.StatusOK, "pause a provider")
		var out providerResponse
		resp.into(t, &out)
		if !out.Paused {
			t.Fatalf("press %d left the provider unpaused", i+1)
		}
		if out.PausedReason == "" {
			t.Error("the reason was not kept, so the card cannot say why new machines stopped")
		}
		if !strings.Contains(out.Held, "paused") {
			t.Errorf("a paused provider says %q, not why it is buying nothing", out.Held)
		}
	}
	for i := range 2 {
		resp := h.do(request{method: http.MethodPost, path: "/api/v1/providers/" + prov.ID + "/resume", cookie: cookie})
		resp.mustStatus(t, http.StatusOK, "resume a provider")
		var out providerResponse
		resp.into(t, &out)
		if out.Paused {
			t.Fatalf("press %d left the provider paused", i+1)
		}
		if out.PausedReason != "" {
			t.Error("a resumed provider still carries the reason it was paused")
		}
		if out.Held != "" {
			t.Errorf("a resumed provider is still held: %q", out.Held)
		}
	}

	// Both halves are audited, because "who stopped the fleet buying machines
	// at three in the morning" is exactly what an audit log is read for.
	audit := h.do(request{method: http.MethodGet, path: "/api/v1/audit?limit=200", cookie: cookie})
	audit.mustStatus(t, http.StatusOK, "audit")
	for _, action := range []string{"provider.pause", "provider.resume"} {
		if !strings.Contains(string(audit.body), action) {
			t.Errorf("%s was not audited", action)
		}
	}
}

// A preflight is the one call an operator can make against a hypervisor without
// risking anything, and its answer is recorded so the warning the UI is showing
// is quieted by a check run in a terminal.
func TestCheckingAProviderRecordsWhatItFound(t *testing.T) {
	h := newHarness(t)
	operator, _ := h.user("operator", store.RoleOperator)
	cookie := h.session(operator)
	prov := h.provider("proxmox-lab")

	resp := h.do(request{method: http.MethodPost, path: "/api/v1/providers/" + prov.ID + "/check", cookie: cookie})
	resp.mustStatus(t, http.StatusOK, "check a provider")
	var report providerCheckResponse
	resp.into(t, &report)
	if !report.Reachable {
		t.Fatalf("the fake provider reported itself unreachable: %s", truncate(resp.body))
	}
	if report.ProviderID != prov.ID {
		t.Errorf("the report is about %q rather than the provider that was checked", report.ProviderID)
	}

	row, err := h.st.GetProvider(h.ctx, prov.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if row.LastCheckAt == nil {
		t.Error("the check was not recorded on the row, so the page still says it has never been verified")
	}
}

// The sentence on a card is a finding's title, and the detail that says what to
// do about it used to exist only in the tab that pressed Check. Reading the
// provider back has to carry the whole result, or the next page load is
// "no bridge called vmbr0" again with nothing to act on.
func TestAProviderKeepsTheWholeOfItsLastCheck(t *testing.T) {
	h := newHarness(t)
	operator, _ := h.user("operator", store.RoleOperator)
	cookie := h.session(operator)
	prov := h.provider("proxmox-lab")

	h.do(request{method: http.MethodPost, path: "/api/v1/providers/" + prov.ID + "/check", cookie: cookie}).
		mustStatus(t, http.StatusOK, "check a provider")

	resp := h.do(request{method: http.MethodGet, path: "/api/v1/providers/" + prov.ID, cookie: cookie})
	resp.mustStatus(t, http.StatusOK, "read the provider back")
	var got struct {
		LastCheck *providerCheckResponse `json:"last_check"`
	}
	resp.into(t, &got)
	if got.LastCheck == nil {
		t.Fatalf("the provider does not carry its last check: %s", truncate(resp.body))
	}
	if got.LastCheck.ProviderID != prov.ID || !got.LastCheck.Reachable || got.LastCheck.CheckedAt.IsZero() {
		t.Errorf("the kept check is not the one that ran: %+v", got.LastCheck)
	}
}

// A pool nobody rents for looks the same whichever selector said no: an empty
// Machines list and a queue. The pairings are where an operator finds out whose
// setting it was, in the words of the setting.
func TestPairingsSayWhichSideRefusedAndWhy(t *testing.T) {
	h := newHarness(t)
	viewer, _ := h.user("viewer", store.RoleViewer)
	cookie := h.session(viewer)
	inst := h.installation()

	open := h.provider("open-lab")
	picky := h.provider("picky-lab")
	picky.PoolSelector = store.StringMap{"gpu": ""}
	if err := h.st.UpdateProvider(h.ctx, picky); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	pool := h.pool(inst, "linux-x64")
	guarded := h.pool(inst, "linux-guarded")
	guarded.ProviderSelector = store.StringMap{"name": "somewhere-else"}
	if err := h.st.UpdatePool(h.ctx, guarded); err != nil {
		t.Fatalf("UpdatePool: %v", err)
	}

	resp := h.do(request{method: http.MethodGet, path: "/api/v1/providers/pairings", cookie: cookie})
	resp.mustStatus(t, http.StatusOK, "read the pairings")
	var list struct {
		Items []controller.PairingView `json:"items"`
	}
	resp.into(t, &list)
	if len(list.Items) != 4 {
		t.Fatalf("%d pairings for two providers and two pools, want 4: %s", len(list.Items), truncate(resp.body))
	}
	find := func(provider, pool string) controller.PairingView {
		for _, it := range list.Items {
			if it.ProviderName == provider && it.PoolName == pool {
				return it
			}
		}
		t.Fatalf("no pairing of %s and %s", provider, pool)
		return controller.PairingView{}
	}

	if got := find(open.Name, pool.Name); !got.Serves || got.Why != "" {
		t.Errorf("two unrestricted sides do not serve each other: %+v", got)
	}
	got := find(picky.Name, pool.Name)
	if got.Serves || got.By != "provider" || !strings.Contains(got.Why, "gpu") || got.Fix == "" {
		t.Errorf("the provider's refusal is not reported as the provider's: %+v", got)
	}
	got = find(open.Name, guarded.Name)
	if got.Serves || got.By != "pool" || !strings.Contains(got.Why, "somewhere-else") {
		t.Errorf("the pool's refusal is not reported as the pool's: %+v", got)
	}
}

// A pool with work queued and nothing rented for it looks the same whether this
// controller may not rent at all, the fleet may rent none, or no provider builds
// a machine that suits the pool, and the three call for different things. The
// route is where an operator finds out which. It is a viewer's: reading the
// answer changes nothing, and acting on it is what takes a role.
func TestTheRentingRouteSaysWhyNothingIsRentedWhileRentingIsOff(t *testing.T) {
	// A fleet ceiling, so that the switch is the only thing standing in the way
	// and the answer under test is "renting is off", not "the fleet may rent none".
	h := newHarness(t, func(c *config.Config) { c.Provider.MaxMachines = 10 })
	viewer, _ := h.user("viewer", store.RoleViewer)
	cookie := h.session(viewer)

	// read returns the view and the pools as they were written. A nil slice and
	// an empty one decode alike, and a page that is sent null has to be written
	// to cope with it, so the text is kept to compare.
	read := func() (controller.RentingView, string) {
		t.Helper()
		resp := h.do(request{method: http.MethodGet, path: "/api/v1/providers/renting", cookie: cookie})
		resp.mustStatus(t, http.StatusOK, "read the renting view")
		var view controller.RentingView
		resp.into(t, &view)
		var written struct {
			Pools json.RawMessage `json:"pools"`
		}
		resp.into(t, &written)
		return view, string(written.Pools)
	}

	// With no provider there is nothing to switch on, and a fleet told to turn on
	// renting would be sent to a switch that changes nothing.
	view, pools := read()
	if view.State != "none" || view.Setting != "" || view.Fix != "" {
		t.Errorf("a fleet with no provider is %q with setting %q and fix %q, want none and neither", view.State, view.Setting, view.Fix)
	}
	if pools != "[]" {
		t.Errorf("pools = %s for a fleet with no provider, want []", pools)
	}

	// A provider and no machine pass yet. The switches are read with the request,
	// so the state is already known; there is no verdict to give, and "no
	// verdicts" is an empty list and never null.
	h.provider("lab")
	view, pools = read()
	if view.State != "off" || view.Setting != "provider.enabled" || view.Enabled {
		t.Errorf("before any pass the state is %q (setting %q, enabled %v), want off, provider.enabled and not enabled",
			view.State, view.Setting, view.Enabled)
	}
	if pools != "[]" || view.DecidedAt != nil {
		t.Errorf("before any pass pools = %s decided at %v, want [] and no time", pools, view.DecidedAt)
	}

	// Work waiting and no host to take it: the pass decides what it would rent,
	// and the verdict says that it is the switch, not the work, that stops it.
	// The report is the pass's, so one has to have run.
	pool := h.pool(h.installation(), "linux-x64")
	h.job(pool, store.JobQueued)
	if err := h.ctrl.ReconcileMachines(h.ctx); err != nil {
		t.Fatalf("ReconcileMachines: %v", err)
	}
	view, _ = read()
	if view.State != "off" || view.Setting != "provider.enabled" || view.Summary == "" || view.Fix == "" {
		t.Errorf("the fleet is %q (setting %q) with summary %q and fix %q, want off, provider.enabled and words for both",
			view.State, view.Setting, view.Summary, view.Fix)
	}
	if view.DecidedAt == nil {
		t.Error("the view carries no decision time, so a reader cannot tell how old the verdicts are")
	}
	if len(view.Pools) != 1 {
		t.Fatalf("%d verdicts for one pool with work waiting, want 1: %+v", len(view.Pools), view.Pools)
	}
	got := view.Pools[0]
	if got.PoolID != pool.ID || got.PoolName != pool.Name || got.ProviderName != "lab" {
		t.Errorf("the verdict is about pool %s (%s) and provider %q, want %s (%s) and lab", got.PoolID, got.PoolName, got.ProviderName, pool.ID, pool.Name)
	}
	if got.Machines != 1 || !strings.HasPrefix(got.Summary, "Would rent 1 machine") {
		t.Errorf("the verdict says %q for %d machines, want it to say 1 machine would be rented (the ceiling of 10 leaves the switch as the only thing in the way)", got.Summary, got.Machines)
	}
	if got.State != controller.RentingOff || got.Setting != "provider.enabled" || got.Summary == "" || got.Fix == "" {
		t.Errorf("the pool is %q (setting %q) with summary %q and fix %q, want off, provider.enabled and words for both",
			got.State, got.Setting, got.Summary, got.Fix)
	}
}

// The pool's half of the agreement has to survive being written and read: a
// selector that silently came back empty would leave every provider free to rent
// for a pool somebody had restricted.
func TestAPoolKeepsItsProviderSelector(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	pool := h.pool(h.installation(), "linux-x64")

	res := h.do(request{method: http.MethodPatch, path: "/api/v1/pools/" + pool.ID, cookie: cookie,
		body: map[string]any{"provider_selector": map[string]string{"site": "garage"}}})
	res.mustStatus(t, http.StatusOK, "set a pool's provider selector")
	var saved poolResponse
	res.into(t, &saved)
	if saved.ProviderSelector["site"] != "garage" {
		t.Fatalf("the selector did not come back: %+v", saved.ProviderSelector)
	}
	row, err := h.st.GetPool(h.ctx, pool.ID)
	if err != nil {
		t.Fatalf("GetPool: %v", err)
	}
	if row.ProviderSelector["site"] != "garage" {
		t.Errorf("the selector was not stored: %+v", row.ProviderSelector)
	}

	res = h.do(request{method: http.MethodPatch, path: "/api/v1/pools/" + pool.ID, cookie: cookie,
		body: map[string]any{"provider_selector": map[string]string{" ": "x"}}})
	res.mustStatus(t, http.StatusUnprocessableEntity, "an empty selector key")
}

// A build with no driver for a stored provider's kind must say so rather than
// answering 500: the row was legal when it was written, and the fix is an
// upgrade, which is a sentence rather than a request ID.
func TestAskingADriverThisBuildDoesNotShipSaysSoRatherThanFailing(t *testing.T) {
	h := newHarness(t)
	operator, _ := h.user("operator", store.RoleOperator)
	cookie := h.session(operator)

	// Written through the store, because the API refuses to create one.
	row := &store.Provider{
		Kind: store.ProviderProxmox, Name: "somebody-elses-build", MachineCapacity: 2,
		MachineBackend: store.BackendDocker, MaxCreatesInFlight: 1, Enabled: true,
	}
	if err := h.st.CreateProvider(h.ctx, row); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	resp := h.do(request{method: http.MethodPost, path: "/api/v1/providers/" + row.ID + "/check", cookie: cookie})
	resp.mustStatus(t, http.StatusConflict, "check a provider this build cannot speak")
	if msg := resp.errorMessage(t); !strings.Contains(msg, string(store.ProviderProxmox)) {
		t.Errorf("the refusal does not name the kind: %q", msg)
	}
}

// The orphan review is read before somebody destroys something, so the three
// sections have to mean different things: a row holding nothing can be released
// with nothing lost, and a row nobody can vouch for cannot be touched at all.
func TestTheOrphanReviewSeparatesRowsThatHoldNothingFromOnesNobodyCanVouchFor(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")

	empty := &store.Machine{ProviderID: prov.ID, Name: "zoomies-mach-empty", State: store.MachinePlanned}
	if err := h.st.CreateMachine(h.ctx, empty); err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	held := h.machine(prov, "zoomies-mach-held")
	if _, err := h.st.TransitionMachine(h.ctx, held.ID, store.MachineQuarantined, "ownership could not be proved"); err != nil {
		t.Fatalf("TransitionMachine: %v", err)
	}

	resp := h.do(request{method: http.MethodGet, path: "/api/v1/providers/" + prov.ID + "/orphans", cookie: cookie})
	resp.mustStatus(t, http.StatusOK, "orphan review")
	var out providerOrphansResponse
	resp.into(t, &out)

	if len(out.NoResource) != 1 || out.NoResource[0].ID != empty.ID {
		t.Errorf("the row that holds nothing is not in no_resource: %+v", out.NoResource)
	}
	if len(out.Unverified) != 1 || out.Unverified[0].ID != held.ID {
		t.Errorf("the quarantined machine is not in unverified: %+v", out.Unverified)
	}
	// Nothing this fleet does not account for was invented: the untracked list
	// comes from a sweep against a live provider, and none has run.
	if len(out.Untracked) != 0 {
		t.Errorf("untracked resources were reported with no sweep behind them: %+v", out.Untracked)
	}
}

// Discovery fills the guided form. A driver that cannot list what its
// credential can see says so, so the form asks for identifiers instead of
// waiting for a menu that will never arrive.
func TestDiscoveryReturnsTheChoicesAFormCanOffer(t *testing.T) {
	h := newHarness(t)
	operator, _ := h.user("operator", store.RoleOperator)
	cookie := h.session(operator)
	prov := h.provider("proxmox-lab")

	resp := h.do(request{method: http.MethodGet, path: "/api/v1/providers/" + prov.ID + "/discovery", cookie: cookie})
	resp.mustStatus(t, http.StatusOK, "discovery")
	var out providerDiscoveryResponse
	resp.into(t, &out)
	if len(out.Nodes) == 0 {
		t.Fatalf("no nodes were offered: %s", truncate(resp.body))
	}
	// Every list is present even when it is empty, because a null in a
	// generated client is a different thing from "this provider has none".
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.body, &raw); err != nil {
		t.Fatalf("reading the discovery response: %v", err)
	}
	for _, key := range []string{"nodes", "storages", "bridges", "templates"} {
		if v, ok := raw[key]; !ok || string(v) == "null" {
			t.Errorf("%s is missing or null", key)
		}
	}
}

// lastMachine is the most recently created machine for a provider, which is the
// one a test just made.
func (h *harness) lastMachine(providerID string) *store.Machine {
	h.t.Helper()
	machines, err := h.st.ListMachinesForProvider(h.ctx, providerID)
	if err != nil {
		h.t.Fatalf("ListMachinesForProvider: %v", err)
	}
	if len(machines) == 0 {
		h.t.Fatal("no machines for this provider")
	}
	return machines[len(machines)-1]
}

// An address that would put the credential on the wire in the clear is refused
// where it can still be changed.
//
// The driver refuses it too, but only at the first call, and by then the row is
// saved: the check answers 500 with the driver's sentence in the log and
// nowhere else, the machine loop files the same error under a kind the problems
// drawer has no case for, and the machines sit in `planned` with nothing on any
// page saying why. Turning certificate verification off is an answer to a
// different question and must not read as a way round this one -- the
// credential on this row can create and destroy machines.
func TestAnEndpointThatWouldSendTheCredentialInTheClearIsRefused(t *testing.T) {
	// Allowed, so that the loopback half below asks only about plain HTTP;
	// the address guard has its own test.
	h := newHarness(t, allowPrivateEgress)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)

	draft := func(endpoint string, insecure bool) map[string]any {
		return map[string]any{
			"kind": "fake", "name": "draft", "endpoint": endpoint,
			"insecure_skip_verify": insecure,
			"settings":             map[string]string{"zone": "zone-a"},
		}
	}
	endpointError := func(t *testing.T, body map[string]any) string {
		t.Helper()
		resp := h.do(request{method: http.MethodPost, path: "/api/v1/providers/validate",
			cookie: cookie, body: body})
		resp.mustStatus(t, http.StatusOK, "validate a draft")
		var out validateProviderResponse
		resp.into(t, &out)
		for _, e := range out.Errors {
			if e.Field == "endpoint" {
				return e.Message
			}
		}
		return ""
	}

	for _, insecure := range []bool{false, true} {
		got := endpointError(t, draft("http://pve.example.com:8006", insecure))
		if got == "" {
			t.Fatalf("plain HTTP was accepted with insecure_skip_verify=%v", insecure)
		}
		if !strings.Contains(got, "in the clear") || !strings.Contains(got, "does not permit this") {
			t.Errorf("the refusal does not say why, or reads as overridable: %q", got)
		}
	}

	// Loopback carries nothing off the box, so it stays allowed.
	for _, endpoint := range []string{"http://localhost:8006", "http://127.0.0.1:8006", "http://[::1]:8006"} {
		if got := endpointError(t, draft(endpoint, false)); got != "" {
			t.Errorf("%s was refused: %s", endpoint, got)
		}
	}
	if got := endpointError(t, draft("https://pve.example.com:8006", false)); got != "" {
		t.Errorf("https was refused: %s", got)
	}
}

func privateAddress(t *testing.T) string {
	t.Helper()
	identity := tailcat.NewPrivateKey()
	identity.Public.RegionID = 1
	return string(identity.Public.Addr())
}

// The gateway's address is a lasting capability to open connections to a
// hypervisor's API, so it is handled the way the credential is: it goes in
// once, is never returned, and a blank box on an edit leaves it alone.
func TestAProvidersPrivateConnectionAddressIsSealedAndNeverReturned(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	address := privateAddress(t)

	created := h.do(request{method: http.MethodPost, path: "/api/v1/providers", cookie: cookie,
		body: map[string]any{"kind": "fake", "name": "home-lab", "endpoint": "https://pve.lan:8006",
			"credential": "zoomies@pve!ci=secret", "tailcat_address": address, "settings": map[string]string{"zone": "zone-a"}}})
	created.mustStatus(t, http.StatusCreated, "create a private provider")
	var made providerResponse
	created.into(t, &made)
	if made.Connection != "tailcat" {
		t.Fatalf("connection = %q, want tailcat", made.Connection)
	}
	for _, resp := range []*response{
		created,
		h.do(request{method: http.MethodGet, path: "/api/v1/providers", cookie: cookie}),
		h.do(request{method: http.MethodGet, path: "/api/v1/providers/" + made.ID, cookie: cookie}),
		h.do(request{method: http.MethodGet, path: "/api/v1/audit", cookie: cookie}),
	} {
		if strings.Contains(string(resp.body), address) {
			t.Fatalf("a response carries the private connection address: %s", truncate(resp.body))
		}
	}
	row, err := h.st.GetProvider(h.ctx, made.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.TailcatAddressEnc) == 0 || strings.Contains(string(row.TailcatAddressEnc), address) {
		t.Fatal("the address was not sealed into the row")
	}

	// A rename from a form whose address box is blank keeps the connection.
	h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + made.ID, cookie: cookie,
		body: map[string]any{"name": "home-lab-2", "tailcat_address": "", "connection": "tailcat"}}).mustStatus(t, http.StatusOK, "rename")
	after, _ := h.st.GetProvider(h.ctx, made.ID)
	if string(after.TailcatAddressEnc) != string(row.TailcatAddressEnc) {
		t.Fatal("an edit that did not give an address changed the sealed one")
	}
	// The dry run of that same edit is judged against the stored address.
	dry := h.do(request{method: http.MethodPost, path: "/api/v1/providers/validate?id=" + made.ID, cookie: cookie,
		body: map[string]any{"kind": "fake", "name": "home-lab-2", "endpoint": "https://pve.lan:8006", "connection": "tailcat", "settings": map[string]string{"zone": "zone-a"}}})
	dry.mustStatus(t, http.StatusOK, "validate")
	var verdict validateProviderResponse
	dry.into(t, &verdict)
	if !verdict.Valid {
		t.Fatalf("the dry run of an edit that keeps the connection was refused: %+v", verdict.Errors)
	}

	// Choosing a direct connection is the deliberate act that clears it.
	direct := h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + made.ID, cookie: cookie,
		body: map[string]any{"connection": "direct"}})
	direct.mustStatus(t, http.StatusOK, "switch to direct")
	var view providerResponse
	direct.into(t, &view)
	if view.Connection != "direct" {
		t.Fatalf("connection = %q after switching to direct", view.Connection)
	}
	cleared, _ := h.st.GetProvider(h.ctx, made.ID)
	if len(cleared.TailcatAddressEnc) != 0 {
		t.Fatal("switching to a direct connection left the address sealed on the row")
	}
}

func TestAPrivateProviderNeedsAWholeAddressAndPrivateConnectionsTurnedOn(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	base := map[string]any{"kind": "fake", "name": "home-lab", "endpoint": "https://pve.lan:8006", "credential": "x", "settings": map[string]string{"zone": "zone-a"}}
	with := func(extra map[string]any) map[string]any {
		body := map[string]any{}
		for k, v := range base {
			body[k] = v
		}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	for name, tc := range map[string]struct {
		body  map[string]any
		field string
	}{
		"no address":         {with(map[string]any{"connection": "tailcat"}), "tailcat_address"},
		"not an address":     {with(map[string]any{"tailcat_address": "tc-not-really-secret"}), "tailcat_address"},
		"address but direct": {with(map[string]any{"connection": "direct", "tailcat_address": privateAddress(t)}), "connection"},
		"neither":            {with(map[string]any{"connection": "carrier-pigeon"}), "connection"},
	} {
		resp := h.do(request{method: http.MethodPost, path: "/api/v1/providers", cookie: cookie, body: tc.body})
		resp.mustStatus(t, http.StatusUnprocessableEntity, name)
		if !strings.Contains(string(resp.body), `"field":"`+tc.field+`"`) {
			t.Errorf("%s: the refusal names the wrong field: %s", name, truncate(resp.body))
		}
		if strings.Contains(string(resp.body), "tc-not-really-secret") {
			t.Errorf("%s: the refusal quotes the address", name)
		}
	}

	off := newHarness(t, func(c *config.Config) { c.Server.TailcatEnabled = false })
	offAdmin, _ := off.user("admin", store.RoleAdmin)
	resp := off.do(request{method: http.MethodPost, path: "/api/v1/providers", cookie: off.session(offAdmin),
		body: with(map[string]any{"tailcat_address": privateAddress(t)})})
	resp.mustStatus(t, http.StatusUnprocessableEntity, "private connections off")
	if !strings.Contains(string(resp.body), "server.tailcat_enabled") {
		t.Fatalf("the refusal does not say what to turn on: %s", truncate(resp.body))
	}
}

// The wizard can ask what a credential sees before it saves anything, so the
// nodes and storages are a menu rather than identifiers somebody looks up.
// What it may not do is ask about a draft the create would refuse, or ask
// without a credential: both are the create's own field errors, and nothing
// is dialled.
func TestADraftCanBeAskedWhatItsCredentialSees(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)

	found := h.do(request{method: http.MethodPost, path: "/api/v1/providers/discover", cookie: cookie,
		body: map[string]any{"kind": "fake", "name": "draft", "endpoint": "https://pve.example.com:8006",
			"credential": "zoomies@pve!ci=secret", "settings": map[string]string{"zone": "zone-a"}}})
	found.mustStatus(t, http.StatusOK, "discover for a draft")
	var discovery providerDiscoveryResponse
	found.into(t, &discovery)
	if len(discovery.Nodes) == 0 {
		t.Error("the fake sees nodes, and the draft was told about none")
	}
	if discovery.Unavailable != "" {
		t.Errorf("a provider that answered is reported unavailable: %q", discovery.Unavailable)
	}

	// The connect step is all a wizard has when it asks: no name yet, none of
	// the placement settings the menu is for. That is enough.
	bare := h.do(request{method: http.MethodPost, path: "/api/v1/providers/discover", cookie: cookie,
		body: map[string]any{"kind": "fake", "endpoint": "https://pve.example.com:8006", "credential": "tok"}})
	bare.mustStatus(t, http.StatusOK, "discover with only the connect step filled in")

	noToken := h.do(request{method: http.MethodPost, path: "/api/v1/providers/discover", cookie: cookie,
		body: map[string]any{"kind": "fake", "name": "draft", "endpoint": "https://pve.example.com:8006",
			"settings": map[string]string{"zone": "zone-a"}}})
	noToken.mustStatus(t, http.StatusUnprocessableEntity, "discover with no credential")
	if !strings.Contains(string(noToken.body), `"credential"`) {
		t.Errorf("the refusal does not name the credential field: %s", string(noToken.body))
	}

	// Nothing was created along the way: discovery is a question, not a save.
	listed := h.do(request{method: http.MethodGet, path: "/api/v1/providers", cookie: cookie})
	if strings.Contains(string(listed.body), `"draft"`) {
		t.Errorf("asking about a draft saved it: %s", string(listed.body))
	}
}

// The kinds route carries what to prepare and where each answer is found, in
// the shape the spec promises: a guide that is a list even when a driver has
// nothing to say, because the wizard iterates it.
func TestProviderKindsCarryAGuideAndAnEndpointExample(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	kinds := h.do(request{method: http.MethodGet, path: "/api/v1/providers/kinds", cookie: cookie})
	kinds.mustStatus(t, http.StatusOK, "list provider kinds")
	if !strings.Contains(string(kinds.body), `"guide":[`) {
		t.Errorf("the guide is missing or null rather than a list: %s", string(kinds.body))
	}
}

// A provider's credential can create and destroy machines, so an endpoint on
// this machine or its network is refused unless the platform allows it.
func TestADirectProviderOnAPrivateAddressIsRefusedUntilThePlatformAllowsIt(t *testing.T) {
	for _, tc := range []struct {
		name     string
		endpoint string
		allow    bool
		refused  bool
	}{
		{"a LAN hypervisor by default", "https://10.0.0.5:8006", false, true},
		{"the metadata address by default", "https://169.254.169.254", false, true},
		{"a LAN hypervisor once allowed", "https://10.0.0.5:8006", true, false},
		{"a public hypervisor", "https://pve.example.com:8006", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, func(c *config.Config) { c.Security.AllowPrivateEgress = tc.allow })
			admin, _ := h.user("admin", store.RoleAdmin)
			cookie := h.session(admin)
			resp := h.do(request{method: http.MethodPost, path: "/api/v1/providers/validate", cookie: cookie, body: map[string]any{
				"kind": "fake", "name": "draft", "endpoint": tc.endpoint,
				"settings": map[string]string{"zone": "zone-a"},
			}})
			resp.mustStatus(t, http.StatusOK, "validate a draft")
			var out validateProviderResponse
			resp.into(t, &out)
			var got string
			for _, e := range out.Errors {
				if e.Field == "endpoint" {
					got = e.Message
				}
			}
			switch {
			case tc.refused && !strings.Contains(got, "security.allow_private_egress"):
				t.Errorf("%s was not refused by a message naming the setting: %q", tc.endpoint, got)
			case !tc.refused && got != "":
				t.Errorf("%s was refused: %s", tc.endpoint, got)
			}
		})
	}
}
