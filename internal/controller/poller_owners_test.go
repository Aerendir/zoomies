package controller

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/store"
)

// A poller sweep meets many jobs and few repositories, and used to ask the store
// which installation covers a job's repository once for every job: a query
// parsed afresh each time, 13% of a sweep over a hundred known jobs in a profile.
// Which installation covers a repository does not change in the milliseconds a
// sweep takes, so it is asked once for each repository, and not again by the next
// sweep, which asks for itself.

func TestASweepAsksWhichInstallationCoversARepositoryOncePerRepository(t *testing.T) {
	var asked []string
	owners := newRepoOwners(func(_ context.Context, repo string) (string, bool) {
		asked = append(asked, repo)
		return "inst_for_" + repo, true
	})

	for _, repo := range []string{"acme/a", "acme/b", "acme/a", "acme/a", "acme/b", "acme/c", "acme/a"} {
		if got := owners.of(context.Background(), repo); got != "inst_for_"+repo {
			t.Errorf("of(%q) = %q, want the answer for that repository", repo, got)
		}
	}
	if want := []string{"acme/a", "acme/b", "acme/c"}; !slices.Equal(asked, want) {
		t.Errorf("the store was asked about %v, want each repository once, in the order they came: %v", asked, want)
	}
}

// The memo is of no use unless the sweep goes through it: nine jobs in three
// repositories ask three questions, and each job is attributed to the answer for
// its own repository.
func TestASweepGoesThroughTheAnswersForTheInstallationsCoveringItsRepositories(t *testing.T) {
	h := newHarness(t)
	polled := h.installationOn("elsewhere", store.TargetOrg)
	covering := map[string]*store.Installation{}
	for _, repo := range []string{"acme/a", "acme/b", "acme/c"} {
		covering[repo] = h.installationOn(repo, store.TargetRepo)
	}

	asked := map[string]int{}
	owners := newRepoOwners(func(_ context.Context, repo string) (string, bool) {
		asked[repo]++
		return covering[repo].ID, true
	})

	var jobs []github.QueuedJob
	repoOf := map[int64]string{}
	for i, repo := range []string{"acme/a", "acme/b", "acme/c", "acme/a", "acme/b", "acme/c", "acme/a", "acme/b", "acme/c"} {
		id := int64(500 + i)
		repoOf[id] = repo
		jobs = append(jobs, github.QueuedJob{
			ID: id, RunID: 7_000_000 + id, RunNumber: id, Repo: repo, WorkflowName: "CI", JobName: "build",
			Labels: []string{"self-hosted", "linux", "x64"}, QueuedAt: time.Now().Add(-time.Minute),
			HTMLURL: "https://github.com/" + repo + "/actions/runs/1", Status: "queued",
		})
	}

	if _, err := h.c.ingestQueuedJobsWith(h.ctx, polled, jobs, owners); err != nil {
		t.Fatalf("ingestQueuedJobsWith: %v", err)
	}
	for repo, n := range asked {
		if n != 1 {
			t.Errorf("%s was asked about %d times, want once", repo, n)
		}
	}
	if len(asked) != 3 {
		t.Errorf("the sweep asked about %v, want the three repositories it met", asked)
	}
	for id, repo := range repoOf {
		got, err := h.st.GetJobByGitHubID(h.ctx, id)
		if err != nil {
			t.Fatalf("job %d: %v", id, err)
		}
		if got.InstallationID != covering[repo].ID {
			t.Errorf("job %d in %s is attributed to %s, want %s", id, repo, got.InstallationID, covering[repo].ID)
		}
	}
}

// A lookup that failed for a reason other than there being no installation is
// not an answer. The repository's next job asks again, as it always did, rather
// than the whole sweep inheriting one failed read.
func TestAnAnswerThatCouldNotBeSettledIsAskedForAgain(t *testing.T) {
	asks := 0
	owners := newRepoOwners(func(context.Context, string) (string, bool) {
		asks++
		if asks <= 2 {
			return "inst_fallback", false
		}
		return "inst_real", true
	})

	var got []string
	for range 5 {
		got = append(got, owners.of(context.Background(), "acme/a"))
	}
	if want := []string{"inst_fallback", "inst_fallback", "inst_real", "inst_real", "inst_real"}; !slices.Equal(got, want) {
		t.Errorf("answers = %v, want %v", got, want)
	}
	if asks != 3 {
		t.Errorf("the store was asked %d times, want 3: twice for the failures and once for the answer that was kept", asks)
	}
}

// Repositories are told apart as they are written. Folding the case here would
// be a second opinion about what the store treats as one repository, and the
// cost of not doing it is one more question.
func TestRepositoriesWrittenDifferentlyAreAskedAboutSeparately(t *testing.T) {
	asks := 0
	owners := newRepoOwners(func(_ context.Context, repo string) (string, bool) {
		asks++
		return "inst_for_" + repo, true
	})
	for _, repo := range []string{"acme/widgets", "Acme/Widgets", "acme/widgets"} {
		if got := owners.of(context.Background(), repo); got != "inst_for_"+repo {
			t.Errorf("of(%q) = %q", repo, got)
		}
	}
	if asks != 2 {
		t.Errorf("the store was asked %d times, want 2", asks)
	}
}

