package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/store"
)

const wantPoolOffSentence = "Would rent 1 machine for this pool, but renting is switched off."

// rentingExplained is a fleet that would rent a machine were renting on: one
// scheduling pass so the problems list has the scheduler's plan, then one
// machine pass so there is a verdict to lay over it. The verdict is the last
// machine pass's, so a test that ran only one of the two would be asserting on
// a pool the other half knows nothing about.
func (h *harness) rentingExplained(t *testing.T) (*store.Pool, *store.Job) {
	t.Helper()
	pool, _ := h.rentingOffFleet(t)
	if err := h.c.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	h.machinePass(t)
	jobs, err := h.st.ListQueuedJobs(h.ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("queued jobs = %d, %v; want the one queueWork made", len(jobs), err)
	}
	return pool, jobs[0]
}

func (h *harness) capacityProblemFor(t *testing.T, poolID string) Problem {
	t.Helper()
	for _, p := range h.c.PoolCapacityProblems() {
		if p.Code == "pool.no_capacity" && p.TargetID == poolID {
			return p
		}
	}
	t.Fatalf("no pool.no_capacity for %s in %+v", poolID, h.c.PoolCapacityProblems())
	return Problem{}
}

// A pool with nowhere to run says the scheduler's half of the story, and the
// operator who has a provider configured is left asking why nothing is rented.
// The verdict answers that in the same entry rather than on another page.
func TestAPoolWaitingForAHostSaysARentalIsOffWhenItIs(t *testing.T) {
	h := newHarness(t)
	pool, _ := h.rentingExplained(t)

	p := h.capacityProblemFor(t, pool.ID)
	if !strings.Contains(p.Detail, wantPoolOffSentence) {
		t.Errorf("detail = %q, want it to contain %q", p.Detail, wantPoolOffSentence)
	}
	if !strings.HasPrefix(p.Fix, "Turn on Rent machines in Settings") {
		t.Errorf("fix = %q, want it to start with the switch to turn on", p.Fix)
	}
	if !strings.HasSuffix(p.Fix, " "+h.planFor(t, pool.ID).BlockedFix) {
		t.Errorf("fix = %q, want the scheduler's own fix kept at the end", p.Fix)
	}
}

func (h *harness) planFor(t *testing.T, poolID string) (pp struct{ Blocked, BlockedFix string }) {
	t.Helper()
	plan, _ := h.c.getLastPlan()
	for _, got := range plan.Pools {
		if got.PoolID == poolID {
			pp.Blocked, pp.BlockedFix = got.Blocked, got.BlockedFix
			return pp
		}
	}
	t.Fatalf("the plan has no pool %s", poolID)
	return pp
}

func TestTheQueuedJobExplanationCarriesTheRentingSentence(t *testing.T) {
	h := newHarness(t)
	pool, job := h.rentingExplained(t)

	got, err := h.c.ExplainJob(h.ctx, job.ID, 0)
	if err != nil {
		t.Fatalf("ExplainJob: %v", err)
	}
	if !got.Blocked {
		t.Fatalf("explanation = %+v, want a blocked job", got)
	}
	if !strings.Contains(got.Detail, h.planFor(t, pool.ID).Blocked) || !strings.Contains(got.Detail, wantPoolOffSentence) {
		t.Errorf("detail = %q, want the scheduler's sentence and then %q", got.Detail, wantPoolOffSentence)
	}
	if !strings.HasPrefix(got.Fix, "Turn on Rent machines in Settings") {
		t.Errorf("fix = %q, want it to start with the switch to turn on", got.Fix)
	}
}

func TestTheJobVerdictCarriesTheRentingSentenceAsEvidenceAndANextStep(t *testing.T) {
	h := newHarness(t)
	_, job := h.rentingExplained(t)

	got, err := h.c.ExplainJob(h.ctx, job.ID, 0)
	if err != nil {
		t.Fatalf("ExplainJob: %v", err)
	}
	var found bool
	for _, e := range got.Evidence {
		if e.Kind == "plan_reason" && e.Label == "renting" {
			found = true
			if e.Value != wantPoolOffSentence {
				t.Errorf("renting evidence = %q, want %q", e.Value, wantPoolOffSentence)
			}
		}
	}
	if !found {
		t.Errorf("evidence = %+v, want a renting entry", got.Evidence)
	}
	var step *NextStep
	for i := range got.NextSteps {
		if got.NextSteps[i].Link == "/providers" {
			step = &got.NextSteps[i]
		}
	}
	if step == nil || step.Kind != "change" || !strings.HasPrefix(step.Text, "Turn on Rent machines in Settings") {
		t.Errorf("next steps = %+v, want a change to /providers that turns renting on", got.NextSteps)
	}
}

