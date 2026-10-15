package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// GitHub's default for a job is six hours. On the Zoomies fleet a hung job
// holds a slot for all of it (dev-watchdog.yml's header records a run that did),
// and the slot is what every other pull request is queueing for. A timeout
// turns a hang into a failure somebody sees, so every job has one. The values
// sit at two to three times the longest successful run seen, because the limit
// is there to catch a hang, not a slow machine.
func TestEveryWorkflowJobHasATimeout(t *testing.T) {
	files, err := filepath.Glob(workflowDir + "/*.yml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no workflows found (%v); the test is looking in the wrong place", err)
	}
	var without []string
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var workflow struct {
			Jobs map[string]struct {
				Uses    string `yaml:"uses"`
				Timeout any    `yaml:"timeout-minutes"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(raw, &workflow); err != nil {
			t.Errorf("%s does not parse: %v", filepath.Base(file), err)
			continue
		}
		for id, job := range workflow.Jobs {
			// A job that calls a reusable workflow takes its limits from the
			// jobs in that workflow.
			if job.Uses != "" {
				continue
			}
			if job.Timeout == nil {
				without = append(without, filepath.Base(file)+": "+id)
			}
		}
	}
	sort.Strings(without)
	if len(without) > 0 {
		t.Errorf("these jobs have no timeout-minutes and would run for up to six hours; add one at about two to three times the longest successful run:\n  %s", strings.Join(without, "\n  "))
	}
}

// A space in a workflow's file name made `open code.yml` the one file in the
// directory that every command, grep and doc link has to quote.
func TestWorkflowFileNamesAreLowerCaseWithHyphens(t *testing.T) {
	entries, err := os.ReadDir(workflowDir)
	if err != nil {
		t.Fatal(err)
	}
	shape := regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.yml$`)
	for _, entry := range entries {
		if !entry.IsDir() && !shape.MatchString(entry.Name()) {
			t.Errorf("the workflow file %q should be lower-case words joined by hyphens, so nothing that names it needs quoting", entry.Name())
		}
	}
}
