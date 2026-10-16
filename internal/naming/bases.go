package naming

import (
	"fmt"
	"regexp"
	"strings"
)

// A runner image is built FROM a base pinned by digest, because a tag moves and
// the person pulling an image in six months should get the base it was tested
// on. The cost of a pin is that it never moves on its own: Dependabot cannot see
// these (they reach the Dockerfile as build arguments, from the table in
// images.go), so five of the seven were behind their registries, including every
// Ubuntu and Debian base, until something refreshed them.
//
// RefreshBases is that refresh as a pure function of the table's source and a
// resolver, so the part that decides what changes can be tested without a
// registry. The generator in gen_base_digests.go supplies the real one, and
// `make generate` then rewrites everything derived from the table.

var baseReference = regexp.MustCompile(`Base: "([^"@\s]+)@(sha256:[0-9a-f]{64})"`)
var digestForm = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// BaseChange is one base whose pin moved.
type BaseChange struct {
	Ref      string // the image and tag, such as ubuntu:24.04
	Old, New string // the digests, as sha256:...
}

// RefreshBases returns images.go's source with every base's digest replaced by
// what resolve says its tag points at today, and the bases that moved. resolve
// is given the reference without its digest. If it fails for any base nothing
// is returned, so a registry that is down for the fourth of seven cannot leave
// a table with three new pins and four old ones.
func RefreshBases(source string, resolve func(ref string) (string, error)) (string, []BaseChange, error) {
	found := baseReference.FindAllStringSubmatch(source, -1)
	if len(found) == 0 {
		return "", nil, fmt.Errorf("no base image with a digest was found; the table's shape has changed and this needs to follow it")
	}
	current := map[string]string{} // ref -> what the tag points at now
	for _, m := range found {
		ref := m[1]
		if _, done := current[ref]; done {
			continue
		}
		digest, err := resolve(ref)
		if err != nil {
			return "", nil, fmt.Errorf("resolving %s: %w", ref, err)
		}
		if !digestForm.MatchString(digest) {
			return "", nil, fmt.Errorf("resolving %s: %q is not a sha256 digest", ref, digest)
		}
		current[ref] = digest
	}

	var changes []BaseChange
	seen := map[string]bool{}
	updated := baseReference.ReplaceAllStringFunc(source, func(match string) string {
		m := baseReference.FindStringSubmatch(match)
		ref, old := m[1], m[2]
		if now := current[ref]; now != old {
			if !seen[ref] {
				seen[ref] = true
				changes = append(changes, BaseChange{Ref: ref, Old: old, New: now})
			}
			return strings.Replace(match, old, now, 1)
		}
		return match
	})
	return updated, changes, nil
}
