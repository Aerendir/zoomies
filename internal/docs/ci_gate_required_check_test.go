package docs

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Branch protection can require a status check only by name, and ci.yml has
// seventeen jobs, several of them matrices and most of them skipped by the
// changes filter, so no one of them is a name a ruleset can require: a renamed
// or skipped job would either block every merge or protect nothing. The
// ci-gate job is the one name to require. It waits on every other job and
// fails if any of them failed or was cancelled, so a pull request whose CI
// never ran, or ran red, has no passing check to merge on. Two pull requests
// were merged with no CI run on their heads before it existed.
//
// The gate protects only the jobs it waits on, and a job added later is the
// one most easily forgotten, so this test holds its needs to the whole list.
func TestTheCIGateWaitsOnEveryOtherJob(t *testing.T) {
	var wf struct {
		Jobs map[string]struct {
			Needs jobNeeds `yaml:"needs"`
			If    string   `yaml:"if"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(workflowFiles(t)["ci.yml"]), &wf); err != nil {
		t.Fatalf("parsing ci.yml: %v", err)
	}
	gate, ok := wf.Jobs["ci-gate"]
	if !ok {
		t.Fatal("ci.yml has no ci-gate job; it is the one status check branch protection requires, so without it a pull request can merge with no CI run on its head")
	}

	var others []string
	for id := range wf.Jobs {
		if id != "ci-gate" {
			others = append(others, id)
		}
	}
	slices.Sort(others)
	needs := slices.Clone([]string(gate.Needs))
	slices.Sort(needs)
	if !slices.Equal(needs, others) {
		t.Errorf("ci-gate needs %v, want every other job %v; a job the gate does not wait on can fail without blocking a merge", needs, others)
	}

	// A skipped job is a job the changes filter decided this run did not need,
	// and a dependency that was skipped skips its dependants too, so the gate
	// has to run whatever the jobs before it did, short of the run itself being
	// cancelled. always() would also start it on the closed run.
	if !strings.HasPrefix(gate.If, "${{ !cancelled() && "+closedGuard) {
		t.Errorf("ci-gate's condition is %q; it must lead with !cancelled() and the closed guard, or a skipped job skips the gate and the merge has no check to wait for", gate.If)
	}
}

// jobNeeds reads a job's needs, which GitHub lets a workflow write as one name
// or as a list of them; ci.yml uses both.
type jobNeeds []string

func (n *jobNeeds) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*n = jobNeeds{node.Value}
		return nil
	}
	var list []string
	if err := node.Decode(&list); err != nil {
		return err
	}
	*n = list
	return nil
}
