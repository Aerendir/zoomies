package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

// A workflow_job delivery used to ask the store which installation covers its
// repository twice: once to pick the secret the signature is checked with, and
// again to say which installation the job belongs to. It is one question about
// one repository asked a few hundred microseconds apart, and in a profile each
// asking was about 6% of a delivery.

// countInstallationQuestions makes the controller ask which installation covers
// a repository through a counter, and returns the count.
func (h *harness) countInstallationQuestions(failFirst bool) *int {
	h.t.Helper()
	asked := new(int)
	h.c.findInstallation = func(ctx context.Context, repo string) (*store.Installation, error) {
		*asked++
		if failFirst && *asked == 1 {
			return nil, errors.New("the database is busy")
		}
		return h.st.FindInstallationByTarget(ctx, repo)
	}
	return asked
}

func TestADeliveryAsksWhichInstallationCoversItsRepositoryOnce(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	asked := h.countInstallationQuestions(false)

	rec := h.deliverJob(jobEvent{Action: "queued", JobID: 1, RunID: 10, Labels: []string{"self-hosted", "linux"}})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("the delivery was answered %d: %s", rec.Code, rec.Body)
	}
	if *asked != 1 {
		t.Errorf("the delivery asked which installation covers its repository %d times; want once", *asked)
	}
	job, err := h.st.GetJobByGitHubID(h.ctx, 1)
	if err != nil {
		t.Fatalf("reading the job back: %v", err)
	}
	if job.InstallationID != inst.ID {
		t.Errorf("the job belongs to installation %q; want %q, the one that covers its repository", job.InstallationID, inst.ID)
	}
}

// The answer that no installation covers a repository is as good as any other:
// it is kept for the rest of the delivery too, and the job is recorded with none,
// as it always was.
func TestADeliveryForARepositoryNoInstallationCoversAsksOnce(t *testing.T) {
	h := newHarness(t)
	h.installation() // covers acme and nothing else
	asked := h.countInstallationQuestions(false)

	rec := h.deliverJob(jobEvent{Action: "queued", JobID: 2, RunID: 11, Repo: "stranger/repo", Labels: []string{"self-hosted", "linux"}})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("the delivery was answered %d: %s", rec.Code, rec.Body)
	}
	if *asked != 1 {
		t.Errorf("the delivery asked which installation covers its repository %d times; want once", *asked)
	}
	job, err := h.st.GetJobByGitHubID(h.ctx, 2)
	if err != nil {
		t.Fatalf("reading the job back: %v", err)
	}
	if job.InstallationID != "" {
		t.Errorf("the job belongs to installation %q; want none, nothing covers stranger/repo", job.InstallationID)
	}
}

// Only an answer is kept. A lookup that failed says nothing about the
// repository, so the job's own question is asked again, as it always was, and a
// busy database for a moment does not cost a job its installation.
func TestAFailedInstallationLookupIsAskedAgainWhenTheJobIsApplied(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	asked := h.countInstallationQuestions(true)

	rec := h.deliverJob(jobEvent{Action: "queued", JobID: 3, RunID: 12, Labels: []string{"self-hosted", "linux"}})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("the delivery was answered %d: %s", rec.Code, rec.Body)
	}
	if *asked != 2 {
		t.Errorf("the delivery asked %d times after the first failed; want a second, and no more", *asked)
	}
	job, err := h.st.GetJobByGitHubID(h.ctx, 3)
	if err != nil {
		t.Fatalf("reading the job back: %v", err)
	}
	if job.InstallationID != inst.ID {
		t.Errorf("the job belongs to installation %q; want %q, which the second question found", job.InstallationID, inst.ID)
	}
}

// deliveryOwner on its own, with a store that says what it is told.

type ownerReply struct {
	inst *store.Installation
	err  error
}

// scriptedOwner is a deliveryOwner whose store gives each reply in turn, the last
// one for ever, and a count of the times it was asked.
func scriptedOwner(replies ...ownerReply) (*deliveryOwner, *int) {
	asked := new(int)
	return &deliveryOwner{ask: func(context.Context, string) (*store.Installation, error) {
		r := replies[min(*asked, len(replies)-1)]
		*asked++
		return r.inst, r.err
	}}, asked
}

func TestADeliveryOwnerAsksTheStoreOnceForAnInstallationItFound(t *testing.T) {
	inst := &store.Installation{ID: "ins_a"}
	owners, asked := scriptedOwner(ownerReply{inst: inst})
	for range 3 {
		got, err := owners.of(context.Background(), "acme/widgets")
		if err != nil || got != inst {
			t.Fatalf("of = %v, %v; want the installation", got, err)
		}
	}
	if *asked != 1 {
		t.Errorf("the store was asked %d times; want once", *asked)
	}
}

func TestADeliveryOwnerAsksTheStoreOnceWhenNoInstallationCoversTheRepository(t *testing.T) {
	owners, asked := scriptedOwner(ownerReply{err: fmt.Errorf("installation for acme/widgets: %w", store.ErrNotFound)})
	for range 3 {
		if got, err := owners.of(context.Background(), "acme/widgets"); got != nil || !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("of = %v, %v; want no installation and ErrNotFound", got, err)
		}
	}
	if *asked != 1 {
		t.Errorf("the store was asked %d times; want once", *asked)
	}
}

func TestADeliveryOwnerDoesNotKeepAFailedLookup(t *testing.T) {
	inst := &store.Installation{ID: "ins_a"}
	busy := errors.New("the database is busy")
	owners, asked := scriptedOwner(ownerReply{err: busy}, ownerReply{inst: inst})

	if _, err := owners.of(context.Background(), "acme/widgets"); !errors.Is(err, busy) {
		t.Fatalf("the first of = %v; want the failure", err)
	}
	for range 2 {
		if got, err := owners.of(context.Background(), "acme/widgets"); err != nil || got != inst {
			t.Fatalf("of after the failure = %v, %v; want the installation the store then found", got, err)
		}
	}
	if *asked != 2 {
		t.Errorf("the store was asked %d times; want twice, the failure and then the answer", *asked)
	}
}

// A delivery names one repository, and the owner remembers one. A question about
// another goes to the store, and the first is forgotten for it.
func TestADeliveryOwnerRemembersTheRepositoryItWasAskedAbout(t *testing.T) {
	a, b := &store.Installation{ID: "ins_a"}, &store.Installation{ID: "ins_b"}
	owners := &deliveryOwner{}
	var asked []string
	owners.ask = func(_ context.Context, repo string) (*store.Installation, error) {
		asked = append(asked, repo)
		if repo == "acme/widgets" {
			return a, nil
		}
		return b, nil
	}
	for _, repo := range []string{"acme/widgets", "acme/widgets", "other/gadgets", "other/gadgets", "acme/widgets"} {
		want := a
		if repo == "other/gadgets" {
			want = b
		}
		if got, err := owners.of(context.Background(), repo); err != nil || got != want {
			t.Fatalf("of(%q) = %v, %v; want %s", repo, got, err, want.ID)
		}
	}
	if want := []string{"acme/widgets", "other/gadgets", "acme/widgets"}; !slices.Equal(asked, want) {
		t.Errorf("the store was asked about %v; want %v", asked, want)
	}
}
