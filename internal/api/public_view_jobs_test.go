package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/events"
	"github.com/eyupio/zoomies/internal/store"
)

// staffName is an account of the kind a public view promises to say nothing
// about. It is distinctive on purpose: the assertions below search whole bodies
// for it, and a common word would find itself in somebody's job name.
const staffName = "ada-lovelace"

// failedJobOnARunner is a failed job and the runner that was executing it,
// which the runner's page and frames still point at.
func (h *harness) failedJobOnARunner(t *testing.T) (*store.Job, *store.Runner) {
	t.Helper()
	inst := h.installation()
	pool := h.pool(inst, "linux")
	run := h.runner(pool, h.host("vm-1"), store.RunnerIdle)
	done := time.Now().UTC().Add(-time.Minute)
	started := done.Add(-2 * time.Minute)
	failed, err := h.st.UpsertJob(h.ctx, &store.Job{
		GitHubJobID: 7201, GitHubRunID: 56, Repo: "acme/widgets", Workflow: "CI", JobName: "build",
		Labels: store.StringSlice{"self-hosted", "linux", "x64"}, State: store.JobCompleted,
		Conclusion: "failure", InstallationID: inst.ID, PoolID: pool.ID, Matched: true,
		QueuedAt: started.Add(-time.Minute), StartedAt: &started, CompletedAt: &done, RunAttempt: 1,
	})
	if err != nil {
		t.Fatalf("UpsertJob failed: %v", err)
	}
	if run, _, err = h.st.StartRunnerJob(h.ctx, run.ID, failed.ID, ""); err != nil {
		t.Fatalf("StartRunnerJob failed: %v", err)
	}
	return failed, run
}

// markAsKnown has the operator named staffName press the button, through the
// route, so the timeline entry and the job.updated frame are the real ones.
func (h *harness) markAsKnown(t *testing.T, failed *store.Job) {
	t.Helper()
	_, operator := h.user(staffName, store.RoleOperator)
	h.do(request{method: "POST", path: "/api/v1/jobs/" + failed.ID + "/acknowledge", cookie: operator}).
		mustStatus(t, http.StatusOK, "marking the failure as known")
}

// publishRunner puts the runner's frame on the bus as the controller renders
// it, which is how a runner still pointed at a failed job reaches the stream
// and the recent-events route.
func (h *harness) publishRunner(t *testing.T, run *store.Runner) {
	t.Helper()
	view, err := h.ctrl.RunnerRenderer(h.ctx, []*store.Runner{run})
	if err != nil {
		t.Fatalf("RunnerRenderer failed: %v", err)
	}
	h.ctrl.Events().Publish(events.KindRunnerUpdated, "runner:"+run.ID, view.View(run))
}

