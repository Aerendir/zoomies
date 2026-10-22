package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/eyupio/zoomies/internal/github"
	"github.com/eyupio/zoomies/internal/prrepair"
	"github.com/eyupio/zoomies/internal/store"
)

const (
	// errorOutputFor is how long a read of a job's error output is reused. Opening
	// a failure twice, or two people opening the same one, is one download of the
	// log; a minute is short enough that a re-run's output is not hidden for long.
	errorOutputFor = time.Minute
	// errorOutputKept bounds the copies held, so a long run of different
	// failures cannot grow the controller's memory.
	errorOutputKept = 256
)

// ErrEliErrorOutputDenied is a caller who may not read a job's error output:
// no confirmed GitHub account, no write access to the repository through it, or
// a repository no installation serves. They are one error on purpose; which of
// them it was is not something to tell a person who may not know the repository.
var ErrEliErrorOutputDenied = errors.New("controller: error output is only for people who can write to the repository")

// EliErrorOutput is the lines of a failed job's log that say why it failed.
// Note is a sentence for the person when there are none to show.
type EliErrorOutput struct {
	Lines []string `json:"lines"`
	Note  string   `json:"note,omitempty"`
}

// EliErrorOutput reads the error lines of one job for a person who could fix
// the pull request. It is on demand and it keeps nothing: the lines are never
// stored and never logged, because a log is where a job's secrets end up and
// the repair record is read by more people than could read the job. They are
// passed through the same redaction the model's evidence gets, held in memory
// for a minute so a second look is not a second download, and handed back.
//
// The access check runs on every call, before the copy in memory is looked at.
// The copy belongs to a job, not to a person, and the one person who could read
// it must not be how another one does.
func (c *Controller) EliErrorOutput(ctx context.Context, user, repo string, jobID int64) (EliErrorOutput, error) {
	repo = normaliseRepairRepo(repo)
	if jobID < 1 {
		return EliErrorOutput{}, fmt.Errorf("controller: a job id is needed to read its error output")
	}
	id, err := c.eliIdentity(ctx, user)
	if errors.Is(err, store.ErrNotFound) {
		return EliErrorOutput{}, ErrEliErrorOutputDenied
	}
	if err != nil {
		return EliErrorOutput{}, err
	}
	inst, err := c.st.FindInstallationByTarget(ctx, repo)
	if errors.Is(err, store.ErrNotFound) {
		return EliErrorOutput{}, ErrEliErrorOutputDenied
	}
	if err != nil {
		return EliErrorOutput{}, err
	}
	raw, err := c.ClientFor(ctx, inst.ID)
	if err != nil {
		return EliErrorOutput{}, fmt.Errorf("the GitHub installation for %s is unavailable: %w", repo, err)
	}
	client, ok := raw.(github.RepairClient)
	if !ok {
		return EliErrorOutput{}, ErrEliErrorOutputDenied
	}
	actor, err := client.RepairActor(ctx, repo, id.GitHubLogin)
	if err != nil || !actor.CanWrite || actor.ID != id.GitHubUserID {
		return EliErrorOutput{}, ErrEliErrorOutputDenied
	}

	key := repo + "#" + strconv.FormatInt(jobID, 10)
	if out, ok := c.errorOutput.get(key, c.Now()); ok {
		return out, nil
	}
	lines, err := client.RepairFailures(ctx, repo, github.RepairPull{}, jobID)
	if err != nil {
		return EliErrorOutput{}, err
	}
	out := EliErrorOutput{Lines: make([]string, 0, len(lines))}
	for _, line := range lines {
		out.Lines = append(out.Lines, prrepair.Redact(line))
	}
	if len(out.Lines) == 0 {
		out.Note = "GitHub no longer has this log, or the job printed no error lines."
	}
	c.errorOutput.put(key, out, c.Now())
	return out, nil
}

// errorOutputCache is the last few reads of a job's error output. It is the
// only place those lines live, in memory, and a restart forgets them.
type errorOutputCache struct {
	mu      sync.Mutex
	entries map[string]errorOutputEntry
}

type errorOutputEntry struct {
	out     EliErrorOutput
	expires time.Time
}

func (k *errorOutputCache) get(key string, now time.Time) (EliErrorOutput, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	e, ok := k.entries[key]
	if !ok || !now.Before(e.expires) {
		return EliErrorOutput{}, false
	}
	e.out.Lines = slices.Clone(e.out.Lines)
	return e.out, true
}

// put keeps out for errorOutputFor. It first drops every copy that has
// expired, then the oldest of the rest while the cache is full, so the bound
// holds however fast different jobs are asked for.
func (k *errorOutputCache) put(key string, out EliErrorOutput, now time.Time) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.entries == nil {
		k.entries = map[string]errorOutputEntry{}
	}
	for held, e := range k.entries {
		if !now.Before(e.expires) {
			delete(k.entries, held)
		}
	}
	for len(k.entries) >= errorOutputKept {
		oldest, oldestAt := "", time.Time{}
		for held, e := range k.entries {
			if oldest == "" || e.expires.Before(oldestAt) {
				oldest, oldestAt = held, e.expires
			}
		}
		delete(k.entries, oldest)
	}
	out.Lines = slices.Clone(out.Lines)
	k.entries[key] = errorOutputEntry{out: out, expires: now.Add(errorOutputFor)}
}

func (k *errorOutputCache) size() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.entries)
}
