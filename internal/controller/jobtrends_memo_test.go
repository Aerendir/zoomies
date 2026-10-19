package controller

import (
	"context"
	"sync"
	"testing"
	"time"
)

// The slower-job check reads eight days of jobs, which is most of what a pass that
// someone is watching does on a fleet with a lot of them, and what it finds does
// not change in a minute, so it keeps its answer for slowerFor. Everything else in
// the problems list is worked out afresh for each request, so these hold the
// exception to what it promises and no further: the answer lasts the minute and
// not longer, a failure is not kept, and nobody is handed the copy the next
// caller reads.

const slowerCode = "jobs.duration_regressed"

// seedRegression is a job that took five minutes two days ago and eleven in the last
// twelve hours, which is the shape the check reports.
func seedRegression(t *testing.T, h *harness, inst string) {
	t.Helper()
	n := int64(1000)
	for i := 0; i < 12; i++ {
		n++
		seedJob(t, h, inst, n, "Go (api)", "success", 2*24*time.Hour+time.Duration(i)*time.Hour, 5*time.Minute, "")
		n++
		seedJob(t, h, inst, n, "Go (api)", "success", time.Duration(i+1)*time.Hour, 11*time.Minute, "")
	}
}

// seedRecovery is forty more runs of the same job in the last day that took the five
// minutes it used to, which put its typical time back and end the regression.
func seedRecovery(t *testing.T, h *harness, inst string) {
	t.Helper()
	for i := 0; i < 40; i++ {
		seedJob(t, h, inst, int64(2000+i), "Go (api)", "success", 13*time.Hour+time.Duration(i)*time.Minute, 5*time.Minute, "")
	}
}

// forgetSlowerJobs makes the next slower-job check read the jobs, as it does a
// minute after the last time.
func forgetSlowerJobs(c *Controller) {
	c.slower.mu.Lock()
	defer c.slower.mu.Unlock()
	c.slower.read = time.Time{}
}

// slowerReadAt is when the slower-job check last read the jobs.
func slowerReadAt(c *Controller) time.Time {
	c.slower.mu.Lock()
	defer c.slower.mu.Unlock()
	return c.slower.read
}

// checkSlowerMix fails a benchmark that is about to time passes with the mix it
// says it times: one pass that reads the jobs for every five that reuse what it
// found. Without it a memo that never forgot, or one that never kept anything,
// would be timed under the same name.
func checkSlowerMix(b *testing.B, h *harness) {
	b.Helper()
	before := slowerReadAt(h.c)
	forgetSlowerJobs(h.c)
	if err := h.c.Reconcile(h.ctx); err != nil {
		b.Fatalf("Reconcile: %v", err)
	}
	read := slowerReadAt(h.c)
	if !read.After(before) {
		b.Fatal("a pass after the slower-job check was made to forget did not read the jobs again")
	}
	if err := h.c.Reconcile(h.ctx); err != nil {
		b.Fatalf("Reconcile: %v", err)
	}
	if !slowerReadAt(h.c).Equal(read) {
		b.Fatal("the pass after one that read the jobs read them again, so five passes in six are not the cheap ones")
	}
}

func TestWhatTheSlowerJobCheckFoundIsReusedForAMinuteAndNoLonger(t *testing.T) {
	h := newHarness(t)
	inst := h.installation().ID
	seedRegression(t, h, inst)
	if h.problemOrNil(slowerCode) == nil {
		t.Fatal("a job that got much slower was not said to be")
	}

	// The jobs now say it has recovered, but what was found is reused for the minute.
	seedRecovery(t, h, inst)
	h.offset.Add(int64(slowerFor - time.Second))
	if h.problemOrNil(slowerCode) == nil {
		t.Error("the jobs were read again before the minute was up")
	}

	h.offset.Add(int64(2 * time.Second))
	if p := h.problemOrNil(slowerCode); p != nil {
		t.Errorf("after the minute the check still said %q, though the jobs say the job has recovered", p.Title)
	}
}

// The jobs are read again after a failure, and the failure is not what is reused:
// an answer of nothing, kept for a minute, would hide a regression for that long.
func TestAFailedReadOfTheJobsIsNotKeptByTheSlowerJobCheck(t *testing.T) {
	h := newHarness(t)
	inst := h.installation().ID
	seedRegression(t, h, inst)

	cancelled, cancel := context.WithCancel(h.ctx)
	cancel()
	var out []Problem
	if err := h.c.jobsSlowerProblems(cancelled, &out); err == nil {
		t.Fatal("a read with a cancelled context said it worked")
	}
	if len(out) != 0 {
		t.Errorf("a failed read added %d problems", len(out))
	}
	if h.problemOrNil(slowerCode) == nil {
		t.Error("the check reused the failure and did not read the jobs again")
	}
}

func TestEachCallerOfTheSlowerJobCheckGetsItsOwnCopy(t *testing.T) {
	h := newHarness(t)
	inst := h.installation().ID
	seedRegression(t, h, inst)

	var first, second []Problem
	if err := h.c.jobsSlowerProblems(h.ctx, &first); err != nil || len(first) != 1 {
		t.Fatalf("the first call gave %d problems, err = %v; want 1", len(first), err)
	}
	first[0].Title = "changed by the first caller"
	if err := h.c.jobsSlowerProblems(h.ctx, &second); err != nil || len(second) != 1 {
		t.Fatalf("the second call gave %d problems, err = %v; want 1", len(second), err)
	}
	if second[0].Title == first[0].Title {
		t.Error("the second caller was handed the first caller's copy")
	}

	// It adds to what the caller has, and does not replace it.
	out := []Problem{{Code: "already.there"}}
	if err := h.c.jobsSlowerProblems(h.ctx, &out); err != nil || len(out) != 2 || out[0].Code != "already.there" || out[1].Code != slowerCode {
		t.Errorf("appending to a list gave %+v, err = %v", out, err)
	}
}

// A clock that is put back, by an operator or by the network time service, is not a
// reason to go on trusting an answer from before it: the minute is measured from a
// moment that, as far as the clock now says, has not come yet.
func TestTheSlowerJobCheckReadsAgainWhenTheClockGoesBack(t *testing.T) {
	h := newHarness(t)
	inst := h.installation().ID
	seedRegression(t, h, inst)
	if h.problemOrNil(slowerCode) == nil {
		t.Fatal("a job that got much slower was not said to be")
	}
	seedRecovery(t, h, inst)

	h.offset.Add(-int64(time.Hour))
	if p := h.problemOrNil(slowerCode); p != nil {
		t.Errorf("an answer from an hour in the future was reused: %q", p.Title)
	}
}

// Callers that arrive together read the jobs once between them, and all get the answer.
func TestCallersOfTheSlowerJobCheckThatArriveTogetherAllGetTheAnswer(t *testing.T) {
	h := newHarness(t)
	inst := h.installation().ID
	seedRegression(t, h, inst)

	const callers = 8
	got := make([][]Problem, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = h.c.jobsSlowerProblems(h.ctx, &got[i])
		}()
	}
	wg.Wait()
	for i := range got {
		if errs[i] != nil || len(got[i]) != 1 || got[i][0].Code != slowerCode {
			t.Errorf("caller %d got %+v, err = %v; want the one problem", i, got[i], errs[i])
		}
	}
}
