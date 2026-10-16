package docs

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// The UI's unit tests (web/unit, 900-odd of them) were written, documented in
// CONTRIBUTING.md and cited as a guard by AGENTS.md, and no job ran them. A
// change could break every one and merge green. They take about ten seconds, so
// there is no reason for them to be optional; this keeps the step in the job
// that already installs the UI's dependencies, where a removal would be seen.
//
// The workflow is parsed rather than searched, so a mention in a comment, or a
// step that is present and never runs, does not satisfy it.
func TestCIRunsTheUIUnitTests(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("reading ci.yml: %v", err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Run              string `yaml:"run"`
				If               string `yaml:"if"`
				WorkingDirectory string `yaml:"working-directory"`
				ContinueOnError  any    `yaml:"continue-on-error"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatalf("ci.yml does not parse: %v", err)
	}
	ui, ok := workflow.Jobs["ui"]
	if !ok {
		t.Fatal("ci.yml has no ui job any more; the UI's checks have moved and this test should follow them")
	}
	for _, step := range ui.Steps {
		if step.Run != "npm run test:unit" {
			continue
		}
		if step.WorkingDirectory != "web" {
			t.Errorf("the unit test step runs in %q, not web/, where the tests and their runner live", step.WorkingDirectory)
		}
		if step.If != "" {
			t.Errorf("the unit test step is conditional (%q), so it can be skipped on a change that breaks it", step.If)
		}
		if step.ContinueOnError != nil {
			t.Errorf("the unit test step sets continue-on-error (%v), which makes it unable to fail the job", step.ContinueOnError)
		}
		return
	}
	t.Error("the ui job in ci.yml never runs `npm run test:unit`, so the UI's unit tests, which CONTRIBUTING.md asks every UI change to pass, are not run by anything")
}
