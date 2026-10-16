package mcp

import (
	"go/build"
	"slices"
	"strings"
	"testing"
)

// AGENTS.md says a tool calls the REST API through the mcp.API interface and
// nothing else (no store, no controller), "so it can never do what the caller's
// token could not". That holds only while nothing in this package can reach the
// fleet except through that interface, so this lists what the package may import
// and fails on anything else, instead of trusting review to notice a convenient
// import of the store in a tool that "only needs to read one row".
//
// Each entry says why it is safe. A new helper that only computes belongs here
// with a reason; one that reaches the fleet does not.
var mayImport = map[string]string{
	"github.com/eyupio/zoomies/internal/analysis": "evidence-backed findings computed from snapshots handed to it; it reads no fleet state itself",
	"github.com/eyupio/zoomies/internal/version":  "build metadata stamped in at link time",
}

func TestMCPReachesTheFleetOnlyThroughTheAPIInterface(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("reading this package: %v", err)
	}
	if len(pkg.Imports) == 0 {
		t.Fatal("no imports were found at all; the walk is wrong, not the package")
	}
	allowed := make([]string, 0, len(mayImport))
	for path := range mayImport {
		allowed = append(allowed, path)
	}
	slices.Sort(allowed)
	for _, path := range pkg.Imports {
		first, _, _ := strings.Cut(path, "/")
		if strings.Contains(first, ".") && strings.HasPrefix(path, "github.com/eyupio/zoomies/") {
			if _, ok := mayImport[path]; !ok {
				t.Errorf("internal/mcp imports %s; it may import only %s, because everything else it does goes through the mcp.API interface and so cannot exceed the caller's token. A package that only computes is added to mayImport with its reason; one that reads or acts on the fleet is not",
					path, strings.Join(allowed, ", "))
			}
		}
		for _, banned := range []string{"database/sql", "net/http/httptest"} {
			if path == banned {
				t.Errorf("internal/mcp imports %s, which is a way around the API interface", path)
			}
		}
	}
}
