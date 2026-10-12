package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// pureImports are the standard-library packages that compute and do no I/O.
// An analysis that could open a file, a socket or the database would answer
// from whatever it found there, not from the snapshot its caller can show.
var pureImports = map[string]bool{
	"cmp": true, "errors": true, "fmt": true, "math": true, "slices": true,
	"sort": true, "strconv": true, "strings": true, "time": true, "unicode/utf8": true,
}

// Test files are skipped: finding_test.go reads the catalog to prove every
// code a finding names exists, which is a check on the package, not a path
// by which an analysis could reach the fleet.
func TestTheAnalysisPackageIsPure(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		timeAlias := "time"
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			switch {
			case strings.Contains(path, "github.com/eyupio/zoomies/"):
				t.Errorf("%s imports %s; analysis must not reach the fleet; the controller assembles the snapshot", name, path)
			case !pureImports[path]:
				t.Errorf("%s imports %s; analysis must not fetch its own evidence", name, path)
			case path == "time" && imp.Name != nil:
				timeAlias = imp.Name.Name
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Name != timeAlias {
				return true
			}
			switch sel.Sel.Name {
			case "Now", "Since", "Until", "After", "AfterFunc", "Sleep", "Tick", "NewTicker", "NewTimer":
				t.Errorf("%s reads an implicit clock through %s", name, sel.Sel.Name)
			}
			return true
		})
	}
}
