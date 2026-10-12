package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

const pastedKey = "-----BEGIN PRIVATE KEY-----\nbm90IGEgcmVhbCBrZXk=\n-----END PRIVATE KEY-----"

// The certificate this controller trusts for the hypervisor and the one a
// machine trusts for this controller are different certificates: a cluster's own
// authority has no reason to have signed the controller. Both are returned, both
// are kept by an edit that does not name them, and each is cleared only by
// being emptied itself.
func TestAProviderKeepsTheCertificateItsMachinesTrustApartFromTheOneItTrusts(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	clusters := strings.TrimSpace(setupCertificate(t))
	controllers := strings.TrimSpace(setupCertificate(t))

	created := h.do(request{method: http.MethodPost, path: "/api/v1/providers", cookie: cookie,
		body: map[string]any{"kind": "fake", "name": "proxmox-lab", "endpoint": "https://pve.example.com:8006",
			"credential": "zoomies@pve!ci=secret", "ca_pem": clusters, "machine_ca_pem": controllers,
			"settings": map[string]string{"zone": "zone-a"}}})
	created.mustStatus(t, http.StatusCreated, "create provider")
	var made providerResponse
	created.into(t, &made)
	if made.CAPEM != clusters || made.MachineCAPEM != controllers {
		t.Fatalf("the created provider answers ca_pem %q and machine_ca_pem %q", made.CAPEM, made.MachineCAPEM)
	}

	h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + made.ID, cookie: cookie,
		body: map[string]any{"name": "proxmox-lab-2"}}).mustStatus(t, http.StatusOK, "rename the provider")
	row, err := h.st.GetProvider(h.ctx, made.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if row.CAPEM != clusters || row.MachineCAPEM != controllers {
		t.Errorf("a rename changed the certificates to %q and %q", row.CAPEM, row.MachineCAPEM)
	}

	h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + made.ID, cookie: cookie,
		body: map[string]any{"machine_ca_pem": ""}}).mustStatus(t, http.StatusOK, "clear the machines' certificate")
	row, err = h.st.GetProvider(h.ctx, made.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if row.MachineCAPEM != "" {
		t.Errorf("an empty machine certificate left %q stored", row.MachineCAPEM)
	}
	if row.CAPEM != clusters {
		t.Errorf("clearing the machines' certificate changed the one for the hypervisor to %q", row.CAPEM)
	}
}

// Text that is not a certificate was accepted and stored, and found out later:
// a key in the box is copied into every machine, and a note in it fails on the
// first call to the hypervisor, with the reason in a log. It is refused on save
// instead, naming the box it is in, because two boxes now hold certificates and
// "invalid certificate" would not say which.
func TestACertificateFieldHoldingSomethingElseIsRefusedBySayingWhichAndWhy(t *testing.T) {
	for _, field := range []string{"ca_pem", "machine_ca_pem"} {
		for name, tc := range map[string]struct {
			text string
			want string
		}{
			"a private key":                  {pastedKey, "private key"},
			"a certificate with a key after": {strings.TrimSpace(setupCertificate(t)) + "\n" + pastedKey, "private key"},
			"text that is no certificate":    {"this is not a certificate", "BEGIN CERTIFICATE"},
		} {
			t.Run(field+" with "+name, func(t *testing.T) {
				h := newHarness(t)
				admin, _ := h.user("admin", store.RoleAdmin)
				cookie := h.session(admin)

				resp := h.do(request{method: http.MethodPost, path: "/api/v1/providers", cookie: cookie,
					body: map[string]any{"kind": "fake", "name": "proxmox-lab", "endpoint": "https://pve.example.com:8006",
						"credential": "zoomies@pve!ci=secret", field: tc.text, "settings": map[string]string{"zone": "zone-a"}}})
				resp.mustStatus(t, http.StatusUnprocessableEntity, "create a provider with "+name+" in "+field)
				body := string(resp.body)
				if !strings.Contains(body, `"field":"`+field+`"`) {
					t.Errorf("the refusal does not name %s: %s", field, truncate(resp.body))
				}
				if !strings.Contains(body, tc.want) {
					t.Errorf("the refusal does not say %q: %s", tc.want, truncate(resp.body))
				}
				if strings.Contains(body, "bm90IGEgcmVhbCBrZXk") {
					t.Errorf("the refusal repeats the key back: %s", truncate(resp.body))
				}
				if rows, err := h.st.ListProviders(h.ctx); err != nil || len(rows) != 0 {
					t.Errorf("a refused provider was stored anyway (%d rows, err %v)", len(rows), err)
				}
			})
		}
	}
}

// The check is of what a request names. A provider whose stored text is already
// not a certificate (it was accepted before this was checked) must still be
// renamable, paused and resumed: refusing every edit until the box is fixed
// would lock the operator out of the form that fixes it.
func TestRenamingAProviderIsNotRefusedForACertificateItAlreadyHolds(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	row, err := h.st.GetProvider(h.ctx, prov.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	row.CAPEM, row.MachineCAPEM = pastedKey, "this is not a certificate"
	if err := h.st.UpdateProvider(h.ctx, row); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}

	h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + prov.ID, cookie: cookie,
		body: map[string]any{"name": "proxmox-lab-2"}}).mustStatus(t, http.StatusOK, "rename a provider holding text that is no certificate")
}

// The audit trail is read by every signed-in viewer and redacts by field name,
// which says nothing about a box called machine_ca_pem. An upgrade copies
// whatever ca_pem held into it, so a key pasted there by mistake long ago is
// copied too, and clearing the new box (which the docs tell an operator with a
// public certificate to do) would write that key into the audit as the "before".
func TestClearingAMachineCertificateThatHoldsAKeyDoesNotAuditTheKey(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.user("admin", store.RoleAdmin)
	viewer, _ := h.user("viewer", store.RoleViewer)
	cookie := h.session(admin)
	prov := h.provider("proxmox-lab")
	row, err := h.st.GetProvider(h.ctx, prov.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	row.CAPEM, row.MachineCAPEM = pastedKey, pastedKey // what the upgrade copy leaves behind
	if err := h.st.UpdateProvider(h.ctx, row); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}

	h.do(request{method: http.MethodPatch, path: "/api/v1/providers/" + prov.ID, cookie: cookie,
		body: map[string]any{"machine_ca_pem": ""}}).mustStatus(t, http.StatusOK, "clear the machines' certificate")

	res := h.do(request{method: http.MethodGet, path: "/api/v1/audit?action=provider.update", cookie: h.session(viewer)})
	res.mustStatus(t, http.StatusOK, "read the audit as a viewer")
	var events page[store.AuditEvent]
	res.into(t, &events)
	recorded := false
	for _, e := range events.Items {
		for _, picture := range []string{e.Before, e.After} {
			if strings.Contains(picture, "PRIVATE KEY") || strings.Contains(picture, "bm90IGEgcmVhbCBrZXk") {
				t.Errorf("a viewer can read the pasted key in the audit: %s", picture)
			}
		}
		if strings.Contains(e.Before, "machine_ca_pem") {
			recorded = true
		}
	}
	if !recorded {
		t.Errorf("the change to machine_ca_pem is not in the audit at all: %s", truncate(res.body))
	}
}
