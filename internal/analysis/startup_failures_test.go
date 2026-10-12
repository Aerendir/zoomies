package analysis

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// startBase is 09:00 on the day of the period, so a failure m minutes in
// reads as 09:mm in a sentence.
var startBase = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

var startNames = map[string]string{
	"host_a": "demo-builder-1", "host_b": "demo-builder-2", "host_c": "demo-builder-3",
	"pool_linux": "linux", "pool_arm": "arm",
}

func startMinute(m int) time.Time { return startBase.Add(time.Duration(m) * time.Minute) }

func tp(t time.Time) *time.Time { return &t }

// failed is one failed start per minute given, each failing half a minute
// after it was asked for and before its container started.
func failed(host, pool, fault string, minutes ...int) []StartAttempt {
	var out []StartAttempt
	for _, m := range minutes {
		out = append(out, StartAttempt{RunnerID: fmt.Sprintf("run_%s_%s_f%d", host, pool, m), PoolID: pool,
			PoolName: startNames[pool], HostID: host, HostName: startNames[host], RequestedAt: startMinute(m),
			EndedAt: tp(startMinute(m).Add(30 * time.Second)), Outcome: StartFailed, FaultKind: fault,
			Message: "pull access denied for ghcr.io/acme/runner"})
	}
	return out
}

// started is one start per minute given that registered 40 seconds after it
// was asked for.
func started(host, pool string, minutes ...int) []StartAttempt {
	var out []StartAttempt
	for _, m := range minutes {
		out = append(out, StartAttempt{RunnerID: fmt.Sprintf("run_%s_%s_s%d", host, pool, m), PoolID: pool,
			PoolName: startNames[pool], HostID: host, HostName: startNames[host], RequestedAt: startMinute(m),
			ContainerStartedAt: tp(startMinute(m).Add(10 * time.Second)), RegisteredAt: tp(startMinute(m).Add(40 * time.Second)),
			Outcome: StartStarted})
	}
	return out
}

func unknownStarts(host, pool string, minutes ...int) []StartAttempt {
	var out []StartAttempt
	for _, m := range minutes {
		out = append(out, StartAttempt{RunnerID: fmt.Sprintf("run_%s_%s_u%d", host, pool, m), PoolID: pool,
			PoolName: startNames[pool], HostID: host, HostName: startNames[host], RequestedAt: startMinute(m),
			Outcome: StartUnknown})
	}
	return out
}

// minutesFrom is n consecutive minutes from the first.
func minutesFrom(first, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = first + i
	}
	return out
}

func startSnapshot(groups ...[]StartAttempt) StartupSnapshot {
	return StartupSnapshot{Since: startBase.Add(-3 * time.Hour), Until: startBase.Add(21 * time.Hour), Attempts: slices.Concat(groups...)}
}

func mustStartup(t *testing.T, s StartupSnapshot) StartupFailuresFinding {
	t.Helper()
	f, err := StartupFailures(s)
	if err != nil {
		t.Fatal(err)
	}
	if f.Analysis != "startup_failures" || f.Version != StartupFailuresVersion {
		t.Errorf("finding is %s/%s, want startup_failures/%s", f.Analysis, f.Version, StartupFailuresVersion)
	}
	if f.NextSteps == nil || f.Codes == nil || f.Evidence.Groups == nil {
		t.Error("next steps, codes and groups must be empty lists, never nil")
	}
	if (f.Confidence == High) != (f.ConfidenceReason == "") {
		t.Errorf("confidence %s with reason %q: a reason is given exactly when it is not high", f.Confidence, f.ConfidenceReason)
	}
	for _, text := range []string{f.Summary, f.ConfidenceReason} {
		if strings.Contains(text, "—") || strings.Contains(text, "--") {
			t.Errorf("a dash standing in for punctuation in %q", text)
		}
	}
	for _, step := range f.NextSteps {
		if step.Code != "" && !slices.Contains(f.Codes, step.Code) {
			t.Errorf("codes %v leave out step %+v's", f.Codes, step)
		}
	}
	assertNoCommand(t, f.NextSteps)
	return f
}

func wantSummary(t *testing.T, f StartupFailuresFinding, want string) {
	t.Helper()
	if f.Summary != want {
		t.Errorf("summary\n got %q\nwant %q", f.Summary, want)
	}
}

func wantConfidence(t *testing.T, f StartupFailuresFinding, want Confidence, reason string) {
	t.Helper()
	if f.Confidence != want || !strings.Contains(f.ConfidenceReason, reason) {
		t.Errorf("confidence %s (%q), want %s with %q", f.Confidence, f.ConfidenceReason, want, reason)
	}
}

