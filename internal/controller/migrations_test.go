package controller

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/migrate"
	"github.com/eyupio/zoomies/internal/store"
)

// Paging through an organisation has to reach every repository in it, whatever
// case its name is written in.
//
// The cursor is a repository name, and it is compared in lowercase so that the
// one the browser sends back matches whatever GitHub returned. That only works
// if the listing is ordered the same way: sorted by raw bytes, every
// capitalised name sorts before every lowercase one, the binary search for the
// cursor runs over an order it does not share, and the repositories caught
// between the two are returned on no page at all. They are not merely
// misordered -- they are unreachable, and a repository the wizard never lists
// is one nobody can migrate.
func TestMigrationPagingReachesEveryRepositoryWhateverItsCase(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()

	// Names GitHub would really return: some capitalised, some not, mixed
	// together so that no single ordering happens to work by luck.
	var want []string
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("acme/repo%02d", i)
		if i%2 == 0 {
			name = fmt.Sprintf("acme/Repo%02d", i)
		}
		h.gh.AddRepo(name)
		want = append(want, strings.ToLower(name))
	}

	client, err := h.c.ClientFor(h.ctx, inst.ID)
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}

	// A page size that does not divide the fleet evenly, so the last page is a
	// partial one and the cursor is exercised more than once.
	const perPage = 5
	seen := map[string]int{}
	cursor := ""
	for page := 0; ; page++ {
		if page > len(want) {
			t.Fatalf("paging did not terminate after %d pages", page)
		}
		repos, next, total, err := migrationRepos(h.ctx, client, nil, cursor, perPage)
		if err != nil {
			t.Fatalf("migrationRepos(cursor=%q): %v", cursor, err)
		}
		if total != len(want) {
			t.Errorf("total = %d on page %d, want %d: the wizard reports which slice of the organisation is on screen", total, page, len(want))
		}
		for _, r := range repos {
			seen[strings.ToLower(r.FullName)]++
		}
		if next == "" {
			break
		}
		cursor = next
	}

	for _, name := range want {
		switch seen[name] {
		case 1:
		case 0:
			t.Errorf("%s appeared on no page, so it can never be migrated", name)
		default:
			t.Errorf("%s appeared on %d pages, so the wizard offers it twice", name, seen[name])
		}
	}
}

// The cursor is matched without regard to case, so a client that lowercases the
// name it was handed still gets the page after it rather than a repeat of the
// one it has.
func TestMigrationPagingAcceptsACursorInAnyCase(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()

	for _, name := range []string{"acme/Alpha", "acme/Bravo", "acme/Charlie", "acme/Delta"} {
		h.gh.AddRepo(name)
	}

	client, err := h.c.ClientFor(h.ctx, inst.ID)
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}

	first, next, _, err := migrationRepos(h.ctx, client, nil, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if next == "" {
		t.Fatalf("first page of four repositories at two a page says it is the whole organisation")
	}

	for _, cursor := range []string{next, strings.ToLower(next), strings.ToUpper(next)} {
		rest, _, _, err := migrationRepos(h.ctx, client, nil, cursor, 2)
		if err != nil {
			t.Fatalf("second page (cursor %q): %v", cursor, err)
		}
		for _, r := range rest {
			for _, had := range first {
				if strings.EqualFold(r.FullName, had.FullName) {
					t.Errorf("cursor %q returned %s again, which the operator has already seen", cursor, r.FullName)
				}
			}
		}
	}
}

