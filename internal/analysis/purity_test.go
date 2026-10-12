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

func TestAnalysisHasNoIOOrImplicitClock(t *testing.T) {
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
			switch path {
			case "errors", "sort":
			case "time":
				if imp.Name != nil {
					timeAlias = imp.Name.Name
				}
			default:
				t.Errorf("%s imports %s; analysis must not fetch its own evidence", name, path)
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