func dominantGroup(t *testing.T, f StartupFailuresFinding) StartupGroup {
	t.Helper()
	if len(f.Evidence.Groups) == 0 {
		t.Fatalf("no groups behind dominant %q", f.Dominant)
	}
	g := f.Evidence.Groups[0]
	if got := g.Scope + ":" + g.ID; got != f.Dominant {
		t.Fatalf("the first group is %s, but the finding leads with %s", got, f.Dominant)
	}
	return g
}

func stepSettings(steps []NextStep) []string {
	var out []string
	for _, s := range steps {
		out = append(out, s.Setting)
	}
	return out
}

func TestStartupFailuresNamesTheHostThatKeepsFailing(t *testing.T) {
	f := mustStartup(t, startSnapshot(
		started("host_b", "pool_linux", -60),
		failed("host_b", "pool_linux", "image", 0, 10, 20, 30, 40, 50),
		started("host_a", "pool_linux", minutesFrom(-170, 12)...)))
	if f.Dominant != "host:host_b" {
		t.Errorf("dominant %q, want host:host_b", f.Dominant)
	}
	wantConfidence(t, f, High, "")
	wantSummary(t, f, "Host demo-builder-2 failed to start 6 of 7 runners in this period, failing since 2026-10-11 09:00 UTC, before the container started: the runner image could not be pulled.")
	if !slices.Contains(f.Codes, "host.image_pull_failed") {
		t.Errorf("codes %v, want host.image_pull_failed", f.Codes)
	}
	g := dominantGroup(t, f)
	want := StartupGroup{Scope: "host", ID: "host_b", Name: "demo-builder-2", Starts: 7, Failures: 6,
		FailingSince: startMinute(0).Add(30 * time.Second), LastAt: startMinute(50).Add(30 * time.Second),
		Stage: "before_container", FaultKind: "image", Explanation: "image", Explained: 6,
		LastRunnerID: "run_host_b_pool_linux_f50", LastMessage: "pull access denied for ghcr.io/acme/runner"}
	if g != want {
		t.Errorf("group\n got %+v\nwant %+v", g, want)
	}
	// The pool started twelve runners elsewhere, so 6 of its 19 is not a
	// pool that keeps failing, and listing it would point at the wrong place.
	if len(f.Evidence.Groups) != 1 {
		t.Errorf("groups %+v, want the host alone", f.Evidence.Groups)
	}
	e := f.Evidence
	if e.Starts != 19 || e.Failures != 6 || e.Unknown != 0 || e.GroupsCut != 0 || e.Truncated {
		t.Errorf("evidence %+v", e)
	}
	if got := stepSettings(f.NextSteps); !slices.Equal(got, []string{"pool.image", "host.cordoned"}) {
		t.Errorf("steps name %v, want the pool's image, then cordoning the host whose pool started elsewhere", got)
	}
}

func TestFailuresSpreadAcrossHostsNameThePool(t *testing.T) {
	f := mustStartup(t, startSnapshot(
		started("host_c", "pool_linux", -100, -90),
		failed("host_a", "pool_linux", "", 0, 10, 20, 30),
		failed("host_b", "pool_linux", "", 5, 15, 25, 35)))
	if f.Dominant != "pool:pool_linux" {
		t.Errorf("dominant %q, want pool:pool_linux", f.Dominant)
	}
	wantSummary(t, f, "Pool linux failed to start 8 of 10 runners in this period, failing since 2026-10-11 09:00 UTC, before the container started, and no image fault, open incident or throttle explains them.")
	var order []string
	for _, g := range f.Evidence.Groups {
		order = append(order, g.Scope+":"+g.ID)
	}
	if want := []string{"pool:pool_linux", "host:host_a", "host:host_b"}; !slices.Equal(order, want) {
		t.Errorf("groups %v, want %v: most failures first, then by name", order, want)
	}
	// A pool is no host to cordon.
	if slices.Contains(stepSettings(f.NextSteps), "host.cordoned") {
		t.Errorf("steps %+v cordon a host for a pool's failures", f.NextSteps)
	}
}

func TestARuntimeIncidentExplainsTheFailures(t *testing.T) {
	s := startSnapshot(failed("host_b", "pool_linux", "backend", 0, 10, 20, 30), started("host_a", "pool_linux", minutesFrom(-100, 10)...))
	s.Incidents = []HostIncident{{HostID: "host_b", Kind: "runtime", Since: startMinute(-5)}}
	f := mustStartup(t, s)
	if f.Dominant != "host:host_b" {
		t.Errorf("dominant %q, want host:host_b", f.Dominant)
	}
	wantConfidence(t, f, High, "")
	wantSummary(t, f, "Host demo-builder-2 failed to start 4 of 4 runners in this period, failing since 2026-10-11 09:00 UTC, before the container started: they came while the container runtime on demo-builder-2 was failing, as it has been since 2026-10-11 08:55 UTC.")
	if g := dominantGroup(t, f); g.Explanation != "runtime_incident" || g.Explained != 4 {
		t.Errorf("explanation %q covering %d, want runtime_incident covering 4", g.Explanation, g.Explained)
	}
	if len(f.NextSteps) == 0 || f.NextSteps[0].Code != "host.runtime_recovering" || f.NextSteps[0].Setting != "host.capacity" {
		t.Errorf("first step %+v, want host.runtime_recovering", f.NextSteps)
	}
}

