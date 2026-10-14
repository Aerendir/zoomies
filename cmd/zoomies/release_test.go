package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The three subcommands are one mechanism, so minting, signing and verifying in
// turn is the one test that proves the mechanism works: what the running binary
// checks a download against is what the pipeline signs with.
func TestReleaseKeygenSignAndVerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	runCLI(t, "release", "keygen", "--out", dir)

	sums := filepath.Join(t.TempDir(), "checksums.txt")
	if err := os.WriteFile(sums, []byte("abc123  zoomies_linux_amd64\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envelope, _ := runCLI(t, "release", "sign", "--key-file", filepath.Join(dir, "release.key"), sums)
	sig := filepath.Join(filepath.Dir(sums), "checksums.txt.ed25519")
	if err := os.WriteFile(sig, []byte(envelope), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := runCLI(t, "release", "verify", "--key-file", filepath.Join(dir, "release.pub"), sums, sig)
	if !strings.Contains(out, "valid") {
		t.Fatalf("verify said %q", out)
	}
}

// A checksums file changed after it was signed must not verify: it is the
// binary that is held to the checksums, so a changed checksums file is a
// changed binary.
func TestReleaseVerifyRefusesATamperedChecksumsFile(t *testing.T) {
	dir := t.TempDir()
	runCLI(t, "release", "keygen", "--out", dir)

	sums := filepath.Join(t.TempDir(), "checksums.txt")
	if err := os.WriteFile(sums, []byte("abc123  zoomies_linux_amd64\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envelope, _ := runCLI(t, "release", "sign", "--key-file", filepath.Join(dir, "release.key"), sums)
	sig := filepath.Join(filepath.Dir(sums), "checksums.txt.ed25519")
	if err := os.WriteFile(sig, []byte(envelope), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sums, []byte("def456  zoomies_linux_amd64\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runCLIFailing(t, "release", "verify", "--key-file", filepath.Join(dir, "release.pub"), sums, sig); code == exitOK {
		t.Fatal("a checksums file changed after signing verified")
	}
}

// keygen must not print the private half: it goes to a file the operator stores
// as a secret, never to a terminal that may be logged.
func TestReleaseKeygenWritesBothHalvesAndPrintsOnlyThePublicPath(t *testing.T) {
	dir := t.TempDir()
	out, _ := runCLI(t, "release", "keygen", "--out", dir)
	key, err := os.ReadFile(filepath.Join(dir, "release.key"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, strings.TrimSpace(string(key))) {
		t.Fatal("the private key was printed")
	}
	if _, err := os.Stat(filepath.Join(dir, "release.pub")); err != nil {
		t.Fatalf("the public key was not written: %v", err)
	}
}