// A repository the operator named by hand is matched without regard to case:
// GitHub resolves a repository URL that way, and an operator who typed the name
// into the wizard should not be told their own repository does not exist.
func TestMigrationReposMatchesANamedRepositoryWhateverItsCase(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	h.gh.AddRepo("acme/Widgets")

	client, err := h.c.ClientFor(h.ctx, inst.ID)
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}

	for _, named := range []string{"acme/Widgets", "acme/widgets", "ACME/WIDGETS"} {
		repos, next, total, err := migrationRepos(h.ctx, client, []string{named}, "", MaxPlanRepos)
		if err != nil {
			t.Fatalf("migrationRepos(%q): %v", named, err)
		}
		if len(repos) != 1 || !strings.EqualFold(repos[0].FullName, "acme/Widgets") {
			t.Fatalf("migrationRepos(%q) = %v, want the one repository", named, repos)
		}
		// A named request is not a page of a listing: there is nothing after it.
		if next != "" || total != 1 {
			t.Errorf("migrationRepos(%q) = cursor %q total %d, want no cursor and a total of 1", named, next, total)
		}
	}
}

// A pool that belongs to another installation is not somewhere this one's jobs
// could be sent, so it is not offered as a migration target. Getting this wrong
// would point one organisation's workflows at another's runners.
func TestMigrationPoolsAreScopedToTheirInstallation(t *testing.T) {
	h := newHarness(t)
	mine := h.installation()
	theirs := h.installationOn("globex", store.TargetOrg)

	wanted := h.pool(mine, "mine-linux")
	h.pool(theirs, "theirs-linux")

	disabled := h.pool(mine, "mine-windows")
	disabled.Enabled = false
	if err := h.st.UpdatePool(h.ctx, disabled); err != nil {
		t.Fatalf("UpdatePool: %v", err)
	}

	pools, err := h.c.migrationPools(h.ctx, mine.ID)
	if err != nil {
		t.Fatalf("migrationPools: %v", err)
	}
	if len(pools) != 1 || pools[0].ID != wanted.ID {
		var got []string
		for _, p := range pools {
			got = append(got, p.Name)
		}
		t.Fatalf("migrationPools = %v, want only the enabled pool of this installation (%s)", got, wanted.Name)
	}
}

