package docs

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// CODEOWNERS is read by GitHub and by nothing in this repository, so a path in
// it that stops existing fails silently: the review it was meant to ask for is
// never requested. The paths in it are the ones CONTRIBUTING.md says need an
// explicit threat review, which are also the ones most likely to be moved, so
// this fails when one of them is not there.
func TestEveryPathInCodeownersExists(t *testing.T) {
	body, err := os.ReadFile("../../.github/CODEOWNERS")
	if err != nil {
		t.Fatalf("reading .github/CODEOWNERS: %v", err)
	}
	var paths int
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			t.Errorf("CODEOWNERS line %q names a path and no owner, so it asks nobody", line)
			continue
		}
		for _, owner := range fields[1:] {
			if !strings.HasPrefix(owner, "@") {
				t.Errorf("CODEOWNERS line %q: %q is not an @user or @org/team", line, owner)
			}
		}
		pattern := fields[0]
		if pattern == "*" {
			continue
		}
		paths++
		rel := strings.Trim(pattern, "/")
		if _, err := os.Stat(filepath.Join("../..", rel)); err != nil {
			t.Errorf("CODEOWNERS names %q, which is not in the repository; the review it asks for would never be requested", pattern)
		}
	}
	if paths == 0 {
		t.Fatal("CODEOWNERS names no path beyond the default; the test is not reading what it thinks it is")
	}
}

// An issue form that GitHub cannot render falls back to a blank issue, and does
// so without an error anywhere the maintainers look. These are the rules of the
// form schema that are easy to get wrong by hand.
func TestTheIssueFormsAreWellFormed(t *testing.T) {
	files, err := filepath.Glob("../../.github/ISSUE_TEMPLATE/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	var forms int
	for _, file := range files {
		name := filepath.Base(file)
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if name == "config.yml" {
			var cfg struct {
				ContactLinks []struct{ Name, URL, About string } `yaml:"contact_links"`
			}
			if err := yaml.Unmarshal(raw, &cfg); err != nil {
				t.Errorf("%s does not parse: %v", name, err)
			}
			for _, link := range cfg.ContactLinks {
				if link.Name == "" || link.URL == "" || link.About == "" {
					t.Errorf("%s has a contact link with a missing name, url or about: %+v", name, link)
				}
			}
			continue
		}
		forms++
		var form struct {
			Name        string   `yaml:"name"`
			Description string   `yaml:"description"`
			Labels      []string `yaml:"labels"`
			Body        []struct {
				Type       string `yaml:"type"`
				ID         string `yaml:"id"`
				Attributes struct {
					Label   string   `yaml:"label"`
					Value   string   `yaml:"value"`
					Options []string `yaml:"options"`
				} `yaml:"attributes"`
			} `yaml:"body"`
		}
		if err := yaml.Unmarshal(raw, &form); err != nil {
			t.Errorf("%s does not parse: %v", name, err)
			continue
		}
		if form.Name == "" || form.Description == "" {
			t.Errorf("%s needs a name and a description, or GitHub will not list it", name)
		}
		if len(form.Body) == 0 {
			t.Errorf("%s has no body elements", name)
		}
		ids := map[string]bool{}
		for i, element := range form.Body {
			if !slices.Contains([]string{"markdown", "textarea", "input", "dropdown", "checkboxes"}, element.Type) {
				t.Errorf("%s: element %d has type %q, which is not a form element", name, i+1, element.Type)
			}
			if element.Type == "markdown" {
				if element.Attributes.Value == "" {
					t.Errorf("%s: markdown element %d has no value", name, i+1)
				}
				continue
			}
			if element.Attributes.Label == "" {
				t.Errorf("%s: %s element %d has no label", name, element.Type, i+1)
			}
			if element.Type == "dropdown" && len(element.Attributes.Options) == 0 {
				t.Errorf("%s: dropdown %q has no options", name, element.Attributes.Label)
			}
			if element.ID != "" {
				if ids[element.ID] {
					t.Errorf("%s: id %q is used twice, and GitHub rejects a form that does so", name, element.ID)
				}
				ids[element.ID] = true
			}
		}
	}
	if forms == 0 {
		t.Fatal("no issue forms were found; the test is looking in the wrong place")
	}
}

// The pull request template is the one place the checklist in CONTRIBUTING.md
// reaches a contributor at the moment they need it, so it has to ask for what
// that page says a pull request must carry.
func TestThePullRequestTemplateAsksForWhatContributingAsksFor(t *testing.T) {
	body, err := os.ReadFile("../../.github/pull_request_template.md")
	if err != nil {
		t.Fatalf("reading the pull request template: %v", err)
	}
	for _, want := range []string{"problem", "approach", "tests", "documentation", "screenshot"} {
		if !strings.Contains(strings.ToLower(string(body)), want) {
			t.Errorf("the pull request template never mentions %q, which CONTRIBUTING.md asks every pull request to cover", want)
		}
	}
}
