package backend

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The entrypoint pipes the runner's output through awk to pick the build log
// out of it, and the log is only live if that awk hands each line on as it
// arrives. gawk does. mawk, the awk a Debian-family image starts with, waits for
// a large block of input first (256 KiB in Ubuntu 24.04's), so a build's log
// stayed empty until the build ended, on every apt image, for want of one
// package. Its -W interactive mode is no remedy either: 24.04's release drops
// lines after one longer than 4 KiB. The package list is where this is decided,
// and putting mawk back would read as a tidy-up.
func TestEveryRunnerImageFamilyInstallsGawkForTheBuildLog(t *testing.T) {
	// A Windows checkout gives the script CRLF line endings, and the Windows job
	// in CI runs this package. The branches are found by the newline before
	// their label, so the endings are made the shell's own first.
	script := strings.ReplaceAll(readFile(t, filepath.Join("..", "..", "deploy", "runner-packages.sh")), "\r\n", "\n")
	for _, family := range []string{"apt", "dnf"} {
		_, branch, ok := strings.Cut(script, "\n  "+family+")\n")
		if !ok {
			t.Fatalf("deploy/runner-packages.sh has no %s branch the way this test reads it", family)
		}
		branch, _, _ = strings.Cut(branch, "\n    ;;")
		// Comments name mawk to say why it is absent, so only what the shell
		// actually runs is read.
		var code []string
		for _, line := range strings.Split(branch, "\n") {
			line, _, _ = strings.Cut(line, "#")
			code = append(code, line)
		}
		packages := strings.Fields(strings.Join(code, " "))
		if !slices.Contains(packages, "gawk") {
			t.Errorf("deploy/runner-packages.sh does not install gawk on the %s images; without it the live build log stays empty until a build ends. Add gawk to that list", family)
		}
		if slices.Contains(packages, "mawk") {
			t.Errorf("deploy/runner-packages.sh installs mawk on the %s images; it holds the live build log back until a build ends. Install gawk instead", family)
		}
	}
}