// A pool failing on several hosts is told which, the one with most of the
// failures first, and its image step is about the pool's image, not a host.
func TestAPoolsExplanationNamesTheHostsBehindIt(t *testing.T) {
	s := startSnapshot(failed("host_a", "pool_linux", "backend", 0, 10, 20), failed("host_b", "pool_linux", "backend", 5, 15))
	s.Incidents = []HostIncident{{HostID: "host_a", Kind: "runtime", Since: startMinute(-5)}, {HostID: "host_b", Kind: "runtime", Since: startMinute(-10)}}
	f := mustStartup(t, s)
	if f.Dominant != "pool:pool_linux" {
		t.Errorf("dominant %q, want pool:pool_linux", f.Dominant)
	}
	if !strings.HasSuffix(f.Summary, ": they came while the container runtime on demo-builder-1 and 1 more host was failing, as it has been since 2026-10-11 08:50 UTC.") {
		t.Errorf("summary %q", f.Summary)
	}

	f = mustStartup(t, startSnapshot(failed("host_a", "pool_linux", "image", 0, 10, 20), failed("host_b", "pool_linux", "image", 5, 15)))
	if len(f.NextSteps) != 1 || !strings.HasPrefix(f.NextSteps[0].Text, "Check that pool linux's image names a tag that exists") {
		t.Errorf("steps %+v, want the pool's image checked", f.NextSteps)
	}
}

// The incident is heard of on the heartbeat after the start that found the
// runtime down, so the first failure can come before it; the rest still did.
func TestARuntimeIncidentHeardOfAfterTheFirstFailureStillExplainsMost(t *testing.T) {
	s := startSnapshot(failed("host_b", "pool_linux", "backend", 0, 10, 20, 30))
	s.Incidents = []HostIncident{{HostID: "host_b", Kind: "runtime", Since: startMinute(5)}}
	f := mustStartup(t, s)
	wantConfidence(t, f, High, "")
	if !strings.HasSuffix(f.Summary, ": 3 of them came while the container runtime on demo-builder-2 was failing, as it has been since 2026-10-11 09:05 UTC.") {
		t.Errorf("summary %q, want the three it covers counted", f.Summary)
	}
}

// Three in four covered is high; two in three is mixed evidence.
func TestAnExplanationCoveringFewerThanThreeInFourIsMedium(t *testing.T) {
	s := startSnapshot(failed("host_b", "pool_linux", "backend", 0, 10, 20))
	s.Incidents = []HostIncident{{HostID: "host_b", Kind: "runtime", Since: startMinute(5)}}
	f := mustStartup(t, s)
	if g := dominantGroup(t, f); g.Explanation != "runtime_incident" || g.Explained != 2 {
		t.Errorf("explanation %q covering %d, want runtime_incident covering 2", g.Explanation, g.Explained)
	}
	wantConfidence(t, f, Medium, "only 2 of its 3 failures came while the container runtime was failing, so the rest may have another cause")
}

// GitHub refusing to register a runner is not the runtime's doing, whatever
// the runtime was doing at the time.
func TestARuntimeIncidentDoesNotExplainAFaultThatNamesAnotherCause(t *testing.T) {
	s := startSnapshot(failed("host_b", "pool_linux", "registration", 0, 10, 20))
	s.Incidents = []HostIncident{{HostID: "host_b", Kind: "runtime", Since: startMinute(-5)}}
	f := mustStartup(t, s)
	if g := dominantGroup(t, f); g.Explanation != "" {
		t.Errorf("explanation %q for registration faults, want none", g.Explanation)
	}
}

// A throttle is timing alone, and a fault that names its own cause outweighs
// it.
func TestAThrottleDoesNotExplainAFaultThatNamesAnotherCause(t *testing.T) {
	s := startSnapshot(failed("host_b", "pool_linux", "config", 0, 10, 20))
	s.Throttles = []HostInterval{{HostID: "host_b", Interval: Interval{From: startMinute(-30)}}}
	if g := dominantGroup(t, mustStartup(t, s)); g.Explanation != "" {
		t.Errorf("explanation %q for config faults, want none", g.Explanation)
	}
}

