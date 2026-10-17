package docs

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// changesScript is the shell that decides what CI runs for a change, taken
// out of ci.yml so the decisions can be asked about directly.
func changesScript(t *testing.T) string {
	t.Helper()
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				ID  string `yaml:"id"`
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(workflowFiles(t)["ci.yml"]), &wf); err != nil {
		t.Fatalf("parsing ci.yml: %v", err)
	}
	for _, step := range wf.Jobs["changes"].Steps {
		if step.ID == "filter" {
			return step.Run
		}
	}
	t.Fatal("ci.yml's changes job has no filter step")
	return ""
}

// decide runs the filter as a push to main, or a pull request, that changed
// these files, with a git that answers only the two questions it asks.
func decide(t *testing.T, event string, files ...string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	stub := "#!/bin/sh\ncase \"$1\" in fetch) exit 0 ;; diff) printf '%s\\n' " + strings.Join(files, " ") + " ;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "output")
	ref := "refs/pull/1/merge"
	if event == "push" {
		ref = "refs/heads/main"
	}
	cmd := exec.Command("sh", "-c", changesScript(t))
	cmd.Env = append(os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"EVENT="+event, "REF="+ref, "BASE=0123abc", "GITHUB_OUTPUT="+out)
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the filter failed for %v: %v\n%s", files, err, msg)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			got[k] = v
		}
	}
	return got
}

// A merge to main used to run everything, so a README edit rebuilt and
// republished every binary and image on the dev channel with nothing in them
// different, and held the fleet for an hour doing it. What a change runs is
// now what its files can affect, and only a change to what ships publishes.
func TestCIRunsOnlyWhatAChangeCanAffect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the filter is POSIX shell, and runs on Linux in CI")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh to run the filter with")
	}
	all := `["controller-1","controller-2","api-1","api-2","store","rest"]`
	for _, tc := range []struct {
		name  string
		event string
		files []string
		want  map[string]string
	}{
		{"the README checks its own facts and nothing else", "push", []string{"README.md"},
			map[string]string{"go": "false", "web": "false", "deploy": "false", "installer": "false", "shards": `["docs"]`, "publish": "false"}},
		// internal/docs holds install.sh's release key to the binary's and its
		// verifier to the other scripts', and its whole-tree scans read every
		// file, so a change to a script people curl runs that package.
		{"the install script is checked and upgraded through, and read by the docs tests, not rebuilt into binaries or images", "push", []string{"install.sh"},
			map[string]string{"go": "false", "deploy": "false", "installer": "true", "shards": `["docs"]`, "publish": "false"}},
		{"the Windows installer is held to the same tests", "push", []string{"install.ps1"},
			map[string]string{"go": "false", "deploy": "false", "installer": "true", "shards": `["docs"]`, "publish": "false"}},
		{"the install and upgrade checks are shell the whole-tree scans read", "push", []string{"test/install/install-check.sh", "test/upgrade/upgrade-check.sh"},
			map[string]string{"go": "false", "deploy": "false", "installer": "true", "shards": `["docs"]`, "publish": "false"}},
		// internal/proxmoxsetup holds its signature check equal to the one the
		// wizard's command carries, and that package is in the rest shard.
		{"the Proxmox connector is run through the tests that hold its signature check to the others", "pull_request", []string{"connect-proxmox.sh"},
			map[string]string{"go": "false", "deploy": "false", "installer": "true", "shards": `["rest"]`, "publish": "false"}},
		{"a README next to the runner image check is prose, and builds no image", "push", []string{"test/runner-image/README.md"},
			map[string]string{"go": "false", "deploy": "false", "installer": "false", "shards": `["docs"]`, "publish": "false"}},
		{"the runner image check itself still builds the images and is read by the whole-tree scans", "pull_request", []string{"test/runner-image/check.sh"},
			map[string]string{"go": "false", "deploy": "true", "installer": "true", "shards": `["docs"]`}},
		{"a README in deploy/ is prose, and publishes nothing", "push", []string{"deploy/marketplace/README.md"},
			map[string]string{"go": "false", "deploy": "false", "installer": "false", "shards": `["docs"]`, "publish": "false"}},
		{"a file nothing reads runs nothing", "push", []string{"LICENSE", ".gitignore"},
			map[string]string{"go": "false", "installer": "false", "shards": "[]", "publish": "false"}},
		{"Go on main is the whole suite and a publish", "push", []string{"internal/store/queries.go"},
			map[string]string{"go": "true", "shards": all, "publish": "true"}},
		{"the UI is embedded in the binary, so it publishes too", "push", []string{"web/src/lib/units.ts"},
			map[string]string{"go": "false", "web": "true", "publish": "true"}},
		{"an image input publishes, so the binaries and images keep naming one commit", "push", []string{"deploy/Dockerfile"},
			map[string]string{"deploy": "true", "shards": `["rest"]`, "publish": "true"}},
		{"the InstaPods scripts are linted and tested, and go into no image", "push", []string{"deploy/instapods/run.sh"},
			map[string]string{"go": "false", "deploy": "false", "installer": "true", "shards": `["rest"]`, "publish": "false"}},
		{"the package-cleanup scripts are linted and read as prose, and ship nothing", "push", []string{".github/scripts/ghcr-prune.sh"},
			map[string]string{"go": "false", "deploy": "false", "installer": "true", "shards": `["docs"]`, "publish": "false"}},
		{"the marketplace bootstrap is linted, tested and part of the image", "push", []string{"deploy/marketplace/bootstrap.sh"},
			map[string]string{"go": "false", "deploy": "true", "installer": "true", "shards": `["rest"]`, "publish": "true"}},
		{"the tool-cache filler is embedded in the binary and linted as shell", "push", []string{"internal/backend/toolfill.sh"},
			map[string]string{"go": "true", "installer": "true", "shards": all, "publish": "true"}},
		{"the demo organisation's workflow templates are prose to this repository", "push", []string{"deploy/demo-org/build.yml"},
			map[string]string{"go": "false", "deploy": "false", "installer": "false", "shards": `["docs"]`, "publish": "false"}},
		{"a pull request never publishes", "pull_request", []string{"internal/store/queries.go"},
			map[string]string{"go": "true", "publish": "false"}},
		{"a change to CI itself runs everything", "pull_request", []string{".github/workflows/ci.yml"},
			map[string]string{"go": "true", "web": "true", "deploy": "true", "installer": "true", "shards": all}},
		{"an unknown kind of file runs too much rather than too little", "pull_request", []string{"something/new.xyz"},
			map[string]string{"go": "true", "shards": all}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decide(t, tc.event, tc.files...)
			for k, want := range tc.want {
				if got[k] != want {
					t.Errorf("%s for %v: %s=%s, want %s (all: %v)", tc.event, tc.files, k, got[k], want, got)
				}
			}
		})
	}
}

