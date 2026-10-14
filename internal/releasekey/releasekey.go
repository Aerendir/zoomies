// Package releasekey is the release signing key: Ed25519, from the standard
// library, so a release can be held to a signature with no external tool and
// no network beyond the release itself.
//
// It is what lets `zoomies upgrade` and an installer refuse a download that is
// not the project's. The checksums file says a download was not corrupted on
// the way; it travels with the binary, so on its own it says nothing about who
// made either. A detached signature over the checksums, checked against a key
// compiled into the running binary, says the release is the project's.
package releasekey

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// SigSuffix is the asset that carries the detached signature over a release's
// checksums.txt, as the release workflow names it.
const SigSuffix = ".ed25519"

// PublicKey is an Ed25519 public key with the id a signature names it by.
type PublicKey struct {
	ID  string
	key ed25519.PublicKey
}

// String is the key in the base64 form the key file holds.
func (k PublicKey) String() string { return base64.StdEncoding.EncodeToString(k.key) }

// ParsePublic reads a base64 Ed25519 public key and derives its id.
func ParsePublic(s string) (PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return PublicKey{}, fmt.Errorf("the release key is not base64: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return PublicKey{}, fmt.Errorf("the release key is %d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
	key := ed25519.PublicKey(raw)
	return PublicKey{ID: keyID(key), key: key}, nil
}

// ParsePrivate reads a base64 Ed25519 private key: the 32-byte seed or the
// 64-byte key, so a key written by any of the usual tools is accepted.
func ParsePrivate(s string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("the private key is not base64: %w", err)
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	}
	return nil, fmt.Errorf("the private key is %d bytes, want %d or %d", len(raw), ed25519.SeedSize, ed25519.PrivateKeySize)
}

// Generate mints a key pair, each in the base64 form the key files hold. The
// private half is a credential: it is stored as a release secret and never
// committed.
func Generate() (public, private string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv), nil
}

// Sign returns the envelope written beside a release's checksums.txt: the
// signing key's id and the detached signature over msg, so a verifier holding
// several keys knows which one to trust.
func Sign(private string, msg []byte) ([]byte, error) {
	priv, err := ParsePrivate(private)
	if err != nil {
		return nil, err
	}
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("the private key has no public half")
	}
	return Encode(keyID(pub), ed25519.Sign(priv, msg)), nil
}

// Encode is the envelope's text: the key id, then the base64 signature.
func Encode(id string, sig []byte) []byte {
	return []byte("key:" + id + "\n" + base64.StdEncoding.EncodeToString(sig) + "\n")
}

func decodeEnvelope(body []byte) (id string, sig []byte, err error) {
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "key:") {
		return "", nil, errors.New("the signature is not a release signature")
	}
	id = strings.TrimSpace(strings.TrimPrefix(lines[0], "key:"))
	if id == "" {
		return "", nil, errors.New("the signature names no key")
	}
	sig, err = base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil {
		return "", nil, fmt.Errorf("the signature is not base64: %w", err)
	}
	return id, sig, nil
}

// Verify holds a detached signature over msg to the keys in set, one base64
// public key per line. It refuses a key id the set does not hold, which is a
// release signed by a key this build does not trust, and a signature that does
// not verify.
func Verify(set string, msg, envelope []byte) error {
	id, sig, err := decodeEnvelope(envelope)
	if err != nil {
		return err
	}
	keys, err := parseSet(set)
	if err != nil {
		return err
	}
	key, ok := keys[id]
	if !ok {
		return fmt.Errorf("the release is signed with key %s, which this build does not trust; it may be signed by a newer key, so upgrade over a release this build accepts", id)
	}
	if !ed25519.Verify(key, msg, sig) {
		return fmt.Errorf("the signature does not match the checksums under key %s", id)
	}
	return nil
}

// IDs returns the ids of the keys in a set, one base64 public key per line, in
// a stable order. An empty set is no keys, which means downloads are not held
// to a signature. It is what a page shows to say which keys this build trusts,
// and it never returns the keys themselves.
func IDs(set string) []string {
	keys, err := parseSet(set)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(keys))
	for id := range keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// keyID names a public key by the first eight bytes of its SHA-256, short
// enough for a signature file and long enough not to be guessed.
func keyID(key ed25519.PublicKey) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:8])
}

func parseSet(set string) (map[string]ed25519.PublicKey, error) {
	out := map[string]ed25519.PublicKey{}
	for _, line := range strings.Split(set, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pub, err := ParsePublic(line)
		if err != nil {
			return nil, err
		}
		out[pub.ID] = pub.key
	}
	return out, nil
}
