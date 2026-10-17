package docs

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/releasekey"
)

// connect-proxmox.sh runs a release binary as root on a Proxmox host, and
// deploy/instapods/fetch.sh runs one on a pod. Both used to check only a
// checksum fetched from the same place as the binary, which says the download
// was not corrupted on the way and nothing about who made it. They now hold the
// checksums to the release signing key, as install.sh and `zoomies upgrade` do.
//
// The two are standalone files, one of them piped from curl, so they cannot
// share code at run time. They share a block of text, and these tests hold the
// copies equal, the key equal to the binary's, and the block correct.

// The canonical copy, embedded into the full command the provider wizard shows,
// and the two standalone scripts that carry the same text.
var downloaderScripts = []string{"internal/proxmoxsetup/release_signature.sh", "connect-proxmox.sh", "deploy/instapods/fetch.sh"}

const (
	signatureBlockStart = "# --- release signature check:"
	signatureBlockEnd   = "# --- end of the release signature check.\n"
)

func scriptText(t *testing.T, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// signatureBlock is the shared text of one script, markers included.
func signatureBlock(t *testing.T, rel string) string {
	t.Helper()
	text := scriptText(t, rel)
	start := strings.Index(text, signatureBlockStart)
	end := strings.Index(text, signatureBlockEnd)
	if start < 0 || end < start {
		t.Fatalf("%s has no release signature check between its markers", rel)
	}
	return text[start : end+len(signatureBlockEnd)]
}

func TestTheDownloaderScriptsCarryTheReleaseKeyTheBinaryDoes(t *testing.T) {
	for _, rel := range downloaderScripts {
		m := regexp.MustCompile(`(?m)^RELEASE_KEY="([^"]*)"$`).FindStringSubmatch(scriptText(t, rel))
		if m == nil {
			t.Errorf("%s has no RELEASE_KEY line", rel)
			continue
		}
		if got := strings.TrimSpace(m[1]); got != strings.TrimSpace(releasekey.Default) {
			t.Errorf("%s carries release key %q and the binary carries %q; a release signed by a key only one of them holds is refused by the other", rel, got, releasekey.Default)
		}
	}
}

func TestTheDownloaderScriptsCarryTheSameSignatureCheck(t *testing.T) {
	first := signatureBlock(t, downloaderScripts[0])
	for _, rel := range downloaderScripts[1:] {
		if got := signatureBlock(t, rel); got != first {
			t.Errorf("%s and %s carry different release signature checks; edit one, copy it into the other, and the tests below say whether it is right",
				downloaderScripts[0], rel)
		}
	}
}

// runBlock runs script after the shared block, in sh, with the notices the real
// scripts print and a die that exits.
func runBlock(t *testing.T, script string, env ...string) (stdout, stderr string, exit int) {
	t.Helper()
	full := "set -eu\n" +
		"die() { printf 'zoomies: %s\\n' \"$*\" >&2; exit 1; }\n" +
		"say() { printf 'zoomies: %s\\n' \"$*\" >&2; }\n" +
		"warn() { printf 'zoomies: warning: %s\\n' \"$*\" >&2; }\n" +
		signatureBlock(t, "connect-proxmox.sh") + "\n" + script
	cmd := exec.Command("sh", "-c", full)
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		exit = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running sh: %v", err)
	}
	return out.String(), errb.String(), exit
}

// A signature has to be demanded of a release that has one to give and not of
// one that cannot. Only v1.3.6.1 and later are signed, so a rule that required
// it of everything would refuse every version published before signing began,
// and one that required it of nothing would let whoever replaces a binary delete
// its signature.
//
// Whoever publishes a release also picks its name, so a name this rule cannot
// read is held to a signature rather than excused from one: v2x.0.0 or a major
// of twenty digits would otherwise be a way to publish a binary with a matching
// checksums.txt and no signature, and have it run on a warning. The numbers are
// never compared unless they are short enough to compare, because the shell's
// [ answers one that overflows with an error status, which the callers read as
// "no". Only dev is exempt: it is built from main, has no release to sign, and
// is never held to a signature.
func TestAReleaseHasToBeSignedFromV136OnAndNotBefore(t *testing.T) {
	for tag, want := range map[string]string{
		"v1.2.0": "no", "v1.3.0": "no", "v1.3.5": "no", "v0.9.9": "no", "v1.0.0": "no", "v1.0.0-RC3": "no",
		"v1.3.6": "yes", "v1.3.6.1": "yes", "v1.3.7": "yes", "v1.3.10": "yes", "v1.4.0": "yes", "v2.0.0": "yes", "v1.3.6-rc1": "yes",
		"dev": "no",
		// Not a release tag at all, or not one this rule can read.
		"": "yes", "v1.3": "yes", "V1.3.7": "yes", "latest": "yes", "v1.x.7": "yes", "v1.3.x": "yes", "v2x.0.0": "yes",
		// Numbers longer than can be compared safely, in each of the three places.
		"v99999999999999999999.0.0": "yes", "v1.99999999999999999999.0": "yes", "v0.0.99999999999999999999": "yes", "v10000.0.0": "yes",
	} {
		// The status is printed rather than the branch taken, so a comparison
		// that errored (2) cannot pass for "not required" (1).
		out, _, _ := runBlock(t, `status=0; signature_required "$TAG" || status=$?; echo "$status"`, "TAG="+tag)
		got := strings.TrimSpace(out)
		switch {
		case got == "0" && want == "yes", got == "1" && want == "no":
		case got == "0" || got == "1":
			t.Errorf("signature_required(%q) exited %s, which says a signature is %s; want %s", tag, got, map[string]string{"0": "required", "1": "not required"}[got], want)
		default:
			t.Errorf("signature_required(%q) exited %q; it must answer 0 (required) or 1 (not required) and never fail", tag, got)
		}
	}
}