// A waiting verdict quotes the scheduler's sentence, which the page already
// shows, so only the part that is news is added; repeating the whole would
// print the same sentence twice in a row.
func TestAWaitingVerdictAddsOnlyTheTailToTheSchedulersSentence(t *testing.T) {
	h := newHarness(t)
	h.enableProviders(t)
	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	h.providerRow(t, "lab")
	// One host that could run the pool is cordoned and another has no slot
	// left, so the pool is blocked, but not for want of a machine: mending the
	// first is cheaper than renting one.
	cordoned := h.host("vm-1")
	if err := h.st.SetHostCordoned(h.ctx, cordoned.ID, true); err != nil {
		t.Fatalf("SetHostCordoned: %v", err)
	}
	full := h.host("vm-2")
	full.Capacity = 0
	if err := h.st.UpdateHost(h.ctx, full); err != nil {
		t.Fatalf("UpdateHost: %v", err)
	}
	h.queueWork(t)
	if err := h.c.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	h.machinePass(t)

	v := h.c.rentingFor(pool.ID)
	if v == nil || v.State != RentingWaiting {
		t.Fatalf("rentingFor = %+v, want a waiting verdict", v)
	}
	p := h.capacityProblemFor(t, pool.ID)
	blocked := h.planFor(t, pool.ID).Blocked
	if want := blocked + ". " + waitingTail; p.Detail != want {
		t.Errorf("detail = %q, want %q", p.Detail, want)
	}
	if strings.Count(p.Detail, blocked) != 1 {
		t.Errorf("detail = %q repeats the scheduler's sentence", p.Detail)
	}
	if p.Fix != h.planFor(t, pool.ID).BlockedFix {
		t.Errorf("fix = %q, want the scheduler's fix unchanged for a waiting pool", p.Fix)
	}
}

// Whatever reaches the problems list is sent again each time it differs, so
// two passes over a fleet that has not moved must read the same, byte for byte.
func TestTheRentingSentenceInTheProblemsListDoesNotChangeBetweenIdenticalPasses(t *testing.T) {
	h := newHarness(t)
	h.rentingExplained(t)
	first, err := json.Marshal(h.c.PoolCapacityProblems())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.c.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	h.machinePass(t)
	second, err := json.Marshal(h.c.PoolCapacityProblems())
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || !strings.Contains(string(first), wantPoolOffSentence) {
		t.Errorf("problems differ between identical passes or lack the renting sentence:\n%s\n%s", first, second)
	}
}

// Most fleets have no provider, and for them nothing may read differently.
func TestAFleetWithoutProvidersReadsAsItDidBeforeRenting(t *testing.T) {
	h := newHarness(t)
	inst := h.installation()
	pool := h.pool(inst, "linux-x64")
	h.queueWork(t)
	if err := h.c.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	h.machinePass(t)

	p := h.capacityProblemFor(t, pool.ID)
	pp := h.planFor(t, pool.ID)
	if p.Detail != pp.Blocked || p.Fix != pp.BlockedFix {
		t.Errorf("detail, fix = %q, %q; want the scheduler's %q, %q", p.Detail, p.Fix, pp.Blocked, pp.BlockedFix)
	}
	jobs, err := h.st.ListQueuedJobs(h.ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("queued jobs = %d, %v", len(jobs), err)
	}
	got, err := h.c.ExplainJob(h.ctx, jobs[0].ID, 0)
	if err != nil {
		t.Fatalf("ExplainJob: %v", err)
	}
	if got.Detail != pp.Blocked {
		t.Errorf("detail = %q, want %q", got.Detail, pp.Blocked)
	}
	for _, e := range got.Evidence {
		if e.Label == "renting" {
			t.Errorf("evidence %+v mentions renting in a fleet without providers", e)
		}
	}
}