// A public view promises that nothing about the people who run the fleet is
// shown, and the name of the operator who marked a failure as known is one of
// them. A visitor is told that a failure was marked and when, which is the
// fleet's state, and never by whom. Every place the API renders a job, a
// runner's current job or a job's timeline is a place the name can leave by, so
// this walks them all, and each is asked again as a signed-in viewer, who is
// owed the name: a test that only watched it disappear would pass for a route
// that had stopped carrying it for everybody.
func TestAVisitorIsToldAFailureWasMarkedAsKnownButNeverWhoMarkedIt(t *testing.T) {
	h := newHarness(t, publicView)
	failed, run := h.failedJobOnARunner(t)
	h.markAsKnown(t, failed)
	h.publishRunner(t, run)
	viewer, _ := h.user("viewer", store.RoleViewer)

	for _, tc := range []struct {
		name, path string
		// keeps is what a visitor is still owed: that it happened, and when.
		keeps string
		// named is whether a signed-in viewer reads the name here at all.
		named bool
	}{
		{"the job", "/api/v1/jobs/" + failed.ID, `"acknowledged_at"`, true},
		{"the failed list", "/api/v1/jobs?failed=true", `"acknowledged_at"`, true},
		{"the job's timeline", "/api/v1/jobs/" + failed.ID + "/events", "marked this failure as known", true},
		{"the runner that ran it", "/api/v1/runners/" + run.ID, `"acknowledged_at"`, true},
		{"the runners list", "/api/v1/runners", `"acknowledged_at"`, true},
		{"the recent events", "/api/v1/events/recent?kinds=job.updated,runner.updated", `"acknowledged_at"`, true},
		{"the workflow runs", "/api/v1/workflow-runs?failed=true", `"github_run_id"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := h.do(request{method: http.MethodGet, path: tc.path})
			seen.mustStatus(t, http.StatusOK, "a visitor reading "+tc.name)
			if strings.Contains(string(seen.body), staffName) {
				t.Errorf("a visitor reading %s was told who marked the failure as known: %s", tc.name, truncate(seen.body))
			}
			if !strings.Contains(string(seen.body), tc.keeps) {
				t.Errorf("a visitor reading %s lost %s along with the name: %s", tc.name, tc.keeps, truncate(seen.body))
			}

			staff := h.do(request{method: http.MethodGet, path: tc.path, cookie: h.session(viewer)})
			staff.mustStatus(t, http.StatusOK, "a signed-in viewer reading "+tc.name)
			if got := strings.Contains(string(staff.body), staffName); got != tc.named {
				t.Errorf("a signed-in viewer reading %s was shown the name = %v, want %v: %s", tc.name, got, tc.named, truncate(staff.body))
			}
		})
	}
}

// The assistant's get_job reads the same routes as the page, as the caller, so
// a visitor's assistant is held to what the visitor is shown without a rule of
// its own, and a token's assistant still reads the name.
func TestAVisitorsAssistantIsNotToldWhoMarkedAFailureAsKnownEither(t *testing.T) {
	h := newHarness(t, publicView)
	failed, _ := h.failedJobOnARunner(t)
	h.markAsKnown(t, failed)

	visitor := h.mcpTool("", "get_job", map[string]any{"job_id": failed.ID})
	if visitor.IsError || !strings.Contains(resultText(visitor), "marked this failure as known") {
		t.Fatalf("a visitor's get_job did not answer with the timeline: %s", resultText(visitor))
	}
	if strings.Contains(resultText(visitor), staffName) {
		t.Errorf("a visitor's assistant was told who marked the failure as known: %s", resultText(visitor))
	}

	staff := h.mcpTool(h.token("agent", store.RoleViewer), "get_job", map[string]any{"job_id": failed.ID})
	if !strings.Contains(resultText(staff), staffName) {
		t.Errorf("a token's assistant lost the name: %s", resultText(staff))
	}
}

// A page that is open when a failure is marked as known learns of it from a
// frame, and a frame is the resource's GET in miniature, so it answers the
// GET's question about who is asking. The job's own frame and the frame of a
// runner still pointed at the job both carry the job.
func TestAVisitorsLiveStreamIsNotToldWhoMarkedAFailureAsKnownEither(t *testing.T) {
	h := newHarness(t, publicView)
	failed, run := h.failedJobOnARunner(t)
	viewer, _ := h.user("viewer", store.RoleViewer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	open := func(cookie string) <-chan sseFrame {
		t.Helper()
		frames, resp := h.openStream(t, ctx, "/api/v1/events", cookie, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("opening the stream answered %d", resp.StatusCode)
		}
		await(t, frames, "the opening comment", func(f sseFrame) bool { return f.comment != "" })
		return frames
	}
	audiences := []struct {
		name   string
		frames <-chan sseFrame
		named  bool
	}{
		{"a visitor", open(""), false},
		{"a signed-in viewer", open(h.session(viewer)), true},
	}

	h.markAsKnown(t, failed)
	h.publishRunner(t, run)

	for _, a := range audiences {
		for _, kind := range []events.Kind{events.KindJobUpdated, events.KindRunnerUpdated} {
			frame := await(t, a.frames, "the "+string(kind)+" frame carrying the acknowledgement", func(f sseFrame) bool {
				return f.event == string(kind) && strings.Contains(f.data, `"acknowledged_at"`)
			})
			if got := strings.Contains(frame.data, staffName); got != a.named {
				t.Errorf("%s was shown a %s frame naming the acknowledger = %v, want %v: %s", a.name, kind, got, a.named, frame.data)
			}
		}
	}
}

// A frame that names somebody and cannot be looked through is dropped for a
// visitor rather than sent as it is: the whole document is exactly what the
// narrowing exists to withhold, and the page repairs itself from the GET on any
// reload. Whoever may read the name is sent it as ever.
func TestAJobFrameThatCannotBeNarrowedIsNotSentToAVisitor(t *testing.T) {
	h := newHarness(t, publicView)
	viewer, _ := h.user("viewer", store.RoleViewer)
	h.ctrl.Events().Publish(events.KindJobUpdated, "job:odd", []map[string]string{{"acknowledged_by": staffName}})

	visitor := h.recent(t, "", "?kinds=job.updated")
	if len(visitor.Items) != 0 {
		t.Errorf("a visitor was sent a frame that could not be narrowed: %v", visitor.Items)
	}
	staff := h.recent(t, h.session(viewer), "?kinds=job.updated")
	if len(staff.Items) != 1 || !strings.Contains(string(staff.Items[0].Data), staffName) {
		t.Errorf("a signed-in viewer was not sent the frame: %v", staff.Items)
	}
}