// runnerImageScript is the shell the images job runs to decide whether to build
// the runner images, taken out of ci.yml like the filter above. GitHub fills in
// ${{ github.event_name }} before a shell sees the script, so it is filled in
// here as a pull request.
func runnerImageScript(t *testing.T) string {
	t.Helper()
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				ID  string `yaml:"id"`
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(workflowFiles(t)["ci.yml"]), &wf); err != nil {
		t.Fatalf("parsing ci.yml: %v", err)
	}
	for _, step := range wf.Jobs["images"].Steps {
		if step.ID == "changed" {
			return strings.ReplaceAll(step.Run, "${{ github.event_name }}", "pull_request")
		}
	}
	t.Fatal("ci.yml's images job has no step with the id changed")
	return ""
}

// buildsRunnerImages runs that step for a pull request that changed these
// files, with a git that answers only the two questions it asks, and says
// whether it chose to build the runner images.
func buildsRunnerImages(t *testing.T, files ...string) bool {
	t.Helper()
	dir := t.TempDir()
	stub := "#!/bin/sh\ncase \"$1\" in cat-file) exit 0 ;; diff) printf '%s\\n' " + strings.Join(files, " ") + " ;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "output")
	cmd := exec.Command("sh", "-c", runnerImageScript(t))
	cmd.Env = append(os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"BASE=0123abc", "PUBLISH=false", "GITHUB_OUTPUT="+out)
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the step failed for %v: %v\n%s", files, err, msg)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if v, ok := strings.CutPrefix(line, "runner="); ok {
			return v == "true"
		}
	}
	t.Fatalf("the step wrote no runner output for %v: %q", files, raw)
	return false
}

// The weekly base-image refresh opens a pull request that moves digests in
// internal/naming/images.go and regenerates what is derived from it, and its
// description says CI builds every variant on that pull request. The step only
// looked under deploy/ and test/runner-image, so that pull request built no
// runner image and a digest nobody had built went unproven. The tool-cache lock
// is copied into every image, and was missed the same way.
func TestTheRunnerImagesAreBuiltWhenAnythingTheyAreMadeFromChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the step is POSIX shell, and runs on Linux in CI")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh to run the step with")
	}
	for _, tc := range []struct {
		name  string
		files []string
		want  bool
	}{
		{"the weekly base refresh moves digests, and the files generated from them", []string{
			"internal/naming/images.go", "Makefile", ".github/workflows/ci.yml", ".github/workflows/release.yml", "docs/naming.md"}, true},
		{"the tool-cache lock is copied into every image", []string{"deploy/toolcache.lock"}, true},
		{"the runner Dockerfile", []string{"deploy/Dockerfile.runner"}, true},
		{"a script the runner Dockerfile copies in", []string{"deploy/runner-apt.sh"}, true},
		{"the runner entrypoint", []string{"deploy/runner-entrypoint.sh"}, true},
		{"the check each built image is run through", []string{"test/runner-image/check.sh"}, true},
		{"prose about the images", []string{"docs/naming.md"}, false},
		{"the catalogue's tests and its generators build nothing", []string{"internal/naming/images_test.go", "internal/naming/gen_base_digests.go"}, false},
		{"the lock's generator is not the lock", []string{"deploy/gen_toolcache_lock.go"}, false},
		{"the controller image is not a runner image", []string{"deploy/Dockerfile"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildsRunnerImages(t, tc.files...); got != tc.want {
				t.Errorf("a pull request changing %v: runner=%v, want %v", tc.files, got, tc.want)
			}
		})
	}
}