func TestAnImagePullIncidentForThePoolExplainsTheFailures(t *testing.T) {
	s := startSnapshot(failed("host_b", "pool_linux", "", 0, 10, 20))
	s.Incidents = []HostIncident{{HostID: "host_b", Kind: "image_pull", PoolID: "pool_linux", Since: startMinute(1)}}
	f := mustStartup(t, s)
	wantConfidence(t, f, High, "")
	wantSummary(t, f, "Host demo-builder-2 failed to start 3 of 3 runners in this period, failing since 2026-10-11 09:00 UTC, before the container started: they came on demo-builder-2, which has been unable to pull pool linux's image since 2026-10-11 09:01 UTC.")
	if g := dominantGroup(t, f); g.Explanation != "image_pull_incident" {
		t.Errorf("explanation %q, want image_pull_incident", g.Explanation)
	}
	if len(f.NextSteps) == 0 || f.NextSteps[0].Code != "host.image_pull_failed" || f.NextSteps[0].Setting != "pool.image" {
		t.Errorf("first step %+v, want host.image_pull_failed on pool.image", f.NextSteps)
	}
}

func TestAnImagePullIncidentForAnotherPoolExplainsNothing(t *testing.T) {
	s := startSnapshot(failed("host_b", "pool_linux", "", 0, 10, 20))
	s.Incidents = []HostIncident{{HostID: "host_b", Kind: "image_pull", PoolID: "pool_arm", Since: startMinute(-1)}}
	if g := dominantGroup(t, mustStartup(t, s)); g.Explanation != "" {
		t.Errorf("explanation %q from another pool's image, want none", g.Explanation)
	}
}

// GitHub refusing a registration is not the image, whatever the host says
// about pulling it.
func TestAnImagePullIncidentDoesNotExplainAFaultThatNamesAnotherCause(t *testing.T) {
	s := startSnapshot(failed("host_b", "pool_linux", "registration", 0, 10, 20))
	s.Incidents = []HostIncident{{HostID: "host_b", Kind: "image_pull", PoolID: "pool_linux", Since: startMinute(-1)}}
	if g := dominantGroup(t, mustStartup(t, s)); g.Explanation != "" {
		t.Errorf("explanation %q for registration faults, want none", g.Explanation)
	}
}

// A fault recorded says more than none, so at a tie it is the one named.
func TestAGroupsFaultKindPrefersOneRecordedAtATie(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "", 0, 10), failed("host_b", "pool_linux", "registration", 20, 30)))
	if g := dominantGroup(t, f); g.FaultKind != "registration" {
		t.Errorf("fault kind %q, want registration", g.FaultKind)
	}
	if !strings.HasSuffix(f.Summary, ": 2 of them recorded the fault registration.") {
		t.Errorf("summary %q", f.Summary)
	}
}

// Half is not most: three image faults among six failures do not say the
// image is why the host fails, and four among seven do, though not firmly.
func TestImageFaultsExplainOnlyWhenMoreThanHalf(t *testing.T) {
	half := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "image", 0, 10, 20), failed("host_b", "pool_linux", "", 30, 40, 50)))
	if g := dominantGroup(t, half); g.Explanation != "" {
		t.Errorf("3 image faults of 6 explained as %q, want none", g.Explanation)
	}
	most := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "image", 0, 10, 20, 30), failed("host_b", "pool_linux", "", 40, 50, 60)))
	if g := dominantGroup(t, most); g.Explanation != "image" || g.Explained != 4 {
		t.Errorf("4 image faults of 7 explained as %q covering %d, want image covering 4", g.Explanation, g.Explained)
	}
	wantConfidence(t, most, Medium, "only 4 of its 7 failures recorded the image fault")
	if !strings.HasSuffix(most.Summary, ": 4 of them because the runner image could not be pulled.") {
		t.Errorf("summary %q, want the image faults counted", most.Summary)
	}
}