// Only a failed read is not an answer. A repository that nothing covers is
// answered, with the installation that was polled, and so is one that something
// covers; a lookup that could not be made is neither, and is asked again.
func TestOnlyALookupThatFailedIsNotAnAnswerToKeep(t *testing.T) {
	h := newHarness(t)
	org := h.installationOn("acme", store.TargetOrg)
	polled := h.installationOn("elsewhere", store.TargetOrg)
	cancelled, cancel := context.WithCancel(h.ctx)
	cancel()

	for _, tc := range []struct {
		name    string
		ctx     context.Context
		repo    string
		wantID  string
		settled bool
	}{
		{"a repository something covers", h.ctx, "acme/widgets", org.ID, true},
		{"a repository nothing covers", h.ctx, "stranger/library", polled.ID, true},
		{"a lookup that could not be made", cancelled, "acme/widgets", polled.ID, false},
	} {
		id, settled := h.c.owningInstallation(tc.ctx, tc.repo, polled)
		if id != tc.wantID || settled != tc.settled {
			t.Errorf("%s: owningInstallation = %q, settled %t; want %q, settled %t", tc.name, id, settled, tc.wantID, tc.settled)
		}
	}
}

// What the memo must not change: each polled job is attributed to the
// installation that covers its repository, resolved the way the webhook path
// resolves it, a repository's own installation before its organisation's, and
// the installation that was polled when nothing covers the repository.
func TestPolledJobsAreAttributedToTheInstallationCoveringTheirRepository(t *testing.T) {
	h := newHarness(t)
	org := h.installationOn("acme", store.TargetOrg)
	special := h.installationOn("acme/special", store.TargetRepo)
	polled := h.installationOn("elsewhere", store.TargetOrg)

	var jobs []github.QueuedJob
	want := map[int64]string{}
	id := int64(100)
	for _, c := range []struct {
		repo string
		by   *store.Installation
	}{
		{"acme/widgets", org}, {"acme/special", special}, {"stranger/library", polled}, {"acme/gadgets", org},
	} {
		for range 3 {
			id++
			jobs = append(jobs, github.QueuedJob{
				ID: id, RunID: 7_000_000 + id, RunNumber: id, Repo: c.repo, WorkflowName: "CI", JobName: "build",
				Labels: []string{"self-hosted", "linux", "x64"}, QueuedAt: time.Now().Add(-time.Minute),
				HTMLURL: "https://github.com/" + c.repo + "/actions/runs/1", Status: "queued",
			})
			want[id] = c.by.ID
		}
	}

	if _, err := h.c.ingestQueuedJobs(h.ctx, polled, jobs); err != nil {
		t.Fatalf("ingestQueuedJobs: %v", err)
	}
	for _, q := range jobs {
		got, err := h.st.GetJobByGitHubID(h.ctx, q.ID)
		if err != nil {
			t.Fatalf("job %d: %v", q.ID, err)
		}
		if got.InstallationID != want[q.ID] {
			t.Errorf("job %d in %s is attributed to %s, want %s", q.ID, q.Repo, got.InstallationID, want[q.ID])
		}
	}
}

// The memo lives as long as one sweep. An installation added for a repository
// between two sweeps is the one the next sweep attributes its jobs to.
func TestTheNextSweepAsksAgainWhichInstallationCoversARepository(t *testing.T) {
	h := newHarness(t)
	org := h.installationOn("acme", store.TargetOrg)
	jobs := func(first int64) []github.QueuedJob {
		out := make([]github.QueuedJob, 3)
		for i := range out {
			n := first + int64(i)
			out[i] = github.QueuedJob{
				ID: n, RunID: 7_000_000 + n, RunNumber: n, Repo: "acme/widgets", WorkflowName: "CI", JobName: "build",
				Labels: []string{"self-hosted", "linux", "x64"}, QueuedAt: time.Now().Add(-time.Minute),
				HTMLURL: "https://github.com/acme/widgets/actions/runs/1", Status: "queued",
			}
		}
		return out
	}
	attributedTo := func(first int64) []string {
		var got []string
		for i := range int64(3) {
			j, err := h.st.GetJobByGitHubID(h.ctx, first+i)
			if err != nil {
				t.Fatalf("job %d: %v", first+i, err)
			}
			got = append(got, j.InstallationID)
		}
		return got
	}

	if _, err := h.c.ingestQueuedJobs(h.ctx, org, jobs(10)); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if got, want := attributedTo(10), []string{org.ID, org.ID, org.ID}; !slices.Equal(got, want) {
		t.Fatalf("first sweep attributed the jobs to %v, want %v", got, want)
	}

	own := h.installationOn("acme/widgets", store.TargetRepo)
	if _, err := h.c.ingestQueuedJobs(h.ctx, org, jobs(20)); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if got, want := attributedTo(20), []string{own.ID, own.ID, own.ID}; !slices.Equal(got, want) {
		t.Errorf("the sweep after the repository got an installation of its own attributed the jobs to %v, want %v", got, want)
	}
}
