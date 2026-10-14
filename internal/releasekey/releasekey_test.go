package releasekey

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestASignatureVerifiesUnderTheKeyThatSignedIt(t *testing.T) {
	pub, priv, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("deadbeef00  zoomies_linux_amd64\n")
	envelope, err := Sign(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, msg, envelope); err != nil {
		t.Fatalf("a signature made with the key did not verify: %v", err)
	}
}

// The checksums are what the binary is held to, so a changed checksums file
// must not verify even with a real signature over the original.
func TestATamperedChecksumsFileDoesNotVerify(t *testing.T) {
	pub, priv, _ := Generate()
	envelope, _ := Sign(priv, []byte("good"))
	if err := Verify(pub, []byte("tampered"), envelope); err == nil {
		t.Fatal("a changed checksums file verified")
	}
}

func TestAKeyTheSetDoesNotHoldIsRefused(t *testing.T) {
	other, _, _ := Generate()
	_, priv, _ := Generate()
	envelope, _ := Sign(priv, []byte("msg"))
	err := Verify(other, []byte("msg"), envelope)
	if err == nil || !strings.Contains(err.Error(), "does not trust") {
		t.Fatalf("err = %v", err)
	}
}

// A key is rotated by carrying both for a release: a build with both accepts
// either, and a build with only the old one refuses the new, which is the
// order the rotation relies on.
func TestARotatedKeyVerifiesWhileBothAreCarried(t *testing.T) {
	oldPub, oldPriv, _ := Generate()
	newPub, newPriv, _ := Generate()
	set := oldPub + "\n# the new key\n" + newPub
	oldEnvelope, _ := Sign(oldPriv, []byte("m"))
	if err := Verify(set, []byte("m"), oldEnvelope); err != nil {
		t.Fatalf("the old key under a set with both: %v", err)
	}
	newEnvelope, _ := Sign(newPriv, []byte("m"))
	if err := Verify(set, []byte("m"), newEnvelope); err != nil {
		t.Fatalf("the new key under a set with both: %v", err)
	}
	if err := Verify(oldPub, []byte("m"), newEnvelope); err == nil {
		t.Fatal("a build carrying only the old key accepted the new key")
	}
}

func TestMalformedKeysAndSignaturesAreRefused(t *testing.T) {
	if _, err := ParsePublic("not base64!"); err == nil {
		t.Fatal("a non-base64 public key was accepted")
	}
	if _, err := ParsePublic(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("a public key of the wrong length was accepted")
	}
	if _, err := ParsePrivate("not base64!"); err == nil {
		t.Fatal("a non-base64 private key was accepted")
	}
	if _, err := Sign(base64.StdEncoding.EncodeToString([]byte("short")), []byte("m")); err == nil {
		t.Fatal("a private key of the wrong length was accepted")
	}
	if err := Verify("also not base64!", []byte("m"), Encode("id", []byte("sig"))); err == nil {
		t.Fatal("a malformed key set was accepted")
	}
	if err := Verify("", []byte("m"), []byte("not a signature")); err == nil {
		t.Fatal("a malformed envelope was accepted")
	}
	if err := Verify("", []byte("m"), Encode("", []byte("sig"))); err == nil {
		t.Fatal("an envelope naming no key was accepted")
	}
}

func TestTheKeyIDIsStable(t *testing.T) {
	pub, _, _ := Generate()
	parsed, err := ParsePublic(pub)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ID == "" || len(parsed.ID) != 16 {
		t.Fatalf("id = %q, want 16 hex characters", parsed.ID)
	}
	again, _ := ParsePublic(pub)
	if again.ID != parsed.ID {
		t.Fatalf("id changed between parses: %q then %q", parsed.ID, again.ID)
	}
}
