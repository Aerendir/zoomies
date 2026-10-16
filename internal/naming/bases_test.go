package naming

import (
	"errors"
	"os"
	"strings"
	"testing"
)

const (
	oldDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	newDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

const sample = `var runnerImages = []Image{
	{OS: OSUbuntu, Version: "24.04", Base: "ubuntu:24.04@` + oldDigest + `", Family: FamilyAPT},
	{OS: OSDebian, Version: "13", Base: "debian:13-slim@` + oldDigest + `", Family: FamilyAPT},
}
`

func TestRefreshBasesMovesOnlyThePinsThatChanged(t *testing.T) {
	resolve := func(ref string) (string, error) {
		if ref == "ubuntu:24.04" {
			return newDigest, nil
		}
		return oldDigest, nil // debian has not moved
	}
	got, changes, err := RefreshBases(sample, resolve)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(sample, "ubuntu:24.04@"+oldDigest, "ubuntu:24.04@"+newDigest, 1)
	if got != want {
		t.Errorf("RefreshBases rewrote more or less than the one pin that moved:\n%s", got)
	}
	if len(changes) != 1 || changes[0] != (BaseChange{Ref: "ubuntu:24.04", Old: oldDigest, New: newDigest}) {
		t.Errorf("changes = %+v, want exactly the ubuntu pin", changes)
	}
}

func TestRefreshBasesChangesNothingWhenNothingMoved(t *testing.T) {
	got, changes, err := RefreshBases(sample, func(string) (string, error) { return oldDigest, nil })
	if err != nil || got != sample || len(changes) != 0 {
		t.Errorf("a refresh that found nothing new returned changes=%v err=%v and altered the source: %v", changes, err, got != sample)
	}
}

// A registry that answers for some bases and not others must not leave the
// table half refreshed: some bases on new pins and some on old ones is a state
// nobody tested.
func TestRefreshBasesLeavesNothingHalfDone(t *testing.T) {
	calls := 0
	got, changes, err := RefreshBases(sample, func(ref string) (string, error) {
		calls++
		if calls == 2 {
			return "", errors.New("the registry answered 503")
		}
		return newDigest, nil
	})
	if err == nil || !strings.Contains(err.Error(), "debian:13-slim") {
		t.Fatalf("err = %v, want one naming the base that failed", err)
	}
	if got != "" || changes != nil {
		t.Error("a failed refresh returned a source or changes, so a caller could write a half-refreshed table")
	}
}

func TestRefreshBasesRefusesAnAnswerThatIsNotADigest(t *testing.T) {
	for _, bad := range []string{"", "latest", "sha256:abc", "md5:" + strings.Repeat("a", 32)} {
		if _, _, err := RefreshBases(sample, func(string) (string, error) { return bad, nil }); err == nil {
			t.Errorf("a resolver answering %q was accepted and would have been written into the table", bad)
		}
	}
}

func TestRefreshBasesNoticesWhenTheTableChangedShape(t *testing.T) {
	if _, _, err := RefreshBases("var runnerImages = []Image{}\n", func(string) (string, error) { return newDigest, nil }); err == nil {
		t.Error("a source with no pinned base was treated as already refreshed, which would make the weekly job succeed forever without doing anything")
	}
}

// The real table, not a sample: the pattern has to find every row, or the
// refresh would quietly skip an image added in a shape it does not match.
func TestRefreshBasesFindsEveryImageInTheRealTable(t *testing.T) {
	source, err := os.ReadFile("images.go")
	if err != nil {
		t.Fatal(err)
	}
	asked := map[string]bool{}
	got, changes, err := RefreshBases(string(source), func(ref string) (string, error) {
		asked[ref] = true
		// Echo what is pinned, so nothing changes.
		for _, m := range baseReference.FindAllStringSubmatch(string(source), -1) {
			if m[1] == ref {
				return m[2], nil
			}
		}
		return "", errors.New("unknown")
	})
	if err != nil || got != string(source) || len(changes) != 0 {
		t.Fatalf("an echoing resolver changed the real table: err=%v changes=%v", err, changes)
	}
	if len(asked) != len(Images()) {
		t.Errorf("RefreshBases asked about %d bases and the catalogue has %d images", len(asked), len(Images()))
	}
}
