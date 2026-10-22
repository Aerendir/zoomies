package store

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// The Jobs page's All view asks for the jobs in all four states, newest first, and
// SQLite used to answer by seeking the state index for each of the four and then
// sorting every row it found, read whole: 61 ms over 10,000 jobs where reading
// them in queued order and stopping after fifty takes less than one, and the
// first grows with the table. This holds the plan, because a result that was right
// and slow would pass every other test here.
func TestAStateListThatIncludesCompletedAndAnotherIsReadInQueuedOrder(t *testing.T) {
	s := newTestStore(t)
	all := []JobState{JobWaiting, JobQueued, JobInProgress, JobCompleted}
	for name, states := range map[string][]JobState{
		"every state":             all,
		"completed and queued":    {JobCompleted, JobQueued},
		"completed and running":   {JobInProgress, JobCompleted},
		"completed named twice":   {JobCompleted, JobQueued, JobCompleted},
		"completed and a mistake": {"bogus", JobCompleted},
	} {
		where, args := jobWhere(JobFilter{ManagedOnly: true, States: states}, s.Now())
		plan := explain(t, s, `SELECT `+jobCols+` FROM jobs `+where+` ORDER BY queued_at DESC, id ASC LIMIT 50`, args...)
		if !strings.Contains(plan, "idx_jobs_queued_id") || strings.Contains(plan, "TEMP B-TREE") {
			t.Errorf("%s: the page is not read in queued order from idx_jobs_queued_id: %s", name, plan)
		}
	}
}

// The other half of the same choice. The live states are few, and an index finds
// them at once; read in queued order they would be looked for in a history that
// grows without bound, for a page that is nearly empty. Completed on its own is
// left to the index as well: it reads it in queued order and, given a window of
// dates, only the window.
func TestAStateListWithoutBothCompletedAndAnotherIsFoundThroughTheStateIndex(t *testing.T) {
	s := newTestStore(t)
	for name, states := range map[string][]JobState{
		"running":                      {JobInProgress},
		"queued":                       {JobQueued},
		"queued and running":           {JobQueued, JobInProgress},
		"everything in hand":           {JobWaiting, JobQueued, JobInProgress},
		"running named twice":          {JobInProgress, JobInProgress},
		"waiting for a review":         {JobWaiting},
		"completed alone":              {JobCompleted},
		"completed alone, named twice": {JobCompleted, JobCompleted},
	} {
		where, args := jobWhere(JobFilter{ManagedOnly: true, States: states}, s.Now())
		plan := explain(t, s, `SELECT `+jobCols+` FROM jobs `+where+` ORDER BY queued_at DESC, id ASC LIMIT 50`, args...)
		if !strings.Contains(plan, "(state=?)") || strings.Contains(plan, "SCAN jobs") {
			t.Errorf("%s: the states are not sought through an index on the state: %s", name, plan)
		}
	}
}

func TestAStateListNamesEachStateOnce(t *testing.T) {
	for name, c := range map[string]struct {
		states []JobState
		term   string
		args   []any
	}{
		"one":          {[]JobState{JobQueued}, "state IN (?)", []any{"queued"}},
		"two":          {[]JobState{JobQueued, JobInProgress}, "state IN (?,?)", []any{"queued", "in_progress"}},
		"twice":        {[]JobState{JobQueued, JobQueued, JobWaiting}, "state IN (?,?)", []any{"queued", "waiting"}},
		"completed":    {[]JobState{JobCompleted}, "state IN (?)", []any{"completed"}},
		"and another":  {[]JobState{JobQueued, JobCompleted}, "+state IN (?,?)", []any{"queued", "completed"}},
		"only twice":   {[]JobState{JobCompleted, JobCompleted}, "state IN (?)", []any{"completed"}},
		"not a state":  {[]JobState{"bogus"}, "state IN (?)", []any{"bogus"}},
		"among others": {[]JobState{"bogus", JobCompleted}, "+state IN (?,?)", []any{"bogus", "completed"}},
	} {
		term, args := jobStateTerm(c.states)
		// The order of the arguments is not the list's meaning, only that there is one
		// for each placeholder.
		got := make([]string, len(args))
		for i, a := range args {
			got[i] = a.(string)
		}
		want := make([]string, len(c.args))
		for i, a := range c.args {
			want[i] = a.(string)
		}
		slices.Sort(got)
		slices.Sort(want)
		if term != c.term || !slices.Equal(got, want) {
			t.Errorf("%s: jobStateTerm(%v) = %q, %v; want %q, %v", name, c.states, term, args, c.term, c.args)
		}
	}
}

