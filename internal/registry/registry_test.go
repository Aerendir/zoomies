package registry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSplitReadsEachShapeOfReference(t *testing.T) {
	for _, tc := range []struct{ ref, host, repo, tag string }{
		// Docker Hub's official images have no host and an implied library/.
		{"ubuntu:24.04", "docker.io", "library/ubuntu", "24.04"},
		{"debian:13-slim", "docker.io", "library/debian", "13-slim"},
		// A user or organisation repository on Docker Hub keeps its own path.
		{"rockylinux/rockylinux:9", "docker.io", "rockylinux/rockylinux", "9"},
		{"ghcr.io/eyupio/zoomies:v1.3.0", "ghcr.io", "eyupio/zoomies", "v1.3.0"},
		// A digest already on the reference is not part of the question.
		{"ubuntu:24.04@sha256:" + strings.Repeat("a", 64), "docker.io", "library/ubuntu", "24.04"},
	} {
		host, repo, tag, err := Split(tc.ref)
		if err != nil || host != tc.host || repo != tc.repo || tag != tc.tag {
			t.Errorf("Split(%q) = %q, %q, %q, %v; want %q, %q, %q", tc.ref, host, repo, tag, err, tc.host, tc.repo, tc.tag)
		}
	}
	if _, _, _, err := Split("ubuntu"); err == nil {
		t.Error("a reference with no tag was accepted, and a registry would answer it with a 404 that looks like a missing image")
	}
}

// fakeRegistry answers the two requests Resolve makes: a token, then a manifest
// HEAD that must carry that token and every manifest type.
func fakeRegistry(t *testing.T, digest string, manifestStatus int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("scope"); got != "repository:library/ubuntu:pull" {
			t.Errorf("the token was asked for scope %q", got)
		}
		_, _ = w.Write([]byte(`{"token":"t0k3n"}`))
	})
	mux.HandleFunc("/v2/library/ubuntu/manifests/24.04", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("the manifest was fetched with %s; a HEAD is all that is needed and pulls nothing", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer t0k3n" {
			t.Errorf("the manifest request carried %q, not the token", got)
		}
		accept := r.Header.Get("Accept")
		for _, want := range []string{"application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json"} {
			if !strings.Contains(accept, want) {
				t.Errorf("Accept lacks %s, so a multi-architecture image would 404", want)
			}
		}
		if digest != "" {
			w.Header().Set("Docker-Content-Digest", digest)
		}
		w.WriteHeader(manifestStatus)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestResolveReturnsTheDigestTheRegistryNames(t *testing.T) {
	want := "sha256:" + strings.Repeat("c", 64)
	srv := fakeRegistry(t, want, http.StatusOK)
	got, err := Resolver{BaseURL: srv.URL}.Resolve("ubuntu:24.04@sha256:" + strings.Repeat("0", 64))
	if err != nil || got != want {
		t.Errorf("Resolve = %q, %v; want %q", got, err, want)
	}
}

func TestResolveRefusesAnAnswerItCannotPin(t *testing.T) {
	for name, tc := range map[string]struct {
		digest string
		status int
	}{
		"an unknown tag":           {"", http.StatusNotFound},
		"no digest header":         {"", http.StatusOK},
		"a digest of another kind": {"md5:abc", http.StatusOK},
	} {
		srv := fakeRegistry(t, tc.digest, tc.status)
		if got, err := (Resolver{BaseURL: srv.URL}).Resolve("ubuntu:24.04"); err == nil {
			t.Errorf("%s: Resolve returned %q and no error, so a bad answer would be written into a pin", name, got)
		}
	}
}

func TestResolveSaysWhenTheRegistryIssuesNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	if _, err := (Resolver{BaseURL: srv.URL}).Resolve("ubuntu:24.04"); err == nil || !strings.Contains(err.Error(), "token") {
		t.Errorf("Resolve with no token gave %v; it should name the token", err)
	}
}
