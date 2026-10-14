package optimisation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOptimisationComputesOnlyFromSuppliedSourceAndEvidence(t *testing.T) {
	allowed := map[string]bool{"crypto/sha256": true, "encoding/hex": true, "errors": true, "path": true, "strconv": true, "strings": true, "github.com/eyupio/zoomies/internal/kennel/workflow": true, "github.com/eyupio/zoomies/internal/prrepair": true}
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
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if !allowed[p] || imp.Name != nil {
				t.Errorf("%s imports unsupported dependency %s", name, p)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			s, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := s.X.(*ast.Ident)
			if !ok {
				return true
			}
			if id.Name == "workflow" && s.Sel.Name != "CheckoutDepthChange" || id.Name == "prrepair" && s.Sel.Name != "File" && s.Sel.Name != "Edit" && s.Sel.Name != "ApplySuppliedEdits" {
				t.Errorf("%s crosses the supplied preview boundary through %s.%s", name, id.Name, s.Sel.Name)
			}
			return true
		})
	}
}