// managedByHand is the Jobs page's "this fleet had a hand in it", worked out in Go
// from the job and not in SQL, so that the listing can be held to something other
// than itself.
func managedByHand(j *Job) bool {
	return j.Matched || j.PoolID != "" || j.RunnerID != "" ||
		(!j.Matched && (j.State == JobQueued || j.State == JobWaiting) && !HostedJob(j.Labels))
}

// Whichever states are asked for, and however the list is spelled, the page and the
// count are the jobs in those states and no others, in the order the page shows
// them. The plan above is what keeps the All view fast; this is what keeps it right
// when the planner reads the states as a test and not as a seek.
func TestEveryChoiceOfStatesListsTheJobsInThemAndNoOthers(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	s := newTestStoreAt(t, func() time.Time { return now })
	jobs, _ := pageHistory(t, s, now, 120)

	byQueued := slices.Clone(jobs)
	slices.SortFunc(byQueued, func(a, b *Job) int {
		switch {
		case a.QueuedAt.After(b.QueuedAt):
			return -1
		case a.QueuedAt.Before(b.QueuedAt):
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	seen := map[JobState]int{}
	for _, j := range jobs {
		seen[j.State]++
	}
	for _, st := range []JobState{JobWaiting, JobQueued, JobInProgress, JobCompleted} {
		if seen[st] == 0 {
			t.Fatalf("the history has no job in the %s state, so no choice of states is held to it", st)
		}
	}

	states := []JobState{JobWaiting, JobQueued, JobInProgress, JobCompleted}
	choices := 0
	for mask := 1; mask < 1<<len(states); mask++ {
		var choice []JobState
		for i, st := range states {
			if mask>>i&1 == 1 {
				choice = append(choice, st)
			}
		}
		// The same states, named again, as a caller that builds the list from several
		// places might.
		for _, spelling := range [][]JobState{choice, append(slices.Clone(choice), choice[0])} {
			for _, managedOnly := range []bool{false, true} {
				var want []string
				for _, j := range byQueued {
					if slices.Contains(choice, j.State) && (!managedOnly || managedByHand(j)) {
						want = append(want, j.ID)
					}
				}
				f := JobFilter{States: spelling, ManagedOnly: managedOnly}
				for _, p := range []Page{{Limit: 7}, {Limit: 7, Offset: 14}, {Limit: 7, Offset: 112}, {Limit: 500}} {
					got, total, err := s.ListJobs(ctx, f, p)
					if err != nil {
						t.Fatalf("ListJobs(%v, managed=%v, %+v): %v", spelling, managedOnly, p, err)
					}
					from := min(p.Offset, len(want))
					to := min(from+p.limit(50, 500), len(want))
					if total != len(want) || !slices.Equal(idsOf(got), want[from:to]) {
						t.Errorf("states %v, managed only %v, page %+v: got %d jobs of %d, %v; want %d of %d, %v",
							spelling, managedOnly, p, len(got), total, idsOf(got), to-from, len(want), want[from:to])
					}
				}
				// Read by keyset instead, a page at a time, it is the same listing.
				var walked []string
				var cursor *JobCursor
				for range len(want)/7 + 2 {
					page, total, next, err := s.ListJobsAfter(ctx, f, cursor, 7)
					if err != nil {
						t.Fatalf("ListJobsAfter(%v): %v", spelling, err)
					}
					if total != len(want) {
						t.Errorf("states %v, managed only %v: the keyset count is %d; want %d", spelling, managedOnly, total, len(want))
					}
					walked = append(walked, idsOf(page)...)
					if next == nil {
						break
					}
					cursor = next
				}
				if !slices.Equal(walked, want) {
					t.Errorf("states %v, managed only %v: the keyset walk found %d jobs; want %d", spelling, managedOnly, len(walked), len(want))
				}
				choices++
			}
		}
	}
	if want := 15 * 2 * 2; choices != want {
		t.Fatalf("held %d choices of states; want %d", choices, want)
	}
}

func idsOf(jobs []*Job) []string {
	ids := make([]string, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
	}
	return ids
}