// The badge is on by default and rides in the same pull request as the
// runs-on change, in the README GitHub renders, at the end of the row of
// badges that is already there.
func TestApplyMigrationAddsTheBadgeByDefault(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	h.pool(inst, "linux-x64", "zoomies-linux-x64")
	h.gh.AddWorkflow("acme/widgets", ".github/workflows/ci.yml", "jobs:\n  build:\n    runs-on: ubuntu-latest\n")
	h.gh.AddFile("acme/widgets", "README.md", "# Widgets\n\n[![CI](https://x/ci.svg)](https://x)\n\nProse.\n")

	out, err := h.c.ApplyMigration(h.ctx, MigrationApplyRequest{
		InstallationID: inst.ID,
		Repos:          []string{"acme/widgets"},
		Mapping:        map[string]string{"ubuntu-latest": "zoomies-linux-x64"},
	})
	if err != nil {
		t.Fatalf("ApplyMigration: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].Status != "opened" {
		t.Fatalf("results = %+v", out.Results)
	}
	res := out.Results[0]
	if res.Badge != BadgeAdded || res.BadgeReason != "" {
		t.Errorf("badge = %q (%q), want %q", res.Badge, res.BadgeReason, BadgeAdded)
	}
	// The pull request changed one workflow file; the README is not counted
	// as one, because "files" on the results screen means workflow files.
	if res.Workflows != 1 || res.Jobs != 1 {
		t.Errorf("workflows=%d jobs=%d, want 1 and 1", res.Workflows, res.Jobs)
	}
	got, _ := h.gh.FileContent("acme/widgets", "README.md")
	want := "# Widgets\n\n[![CI](https://x/ci.svg)](https://x)\n" + migrate.BadgeMarkdown + "\n\nProse.\n"
	if got != want {
		t.Errorf("README on GitHub:\n%s\nwant:\n%s", got, want)
	}
	if wf, _ := h.gh.FileContent("acme/widgets", ".github/workflows/ci.yml"); !strings.Contains(wf, "zoomies-linux-x64") {
		t.Errorf("the workflow was not rewritten alongside the badge: %q", wf)
	}
	if body := h.gh.PullRequestBody("acme/widgets", 1); !strings.Contains(body, migrate.BadgeMarkdown) {
		t.Errorf("the pull request body does not say the badge is in it:\n%s", body)
	}
}

// The README is somebody else's front page, so one word from the operator
// leaves it alone -- and a README that is missing, not Markdown, or already
// badged is reported rather than either failing the migration or being
// written into anyway.
func TestApplyMigrationBadgeIsDeclinedAndReported(t *testing.T) {
	no := false
	cases := map[string]struct {
		readme  string // "" for no README
		path    string
		badge   *bool
		want    string
		touched bool
	}{
		"declined":         {readme: "# W\n", path: "README.md", badge: &no, want: BadgeOff},
		"no readme":        {want: BadgeNoReadme},
		"not markdown":     {readme: "Widgets\n=======\n", path: "README.rst", want: BadgeNoReadme},
		"already there":    {readme: "# W\n\n" + migrate.BadgeMarkdown + "\n", path: "README.md", want: BadgePresent},
		"explicitly asked": {readme: "# W\n", path: "README.md", badge: ptr(true), want: BadgeAdded, touched: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			inst := h.installation()
			h.pool(inst, "linux-x64", "zoomies-linux-x64")
			h.gh.AddWorkflow("acme/widgets", ".github/workflows/ci.yml", "jobs:\n  build:\n    runs-on: ubuntu-latest\n")
			if tc.path != "" {
				h.gh.AddFile("acme/widgets", tc.path, tc.readme)
			}
			out, err := h.c.ApplyMigration(h.ctx, MigrationApplyRequest{
				InstallationID: inst.ID,
				Repos:          []string{"acme/widgets"},
				Mapping:        map[string]string{"ubuntu-latest": "zoomies-linux-x64"},
				Badge:          tc.badge,
			})
			if err != nil {
				t.Fatalf("ApplyMigration: %v", err)
			}
			res := out.Results[0]
			if res.Status != "opened" {
				t.Fatalf("the migration itself must go ahead whatever the badge does: %+v", res)
			}
			if res.Badge != tc.want {
				t.Errorf("badge = %q (%q), want %q", res.Badge, res.BadgeReason, tc.want)
			}
			if tc.want == BadgeNoReadme && res.BadgeReason == "" {
				t.Error("no_readme carries no reason, so the results screen cannot say which kind of nowhere")
			}
			if tc.path != "" {
				got, _ := h.gh.FileContent("acme/widgets", tc.path)
				if touched := got != tc.readme; touched != tc.touched {
					t.Errorf("README changed = %v, want %v:\n%s", touched, tc.touched, got)
				}
			}
			if body := h.gh.PullRequestBody("acme/widgets", 1); strings.Contains(body, "### The badge") != tc.touched {
				t.Errorf("the body mentions the badge = %v, want %v", !tc.touched, tc.touched)
			}
		})
	}
}

// workflowGate is a GitHub client that is asked only for workflows and holds
// each answer until it is released, which is what reading a large organisation
// looks like from this side. Every other method is the embedded nil
// interface's, so a read that strayed into asking for anything else fails loudly.
type workflowGate struct {
	github.Client
	release chan struct{}
	calls   atomic.Int32
	// panicOn is the repository whose listing blows up.
	panicOn string
}

