package docs

import (
	"path"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A shell script that no job lints is one that breaks the first time it runs,
// and the scripts that were left out were the ones that run unattended: the
// package-cleanup scripts delete from a registry on a schedule, the marketplace
// bootstrap runs as root on a new machine, and nobody reads either on a pull
// request. The installer job lists what it lints, so this holds the list to
// the tree: every tracked shell script is named there, directly or by a glob.
func TestEveryShellScriptIsLintedByTheInstallerJob(t *testing.T) {
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(workflowFiles(t)["ci.yml"]), &wf); err != nil {
		t.Fatalf("parsing ci.yml: %v", err)
	}
	var script strings.Builder
	for _, step := range wf.Jobs["installer"].Steps {
		script.WriteString(step.Run + "\n")
	}
	if script.Len() == 0 {
		t.Fatal("ci.yml has no installer job to read")
	}
	// A path or a glob that ends in .sh, wherever the job writes one.
	named := regexp.MustCompile(`[A-Za-z0-9_./*-]+\.sh`).FindAllString(script.String(), -1)

	for _, file := range trackedFiles(t) {
		if !strings.HasSuffix(file, ".sh") {
			continue
		}
		linted := false
		for _, pattern := range named {
			if ok, _ := path.Match(pattern, file); ok {
				linted = true
				break
			}
		}
		if !linted {
			t.Errorf("%s is shell, and the installer job in ci.yml neither names it nor globs it; add it to the list there (and to the changes job's routing if its directory is not already sent to the installer job)", file)
		}
	}
}
