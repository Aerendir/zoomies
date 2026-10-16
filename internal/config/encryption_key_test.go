package config

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eyupio/zoomies/internal/cryptox"
)

func newKey(t *testing.T) *cryptox.Key {
	t.Helper()
	key, err := cryptox.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// The text form of a key, as ZOOMIES_ENCRYPTION_KEY carries it.
func keyText(t *testing.T, key *cryptox.Key) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "k")
	if err := cryptox.WriteKeyFile(path, key); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func keyFile(t *testing.T, key *cryptox.Key) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "encryption.key")
	if err := cryptox.WriteKeyFile(path, key); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTheKeyItselfBeatsTheKeyFileAndSpaceAroundEitherIsIgnored(t *testing.T) {
	inline, other := newKey(t), newKey(t)
	c := Default()
	c.Security.EncryptionKey = "  " + keyText(t, inline) + "\n"
	c.Security.EncryptionKeyFile = "  " + keyFile(t, other) + "  "
	origin, path := c.EncryptionKeyOrigin()
	if origin != KeyFromValue || path != "" {
		t.Fatalf("origin = %v, %q; the key itself should win and have no path", origin, path)
	}
	got, err := c.ResolveEncryptionKey()
	if err != nil || got.Fingerprint() != inline.Fingerprint() {
		t.Fatalf("resolved %v, %v; want the inline key %s, not the file's %s", got, err, inline.Fingerprint(), other.Fingerprint())
	}
}

func TestTheKeyFileIsUsedWhenThereIsNoKeyItself(t *testing.T) {
	file := newKey(t)
	want := keyFile(t, file)
	c := Default()
	c.Security.EncryptionKey = "   "
	c.Security.EncryptionKeyFile = " " + want + " "
	origin, path := c.EncryptionKeyOrigin()
	if origin != KeyFromFile || path != want {
		t.Fatalf("origin = %v, %q; want the file %q, with its path trimmed", origin, path, want)
	}
	got, err := c.ResolveEncryptionKey()
	if err != nil || got.Fingerprint() != file.Fingerprint() {
		t.Fatalf("resolved %v, %v; want the file's key %s", got, err, file.Fingerprint())
	}
}

func TestNoKeyConfiguredIsErrNoKey(t *testing.T) {
	c := Default()
	c.Security.EncryptionKey, c.Security.EncryptionKeyFile = "", " "
	if origin, _ := c.EncryptionKeyOrigin(); origin != KeyNone {
		t.Errorf("origin = %v, want none", origin)
	}
	if _, err := c.ResolveEncryptionKey(); !errors.Is(err, cryptox.ErrNoKey) {
		t.Errorf("ResolveEncryptionKey with nothing configured = %v, want ErrNoKey", err)
	}
}

// The failure that matters. A key that is set and cannot be read is an error:
// falling back to the file would seal new secrets with a key the operator did not
// choose, because of a typo they would not see.
func TestAKeyThatIsSetButUnusableNeverFallsBackToTheFile(t *testing.T) {
	c := Default()
	c.Security.EncryptionKey = base64.StdEncoding.EncodeToString([]byte("far too short"))
	c.Security.EncryptionKeyFile = keyFile(t, newKey(t))
	got, err := c.ResolveEncryptionKey()
	if err == nil || got != nil {
		t.Fatalf("an unusable key resolved to %v, %v; it must be an error, and not the file's key", got, err)
	}
}

// A missing file is how the controller tells a first run from a restore that
// left the key behind, so the error has to keep saying so.
func TestAMissingKeyFileIsNotExist(t *testing.T) {
	c := Default()
	c.Security.EncryptionKeyFile = filepath.Join(t.TempDir(), "absent.key")
	if _, err := c.ResolveEncryptionKey(); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing key file = %v, want an error wrapping os.ErrNotExist", err)
	}
}
