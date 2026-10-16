//go:build ignore

// Command gen_base_digests moves each runner image's base-image pin to what its
// tag points at today.
//
// The catalogue in images.go pins every base by digest, so an image built next
// year is built on the base it was tested on rather than on whatever the tag
// has become. A pin that nothing moves is a pin that ages: Dependabot cannot
// see these (they reach the Dockerfile as build arguments), and five of the
// seven were behind their registries, every Ubuntu and Debian one among them,
// until this existed.
//
// It rewrites only the digests in images.go and says which moved. What is
// derived from the table (the Makefile's variant rows, both workflows' build
// matrices and the table in docs/naming.md) is then rewritten by gen_catalogue.go,
// which is why the usual way to run it is:
//
//	make runner-bases
//
// A pull request opened by the weekly workflow carries the result for a person
// to review. It needs network access to the registries and nothing else: it
// asks for manifests and pulls no layers.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eyupio/zoomies/internal/naming"
	"github.com/eyupio/zoomies/internal/registry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen_base_digests:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, "internal", "naming", "images.go")
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, changes, err := naming.RefreshBases(string(source), registry.Resolver{}.Resolve)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Println("every base is already pinned to what its tag points at")
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return err
	}
	for _, c := range changes {
		fmt.Printf("%-26s %s -> %s\n", c.Ref, short(c.Old), short(c.New))
	}
	fmt.Printf("moved %d of the runner images' bases; run `go run internal/naming/gen_catalogue.go` to rewrite what is derived from them\n", len(changes))
	return nil
}

// short is enough of a digest to recognise it by.
func short(digest string) string { return strings.TrimPrefix(digest, "sha256:")[:12] }

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s: run this from the repository", dir)
		}
		dir = parent
	}
}