func needOpenSSLForEd25519(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl is not installed, so a signature cannot be checked here")
	}
	if out, _ := exec.Command("sh", "-c", "openssl pkeyutl -help 2>&1").Output(); !bytes.Contains(out, []byte("rawin")) {
		t.Skip("this openssl predates 3.0 and cannot verify an Ed25519 signature one-shot")
	}
}

type signedRelease struct{ dir, sums, sig, key string }

// newRelease writes a checksums.txt and the signature the release workflow would
// write beside it, made with a key pair of this test's own.
func newRelease(t *testing.T) signedRelease {
	t.Helper()
	pub, priv, err := releasekey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	sums := []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  zoomies_linux_amd64\n")
	envelope, err := releasekey.Sign(priv, sums)
	if err != nil {
		t.Fatal(err)
	}
	r := signedRelease{dir: dir, sums: filepath.Join(dir, "checksums.txt"), sig: filepath.Join(dir, "checksums.txt.ed25519"), key: pub}
	if err := os.WriteFile(r.sums, sums, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.sig, envelope, 0o600); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r signedRelease) verify(t *testing.T, tag string, env ...string) (stderr string, exit int) {
	t.Helper()
	scratch := t.TempDir()
	_, stderr, exit = runBlock(t, `RELEASE_KEY="$KEY"; verify_signature "$TAG" "$SUMS" "$SIG" "$SCRATCH"`,
		append([]string{"TAG=" + tag, "KEY=" + r.key, "SUMS=" + r.sums, "SIG=" + r.sig, "SCRATCH=" + scratch}, env...)...)
	return stderr, exit
}

func TestAValidSignatureIsAcceptedForAnyRelease(t *testing.T) {
	needOpenSSLForEd25519(t)
	r := newRelease(t)
	for _, tag := range []string{"v1.3.6.1", "v1.3.7", "v1.3.5", "dev"} {
		if stderr, exit := r.verify(t, tag); exit != 0 || !strings.Contains(stderr, "matches the release key") {
			t.Errorf("%s with a good signature: exit %d, stderr %q", tag, exit, stderr)
		}
	}
}

