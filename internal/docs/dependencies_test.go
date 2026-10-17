package docs

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// docs/dependencies.md promises that every dependency carries a one-line
// reason, and until this test that was kept by review alone. It slipped twice:
// a direct module that only a test imports had no row, and a row went on naming
// a version two releases after go.mod had moved on. The doc is the place an
// owner decides whether a module still earns its keep, so it has to list the
// modules that are there and none that are gone.
//
// Only the Go table is checked. The UI's rows group packages that ship together
// (`@xterm/xterm` with its addons), which a name-for-name comparison would
// misread, and a row for a package that was dropped is caught by review of the
// change that dropped it.
const dependenciesDoc = "../../docs/dependencies.md"

var (
	// One module path at the start of a table row's first cell.
	dependencyRow = regexp.MustCompile("(?m)^\\| `([^`]+)` \\|")
	// One requirement inside a require block, or a single-line require.
	requirement = regexp.MustCompile(`^\s*(?:require\s+)?(\S+)\s+v\S+(\s+//\s*indirect)?\s*$`)
)

func TestEveryDirectGoModuleHasARowInTheDependenciesPage(t *testing.T) {
	direct, all := goModules(t)
	page, err := os.ReadFile(dependenciesDoc)
	if err != nil {
		t.Fatalf("reading %s: %v", dependenciesDoc, err)
	}
	goTable := section(string(page), "## Go", "## ")
	rows := map[string]bool{}
	for _, m := range dependencyRow.FindAllStringSubmatch(goTable, -1) {
		rows[m[1]] = true
	}
	if len(rows) == 0 {
		t.Fatal("no rows were found under the Go heading of docs/dependencies.md; the page changed shape")
	}

	var missing, gone []string
	for module := range direct {
		if !rows[module] {
			missing = append(missing, module)
		}
	}
	for row := range rows {
		if !all[row] {
			gone = append(gone, row)
		}
	}
	sort.Strings(missing)
	sort.Strings(gone)
	if len(missing) > 0 {
		t.Errorf("go.mod requires these directly and docs/dependencies.md gives no reason for them; add a row saying why each earns its place:\n  %s",
			strings.Join(missing, "\n  "))
	}
	if len(gone) > 0 {
		t.Errorf("docs/dependencies.md has rows for modules go.mod no longer requires; remove them with the code that stopped using them:\n  %s",
			strings.Join(gone, "\n  "))
	}
}

// goModules returns every module go.mod requires directly, and every one it
// requires at all.
func goModules(t *testing.T) (direct, all map[string]bool) {
	t.Helper()
	body, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	direct, all = map[string]bool{}, map[string]bool{}
	inBlock := false
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "require ("):
			inBlock = true
			continue
		case inBlock && trimmed == ")":
			inBlock = false
			continue
		case !inBlock && !strings.HasPrefix(trimmed, "require "):
			continue
		}
		m := requirement.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		all[m[1]] = true
		if m[2] == "" {
			direct[m[1]] = true
		}
	}
	if len(direct) == 0 {
		t.Fatal("no direct requirements were found in go.mod; the test is reading it wrongly")
	}
	return direct, all
}

// section returns the text from the first line starting with start up to the
// next line starting with next, or to the end.
func section(text, start, next string) string {
	i := strings.Index(text, "\n"+start+"\n")
	if i < 0 {
		return ""
	}
	rest := text[i+1+len(start):]
	if j := strings.Index(rest, "\n"+next); j >= 0 {
		return rest[:j]
	}
	return rest
}
