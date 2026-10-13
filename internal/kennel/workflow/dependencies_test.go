package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func dependencyFixture() []byte {
	return []byte("on: push\njobs:\n  private_job:\n    runs-on: linux\n    steps:\n      - uses: private_owner/private_repo@" + strings.Repeat("a", 40) + "\n      - uses: docker://private_image@sha256:" + strings.Repeat("b", 64) + "\n      - uses: private_owner/private_repo@main\n      - uses: ./private-action\n      - uses: ${{ inputs.private_action }}\n      - uses: unrecognised-private-value\n  private_reusable:\n    uses: private_owner/private_repo/.github/workflows/private.yml@" + strings.Repeat("c", 40) + "\n")
}
func TestRootDependencyInventoryDistinguishesLiteralPinsFromUnresolvedReferencesWithoutNames(t *testing.T) {
	e := InspectDependencies(dependencyFixture())
	if !ValidDependencyInventory(e, e.SourceSHA256) || e.Coverage != "enumerated" || e.Counts == nil || *e.Counts != (DependencyCounts{Declarations: 7, CommitPins: 2, DigestPins: 1, Floating: 1, Local: 1, Expressions: 1, Unknown: 1}) {
		t.Fatalf("wrong root inventory: %+v", e)
	}
	if e.References[6].Declaration != "job_workflow" || e.References[6].StepIndex != -1 || e.References[0].Revision == nil || e.References[3].Revision != nil {
		t.Fatal("invented resolution or lost positions")
	}
	b, _ := json.Marshal(e)
	for _, text := range []string{"private_", "private-", "private.yml", "inputs.", "runs-on", "uses:"} {
		if strings.Contains(string(b), text) {
			t.Fatalf("retained raw repository text %s", text)
		}
	}
}
func TestRootDependencyInventoryDiscardsPartialFactsForUnsupportedYAML(t *testing.T) {
	for _, data := range []string{
		"jobs: {a: {steps: [{uses: actions/checkout@main}, {uses: [bad]}]}}",
		"jobs: {a: {steps: [{uses: actions/checkout@main, run: echo private}]}}",
		"jobs: {a: &private {steps: []}, b: *private}",
		"jobs: {a: {steps: [], steps: []}}", "jobs: {}", "jobs: []",
		"jobs: {a: {steps: []}}\n---\njobs: {}", strings.Repeat("x", MaxBytes+1),
	} {
		e := InspectDependencies([]byte(data))
		if len(data) > MaxBytes {
			if e != nil {
				t.Fatal("oversized data retained")
			}
			continue
		}
		if e.Coverage != "unsupported" || e.Counts != nil || len(e.References) != 0 || e.RowsOmitted || !ValidDependencyInventory(e, e.SourceSHA256) {
			t.Fatalf("partial/zero facts invented: %+v", e)
		}
	}
}
func TestRootDependencyInventoryBoundsRowsWithoutClippingFullCounts(t *testing.T) {
	data := "jobs:\n  private:\n    steps:\n" + strings.Repeat("      - uses: private_owner/private_repo@main\n", 101)
	e := InspectDependencies([]byte(data))
	if !ValidDependencyInventory(e, e.SourceSHA256) || !e.RowsOmitted || len(e.References) != 100 || e.Counts.Declarations != 101 || e.Counts.Floating != 101 {
		t.Fatal("full inventory lost under row bounds")
	}
}
func TestRootDependencyInventoryNeverTreatsMalformedWorkflowOrImageReferencesAsPins(t *testing.T) {
	for _, uses := range []string{"docker://image@sha256:not-a-digest", "docker://https://private@sha256:" + strings.Repeat("a", 64), "owner/repo@" + strings.Repeat("a", 39), "private @" + strings.Repeat("a", 40)} {
		e := InspectDependencies([]byte("jobs: {a: {steps: [{uses: '" + uses + "'}]}}"))
		if !ValidDependencyInventory(e, e.SourceSHA256) || e.Counts.CommitPins != 0 || e.Counts.DigestPins != 0 {
			t.Fatalf("malformed literal promoted: %+v", e)
		}
	}
	for _, uses := range []string{"owner/repo@" + strings.Repeat("a", 40), "docker://image@sha256:" + strings.Repeat("a", 64)} {
		e := InspectDependencies([]byte("jobs: {a: {uses: '" + uses + "'}}"))
		if !ValidDependencyInventory(e, e.SourceSHA256) || e.Counts.Unknown != 1 {
			t.Fatal("non-workflow job reference promoted")
		}
	}
}
func TestRetainedDependencyInventoryRefusesContradictoryCountsBindingsAndPositions(t *testing.T) {
	for _, mode := range []string{"version", "digest", "count", "omitted", "position", "fingerprint", "pin", "revision", "unknown_coverage"} {
		e := InspectDependencies(dependencyFixture())
		original := e.SourceSHA256
		switch mode {
		case "version":
			e.Version = "future"
		case "digest":
			e.SourceSHA256 = strings.Repeat("e", 64)
		case "count":
			e.Counts.CommitPins++
		case "omitted":
			e.RowsOmitted = true
		case "position":
			e.References[1].StepIndex = e.References[0].StepIndex
		case "fingerprint":
			e.References[0].ReferenceSHA256 = "private"
		case "pin":
			e.References[0].Pin = "digest"
		case "revision":
			e.References[0].Revision = nil
		case "unknown_coverage":
			e.Coverage = "complete_transitive_graph"
		}
		if ValidDependencyInventory(e, original) {
			t.Fatalf("%s accepted", mode)
		}
	}
}

func FuzzDependencyInventoriesPreserveTheirRetainedInvariants(f *testing.F) {
	f.Add(dependencyFixture())
	f.Add([]byte("jobs: {a: &x {steps: []}, b: *x}"))
	f.Add([]byte("jobs: {a: {uses: owner/repo/.github/workflows/a.yml@main}}"))
	f.Fuzz(func(t *testing.T, data []byte) {
		e := InspectDependencies(data)
		if len(data) > MaxBytes {
			if e != nil {
				t.Fatal("oversized data retained")
			}
			return
		}
		if e == nil || !ValidDependencyInventory(e, e.SourceSHA256) {
			t.Fatal("parser produced inconsistent private metadata")
		}
	})
}

func TestRootUsesEnumerationDistinguishesKnownZeroFromUnsupportedUnknown(t *testing.T) {
	e := InspectDependencies([]byte("jobs: {private_job: {steps: [{run: 'private script imports dependencies'}]}}"))
	if e.Coverage != "enumerated" || e.Counts == nil || e.Counts.Declarations != 0 || !ValidDependencyInventory(e, e.SourceSHA256) {
		t.Fatal("known absence of root uses became unknown")
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "private") {
		t.Fatal("script text retained")
	}
	unknown := InspectDependencies([]byte("jobs: unsupported"))
	if unknown.Counts != nil || unknown.Coverage != "unsupported" {
		t.Fatal("unsupported inventory became zero")
	}
}
