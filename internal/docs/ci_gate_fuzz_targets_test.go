package docs

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// fuzzTriggerPaths reads the paths filters fuzz.yml starts a run on, one list
// for each trigger. They are read one at a time: a package that is listed under
// push and not under pull_request is fuzzed after a merge and never on the pull
// request that broke it, and a search of the whole file for the path cannot tell
// the two lists apart.
func fuzzTriggerPaths(t *testing.T, workflow string) map[string][]string {
	t.Helper()
	var wf struct {
		On struct {
			Push struct {
				Paths []string `yaml:"paths"`
			} `yaml:"push"`
			PullRequest struct {
				Paths []string `yaml:"paths"`
			} `yaml:"pull_request"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal([]byte(workflow), &wf); err != nil {
		t.Fatalf("parsing fuzz.yml: %v", err)
	}
	return map[string][]string{"push": wf.On.Push.Paths, "pull_request": wf.On.PullRequest.Paths}
}

// fuzzTriggerProblems says which triggers do not start a run for a change to a
// target's package.
func fuzzTriggerProblems(target, pkg string, paths map[string][]string) []string {
	want := strings.TrimPrefix(pkg, "./") + "/**"
	var problems []string
	for _, trigger := range []string{"push", "pull_request"} {
		if !slices.Contains(paths[trigger], want) {
			problems = append(problems, target+": fuzz.yml's "+trigger+` paths do not include "`+want+`", so a change to it does not start a run on `+trigger)
		}
	}
	return problems
}

// fuzz.yml names each target it runs, and a fuzz function that is not named
// there is fuzzed by nothing: `go test` runs a Fuzz function once over its seed
// corpus as an ordinary test and never explores. Two of the five in the
// repository had been added to their package and not to the matrix. A target
// also only runs on a pull request that touches its package's paths, so a
// package missing from the trigger list is fuzzed weekly or never.
func TestEveryFuzzTargetIsRunByTheFuzzWorkflow(t *testing.T) {
	found := map[string]string{} // target -> package, as the workflow writes it
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "webdist", "dist", "site":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Relative first, slash-separated second: on Windows the walk's paths use
		// backslashes, and trimming "../../" from them matched nothing.
		dir, err := filepath.Rel("../..", filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := "./" + filepath.ToSlash(dir)
		for _, m := range regexp.MustCompile(`(?m)^func (Fuzz[A-Za-z0-9_]+)\(\w+ \*testing\.F\)`).FindAllStringSubmatch(string(body), -1) {
			found[m[1]] = pkg
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 {
		t.Fatal("no fuzz targets were found; the test is not reading the tree")
	}

	raw, err := os.ReadFile("../../.github/workflows/fuzz.yml")
	if err != nil {
		t.Fatalf("reading fuzz.yml: %v", err)
	}
	workflow := string(raw)
	rows := map[string]string{} // target -> package, from the matrix
	for _, m := range regexp.MustCompile(`\{ target: ([A-Za-z0-9_]+), package: (\./[^ ]+) \}`).FindAllStringSubmatch(workflow, -1) {
		rows[m[1]] = m[2]
	}

	paths := fuzzTriggerPaths(t, workflow)
	if len(paths["push"]) == 0 || len(paths["pull_request"]) == 0 {
		t.Fatalf("fuzz.yml lists no paths under push (%d) or pull_request (%d); the test is reading its triggers wrongly", len(paths["push"]), len(paths["pull_request"]))
	}

	var problems []string
	for target, pkg := range found {
		switch got, ok := rows[target]; {
		case !ok:
			problems = append(problems, target+" ("+pkg+") has no row in fuzz.yml's matrix, so nothing fuzzes it")
		case got != pkg:
			problems = append(problems, target+" is in "+pkg+" but the matrix runs it in "+got)
		}
		problems = append(problems, fuzzTriggerProblems(target, pkg, paths)...)
	}
	for target := range rows {
		if _, ok := found[target]; !ok {
			problems = append(problems, "the matrix runs "+target+", which no test file defines")
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("fuzz.yml and the fuzz targets disagree:\n  %s", strings.Join(problems, "\n  "))
	}
}

// The path check is worth having only if dropping a package from one trigger is
// noticed. The first version searched the whole file for the path, found it in
// the push list, and stayed green with the pull_request list one entry short.
// This deletes the entry from the real workflow's pull_request paths alone, and
// then from the push paths alone, and expects each to be named.
func TestTheFuzzTriggerCheckNoticesAPackageDroppedFromOneTrigger(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/fuzz.yml")
	if err != nil {
		t.Fatalf("reading fuzz.yml: %v", err)
	}
	const (
		target = "a target in the kennel workflow package"
		pkg    = "./internal/kennel/workflow"
		entry  = "      - \"internal/kennel/workflow/**\"\n"
	)
	if problems := fuzzTriggerProblems(target, pkg, fuzzTriggerPaths(t, string(raw))); len(problems) != 0 {
		t.Fatalf("the workflow as it stands is already short of a path: %v", problems)
	}
	for _, trigger := range []string{"push", "pull_request"} {
		head, block, found := strings.Cut(string(raw), "  "+trigger+":\n")
		if !found || !strings.Contains(block, entry) {
			t.Fatalf("fuzz.yml has no %s block with the entry %q to delete; the test is reading it wrongly", trigger, entry)
		}
		mutated := head + "  " + trigger + ":\n" + strings.Replace(block, entry, "", 1)
		problems := fuzzTriggerProblems(target, pkg, fuzzTriggerPaths(t, mutated))
		if len(problems) != 1 || !strings.Contains(problems[0], trigger+" paths") {
			t.Errorf("with %s dropped from the %s paths alone, the check reported %v, want exactly that trigger named", pkg, trigger, problems)
		}
	}
}
