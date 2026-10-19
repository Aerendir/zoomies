package workflow

import (
	"strings"
	"testing"
)

func TestCheckoutLineEditRefusesAmbiguousYAMLAndUnsupportedInputForms(t *testing.T) {
	base := "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@" + strings.Repeat("a", 40) + "\n        with:\n          fetch-depth: 0\n"
	for _, data := range []string{
		strings.Replace(base, "fetch-depth: 0", "fetch-depth: '0'", 1),
		strings.Replace(base, "fetch-depth: 0", "fetch-depth: ${{ inputs.depth }}", 1),
		strings.Replace(base, "with:\n          fetch-depth: 0", "with: {fetch-depth: 0}", 1),
		base + "          fetch-depth: 0\n",
		base + "---\non: push\n",
		strings.Replace(base, "with:", "with: &inputs", 1),
		base + "          repository: other/repo\n",
		base + "          ref: main\n",
	} {
		if _, _, _, err := CheckoutDepthChange([]byte(data), 0, 0); err == nil {
			t.Fatalf("accepted unsupported source: %s", data)
		}
	}
}

func FuzzCheckoutDepthEditPreservesTheDocument(f *testing.F) {
	f.Add([]byte("on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@" + strings.Repeat("a", 40) + "\n        with:\n          fetch-depth: 0 # comment\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		before, after, _, err := CheckoutDepthChange(data, 0, 0)
		if err != nil {
			return
		}
		if len(before) != len(after) || strings.Count(string(data), before) != 1 {
			t.Fatal("ambiguous edit emitted")
		}
		changed := strings.Replace(string(data), before, after, 1)
		if _, err := Inspect([]byte(changed)); err != nil {
			t.Fatal("edit invalidated supported YAML", err)
		}
		if _, _, _, err := CheckoutDepthChange([]byte(changed), 0, 0); err == nil {
			t.Fatal("preview reapplied to shallow checkout")
		}
	})
}

func TestCheckoutNamesRefuseGeneratedMatrixDynamicAndDuplicateSourceNames(t *testing.T) {
	base := "on: push\njobs:\n  test:\n    name: Build\n    runs-on: ubuntu-latest\n    steps:\n      - name: Checkout\n        uses: actions/checkout@" + strings.Repeat("a", 40) + "\n        with:\n          fetch-depth: 0\n      - name: Test\n        run: echo test\n"
	if j, s, err := CheckoutNames([]byte(base), 0, 0); err != nil || j != "Build" || s != "Checkout" {
		t.Fatal(j, s, err)
	}
	for _, data := range []string{
		strings.Replace(base, "    name: Build\n", "", 1),
		strings.Replace(base, "      - name: Checkout\n        uses:", "      - uses:", 1),
		strings.Replace(base, "    name: Build", "    name: ${{ inputs.name }}", 1),
		strings.Replace(base, "    name: Build", "    strategy: {matrix: {os: [linux]}}\n    name: Build", 1),
		strings.Replace(base, "name: Test", "name: Checkout", 1),
		base + "  other:\n    name: Build\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo other\n",
	} {
		if _, _, err := CheckoutNames([]byte(data), 0, 0); err == nil {
			t.Fatal("ambiguous name association accepted")
		}
	}
}

func TestFindCheckoutChoosesTheOnePreviewableStepAndNeverGuesses(t *testing.T) {
	sha := strings.Repeat("a", 40)
	job := func(key, name string) string {
		return "  " + key + ":\n    name: " + name + "\n    runs-on: ubuntu-latest\n    steps:\n      - name: Checkout\n        uses: actions/checkout@" + sha + "\n        with:\n          fetch-depth: 0\n      - name: Test\n        run: echo test\n"
	}
	lint := "  lint:\n    name: Lint\n    runs-on: ubuntu-latest\n    steps:\n      - name: Vet\n        run: echo vet\n"
	two := "on: push\njobs:\n" + lint + job("test", "Build")
	j, s, err := FindCheckout([]byte(two), "Build")
	if err != nil || j != 1 || s != 0 {
		t.Fatalf("want job 1 step 0, got %d %d %v", j, s, err)
	}
	if _, _, err := FindCheckout([]byte(two), "Deploy"); err != ErrCheckoutNotFound {
		t.Fatalf("a job with no such name: %v", err)
	}
	shallow := strings.Replace(job("test", "Build"), "fetch-depth: 0", "fetch-depth: 1", 1)
	if _, _, err := FindCheckout([]byte("on: push\njobs:\n"+shallow), "Build"); err != ErrCheckoutNotFound {
		t.Fatalf("an already shallow checkout is not a candidate: %v", err)
	}
	twice := strings.Replace(job("test", "Build"), "      - name: Test\n        run: echo test\n", "      - name: Checkout again\n        uses: actions/checkout@"+sha+"\n        with:\n          fetch-depth: 0 # again\n", 1)
	if _, _, err := FindCheckout([]byte("on: push\njobs:\n"+twice), "Build"); err != ErrCheckoutAmbiguous {
		t.Fatalf("two candidates must be refused, not picked: %v", err)
	}
}
