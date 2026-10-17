package proxmoxsetup

import (
	_ "embed"
	"strings"
)

// releaseSignatureCheck is the shell that holds a release's checksums to the
// release signing key before they are believed. It is the canonical copy:
// connect-proxmox.sh and deploy/instapods/fetch.sh are standalone files that
// cannot share code with this package at run time, so they carry the same text,
// and a test in internal/docs holds all of them equal and runs it against real
// signatures. The full command Command writes carries it too, without the
// comments.
//
// It is a shell file of its own, not a Go string, so that it is read, linted and
// diffed as shell. The key it names is checked against internal/releasekey by
// the same test.
//
//go:embed release_signature.sh
var releaseSignatureCheck string

// releaseSignatureCode is the check without its comment lines. The full command
// is read and pasted by a person, and what the check is for is explained in the
// script they could have run instead and in the file this comes from.
var releaseSignatureCode = withoutComments(releaseSignatureCheck)

func withoutComments(script string) string {
	var out []string
	for _, line := range strings.Split(strings.TrimSuffix(script, "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		// Comments were the only thing separating the two functions, and the
		// blank lines they leave behind are not worth keeping.
		if strings.TrimSpace(line) == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n") + "\n"
}