func TestAThrottleOverlapExplainsAtMediumConfidence(t *testing.T) {
	// [09:10:30, 09:30:30) holds the failures at 09:10:30 and 09:20:30 and
	// not the one at 09:30:30: exactly half of the four.
	s := startSnapshot(failed("host_b", "pool_linux", "", 0, 10, 20, 30))
	s.Throttles = []HostInterval{{HostID: "host_b", Interval: Interval{From: startMinute(10).Add(30 * time.Second), To: startMinute(30).Add(30 * time.Second)}},
		{HostID: "host_a", Interval: Interval{From: startMinute(-60)}}}
	f := mustStartup(t, s)
	g := dominantGroup(t, f)
	if g.Explanation != "throttle" || g.Explained != 2 {
		t.Errorf("explanation %q covering %d, want throttle covering 2", g.Explanation, g.Explained)
	}
	wantConfidence(t, f, Medium, "only the timing ties the failures to the throttle")
	if !strings.HasSuffix(f.Summary, ": 2 of them came while demo-builder-2 was throttled.") {
		t.Errorf("summary %q", f.Summary)
	}
	if len(f.NextSteps) == 0 || f.NextSteps[0].Code != "host.throttled" || f.NextSteps[0].Setting != "host.capacity" {
		t.Errorf("first step %+v, want host.throttled on host.capacity", f.NextSteps)
	}

	// A second later and only one of the four is inside it.
	s.Throttles[0].From = s.Throttles[0].From.Add(time.Second)
	if g := dominantGroup(t, mustStartup(t, s)); g.Explanation != "" {
		t.Errorf("one failure in four under a throttle explained as %q, want none", g.Explanation)
	}
}

// A throttle still on at the end of the period has no end yet.
func TestAThrottleStillOnCoversEveryFailureAfterItBegan(t *testing.T) {
	s := startSnapshot(failed("host_b", "pool_linux", "backend_busy", 0, 10, 20))
	s.Throttles = []HostInterval{{HostID: "host_b", Interval: Interval{From: startMinute(-30)}}}
	f := mustStartup(t, s)
	if !strings.HasSuffix(f.Summary, ": they came while demo-builder-2 was throttled.") {
		t.Errorf("summary %q", f.Summary)
	}
	// Every failure inside it is still only timing.
	wantConfidence(t, f, Medium, "only the timing ties the failures to the throttle")
}

func TestAnUnexplainedRepeatIsMedium(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "registration", 0, 10, 20), started("host_a", "pool_linux", minutesFrom(-100, 10)...)))
	g := dominantGroup(t, f)
	if g.Explanation != "" || g.FaultKind != "registration" {
		t.Errorf("explanation %q, fault kind %q; want none and registration", g.Explanation, g.FaultKind)
	}
	wantConfidence(t, f, Medium, "an incident that has already cleared is not kept")
	wantSummary(t, f, "Host demo-builder-2 failed to start 3 of 3 runners in this period, failing since 2026-10-11 09:00 UTC, before the container started, and no image fault, open incident or throttle explains them: all of them recorded the fault registration.")
	if len(f.NextSteps) == 0 || f.NextSteps[0].Code != "pool.runners_failing" || !strings.Contains(f.NextSteps[0].Text, "run_host_b_pool_linux_f20") {
		t.Errorf("first step %+v, want pool.runners_failing naming the latest runner", f.NextSteps)
	}
}

func TestNoRepeatedFailureSaysSo(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_a", "pool_linux", "image", 0), started("host_a", "pool_linux", minutesFrom(10, 9)...)))
	if f.Dominant != "" {
		t.Errorf("dominant %q, want none", f.Dominant)
	}
	wantConfidence(t, f, High, "")
	wantSummary(t, f, "No host or pool failed to start runners more than twice in this period: 1 of 10 starts failed.")
	if len(f.Evidence.Groups) != 0 || len(f.NextSteps) != 0 || len(f.Codes) != 0 {
		t.Errorf("groups %+v and steps %+v for nothing that repeats", f.Evidence.Groups, f.NextSteps)
	}
}

// Ten starts is the fewest that can say nothing keeps failing.
func TestNoRepeatOverFewerThanTenStartsIsMedium(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_a", "pool_linux", "", 0), started("host_a", "pool_linux", minutesFrom(10, 8)...)))
	wantConfidence(t, f, Medium, "only 9 starts in this period: too few to say that nothing keeps failing")
}

func TestNoFailedStartSaysEveryRunnerStarted(t *testing.T) {
	f := mustStartup(t, startSnapshot(started("host_a", "pool_linux", minutesFrom(0, 10)...)))
	wantConfidence(t, f, High, "")
	wantSummary(t, f, "No start failed in this period: 10 runners started.")
}

// Three failures that are under half a host's starts do not repeat, and the
// sentence must not claim nobody failed more than twice.
func TestFailuresUnderHalfAreNotARepeatAndSaySo(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_a", "pool_linux", "", 0, 10, 20), started("host_a", "pool_linux", minutesFrom(30, 7)...)))
	if f.Dominant != "" {
		t.Errorf("dominant %q for 3 failures in 10, want none", f.Dominant)
	}
	wantSummary(t, f, "No host or pool failed to start as many as half its runners in this period: 3 of 10 starts failed, the most on one being 3 of 10 on host demo-builder-1.")
}

func TestExactlyHalfFailedIsARepeat(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_a", "pool_linux", "", 0, 10, 20), started("host_a", "pool_linux", minutesFrom(30, 3)...)))
	if f.Dominant != "host:host_a" {
		t.Errorf("dominant %q for 3 failures in 6, want host:host_a", f.Dominant)
	}
}

