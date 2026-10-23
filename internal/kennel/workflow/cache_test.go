package workflow

import (
	"strings"
	"testing"
)

const cacheWorkflow = `on: push
jobs:
  build:
    name: Build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@` + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" + `
      - name: Node
        uses: actions/setup-node@` + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" + `
        with:
          node-version: 20
          # keep this note
          registry-url: https://registry.npmjs.org
      - run: npm test
`

func TestSetupCacheChangeInsertsTheKeyAndPreservesTheRest(t *testing.T) {
	if _, err := Inspect([]byte(cacheWorkflow)); err != nil {
		t.Fatalf("inspect fixture: %v", err)
	}
	before, after, line, err := SetupCacheChange([]byte(cacheWorkflow), 0, 1, "npm")
	if err != nil {
		t.Fatal(err)
	}
	if line != 11 || !strings.Contains(after, "          cache: npm\n") || !strings.Contains(after, "          node-version: 20\n") || strings.Count(after, "cache: npm") != 1 {
		t.Fatalf("line=%d after=%q", line, after)
	}
	if strings.Count(cacheWorkflow, before) != 1 {
		t.Fatalf("preimage is not a unique contiguous slice: %q", before)
	}
	out := strings.Replace(cacheWorkflow, before, after, 1)
	if !strings.Contains(out, "          cache: npm\n          node-version: 20") {
		t.Fatalf("inserted in the wrong place: %s", out)
	}
}

func TestSetupCacheChangePreservesCRLF(t *testing.T) {
	crlf := strings.ReplaceAll(cacheWorkflow, "\n", "\r\n")
	before, after, _, err := SetupCacheChange([]byte(crlf), 0, 1, "pnpm")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after, "          cache: pnpm\r\n") || strings.Contains(after, "\n") && !strings.Contains(after, "\r\n") {
		t.Fatalf("line endings changed: %q", after)
	}
	if strings.Count(crlf, before) != 1 {
		t.Fatal("CRLF preimage is not unique")
	}
}

func TestSetupCacheChangeRefusesUnsupportedSteps(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
		job  int
		step int
		mgmt string
	}{
		{"unpinned action", strings.ReplaceAll(cacheWorkflow, "actions/setup-node@bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "actions/setup-node@v4"), 0, 1, "npm"},
		{"checkout step", cacheWorkflow, 0, 0, "npm"},
		{"cache already set", strings.ReplaceAll(cacheWorkflow, "          node-version: 20\n", "          node-version: 20\n          cache: npm\n"), 0, 1, "npm"},
		{"flow with", strings.ReplaceAll(cacheWorkflow, "        with:\n          node-version: 20\n          # keep this note\n          registry-url: https://registry.npmjs.org", "        with: { node-version: 20 }"), 0, 1, "npm"},
		{"empty with", strings.ReplaceAll(cacheWorkflow, "        with:\n          node-version: 20\n          # keep this note\n          registry-url: https://registry.npmjs.org", "        with:"), 0, 1, "npm"},
		{"unknown manager", cacheWorkflow, 0, 1, "bun"},
		{"wrong job", cacheWorkflow, 1, 1, "npm"},
		{"wrong step", cacheWorkflow, 0, 9, "npm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, err := SetupCacheChange([]byte(tc.doc), tc.job, tc.step, tc.mgmt); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestSetupCacheChangeRefusesAPreimageThatIsNotUnique(t *testing.T) {
	doc := strings.Replace(cacheWorkflow, "      - run: npm test", "      - uses: actions/setup-node@"+strings.Repeat("b", 40)+"\n        with:\n          node-version: 20\n      - run: npm test", 1)
	if _, _, _, err := SetupCacheChange([]byte(doc), 0, 1, "npm"); err == nil || !strings.Contains(err.Error(), "setup cache") {
		t.Fatalf("accepted an ambiguous preimage: %v", err)
	}
}

func TestFindSetupNodeFindsOneAndRefusesNoneOrTwo(t *testing.T) {
	if job, step, err := FindSetupNode([]byte(cacheWorkflow)); err != nil || job != 0 || step != 1 {
		t.Fatalf("job=%d step=%d err=%v", job, step, err)
	}
	none := strings.ReplaceAll(cacheWorkflow, "actions/setup-node@"+strings.Repeat("b", 40), "actions/setup-node@v4")
	if _, _, err := FindSetupNode([]byte(none)); err != ErrSetupNodeNotFound {
		t.Fatalf("none: %v", err)
	}
	two := strings.Replace(cacheWorkflow, "      - run: npm test", "      - uses: actions/setup-node@"+strings.Repeat("c", 40)+"\n        with:\n          node-version: 20\n      - run: npm test", 1)
	if _, _, err := FindSetupNode([]byte(two)); err != ErrSetupNodeAmbiguous {
		t.Fatalf("two: %v", err)
	}
}
