package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/releasekey"
)

// The installer and the binary must trust the same release key: the installer
// holds a first download to it, the binary holds an upgrade to it, and a release
// signed by a key only one of them carries is refused by the other.
func TestTheInstallerCarriesTheReleaseKeyTheBinaryDoes(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^RELEASE_KEY="([^"]*)"$`).FindStringSubmatch(string(body))
	if m == nil {
		t.Fatal("install.sh has no RELEASE_KEY line")
	}
	got := strings.TrimSpace(m[1])
	if got != strings.TrimSpace(releasekey.Default) {
		t.Fatalf("install.sh carries release key %q and the binary carries %q; set both to the same key, or neither", got, releasekey.Default)
	}
	if got != "" {
		if _, err := releasekey.ParsePublic(got); err != nil {
			t.Fatalf("install.sh carries a release key that is not an Ed25519 public key: %v", err)
		}
	}
}