func TestTwoFailuresAreNotARepeatHoweverFewTheStarts(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_b", "pool_arm", "", 0, 10), started("host_a", "pool_linux", minutesFrom(20, 10)...)))
	if f.Dominant != "" {
		t.Errorf("dominant %q for 2 failures of 2, want none", f.Dominant)
	}
	wantSummary(t, f, "No host or pool failed to start runners more than twice in this period: 2 of 12 starts failed.")
}

func TestTooFewStartsIsLow(t *testing.T) {
	f := mustStartup(t, startSnapshot())
	wantConfidence(t, f, Low, "no runners were asked to start in this period")
	wantSummary(t, f, "No runners were asked to start in this period.")
	if f.Dominant != "" {
		t.Errorf("dominant %q with nothing started", f.Dominant)
	}

	f = mustStartup(t, startSnapshot(unknownStarts("host_a", "pool_linux", 0, 10)))
	wantConfidence(t, f, Low, "no start in this period has a recorded outcome")
	wantSummary(t, f, "None of the 2 runners asked to start in this period has a recorded outcome.")

	f = mustStartup(t, startSnapshot(unknownStarts("host_a", "pool_linux", 0)))
	wantSummary(t, f, "The one runner asked to start in this period has no recorded outcome.")
}

func TestAStartWithNoRecordedOutcomeIsNotCountedAsAFailure(t *testing.T) {
	odd := unknownStarts("host_a", "pool_linux", 50)
	odd[0].Outcome = ""
	f := mustStartup(t, startSnapshot(started("host_a", "pool_linux", minutesFrom(0, 18)...), unknownStarts("host_a", "pool_linux", 30), odd))
	e := f.Evidence
	// An outcome the analysis does not know is no outcome, never a failure.
	if e.Starts != 18 || e.Failures != 0 || e.Unknown != 2 {
		t.Errorf("starts %d, failures %d, unknown %d; want 18, 0, 2", e.Starts, e.Failures, e.Unknown)
	}
	// Exactly a tenth unknown is still high.
	wantConfidence(t, f, High, "")
	wantSummary(t, f, "No start failed in this period: 18 runners started; 2 more have no recorded outcome.")
}

func TestMoreThanATenthWithNoOutcomeIsMedium(t *testing.T) {
	f := mustStartup(t, startSnapshot(started("host_a", "pool_linux", minutesFrom(0, 17)...), unknownStarts("host_a", "pool_linux", 30, 31)))
	wantConfidence(t, f, Medium, "2 of the 19 runners asked to start have no recorded outcome")
}

func TestAGroupsStartsWithNoOutcomeAreNamedInItsSentence(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "", 0, 10, 20, 30), unknownStarts("host_b", "pool_linux", 40, 41),
		started("host_a", "pool_linux", minutesFrom(-100, 20)...)))
	if g := dominantGroup(t, f); g.Unknown != 2 {
		t.Errorf("group unknown %d, want 2", g.Unknown)
	}
	if !strings.HasPrefix(f.Summary, "Host demo-builder-2 failed to start 4 of 4 runners in this period (2 more have no recorded outcome), failing since") {
		t.Errorf("summary %q", f.Summary)
	}
}

func TestStartupGroupsAreBounded(t *testing.T) {
	var parts [][]StartAttempt
	for i := range 30 {
		host := fmt.Sprintf("host_%02d", i)
		parts = append(parts, failed(host, "pool_linux", "", minutesFrom(0, i+3)...))
	}
	// Enough starts elsewhere that the pool itself is not failing.
	parts = append(parts, started("host_ok", "pool_linux", minutesFrom(-170, 600)...))
	f := mustStartup(t, startSnapshot(parts...))
	e := f.Evidence
	if len(e.Groups) != MaxStartupGroups || e.GroupsCut != 10 {
		t.Fatalf("%d groups with %d cut, want %d with 10 cut", len(e.Groups), e.GroupsCut, MaxStartupGroups)
	}
	for i, g := range e.Groups {
		if want := fmt.Sprintf("host_%02d", 29-i); g.ID != want || g.Failures != 32-i {
			t.Errorf("group %d is %s with %d failures, want %s with %d", i, g.ID, g.Failures, want, 32-i)
		}
	}
	if f.Dominant != "host:host_29" {
		t.Errorf("dominant %q, want host:host_29", f.Dominant)
	}
}

