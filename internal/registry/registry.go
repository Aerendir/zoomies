// Package registry asks a container registry what a tag points at, without
// pulling a layer. It exists for the generators that pin images by digest: the
// marketplace lock and the runner images' base-image refresh both need the one
// value `docker pull` would pin, and neither should carry its own copy of how
// Docker Hub and GHCR hand it out.
package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ManifestTypes is every media type a registry may answer a manifest request
// with. A multi-architecture image is an index and a single-architecture one is
// a manifest, and both spellings of each are still in the wild; asking for only
// one of them gets a 404 that looks like a missing tag.
const ManifestTypes = "application/vnd.oci.image.index.v1+json, " +
	"application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.docker.distribution.manifest.v2+json"

// Resolver resolves image references. The zero value talks to the real
// registries with a client that gives up after thirty seconds.
type Resolver struct {
	// Client is used for every request; nil means a thirty-second default.
	Client *http.Client
	// BaseURL, when set, stands in for the registry and its token service, so a
	// test can answer on a local server. The reference's own host is still read,
	// to choose how the repository path is spelled.
	BaseURL string
}

func (r Resolver) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Resolve asks the registry what a tag points at, and returns it as
// "sha256:...". GHCR wants a token even for a public repository, and Docker Hub
// does too; both hand one out for the asking. The digest is the header the
// registry answers a manifest request with, which is exactly the value
// `docker pull` would pin.
func (r Resolver) Resolve(ref string) (string, error) {
	host, repo, tag, err := Split(ref)
	if err != nil {
		return "", err
	}
	token, err := r.pullToken(host, repo)
	if err != nil {
		return "", err
	}
	base := "https://" + apiHost(host)
	if r.BaseURL != "" {
		base = strings.TrimRight(r.BaseURL, "/")
	}
	req, err := http.NewRequest(http.MethodHead, base+"/v2/"+repo+"/manifests/"+tag, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", ManifestTypes)
	resp, err := r.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the registry answered %s", resp.Status)
	}
	d := resp.Header.Get("Docker-Content-Digest")
	if !strings.HasPrefix(d, "sha256:") {
		return "", fmt.Errorf("the registry returned no digest")
	}
	return d, nil
}

// Split breaks a reference into the registry it lives on, the repository path
// that registry knows it by, and the tag. A digest on the reference is ignored:
// the question is what the tag points at today. Docker Hub is the awkward one:
// its references carry no host at all, official images live under an implied
// library/ prefix, and its API is on a different name from its references.
func Split(ref string) (host, repo, tag string, err error) {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	rest := ref
	host = "docker.io"
	if i := strings.Index(ref, "/"); i >= 0 && strings.Contains(ref[:i], ".") {
		host, rest = ref[:i], ref[i+1:]
	}
	repo, tag, ok := strings.Cut(rest, ":")
	if !ok {
		return "", "", "", fmt.Errorf("%q names no tag", ref)
	}
	if host == "docker.io" && !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	return host, repo, tag, nil
}

// apiHost is where the API lives, which is not always where references say the
// image does.
func apiHost(host string) string {
	if host == "docker.io" {
		return "registry-1.docker.io"
	}
	return host
}

// pullToken asks for anonymous pull access. Both registries hand one out for
// the asking on a public repository; neither serves a manifest without it.
func (r Resolver) pullToken(host, repo string) (string, error) {
	authURL := "https://" + host + "/token?scope=repository:" + repo + ":pull&service=" + host
	if host == "docker.io" {
		authURL = "https://auth.docker.io/token?scope=repository:" + repo + ":pull&service=registry.docker.io"
	}
	if r.BaseURL != "" {
		authURL = strings.TrimRight(r.BaseURL, "/") + "/token?scope=repository:" + repo + ":pull&service=" + host
	}
	resp, err := r.client().Get(authURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("reading the registry's token: %w", err)
	}
	if out.Token == "" {
		return "", fmt.Errorf("the registry issued no token for %s", repo)
	}
	return out.Token, nil
}
