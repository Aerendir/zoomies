package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The toolchain versions used to be written wherever they were needed, and the
// copies drifted apart without anything failing: the staticcheck job sat on Go
// 1.27.1, fuzz and CodeQL followed go.mod (also 1.27.1), the controller image's
// pinned golang base was 1.27.1, and the rest of CI and the release binaries
// were 1.27.2. The images were built with a different compiler from the
// binaries, and a Dependabot bump moved the image's Node from 22 to 26 while the
// release workflow and every test stayed on 22.
//
// These hold the versions to the one place each is written.

const setupGoAction = "../../.github/actions/setup-go/action.yml"

// The version written in the composite action, as "1.27.2".
func setupGoVersion(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(setupGoAction)
	if err != nil {
		t.Fatalf("reading the setup-go action: %v", err)
	}
	m := regexp.MustCompile(`go-version:\s*"?(\d+\.\d+\.\d+)"?`).FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatal("the setup-go action does not say which Go it installs")
	}
	return m[1]
}

func minor(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

func TestEveryWorkflowGetsItsGoFromTheOneAction(t *testing.T) {
	for name, body := range workflowFiles(t) {
		if strings.Contains(body, "actions/setup-go@") {
			t.Errorf("%s installs Go directly; use ./.github/actions/setup-go, which is the one place the version is written", name)
		}
	}

	// A local action is read from the checked-out tree, so a job that uses one
	// before it has checked out fails with "Can't find action.yml". Check the
	// order, which is easy to get wrong when the step is moved.
	for name, body := range workflowFiles(t) {
		var workflow struct {
			Jobs map[string]struct {
				Steps []struct {
					Uses string `yaml:"uses"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal([]byte(body), &workflow); err != nil {
			t.Errorf("%s does not parse: %v", name, err)
			continue
		}
		for id, job := range workflow.Jobs {
			checkedOut := false
			for _, step := range job.Steps {
				switch {
				case strings.HasPrefix(step.Uses, "actions/checkout@"):
					checkedOut = true
				case strings.HasPrefix(step.Uses, "./.github/actions/") && !checkedOut:
					t.Errorf("%s job %s uses %s before it checks the repository out, so the action cannot be found", name, id, step.Uses)
				}
			}
		}
	}
}

// The actions inside a composite action are third-party code like any other.
func TestCompositeActionsPinTheirActionsToACommit(t *testing.T) {
	files, err := filepath.Glob("../../.github/actions/*/action.yml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no composite actions found (%v)", err)
	}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range usesLine.FindAllStringSubmatch(string(body), -1) {
			ref, comment := m[1], strings.TrimSpace(m[2])
			if strings.HasPrefix(ref, "./") {
				continue
			}
			if !pinned.MatchString(ref) {
				t.Errorf("%s: %q is not pinned to a commit", filepath.Base(filepath.Dir(file)), ref)
			} else if comment == "" {
				t.Errorf("%s: %q has no version comment", filepath.Base(filepath.Dir(file)), ref)
			}
		}
	}
}

func TestTheGoVersionsAgreeWhereTheyHaveToAndOnlyThere(t *testing.T) {
	ci := setupGoVersion(t)

	gomod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	floor := regexp.MustCompile(`(?m)^go (\d+\.\d+\.\d+)$`).FindStringSubmatch(string(gomod))
	if floor == nil {
		t.Fatal("go.mod has no go directive")
	}
	if minor(floor[1]) != minor(ci) {
		t.Errorf("go.mod needs Go %s and CI installs %s: a different minor release means CI is not testing the language the module declares", floor[1], ci)
	}
	if compareVersions(ci, floor[1]) < 0 {
		t.Errorf("CI installs Go %s, below the %s go.mod requires", ci, floor[1])
	}
	if strings.Contains(string(gomod), "\ntoolchain ") {
		t.Error("go.mod has a toolchain line, which raises the Go every contributor needs to CI's; the floor is the go directive and CI's version lives in the setup-go action")
	}

	dockerfile, err := os.ReadFile("../../deploy/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	image := regexp.MustCompile(`(?m)^FROM [^\n]*golang:(\d+\.\d+)-alpine@sha256:[0-9a-f]{64} AS build`).FindStringSubmatch(string(dockerfile))
	if image == nil {
		t.Fatal("deploy/Dockerfile's build stage is not a digest-pinned golang image any more; this test needs to follow it")
	}
	if image[1] != minor(ci) {
		t.Errorf("the controller image builds on golang:%s and CI installs Go %s", image[1], ci)
	}
}

func TestTheNodeMajorIsTheSameEverywhereTheUIIsBuilt(t *testing.T) {
	read := func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		return string(b)
	}
	nodeVersion := regexp.MustCompile(`(?m)^  NODE_VERSION: "(\d+)"$`)
	major := map[string]string{}
	for _, file := range []string{"ci.yml", "release.yml"} {
		m := nodeVersion.FindStringSubmatch(read("../../.github/workflows/" + file))
		if m == nil {
			t.Fatalf("%s no longer sets NODE_VERSION", file)
		}
		major[file+" (NODE_VERSION)"] = m[1]
	}
	major[".nvmrc"] = strings.TrimSpace(read("../../.nvmrc"))
	image := regexp.MustCompile(`(?m)^FROM [^\n]*node:(\d+)-alpine@sha256:[0-9a-f]{64} AS ui`).FindStringSubmatch(read("../../deploy/Dockerfile"))
	if image == nil {
		t.Fatal("deploy/Dockerfile's ui stage is not a digest-pinned node image any more; this test needs to follow it")
	}
	major["deploy/Dockerfile (ui stage)"] = image[1]

	want := major[".nvmrc"]
	for place, got := range major {
		if got != want {
			t.Errorf("%s builds the UI on Node %s but .nvmrc says %s; change them together, because a bump of one alone leaves the others testing something the image does not ship", place, got, want)
		}
	}

	// The floor package.json declares can be lower than what is tested, but not
	// higher: that would be a version CI itself could not install on.
	engines := regexp.MustCompile(`"node":\s*">=(\d+)"`).FindStringSubmatch(read("../../web/package.json"))
	if engines == nil {
		t.Fatal("web/package.json declares no node engine")
	}
	floorMajor, _ := strconv.Atoi(engines[1])
	testedMajor, _ := strconv.Atoi(want)
	if floorMajor > testedMajor {
		t.Errorf("web/package.json requires Node >=%d, above the %d CI uses", floorMajor, testedMajor)
	}

	// The reason the image's major stopped moving on its own.
	dependabot := read("../../.github/dependabot.yml")
	if !regexp.MustCompile(`(?s)package-ecosystem: docker.*?dependency-name: node\s+update-types: \["version-update:semver-major"\]`).MatchString(dependabot) {
		t.Error("dependabot.yml no longer ignores major updates of the node image, so a bump can move the image off the major everything else tests")
	}
}

func TestStaticcheckIsPinnedToARelease(t *testing.T) {
	body := workflowFiles(t)["ci.yml"]
	m := regexp.MustCompile(`dominikh/staticcheck-action@[0-9a-f]{40}[^\n]*\n\s+with:\n\s+version:\s*"?([^"\s]+)"?`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("ci.yml's staticcheck step was not found")
	}
	if m[1] == "latest" || !regexp.MustCompile(`^\d{4}\.\d+(\.\d+)?$`).MatchString(m[1]) {
		t.Errorf("the staticcheck step runs %q; pin a release, so a new one cannot fail CI with no change in the repository", m[1])
	}
}

// compareVersions orders two dotted numeric versions.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if len(as[i]) != len(bs[i]) {
			return len(as[i]) - len(bs[i])
		}
		if c := strings.Compare(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return len(as) - len(bs)
}
