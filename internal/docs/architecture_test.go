package docs

import (
	"bytes"
	"os"
	"testing"
)

// docs/architecture.md is the document AGENTS.md sends every structural
// change to first, and until this test nothing held it to the packages the
// tree actually carries: it stopped at the fleet core, and the assistant,
// repair and analysis subsystems had no architectural rationale anywhere an
// agent was sent to look. The fixed list below is deliberate: every package
// in it is one a structural change is likely to touch wrongly without the
// doc's picture, and a package joining this list is a reviewer's call, made
// in the change that adds the package, not silently by a glob.
func TestArchitectureNamesThePackagesAStructuralChangeTouches(t *testing.T) {
	body, err := os.ReadFile("../../docs/architecture.md")
	if err != nil {
		t.Fatalf("reading docs/architecture.md: %v", err)
	}
	for _, pkg := range []string{
		"internal/registry", "internal/aicontext", "internal/assistant",
		"internal/prrepair", "internal/toolscan", "internal/agentguidance",
		"internal/optimisation", "internal/analysis", "internal/hosttune",
		"internal/catalog",
		// the fleet core stays pinned too, so a rewrite cannot drop it:
		"internal/store", "internal/scheduler", "internal/controller",
		"internal/agent", "internal/api", "internal/events", "internal/backend",
	} {
		if !bytes.Contains(body, []byte("`"+pkg+"`")) {
			t.Errorf("docs/architecture.md no longer names %s; the document AGENTS.md sends structural changes to first has to cover it", pkg)
		}
	}
}
