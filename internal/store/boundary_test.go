package store

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// AGENTS.md: this package is the only place SQL is written, and no other
// package imports database/sql. The rule is what lets a schema change, a
// transaction rule or a lock be reasoned about in one directory, and what keeps
// the single writer (see AGENTS.md here) single. It is the sort of rule that
// holds until the first change that is easier with a query written where the
// data is wanted, so this fails on the import, not on the review that missed it.
//
// Tests are exempt: a test may open a database file to check what the store
// wrote, and several do. Only code that ships is held to it.
func TestNoPackageButTheStoreImportsDatabaseSQL(t *testing.T) {
	forbidden := map[string]string{
		"database/sql":       "SQL is written in internal/store only",
		"modernc.org/sqlite": "the SQLite driver is registered by internal/store only",
	}
	var offenders []string
	scanned := 0
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := repoRelative(t, path)
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "webdist", "dist", "site":
				return filepath.SkipDir
			}
			if rel == "internal/store" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		scanned++
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parsing %s: %v", rel, err)
			return nil
		}
		for _, imp := range file.Imports {
			quoted, _ := strconv.Unquote(imp.Path.Value)
			if why, bad := forbidden[quoted]; bad {
				offenders = append(offenders, rel+" imports "+quoted+": "+why)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 100 {
		t.Fatalf("scanned only %d Go files; the walk is not reading the repository", scanned)
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Errorf("only internal/store may touch the database; move the query into the store and call it:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// repoRelative is a path under the repository root as a slash-separated name.
// The path is made relative with filepath.Rel and only then slash-separated:
// on Windows the walk's paths use backslashes, so trimming a "../../" prefix
// from them matched nothing, and the package that holds the rule was scanned.
func repoRelative(t *testing.T, path string) string {
	t.Helper()
	rel, err := filepath.Rel("../..", path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}