// Whatever the release, a signature that is there and wrong refuses. An old
// release does not have to be signed, but one that claims a signature and fails
// it is not one to run.
func TestASignatureThatDoesNotMatchAlwaysRefuses(t *testing.T) {
	needOpenSSLForEd25519(t)
	for _, tag := range []string{"v1.3.6.1", "v1.3.5", "dev"} {
		tampered := newRelease(t)
		if err := os.WriteFile(tampered.sums, []byte("fedcba9876543210  zoomies_linux_amd64\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if stderr, exit := tampered.verify(t, tag); exit == 0 || !strings.Contains(stderr, "does not match") {
			t.Errorf("%s with checksums changed after signing: exit %d, stderr %q; it must refuse", tag, exit, stderr)
		}

		other := newRelease(t)
		stranger := newRelease(t)
		other.key = stranger.key // the script trusts a key the release was not signed with
		if stderr, exit := other.verify(t, tag); exit == 0 || !strings.Contains(stderr, "does not match") {
			t.Errorf("%s signed by another key: exit %d, stderr %q; it must refuse", tag, exit, stderr)
		}
	}
}

// While a key is rotated releasekey.Default carries more than one key, one per
// line, and a release signed by either must be accepted. The scripts used to
// decode the whole set as though it were one key and so refused a correctly
// signed release for exactly as long as a rotation lasted, which is when it
// matters most.
func TestAKeySetCarriedForARotationAcceptsASignatureFromAnyKeyInIt(t *testing.T) {
	needOpenSSLForEd25519(t)
	for name, arrange := range map[string]func(signer, a, b string) string{
		"the signer alone":                  func(signer, a, b string) string { return signer },
		"the signer first":                  func(signer, a, b string) string { return signer + "\n" + a },
		"the signer last":                   func(signer, a, b string) string { return a + "\n" + signer },
		"the signer in the middle of three": func(signer, a, b string) string { return a + "\n" + signer + "\n" + b },
		"a trailing newline":                func(signer, a, b string) string { return a + "\n" + signer + "\n" },
		"blank lines and stray spaces":      func(signer, a, b string) string { return "\n  " + a + " \n\n" + signer + "\r\n\n" },
	} {
		r := newRelease(t)
		a, b := newRelease(t), newRelease(t)
		r.key = arrange(r.key, a.key, b.key)
		for _, tag := range []string{"v1.3.7", "v1.3.5"} {
			if stderr, exit := r.verify(t, tag); exit != 0 || !strings.Contains(stderr, "matches the release key") {
				t.Errorf("%s, %s: exit %d, stderr %q; a key set holding the signer must accept", name, tag, exit, stderr)
			}
		}
	}
}

func TestAKeySetThatDoesNotHoldTheSignerRefuses(t *testing.T) {
	needOpenSSLForEd25519(t)
	r := newRelease(t)
	a, b := newRelease(t), newRelease(t)
	r.key = a.key + "\n" + b.key
	for _, tag := range []string{"v1.3.7", "v1.3.5"} {
		if stderr, exit := r.verify(t, tag); exit == 0 || !strings.Contains(stderr, "does not match") {
			t.Errorf("%s signed by a key outside a two-key set: exit %d, stderr %q; it must refuse", tag, exit, stderr)
		}
	}
}

// A set with a line that is not a key is a script that was edited badly, and it
// is better to stop than to skip the line and trust what is left of it.
func TestAKeySetWithAnUnreadableLineRefusesInsteadOfSkippingIt(t *testing.T) {
	needOpenSSLForEd25519(t)
	r := newRelease(t)
	r.key = "!!! not a key !!!\n" + r.key
	if stderr, exit := r.verify(t, "v1.3.7"); exit == 0 || !strings.Contains(stderr, "cannot be read") {
		t.Errorf("a key set with a garbage line: exit %d, stderr %q; it must refuse", exit, stderr)
	}
}

func TestAMissingOrUnreadableSignatureRefusesANewReleaseAndWarnsForAnOldOne(t *testing.T) {
	needOpenSSLForEd25519(t)
	for name, break_ := range map[string]func(r signedRelease){
		"no signature file":  func(r signedRelease) { _ = os.Remove(r.sig) },
		"an empty signature": func(r signedRelease) { _ = os.WriteFile(r.sig, nil, 0o600) },
		"a signature that is not base64": func(r signedRelease) {
			_ = os.WriteFile(r.sig, []byte("keyid\n!!! not base64 !!!\n"), 0o600)
		},
	} {
		r := newRelease(t)
		break_(r)
		if stderr, exit := r.verify(t, "v1.3.6.1"); exit == 0 {
			t.Errorf("%s on v1.3.6.1: it ran on, stderr %q; a release this new must be signed", name, stderr)
		}
		stderr, exit := r.verify(t, "v1.3.5")
		if exit != 0 || !strings.Contains(stderr, "warning") || !strings.Contains(stderr, "checksum alone") {
			t.Errorf("%s on v1.3.5: exit %d, stderr %q; an old release has nothing to check and should say so", name, exit, stderr)
		}
		if stderr, exit := r.verify(t, "dev"); exit != 0 || !strings.Contains(stderr, "warning") {
			t.Errorf("%s on dev: exit %d, stderr %q", name, exit, stderr)
		}
	}
}

// A host with no openssl, or one too old to do Ed25519, cannot check a
// signature. That is not the same as a signature that does not match, and it
// must not be reported as one.
func TestAHostThatCannotCheckASignatureSaysSoAndDoesNotCallItAMismatch(t *testing.T) {
	needOpenSSLForEd25519(t)
	r := newRelease(t)
	bin := t.TempDir()
	for _, tool := range []string{"sed", "grep", "cat"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s is not available to build a PATH without openssl", tool)
		}
		if err := os.Symlink(path, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	stderr, exit := r.verify(t, "v1.3.6.1", "PATH="+bin)
	if exit == 0 || !strings.Contains(stderr, "openssl") || strings.Contains(stderr, "does not match") {
		t.Errorf("a new release on a host with no openssl: exit %d, stderr %q; it should refuse and ask for openssl", exit, stderr)
	}
	stderr, exit = r.verify(t, "v1.3.5", "PATH="+bin)
	if exit != 0 || !strings.Contains(stderr, "no openssl") {
		t.Errorf("an old release on a host with no openssl: exit %d, stderr %q; it should warn and go on", exit, stderr)
	}
}

// A signature checked after the checksums have been used proves nothing: the
// order is the whole point. Each script has to verify the signature before it
// reads a checksum out of checksums.txt or compares one with the download.
func TestTheScriptsVerifyTheSignatureBeforeTheyBelieveTheChecksums(t *testing.T) {
	for rel, believes := range map[string]string{
		"connect-proxmox.sh":        "sha256sum -c selected",
		"deploy/instapods/fetch.sh": `WANT=$(awk`,
	} {
		text := scriptText(t, rel)
		verify := strings.Index(text, `verify_signature "`)
		use := strings.Index(text, believes)
		if verify < 0 || use < 0 {
			t.Errorf("%s: could not find both the call to verify_signature (%d) and %q (%d)", rel, verify, believes, use)
			continue
		}
		if verify > use {
			t.Errorf("%s believes its checksums (%q) before it verifies their signature", rel, believes)
		}
	}
}

// install.sh carries its own verifier, because it is the one script that does
// not share the block above: it is best effort, so a host that cannot check a
// signature still installs. Its handling of a rotation's key set is the same as
// the downloaders' and is held to the same cases, with its own rule for what an
// unreadable key means.
func installVerify(t *testing.T, r signedRelease) (stderr string, exit int) {
	t.Helper()
	text := scriptText(t, "install.sh")
	start := strings.Index(text, "\nverify_signature() {")
	if start < 0 {
		t.Fatal("install.sh has no verify_signature function")
	}
	end := strings.Index(text[start+1:], "\n}\n")
	if end < 0 {
		t.Fatal("could not find the end of install.sh's verify_signature")
	}
	fn := text[start : start+1+end+3]
	full := "set -eu\n" +
		"have() { command -v \"$1\" >/dev/null 2>&1; }\n" +
		"die() { printf 'install: %s\\n' \"$*\" >&2; exit 1; }\n" +
		"ok() { printf 'install: %s\\n' \"$*\" >&2; }\n" +
		"warn() { printf 'install: warning: %s\\n' \"$*\" >&2; }\n" +
		"fetch_stdout() { cat \"$SIG\"; }\n" +
		fn + "\n" +
		"RELEASE_KEY=\"$KEY\"; BASE_URL=unused; verify_signature v1.3.7 \"$SUMS\" \"$SCRATCH\"\n"
	cmd := exec.Command("sh", "-c", full)
	cmd.Env = append(os.Environ(), "KEY="+r.key, "SUMS="+r.sums, "SIG="+r.sig, "SCRATCH="+t.TempDir())
	var errb bytes.Buffer
	cmd.Stderr = &errb
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		exit = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running sh: %v", err)
	}
	return errb.String(), exit
}

func TestInstallShAcceptsASignatureFromAnyKeyInARotationSet(t *testing.T) {
	needOpenSSLForEd25519(t)
	for name, arrange := range map[string]func(signer, a, b string) string{
		"the signer alone":                  func(signer, a, b string) string { return signer },
		"the signer first":                  func(signer, a, b string) string { return signer + "\n" + a },
		"the signer last":                   func(signer, a, b string) string { return a + "\n" + signer },
		"the signer in the middle of three": func(signer, a, b string) string { return a + "\n" + signer + "\n" + b },
		"blank lines and stray spaces":      func(signer, a, b string) string { return "\n  " + a + " \n\n" + signer + "\r\n\n" },
	} {
		r := newRelease(t)
		a, b := newRelease(t), newRelease(t)
		r.key = arrange(r.key, a.key, b.key)
		if stderr, exit := installVerify(t, r); exit != 0 || !strings.Contains(stderr, "matches the release key") {
			t.Errorf("install.sh, %s: exit %d, stderr %q; a key set holding the signer must accept", name, exit, stderr)
		}
	}

	outside := newRelease(t)
	x, y := newRelease(t), newRelease(t)
	outside.key = x.key + "\n" + y.key
	if stderr, exit := installVerify(t, outside); exit == 0 || !strings.Contains(stderr, "does not match") {
		t.Errorf("install.sh, signed by a key outside the set: exit %d, stderr %q; it must refuse", exit, stderr)
	}

	garbled := newRelease(t)
	garbled.key = "!!! not a key !!!\n" + garbled.key
	if stderr, exit := installVerify(t, garbled); exit != 0 || !strings.Contains(stderr, "cannot be read") {
		t.Errorf("install.sh, an unreadable key: exit %d, stderr %q; being best effort it warns and carries on", exit, stderr)
	}
}