func TestATruncatedReadIsNeverHighConfidenceAboutStarts(t *testing.T) {
	s := startSnapshot(failed("host_b", "pool_linux", "image", 0, 10, 20), started("host_a", "pool_linux", minutesFrom(-100, 10)...))
	s.Truncated = true
	f := mustStartup(t, s)
	wantConfidence(t, f, Medium, "the newest 5000 were read, so narrow since")
	if !f.Evidence.Truncated {
		t.Error("evidence does not say the read was cut")
	}
}

// Failures before a start that registered ended with it; failing since is
// when the current run of them began.
func TestFailingSinceIsTheFirstFailureAfterTheLastStartThatRegistered(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "", 0), started("host_b", "pool_linux", 5), failed("host_b", "pool_linux", "", 10, 20, 30)))
	g := dominantGroup(t, f)
	if !g.FailingSince.Equal(startMinute(10).Add(30*time.Second)) || g.RecoveredAt != nil {
		t.Errorf("failing since %v, recovered %v; want 09:10:30 and still failing", g.FailingSince, g.RecoveredAt)
	}
}

// A run of failures that a start then broke is said to have ended, not to
// be going on.
func TestARecoveredGroupSaysWhenAStartRegistered(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "", 0, 10, 20), started("host_b", "pool_linux", 30)))
	g := dominantGroup(t, f)
	if !g.FailingSince.Equal(startMinute(0).Add(30*time.Second)) || g.RecoveredAt == nil || !g.RecoveredAt.Equal(startMinute(30).Add(40*time.Second)) {
		t.Errorf("failing since %v, recovered %v; want 09:00:30 and 09:30:40", g.FailingSince, g.RecoveredAt)
	}
	wantSummary(t, f, "Host demo-builder-2 failed to start 3 of 4 runners in this period, failing from 2026-10-11 09:00 UTC until a start registered at 2026-10-11 09:30 UTC, before the container started, and no image fault, open incident or throttle explains them.")

	f = mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "", 0), started("host_b", "pool_linux", 5),
		failed("host_b", "pool_linux", "", 10, 20, 30), started("host_b", "pool_linux", 40)))
	g = dominantGroup(t, f)
	if !g.FailingSince.Equal(startMinute(10).Add(30*time.Second)) || g.RecoveredAt == nil || !g.RecoveredAt.Equal(startMinute(40).Add(40*time.Second)) {
		t.Errorf("failing since %v, recovered %v; want 09:10:30 and 09:40:40", g.FailingSince, g.RecoveredAt)
	}
}

func TestAGroupsStageIsWhereMostOfItsFailuresStopped(t *testing.T) {
	late := failed("host_b", "pool_linux", "", 30)
	late[0].ContainerStartedAt = tp(startMinute(30).Add(5 * time.Second))
	f := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "", 0, 10, 20), late))
	if g := dominantGroup(t, f); g.Stage != "before_container" {
		t.Errorf("stage %q, want before_container", g.Stage)
	}
	if !strings.Contains(f.Summary, ", 3 of them before the container started,") {
		t.Errorf("summary %q, want the stage counted when not every failure stopped there", f.Summary)
	}

	registered := failed("host_b", "pool_linux", "", 40, 50)
	for i := range registered {
		registered[i].ContainerStartedAt = tp(registered[i].RequestedAt.Add(5 * time.Second))
		registered[i].RegisteredAt = tp(registered[i].RequestedAt.Add(10 * time.Second))
	}
	f = mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "", 0, 10), registered, late))
	if g := dominantGroup(t, f); g.Stage != "before_container" {
		t.Errorf("stage %q for a two-two tie, want the earlier, before_container", g.Stage)
	}
	if !strings.Contains(f.Summary, ", 2 of them before the container started,") {
		t.Errorf("summary %q", f.Summary)
	}
	if g := dominantGroup(t, mustStartup(t, startSnapshot(registered, failed("host_b", "pool_linux", "", 0)))); g.Stage != "after_registration" {
		t.Errorf("stage %q, want after_registration", g.Stage)
	}
	inside := failed("host_b", "pool_linux", "", 30, 35)
	for i := range inside {
		inside[i].ContainerStartedAt = tp(inside[i].RequestedAt.Add(5 * time.Second))
	}
	f = mustStartup(t, startSnapshot(inside, failed("host_b", "pool_linux", "", 0)))
	if g := dominantGroup(t, f); g.Stage != "before_registration" {
		t.Errorf("stage %q, want before_registration", g.Stage)
	}
	if !strings.Contains(f.Summary, ", 2 of them after the container started but before the runner registered,") {
		t.Errorf("summary %q", f.Summary)
	}
}

