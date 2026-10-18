package controller

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// unguardedGoroutines are the functions whose goroutine is deliberately not
// behind recoverDetached, with how many each may start and why. A new entry
// wants a reason as good as these.
var unguardedGoroutines = map[string]struct {
	count  int
	reason string
}{
	"spawn": {1, "runs its loop under runGuarded, which starts it again after a wait and records the crash"},
	"Stop":  {1, "only waits on the WaitGroups, so a panic there is a bug in the wait itself and the shutdown should be loud about it"},
}

// A goroutine that panics ends the process unless something on that goroutine
// recovers it, and neither a loop's guard nor net/http's recovery of a request
// reaches one that was started from either. The work that is handed off this
// way is the work that matters most: a provider call, a credential minted at
// GitHub, a delivery. So every goroutine the controller starts has its guard
// where the next person to read the go statement can see it.
func TestEveryGoroutineTheControllerStartsIsGuardedAgainstAPanic(t *testing.T) {
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
		for _, problem := range unguardedGoStatements(fset, file) {
			t.Error(problem)
		}
	}
	if files == 0 {
		t.Fatal("no source files were scanned at all; the walk is wrong, not the rule")
	}
}

// unguardedGoStatements names each go statement in file that is not behind a
// guard, in the words of the failure.
func unguardedGoStatements(fset *token.FileSet, file *ast.File) []string {
	var problems []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		allowed, isAllowed := unguardedGoroutines[fn.Name.Name]
		seen := 0
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			g, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			if isAllowed {
				seen++
				if seen > allowed.count {
					problems = append(problems, fmt.Sprintf("%s: %s may start %d unguarded goroutine(s) (%s) and this is another",
						fset.Position(g.Pos()), fn.Name.Name, allowed.count, allowed.reason))
				}
				return true
			}
			lit, ok := g.Call.Fun.(*ast.FuncLit)
			switch {
			case !ok:
				problems = append(problems, fmt.Sprintf("%s: go starts a named function; start a func literal that defers recoverDetached, so the guard can be seen at the go statement",
					fset.Position(g.Pos())))
			case !defersAGuard(lit.Body):
				problems = append(problems, fmt.Sprintf("%s: this goroutine is not guarded against a panic; defer c.recoverDetached(name) in it, or recover in a deferred function",
					fset.Position(g.Pos())))
			}
			return true
		})
	}
	return problems
}

// defersAGuard reports whether one of the statements the goroutine opens with
// defers recoverDetached, or a function that calls recover. It has to be a
// defer in the goroutine's own body: a guard in a function it calls would not
// stop a panic in the goroutine itself.
func defersAGuard(body *ast.BlockStmt) bool {
	for _, stmt := range body.List {
		d, ok := stmt.(*ast.DeferStmt)
		if !ok {
			continue
		}
		switch fun := d.Call.Fun.(type) {
		case *ast.SelectorExpr:
			if fun.Sel.Name == "recoverDetached" {
				return true
			}
		case *ast.FuncLit:
			if callsRecover(fun.Body) {
				return true
			}
		}
	}
	return false
}

func callsRecover(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "recover" {
				found = true
			}
		}
		return !found
	})
	return found
}

// The check is only worth having if it would catch what it names, so this
// hands it a goroutine of each shape it must refuse and each it must leave
// alone.
func TestTheGoroutineCheckRefusesWhatIsNotGuarded(t *testing.T) {
	check := func(src string) []string {
		t.Helper()
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "x.go", "package controller\n"+src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %q: %v", src, err)
		}
		return unguardedGoStatements(fset, file)
	}

	for name, src := range map[string]string{
		"a bare function literal":                  `func (c *Controller) a() { go func() { work() }() }`,
		"a named function":                         `func (c *Controller) a() { go c.work() }`,
		"a guard that is called and not deferred":  `func (c *Controller) a() { go func() { c.recoverDetached("x"); work() }() }`,
		"a deferred closure that does not recover": `func (c *Controller) a() { go func() { defer func() { done() }(); work() }() }`,
		"a second goroutine where one is allowed":  `func (c *Controller) spawn() { go func() { work() }(); go func() { work() }() }`,
	} {
		if got := check(src); len(got) != 1 {
			t.Errorf("%s: raised %d complaints, want 1: %v", name, len(got), got)
		}
	}
	for name, src := range map[string]string{
		"the helper":                     `func (c *Controller) a() { go func() { defer c.recoverDetached("x"); work() }() }`,
		"the helper behind other defers": `func (c *Controller) a() { go func() { defer wg.Done(); defer c.recoverDetached("x"); work() }() }`,
		"a deferred recover":             `func (c *Controller) a() { go func() { defer func() { if r := recover(); r != nil { note(r) } }(); work() }() }`,
		"the one allowed to go bare":     `func (c *Controller) spawn() { go func() { work() }() }`,
	} {
		if got := check(src); len(got) != 0 {
			t.Errorf("%s: was refused: %v", name, got)
		}
	}
}
