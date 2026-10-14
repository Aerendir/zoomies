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