func (g *workflowGate) ListWorkflows(ctx context.Context, repo string) ([]github.WorkflowFile, error) {
	g.calls.Add(1)
	if repo == g.panicOn {
		panic("a nil pointer where a workflow listing should be")
	}
	select {
	case <-g.release:
		return nil, github.ErrNoWorkflows
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

var quietLog = slog.New(slog.DiscardHandler)

func reposNamed(n int) []github.Repository {
	repos := make([]github.Repository, n)
	for i := range repos {
		repos[i] = github.Repository{FullName: fmt.Sprintf("acme/repo%03d", i)}
	}
	return repos
}

// Reading an organisation's workflows is meant to be a few repositories at a
// time. It used to start a goroutine for every repository at once and leave all
// but four parked on the semaphore, so ten thousand repositories cost ten
// thousand goroutines for a job that never reads more than four.
func TestReadingWorkflowsStartsNoMoreWorkersThanItMayRunAtOnce(t *testing.T) {
	gate := &workflowGate{release: make(chan struct{})}
	repos := reposNamed(20 * planConcurrency)
	before := runtime.NumGoroutine()

	done := make(chan []workflowSource, 1)
	go func() { done <- readWorkflows(context.Background(), gate, repos, quietLog) }()
	eventually(t, 5*time.Second, "the first readers to be waiting on GitHub", func() bool {
		return int(gate.calls.Load()) == planConcurrency
	})

	// The readers, and the goroutine above that is running readWorkflows.
	if extra := runtime.NumGoroutine() - before; extra > 2*planConcurrency {
		t.Errorf("%d goroutines started for %d repositories, of which %d may be read at once", extra, len(repos), planConcurrency)
	}

	close(gate.release)
	got := <-done
	for i, src := range got {
		if src.repo.FullName != repos[i].FullName || src.err != "" {
			t.Fatalf("repository %d came back as %+v, want it read and in order", i, src)
		}
	}
}

// A request that is given up on stops waiting for a slot. The repositories it
// never reached have to say so: the wizard shows an organisation as partly
// read, where a silent empty result would show repositories with nothing to
// migrate.
func TestACancelledReadSaysWhichRepositoriesItNeverReached(t *testing.T) {
	gate := &workflowGate{release: make(chan struct{})}
	repos := reposNamed(3 * planConcurrency)
	archived := repos[2*planConcurrency+1].FullName
	repos[2*planConcurrency+1].Archived = true // never read, so it never has anything to report
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan []workflowSource, 1)
	go func() { done <- readWorkflows(ctx, gate, repos, quietLog) }()
	eventually(t, 5*time.Second, "the first readers to be waiting on GitHub", func() bool {
		return int(gate.calls.Load()) == planConcurrency
	})
	cancel()
	got := <-done

	if calls := int(gate.calls.Load()); calls != planConcurrency {
		t.Errorf("GitHub was asked about %d repositories; none should be asked once the request is cancelled", calls)
	}
	inFlight, unreached := 0, 0
	for _, src := range got {
		switch {
		case src.err == context.Canceled.Error():
			inFlight++
		case strings.Contains(src.err, "cancelled"):
			unreached++
		case src.err != "":
			t.Errorf("%s reported %q, want a cancellation", src.repo.FullName, src.err)
		case src.repo.FullName != archived:
			t.Errorf("%s was neither read nor reported as unread", src.repo.FullName)
		}
	}
	if inFlight != planConcurrency || unreached != len(repos)-planConcurrency-1 {
		t.Errorf("%d reads were cut off and %d repositories were never reached, want %d and %d",
			inFlight, unreached, planConcurrency, len(repos)-planConcurrency-1)
	}
}

// The reads run on goroutines of their own, which a request cannot recover: a
// listing that panics for one repository has to cost that repository and not
// the process, the way one unreadable repository already does.
func TestAPanicReadingOneRepositoryIsRecordedAgainstThatRepository(t *testing.T) {
	repos := reposNamed(5)
	gate := &workflowGate{release: make(chan struct{}), panicOn: repos[2].FullName}
	close(gate.release)

	got := readWorkflows(context.Background(), gate, repos, quietLog)

	for i, src := range got {
		switch {
		case i == 2 && !strings.Contains(src.err, "nil pointer"):
			t.Errorf("the repository whose listing panicked reported %q, want the panic said", src.err)
		case i != 2 && src.err != "":
			t.Errorf("%s reported %q, but nothing was wrong with it", src.repo.FullName, src.err)
		}
	}
}

func ptr[T any](v T) *T { return &v }
