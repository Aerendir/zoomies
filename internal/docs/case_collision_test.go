package docs

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// `.jules/` and `.Jules/` were both tracked for three weeks. Nothing broke on a
// case-sensitive filesystem, which is why nobody noticed: on macOS and Windows,
// whose defaults are case-insensitive, the two merge into one directory without
// a word, and the first file added to both with the same name would collide in
// a checkout the author never makes. This fails on any two tracked paths that
// differ only by case, in the file name or in any directory above it.
func TestNoTwoTrackedPathsDifferOnlyByCase(t *testing.T) {
	out, err := exec.Command("git", "-C", "../..", "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("not a git checkout, so there is no list of tracked paths to compare: %v", err)
	}

	// spellings maps each path prefix, lower-cased, to every way it is written.
	spellings := map[string]map[string]bool{}
	for _, path := range strings.Split(string(out), "\x00") {
		if path == "" {
			continue
		}
		parts := strings.Split(path, "/")
		for i := range parts {
			prefix := strings.Join(parts[:i+1], "/")
			key := strings.ToLower(prefix)
			if spellings[key] == nil {
				spellings[key] = map[string]bool{}
			}
			spellings[key][prefix] = true
		}
	}

	var collisions []string
	for _, written := range spellings {
		if len(written) < 2 {
			continue
		}
		var names []string
		for name := range written {
			names = append(names, name)
		}
		sort.Strings(names)
		collisions = append(collisions, strings.Join(names, "  vs  "))
	}
	sort.Strings(collisions)
	if len(collisions) > 0 {
		t.Errorf("these paths differ only by case; a case-insensitive checkout cannot hold both, so keep one spelling:\n  %s",
			strings.Join(collisions, "\n  "))
	}
}
