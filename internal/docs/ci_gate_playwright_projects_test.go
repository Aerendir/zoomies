package docs

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The Playwright config declares its projects and CI chooses which of them to
// run, in two places that nothing connected. The `transfer` and
// `transfer-mobile` projects were excluded from the two groups that run the
// ordinary specs, by the config's own testIgnore, and were never added to the
// third, so the spec behind them ran in no job. It passes; it simply was not
// asked. This compares the two lists so a project that nobody runs is a failed
// test instead of a silence.
func TestCIRunsEveryPlaywrightProject(t *testing.T) {
	config, err := os.ReadFile("../../web/playwright.config.ts")
	if err != nil {
		t.Fatalf("reading playwright.config.ts: %v", err)
	}
	projects := between(string(config), "projects: [", "webServer:")
	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s{6}name:\s*'([a-z0-9-]+)'`).FindAllStringSubmatch(projects, -1) {
		declared[m[1]] = true
	}
	if len(declared) < 5 {
		t.Fatalf("found %d Playwright projects in the config; the test is not reading it as it was written (%v)", len(declared), keys(declared))
	}

	ci, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("reading ci.yml: %v", err)
	}
	body := string(ci)
	run := map[string]bool{}
	// The two groups that run one project each, by name, from the matrix.
	if m := regexp.MustCompile(`projects: \[([a-z, -]+)\]`).FindStringSubmatch(body); m != nil {
		for _, name := range strings.Split(m[1], ",") {
			if name = strings.TrimSpace(name); name != "other" {
				run[name] = true
			}
		}
	}
	// The third, which lists its projects as flags.
	for _, m := range regexp.MustCompile(`--project=([a-z0-9-]+)`).FindAllStringSubmatch(body, -1) {
		run[m[1]] = true
	}

	var neverRun, unknown []string
	for name := range declared {
		if !run[name] {
			neverRun = append(neverRun, name)
		}
	}
	for name := range run {
		if !declared[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(neverRun)
	sort.Strings(unknown)
	if len(neverRun) > 0 {
		t.Errorf("these Playwright projects are declared in web/playwright.config.ts and run by no CI job; add each to the `other` group's flags in ci.yml:\n  %s",
			strings.Join(neverRun, "\n  "))
	}
	if len(unknown) > 0 {
		t.Errorf("ci.yml runs Playwright projects the config does not declare, which would fail the job:\n  %s", strings.Join(unknown, "\n  "))
	}
}

// between returns the text from the first start to the next end after it.
func between(text, start, end string) string {
	i := strings.Index(text, start)
	if i < 0 {
		return ""
	}
	rest := text[i+len(start):]
	if j := strings.Index(rest, end); j >= 0 {
		return rest[:j]
	}
	return rest
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
