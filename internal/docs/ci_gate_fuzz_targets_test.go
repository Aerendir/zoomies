package docs

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

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
		pkg := "./" + filepath.ToSlash(strings.TrimPrefix(filepath.Dir(path), "../../"))
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

	var problems []string
	for target, pkg := range found {
		switch got, ok := rows[target]; {
		case !ok:
			problems = append(problems, target+" ("+pkg+") has no row in fuzz.yml's matrix, so nothing fuzzes it")
		case got != pkg:
			problems = append(problems, target+" is in "+pkg+" but the matrix runs it in "+got)
		}
		if trigger := `"` + strings.TrimPrefix(pkg, "./") + `/**"`; !strings.Contains(workflow, trigger) {
			problems = append(problems, target+": fuzz.yml's paths do not include "+trigger+", so a change to it does not start a run")
		}
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
