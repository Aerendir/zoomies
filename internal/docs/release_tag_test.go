package docs

import (
	"regexp"
	"testing"

	"github.com/eyupio/zoomies/internal/updates"
)

// Two things decide which tags a release may carry, and they sat in different
// languages: the release workflow, which builds and publishes whatever tag it
// is given, and the updater, which will offer only a tag of exactly three
// numbers. They disagreed, v1.3.6.1 went out as "Latest", and no controller
// could be updated to it. The workflow now refuses a full-release tag the
// updater would not offer, and this holds the two patterns together so that
// neither can be edited alone.
//
// The workflow's pattern is read out of the file, not copied here, so it is the
// one that runs that is compared.
func TestTheReleaseWorkflowAcceptsExactlyTheFullTagsTheUpdaterOffers(t *testing.T) {
	body := workflowFiles(t)["release.yml"]
	if body == "" {
		t.Fatal("release.yml is missing")
	}
	declared := regexp.MustCompile(`FULL_RELEASE_TAG='([^']+)'`).FindStringSubmatch(body)
	if declared == nil {
		t.Fatal("release.yml no longer declares FULL_RELEASE_TAG, so a full release is not held to the updater's tag shape")
	}
	pattern, err := regexp.Compile(declared[1])
	if err != nil {
		t.Fatalf("FULL_RELEASE_TAG in release.yml is not a regular expression: %v", err)
	}
	if !regexp.MustCompile(`=~ \$FULL_RELEASE_TAG`).MatchString(body) {
		t.Error("release.yml declares FULL_RELEASE_TAG and never matches the tag against it")
	}

	for _, tag := range []string{
		// Accepted by both.
		"v1.3.5", "v10.0.12", "v0.0.0",
		// The tag that was published as "Latest" and that no updater would offer.
		"v1.3.6.1",
		"V1.3.5", "1.3.5", "v1.3", "v1.3.5-rc1", "v1.3.5+build",
		"v1.3.5\n", "v1.3.5\nprerelease=false", " v1.3.5", "v1.3.5 ", "v1.3.5;x",
		"v1..5", "v1.-3.5", "",
	} {
		if got, want := pattern.MatchString(tag), updates.ValidTag(tag); got != want {
			t.Errorf("release.yml's FULL_RELEASE_TAG says %v for %q and updates.ValidTag says %v; the workflow would publish a release the updater cannot offer, or refuse one it could",
				got, tag, want)
		}
	}
}
