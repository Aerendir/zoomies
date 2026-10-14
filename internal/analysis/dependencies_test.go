package analysis

import (
	"strings"
	"testing"
)

func dependencyPointer[T any](v T) *T { return &v }
func dependencyEvidenceFixture() *RootDependencies {
	return &RootDependencies{Version: "root-dependencies-v1", Basis: "root_uses_declarations", SourceSHA256: strings.Repeat("c", 64), Coverage: "enumerated", RowsOmitted: dependencyPointer(false), Counts: &RootDependencyCounts{Declarations: dependencyPointer(1), CommitPins: dependencyPointer(1), DigestPins: dependencyPointer(0), Floating: dependencyPointer(0), Local: dependencyPointer(0), Expressions: dependencyPointer(0), Unknown: dependencyPointer(0)}, References: []RootDependencyReference{{JobIndex: dependencyPointer(0), StepIndex: dependencyPointer(0), Line: dependencyPointer(6), Declaration: "step_action", Kind: "remote", Pin: "commit", ReferenceSHA256: strings.Repeat("d", 64), Revision: dependencyPointer(strings.Repeat("a", 40))}}}
}
func TestDependencyViewsRequireBoundCountsAndKnownPositionsWithoutDefaultingMissingFacts(t *testing.T) {
	for _, mode := range []string{"valid", "version", "binding", "missing_count", "count", "missing_position", "position", "revision", "fingerprint", "omission"} {
		e := dependencyEvidenceFixture()
		switch mode {
		case "version":
			e.Version = "future"
		case "binding":
			e.SourceSHA256 = strings.Repeat("b", 64)
		case "missing_count":
			e.Counts.Floating = nil
		case "count":
			*e.Counts.CommitPins = 0
		case "missing_position":
			e.References[0].JobIndex = nil
		case "position":
			*e.References[0].StepIndex = -1
		case "revision":
			e.References[0].Revision = nil
		case "fingerprint":
			e.References[0].ReferenceSHA256 = "private"
		case "omission":
			*e.RowsOmitted = true
		}
		if ValidRootDependencies(e, strings.Repeat("c", 64)) != (mode == "valid") {
			t.Fatalf("%s validated incorrectly", mode)
		}
	}
	e := dependencyEvidenceFixture()
	e.References = []RootDependencyReference{}
	*e.RowsOmitted = true
	if !ValidRootDependencies(e, e.SourceSHA256) {
		t.Fatal("valid summary truncation refused")
	}
	e.Coverage = "unsupported"
	e.Counts = nil
	*e.RowsOmitted = false
	if !ValidRootDependencies(e, e.SourceSHA256) {
		t.Fatal("unsupported unknown became invalid or zero")
	}
}
