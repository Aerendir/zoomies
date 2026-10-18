package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/releasekey"
)

// A pod's Build command runs deploy/instapods/fetch.sh, and so does the nightly
// update.sh that docs/instapods.md recommends, with ZOOMIES_VERSION unset. The
// release it takes then is whichever one GitHub calls the latest, and that is
// the newest by date: whoever can publish a release chooses its name and can
// attach a checksums.txt that matches a binary of their own. They cannot sign
// it. These tests run the real script, end to end, against a stand-in for
// GitHub, to hold it to refusing that.

// fetchRig is a pod's checkout, a home directory per run, and a curl that serves
// release files from a directory instead of GitHub. The script is the one that
// ships, except for its release key: the project's private half is not ours to
// sign with, so the key is swapped for one this test holds both halves of.
type fetchRig struct {
	root     string // the checkout, the fake curl and the releases
	releases string
	priv     string
	asset    string
}

// fakeCurl answers the two kinds of request fetch.sh makes: the redirect from
// .../releases/latest, which names the latest tag, and a download of a release
// file, which is served from $FAKE_RELEASES or is a 404. Every request is
// logged, so a test can say nothing was downloaded.
const fakeCurl = `#!/bin/sh
out=''
url=''
while [ $# -gt 0 ]; do
    case "$1" in
        -o) out=$2; shift 2 ;;
        -w | --retry | --retry-delay | --proto | --proto-redir) shift 2 ;;
        -*) shift ;;
        *) url=$1; shift ;;
    esac
done
printf '%s\n' "$url" >> "$FAKE_LOG"
case "$url" in
    */releases/latest)
        printf '%s' "https://github.com/eyupio/zoomies/releases/tag/$FAKE_LATEST"
        ;;
    */releases/download/*)
        rest=${url#*/releases/download/}
        src="$FAKE_RELEASES/${rest%%/*}/${rest#*/}"
        [ -f "$src" ] || exit 22
        cp "$src" "$out"
        ;;
    *) exit 22 ;;
esac
`