// A pool that fails only on one host, and starts on others, is that host's
// trouble: the host is named, counted on that pool alone, and the step keeps
// the pool off it rather than cordoning a host other pools start on.
func TestAPoolFailingOnOneHostWhileStartingElsewhereNamesTheHost(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "", 0, 10, 20, 30), started("host_a", "pool_linux", -50, -40),
		started("host_b", "pool_arm", minutesFrom(-100, 10)...)))
	if f.Dominant != "host:host_b" {
		t.Errorf("dominant %q, want host:host_b", f.Dominant)
	}
	g := dominantGroup(t, f)
	if g.PoolID != "pool_linux" || g.Starts != 4 || g.Failures != 4 {
		t.Errorf("group %+v, want host_b counted on pool_linux alone, 4 of 4", g)
	}
	wantSummary(t, f, "Host demo-builder-2 failed to start 4 of 4 runners of pool linux in this period, a pool whose runners started on other hosts, failing since 2026-10-11 09:00 UTC, before the container started, and no image fault, open incident or throttle explains them.")
	settings := stepSettings(f.NextSteps)
	if !slices.Contains(settings, "pool.host_selector") || slices.Contains(settings, "host.cordoned") {
		t.Errorf("steps name %v, want the pool's host selector and no cordon", settings)
	}
	if len(f.Evidence.Groups) != 2 || f.Evidence.Groups[1].Scope+":"+f.Evidence.Groups[1].ID != "pool:pool_linux" {
		t.Errorf("groups %+v, want the host on the pool, then the pool", f.Evidence.Groups)
	}
}

func TestAPoolThatFailedOnItsOnlyHostIsNamed(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "", 0, 10, 20, 30), started("host_b", "pool_arm", minutesFrom(-100, 10)...)))
	if f.Dominant != "pool:pool_linux" {
		t.Errorf("dominant %q, want pool:pool_linux", f.Dominant)
	}
}

// A host that only ever ran one pool fails the same starts as the pool; the
// host is the narrower name, and with nowhere else the pool started there is
// nowhere for a cordon to send it.
func TestAHostAndPoolWithTheSameFailuresNameTheHost(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("host_b", "pool_linux", "", 0, 10, 20, 30)))
	if f.Dominant != "host:host_b" {
		t.Errorf("dominant %q, want host:host_b", f.Dominant)
	}
	if settings := stepSettings(f.NextSteps); slices.Contains(settings, "host.cordoned") || slices.Contains(settings, "pool.host_selector") {
		t.Errorf("steps name %v with nowhere else to start", settings)
	}
}

// A runner not yet placed on a host still failed for its pool.
func TestAStartWithNoHostCountsForItsPool(t *testing.T) {
	f := mustStartup(t, startSnapshot(failed("", "pool_linux", "", 0, 10, 20)))
	if f.Dominant != "pool:pool_linux" {
		t.Errorf("dominant %q, want pool:pool_linux", f.Dominant)
	}
	for _, g := range f.Evidence.Groups {
		if g.Scope == "host" {
			t.Errorf("a host group %+v for runners with no host", g)
		}
	}
}

func TestStartsAskedForOutsideThePeriodAreLeftOut(t *testing.T) {
	s := startSnapshot(started("host_a", "pool_linux", minutesFrom(0, 10)...), failed("host_b", "pool_linux", "", -200, -190, -181), failed("host_b", "pool_linux", "", 21*60))
	f := mustStartup(t, s)
	if f.Evidence.Starts != 10 || f.Evidence.Failures != 0 {
		t.Errorf("starts %d, failures %d; want 10 and 0", f.Evidence.Starts, f.Evidence.Failures)
	}
}

func TestStartupFailuresRefusesAPeriodThatDoesNotRun(t *testing.T) {
	for _, s := range []StartupSnapshot{{Until: startBase}, {Since: startBase, Until: startBase}} {
		if _, err := StartupFailures(s); err == nil {
			t.Errorf("period %v to %v accepted", s.Since, s.Until)
		}
	}
}

// The fault kinds and incident kinds are the store's, which this package
// may not import; a renamed kind would otherwise explain nothing, silently.
func TestStartupKindsAreTheStoresOwn(t *testing.T) {
	for got, want := range map[string]store.FaultKind{faultImage: store.FaultImage, faultBackend: store.FaultBackend, faultBackendBusy: store.FaultBackendBusy} {
		if got != string(want) {
			t.Errorf("fault kind %q, the store's is %q", got, want)
		}
	}
	var kinds []string
	ty := reflect.TypeOf(store.HostIncidents{})
	for i := range ty.NumField() {
		name, _, _ := strings.Cut(ty.Field(i).Tag.Get("json"), ",")
		kinds = append(kinds, name)
	}
	if want := []string{IncidentRuntime, IncidentImagePull}; !slices.Equal(kinds, want) {
		t.Errorf("store.HostIncidents holds %v, the analysis reads %v", kinds, want)
	}
}
