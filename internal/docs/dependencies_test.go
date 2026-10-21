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
	// One requirement inside a require block, or a single-line require: the
	// module path, the version, and the marker that says the module is
	// indirect.
	requirement = regexp.MustCompile(`^\s*(?:require\s+)?(\S+)\s+(v\S+?)(\s+//\s*indirect)?\s*$`)
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
	direct, all = map[string]bool{}, map[string]bool{}
	for _, m := range requirementLines(t) {
		all[m[1]] = true
		if m[3] == "" {
			direct[m[1]] = true
		}
	}
	if len(direct) == 0 {
		t.Fatal("no direct requirements were found in go.mod; the test is reading it wrongly")
	}
	return direct, all
}

// requirementLines returns every requirement go.mod states, in the order it
// states them. The block structure is read as well as the lines, so a comment
// that looks like a requirement is not taken for one.
func requirementLines(t *testing.T) [][]string {
	t.Helper()
	body, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	var found [][]string
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
		found = append(found, m)
	}
	return found
}

// moduleVersion returns the version go.mod requires directly for a module.
func moduleVersion(t *testing.T, name string) string {
	t.Helper()
	for _, m := range requirementLines(t) {
		if m[1] == name && m[3] == "" {
			return m[2]
		}
	}
	t.Fatalf("go.mod requires no direct module named %s; the test is reading it wrongly", name)
	return ""
}

// rowFor returns the row a module has in the Go table of the dependencies
// page, or the empty string when it has none.
func rowFor(text, module string) string {
	for _, line := range strings.Split(section(text, "## Go", "## "), "\n") {
		if strings.HasPrefix(line, "| `"+module+"` |") {
			return line
		}
	}
	return ""
}

// The tailscale.com pin rides a pre-release channel, and a pin on that
// channel brings a module graph with it: the version names one module, and the
// hundred and thirty indirect ones arrive alongside it. Nothing in the
// pipeline sees that arrival, because govulncheck and osv-scanner read what
// is there, not what arrived, so the row in docs/dependencies.md is the one
// place an owner defends the pin. It has to name the channel and the rule, or
// a bump can change the graph without anybody being asked about it.
func TestTheTailscalePinNamesItsChannel(t *testing.T) {
	version := moduleVersion(t, "tailscale.com")
	if !strings.Contains(version, "-pre") {
		// A stable tag is an owner decision that leaves no channel policy to
		// defend, so there is nothing for the row to have to say.
		return
	}
	page, err := os.ReadFile(dependenciesDoc)
	if err != nil {
		t.Fatalf("reading %s: %v", dependenciesDoc, err)
	}
	row := rowFor(string(page), "tailscale.com")
	if row == "" {
		t.Fatalf("docs/dependencies.md has no row for tailscale.com; the page changed shape")
	}
	// The row may be reworded as long as it keeps naming the channel and the
	// graph rule, so the test reads the two markers and not the wording.
	for _, marker := range []string{"pre-release", "changes what it pulls in says so"} {
		if !strings.Contains(row, marker) {
			t.Errorf("the tailscale.com row in docs/dependencies.md does not say %q; the pin rides the pre-release channel and carries a module graph with it, and that row is where an owner defends it:\n  %s",
				marker, row)
		}
	}
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
