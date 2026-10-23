package api

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/store"
)

// signingIssuer is an identity provider that publishes a key and can sign a
// token with it, which is what a logout token has to be checked against.
type signingIssuer struct {
	srv *httptest.Server
	key *rsa.PrivateKey
}

func newSigningIssuer(t *testing.T) *signingIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	si := &signingIssuer{key: key}
	mux := http.NewServeMux()
	si.srv = httptest.NewServer(mux)
	t.Cleanup(si.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": si.srv.URL, "authorization_endpoint": si.srv.URL + "/authorize",
			"token_endpoint": si.srv.URL + "/token", "jwks_uri": si.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	return si
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (si *signingIssuer) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": "k1"})
	body, _ := json.Marshal(claims)
	signing := b64(head) + "." + b64(body)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, si.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + b64(sig)
}

// logoutClaims is a valid logout token for subject, which a test then spoils.
func (si *signingIssuer) logoutClaims(subject, jti string) map[string]any {
	now := time.Now()
	return map[string]any{
		"iss": si.srv.URL, "aud": "zoomies", "sub": subject, "jti": jti,
		"iat": now.Unix(), "exp": now.Add(2 * time.Minute).Unix(),
		"events": map[string]any{backchannelLogoutEventForTests: map[string]any{}},
	}
}

const backchannelLogoutEventForTests = "http://schemas.openid.net/event/backchannel-logout"

func backchannelHarness(t *testing.T) (*harness, *signingIssuer) {
	t.Helper()
	si := newSigningIssuer(t)
	h := newHarness(t, func(c *config.Config) {
		c.OIDC = config.OIDC{Enabled: true, Issuer: si.srv.URL, ClientID: "zoomies", ClientSecret: "secret"}
	})
	if h.api.oidcFailure() != nil {
		t.Fatalf("single sign-on did not set up: %v", h.api.oidcFailure())
	}
	return h, si
}

func (h *harness) backchannel(token string) *response {
	h.t.Helper()
	return h.do(request{method: http.MethodPost, path: "/api/v1/auth/oidc/backchannel-logout",
		rawBody: url.Values{"logout_token": {token}}.Encode(),
		headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}})
}

func (h *harness) linkedUser(name, subject string) (*store.User, []string) {
	h.t.Helper()
	u := &store.User{Username: name, Role: store.RoleOperator, OIDCSubject: subject}
	if err := h.st.CreateUser(h.ctx, u); err != nil {
		h.t.Fatal(err)
	}
	return u, []string{h.session(u), h.session(u)}
}

func (h *harness) signedIn(cookie string) bool {
	h.t.Helper()
	return h.do(request{method: http.MethodGet, path: "/api/v1/auth/session", cookie: cookie}).status == http.StatusOK
}

// The portal says a person has signed out there; every session that person has
// here ends, and nobody else's does.
func TestABackChannelLogoutEndsThatPersonsSessionsAndNobodyElses(t *testing.T) {
	h, si := backchannelHarness(t)
	_, alice := h.linkedUser("alice", "sub-alice")
	_, bob := h.linkedUser("bob", "sub-bob")

	h.backchannel(si.sign(t, si.logoutClaims("sub-alice", "jti-1"))).mustStatus(t, http.StatusOK, "logout token")

	for _, c := range alice {
		if h.signedIn(c) {
			t.Error("one of alice's sessions survived her logout")
		}
	}
	for _, c := range bob {
		if !h.signedIn(c) {
			t.Error("bob was signed out by alice's logout")
		}
	}
}

func TestALogoutTokenCannotBePlayedTwice(t *testing.T) {
	h, si := backchannelHarness(t)
	h.linkedUser("alice", "sub-alice")
	tok := si.sign(t, si.logoutClaims("sub-alice", "jti-1"))
	h.backchannel(tok).mustStatus(t, http.StatusOK, "first use")
	h.backchannel(tok).mustStatus(t, http.StatusBadRequest, "replay")
}

func TestALogoutTokenThatIsNotOneIsRefusedAndEndsNothing(t *testing.T) {
	h, si := backchannelHarness(t)
	_, alice := h.linkedUser("alice", "sub-alice")

	spoil := map[string]func(map[string]any){
		"an ID token with a nonce":    func(c map[string]any) { c["nonce"] = "n" },
		"no logout event":             func(c map[string]any) { c["events"] = map[string]any{} },
		"no subject":                  func(c map[string]any) { delete(c, "sub") },
		"no identifier":               func(c map[string]any) { delete(c, "jti") },
		"another audience":            func(c map[string]any) { c["aud"] = "another-client" },
		"another issuer":              func(c map[string]any) { c["iss"] = "https://elsewhere.example" },
		"an old issue time":           func(c map[string]any) { c["iat"] = time.Now().Add(-time.Hour).Unix() },
		"expired":                     func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() },
		"an issue time in the future": func(c map[string]any) { c["iat"] = time.Now().Add(time.Hour).Unix() },
	}
	for name, spoilIt := range spoil {
		c := si.logoutClaims("sub-alice", "jti-"+strings.ReplaceAll(name, " ", "-"))
		spoilIt(c)
		h.backchannel(si.sign(t, c)).mustStatus(t, http.StatusBadRequest, name)
	}
	// Signed by a key the provider does not publish.
	other := newSigningIssuer(t)
	forged := other.sign(t, si.logoutClaims("sub-alice", "jti-forged"))
	h.backchannel(forged).mustStatus(t, http.StatusBadRequest, "a token signed by another key")
	h.backchannel("").mustStatus(t, http.StatusBadRequest, "no token")

	for _, c := range alice {
		if !h.signedIn(c) {
			t.Fatal("a refused logout token ended a session")
		}
	}
}

// A person with no account here is answered the same way as one with an
// account, so a caller with a valid token cannot ask who has one.
func TestALogoutForSomebodyWithNoAccountHereIsAnsweredOK(t *testing.T) {
	h, si := backchannelHarness(t)
	h.backchannel(si.sign(t, si.logoutClaims("sub-nobody", "jti-1"))).mustStatus(t, http.StatusOK, "unknown subject")
}

func TestBackChannelLogoutIsOffWithoutSingleSignOn(t *testing.T) {
	h := newHarness(t)
	h.backchannel("anything").mustStatus(t, http.StatusServiceUnavailable, "no single sign-on")
}

// A person signed in from two browsers has two sessions at the provider. A
// logout token that names one of them ends what that one started here, and the
// other browser stays signed in, which ending "everything this person has"
// would not do.
func TestABackChannelLogoutNamingASessionEndsOnlyTheSessionsMadeFromIt(t *testing.T) {
	h, si := backchannelHarness(t)
	u, _ := h.linkedUser("alice", "sub-alice")
	from := func(sid string) string {
		tok, err := h.ctrl.Auth().NewSSOSession(h.ctx, u, "127.0.0.1", "test", sid)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	desk, phone, password := from("sid-desk"), from("sid-phone"), h.session(u)

	claims := si.logoutClaims("sub-alice", "jti-1")
	claims["sid"] = "sid-desk"
	h.backchannel(si.sign(t, claims)).mustStatus(t, http.StatusOK, "logout of the desk session")

	if h.signedIn(desk) {
		t.Error("the session made from the named provider session survived")
	}
	if !h.signedIn(phone) || !h.signedIn(password) {
		t.Error("a session made from another provider session, or by a password, was ended")
	}
}