func newFetchRig(t *testing.T) *fetchRig {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("fetch.sh is for a pod, which runs Linux, and refuses to run anywhere else")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("fetch.sh downloads amd64 and arm64 builds, and this is %s", runtime.GOARCH)
	}
	needOpenSSLForEd25519(t)

	pub, priv, err := releasekey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	script := scriptText(t, "deploy/instapods/fetch.sh")
	keyed := regexp.MustCompile(`(?m)^RELEASE_KEY="[^"]*"$`).ReplaceAllString(script, `RELEASE_KEY="`+pub+`"`)
	if keyed == script {
		t.Fatal("fetch.sh has no RELEASE_KEY line to swap, so the signatures made here would be checked against the project's key")
	}

	root := t.TempDir()
	r := &fetchRig{root: root, releases: filepath.Join(root, "releases"), priv: priv, asset: "zoomies_linux_" + runtime.GOARCH}
	for path, body := range map[string]string{
		"app/deploy/instapods/fetch.sh": keyed,
		"app/deploy/instapods/lib.sh":   scriptText(t, "deploy/instapods/lib.sh"),
		"bin/curl":                      fakeCurl,
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// release publishes a tag: a binary that records having been run, the
// checksums.txt that matches it, and the signature when asked for one.
func (r *fetchRig) release(t *testing.T, tag string, signed bool) {
	t.Helper()
	dir := filepath.Join(r.releases, tag)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := "#!/bin/sh\n: > \"$HOME/payload-ran\"\necho \"zoomies " + tag + "\"\n"
	sum := sha256.Sum256([]byte(payload))
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + r.asset + "\n")
	files := map[string][]byte{r.asset: []byte(payload), "checksums.txt": sums}
	if signed {
		envelope, err := releasekey.Sign(r.priv, sums)
		if err != nil {
			t.Fatal(err)
		}
		files["checksums.txt.ed25519"] = envelope
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

type fetchRun struct {
	output     string
	exit       int
	installed  bool   // the binary is in place, where the service will start it from
	ran        bool   // the downloaded file was executed at all
	downloaded string // every address the script asked the fake GitHub for
}

// run is a deploy of the controller pod. latest is the tag GitHub's
// /releases/latest redirects to, and env is the rest of the pod's environment,
// ZOOMIES_VERSION among it.
func (r *fetchRig) run(t *testing.T, latest string, env ...string) fetchRun {
	t.Helper()
	home, logPath := t.TempDir(), filepath.Join(t.TempDir(), "requests")
	cmd := exec.Command("sh", filepath.Join(r.root, "app/deploy/instapods/fetch.sh"), "controller")
	cmd.Env = append([]string{
		"PATH=" + filepath.Join(r.root, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home, "FAKE_RELEASES=" + r.releases, "FAKE_LATEST=" + latest, "FAKE_LOG=" + logPath,
	}, env...)
	out, err := cmd.CombinedOutput()
	res := fetchRun{output: string(out)}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.exit = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running sh: %v", err)
	}
	_, statErr := os.Stat(filepath.Join(home, "zoomies/bin/zoomies"))
	res.installed = statErr == nil
	_, statErr = os.Stat(filepath.Join(home, "payload-ran"))
	res.ran = statErr == nil
	if b, err := os.ReadFile(logPath); err == nil {
		res.downloaded = string(b)
	}
	return res
}

func TestTheInstaPodsBuildInstallsALatestReleaseThatIsSigned(t *testing.T) {
	r := newFetchRig(t)
	r.release(t, "v1.3.9", true)
	got := r.run(t, "v1.3.9")
	if got.exit != 0 || !got.installed || !strings.Contains(got.output, "matches the release key") {
		t.Errorf("a signed latest release: exit %d, installed %v, output:\n%s\nwant it installed after the signature was checked", got.exit, got.installed, got.output)
	}
}

// GitHub's latest is the newest release by date, and its name is the
// publisher's to choose. A name that needs no signature (an old version, a
// string the script cannot read as one) is therefore not what "latest" can
// honestly be, since every release from v1.3.6 up is signed: it is refused
// before the binary is fetched, and the message says what to do instead. The
// same script still runs an old release its operator names, below.
func TestTheInstaPodsBuildRefusesALatestReleaseThatNeedsNoSignatureBeforeDownloadingIt(t *testing.T) {
	r := newFetchRig(t)
	for _, tag := range []string{"v0.0.1", "v1.3.5", "dev", "v2x.0.0", "v99999999999999999999.0.0"} {
		r.release(t, tag, false)
	}
	for _, tag := range []string{"v0.0.1", "v1.3.5", "dev"} {
		t.Run(tag, func(t *testing.T) {
			got := r.run(t, tag)
			if got.exit == 0 || got.installed || got.ran {
				t.Errorf("latest = %s with no signature: exit %d, installed %v, run %v; it must stop", tag, got.exit, got.installed, got.ran)
			}
			if strings.Contains(got.downloaded, "/download/"+tag+"/"+r.asset) {
				t.Errorf("latest = %s: the binary was downloaded before the name was refused:\n%s", tag, got.downloaded)
			}
			if !strings.Contains(got.output, tag) || !strings.Contains(got.output, "ZOOMIES_VERSION") {
				t.Errorf("latest = %s: the refusal does not name the release and the variable that chooses another:\n%s", tag, got.output)
			}
		})
	}
	// A name the script cannot read as a version is also held to a signature, so
	// it is refused for the want of one wherever the name came from.
	for _, tag := range []string{"v2x.0.0", "v99999999999999999999.0.0"} {
		t.Run(tag, func(t *testing.T) {
			if got := r.run(t, tag); got.exit == 0 || got.installed || got.ran {
				t.Errorf("latest = %s with no signature: exit %d, installed %v, run %v; it must stop:\n%s", tag, got.exit, got.installed, got.ran, got.output)
			}
		})
	}
}

// Naming a release is the operator's own decision, and docs/upgrading.md says a
// release before signing began is taken on its checksum with a warning. A pod
// pinned to one keeps working.
func TestTheInstaPodsBuildStillTakesAnOldReleaseNamedOutrightOnItsChecksum(t *testing.T) {
	r := newFetchRig(t)
	r.release(t, "v1.3.5", false)
	got := r.run(t, "v1.3.9", "ZOOMIES_VERSION=v1.3.5")
	if got.exit != 0 || !got.installed || !strings.Contains(got.output, "warning") || !strings.Contains(got.output, "checksum alone") {
		t.Errorf("v1.3.5 named by the operator: exit %d, installed %v, output:\n%s\nwant it installed on its checksum with a warning", got.exit, got.installed, got.output)
	}
}

// A name that cannot be read as numbers is not an old release. An operator who
// pins one gets the same answer as for a new release with no signature, because
// the alternative is that the name itself switches the check off.
func TestTheInstaPodsBuildHoldsAReleaseWithAnUnreadableNameToASignature(t *testing.T) {
	r := newFetchRig(t)
	for _, tag := range []string{"v2x.0.0", "v99999999999999999999.0.0"} {
		r.release(t, tag, false)
		t.Run(tag, func(t *testing.T) {
			got := r.run(t, "v1.3.9", "ZOOMIES_VERSION="+tag)
			if got.exit == 0 || got.installed || got.ran || !strings.Contains(got.output, "must be signed") {
				t.Errorf("%s named by the operator, unsigned: exit %d, installed %v, run %v, output:\n%s\nwant it refused for the want of a signature", tag, got.exit, got.installed, got.ran, got.output)
			}
		})
	}
}
