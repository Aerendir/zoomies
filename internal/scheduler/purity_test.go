package scheduler

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
)

// AGENTS.md calls this package pure: Decide takes a snapshot and returns a Plan,
// with no clock read, no database and no network, and that is what makes scaling
// behaviour testable and every decision's reason string the one the decision
// actually had. internal/updates has a test that holds it to the same rule; this
// package, whose reasons operators read most, had only the promise.
//
// It may use the standard library's pure parts, and the two internal packages
// whose types it is written in terms of: the store's domain model (a Runner, a
// Host, a Pool, not the database behind them) and the naming grammar.
var allowedInternal = map[string]string{
	"github.com/eyupio/zoomies/internal/store":  "its domain types; the database behind them is the store's and is never opened here",
	"github.com/eyupio/zoomies/internal/naming": "the grammar for pool, host and runner names, which is pure",
}

// The standard library is not all pure: these reach the network, the disk, a
// process, a database or a source of chance, each of which would let a decision
// depend on something other than the snapshot it was handed. A package listed
// here stands for itself and everything beneath it.
var impurePackages = []string{"net", "os", "database/sql", "math/rand", "crypto/rand", "syscall"}

func TestTheSchedulerImportsNothingThatReachesOutsideTheSnapshot(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("reading this package: %v", err)
	}
	if len(pkg.Imports) == 0 {
		t.Fatal("no imports were found at all; the walk is wrong, not the package")
	}
	for _, problem := range impureImports(pkg.Imports) {
		t.Error(problem)
	}
}

func impureImports(imports []string) []string {
	var problems []string
	for _, path := range imports {
		first, _, _ := strings.Cut(path, "/")
		if strings.Contains(first, ".") {
			if _, ok := allowedInternal[path]; !ok {
				problems = append(problems, "internal/scheduler imports "+path+"; it may import only the standard library's pure parts and "+
					strings.Join(sortedKeys(allowedInternal), ", ")+", and what needs anything else belongs to the controller that calls Decide")
			}
			continue
		}
		for _, banned := range impurePackages {
			if path == banned || strings.HasPrefix(path, banned+"/") {
				problems = append(problems, "internal/scheduler imports "+path+", which reaches outside its snapshot; the network, the disk, a process, a database and a random source belong to the caller")
			}
		}
	}
	return problems
}

func sortedKeys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// The denylist is only worth having if it would catch what it names.
func TestThePurityCheckRefusesWhatReachesOutsideTheSnapshot(t *testing.T) {
	for _, path := range []string{
		"net", "net/http", "os", "os/exec", "database/sql", "math/rand", "math/rand/v2", "crypto/rand", "syscall",
		"github.com/eyupio/zoomies/internal/controller",
		"github.com/eyupio/zoomies/internal/config",
		"github.com/eyupio/zoomies/internal/github",
	} {
		if got := impureImports([]string{path}); len(got) != 1 {
			t.Errorf("importing %s raised %d complaints, want 1", path, len(got))
		}
	}
	for _, path := range []string{
		"fmt", "sort", "strings", "time", "slices", "cmp", "math", "strconv", "unicode",
		"github.com/eyupio/zoomies/internal/store",
		"github.com/eyupio/zoomies/internal/naming",
	} {
		if got := impureImports([]string{path}); len(got) != 0 {
			t.Errorf("importing %s was refused: %v", path, got)
		}
	}
}

// A decision's reason string is built from the snapshot's own instant, so the
// sentence an operator reads and the decision it explains cannot be a
// millisecond apart. A read of the clock inside the package is what would let
// them differ, so the instant arrives as an argument and nothing here asks for it.
func TestTheSchedulerNeverReadsTheClock(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading this package: %v", err)
	}
	files := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files++
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "time" {
				switch sel.Sel.Name {
				case "Now", "Since", "Until", "After", "Sleep", "NewTimer", "NewTicker", "Tick", "AfterFunc":
					t.Errorf("%s: time.%s reads the clock; pass the instant in", fset.Position(sel.Pos()), sel.Sel.Name)
				}
			}
			return true
		})
	}
	if files == 0 {
		t.Fatal("no source files were scanned at all; the walk is wrong, not the rule")
	}
}
