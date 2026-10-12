package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/mcp"
)

// recentServer answers GET /events/recent with body and keeps the query it was
// sent, so a test can say what the command asked for as well as what it showed.
func recentServer(t *testing.T, status int, body string, asked *url.Values) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/events/recent" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if asked != nil {
			*asked = r.URL.Query()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The frames carry a job name a stranger wrote, a branch they chose and a
// topic that may quote either. None of it may reach the terminal as anything
// but inert text, and a name is not on the row at all: the row is what the
// controller says about the event.
const recentBody = `{"items":[
 {"id":"k3f9a2.1204","kind":"job.updated","topic":"job:job_1","at":"2026-10-12T09:00:01Z",
  "data":{"id":"job_1","state":"completed","conclusion":"failure","job_name":"` + hostileJSON + `","head_branch":"` + hostileJSON + `","repo":"acme/widgets"}},
 {"id":"k3f9a2.1205","kind":"scaling.decided","topic":"pool:pool_1","at":"2026-10-12T09:00:02Z",
  "data":{"id":"dec_1","reason":"pool a is full, so it holds at 8 runners"}},
 {"id":"k3f9a2.1206","kind":"host.updated","topic":"host:host_1","at":"2026-10-12T09:00:03Z","data":{"id":"host_1","healthy":false}}
],"next":"k3f9a2.1206","complete":true,"truncated":false,"timed_out":false}`

func TestEventsRecentPrintsOneRowPerFrameAndItsCursor(t *testing.T) {
	var asked url.Values
	srv := recentServer(t, http.StatusOK, recentBody, &asked)

	out, _ := runCLI(t, "events", "recent", "--since", "10m", "--kinds", "job.updated,host.updated",
		"--topic", "job:", "--limit", "20", "--url", srv.URL)

	for key, want := range map[string]string{"since": "10m", "kinds": "job.updated,host.updated", "topic": "job:", "limit": "20"} {
		if got := asked.Get(key); got != want {
			t.Errorf("the request carried %s=%q, want %q", key, got, want)
		}
	}
	if asked.Has("wait") {
		t.Errorf("the request asked to wait without being told to: %v", asked)
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// A header, one row for each of the three frames, and the cursor.
	if len(lines) != 5 {
		t.Fatalf("printed %d lines, want 5 (a header, three frames, the cursor):\n%s", len(lines), out)
	}
	for i, want := range [][]string{
		{"job.updated", "job:job_1", "2026-10-12T09:00:01Z", "state=completed", "conclusion=failure"},
		{"scaling.decided", "pool:pool_1", `reason="pool a is full, so it holds at 8 runners"`},
		{"host.updated", "host:host_1", "healthy=false"},
	} {
		for _, w := range want {
			if !strings.Contains(lines[i+1], w) {
				t.Errorf("row %d does not show %q:\n%s", i+1, w, lines[i+1])
			}
		}
	}
	if !strings.Contains(lines[4], "k3f9a2.1206") || !strings.Contains(lines[4], "--since") {
		t.Errorf("the last line is not the cursor and how to use it:\n%s", lines[4])
	}
	// The names are the stranger's. They are not trusted fields, so they are not
	// shown, and what is shown carries no control character or forged row.
	for _, leaked := range []string{"acme/widgets", "pwned", "deploy  success"} {
		if strings.Contains(out, leaked) {
			t.Errorf("a name a workflow wrote reached the terminal (%q):\n%s", leaked, out)
		}
	}
	assertNoTerminalControl(t, "events recent", out, 5)
}

func TestEventsRecentPrintsAHostileTopicAsInertText(t *testing.T) {
	srv := recentServer(t, http.StatusOK, `{"items":[
 {"id":"k.1","kind":"job.updated","topic":"job:`+shortHostileJSON+`","at":"2026-10-12T09:00:01Z","data":{"id":"job_1","state":"queued"}}
],"next":"k.1","complete":true,"truncated":false,"timed_out":false}`, nil)

	out, _ := runCLI(t, "events", "recent", "--url", srv.URL)

	// A header, the frame and the cursor: a newline in a topic did not add a row.
	assertNoTerminalControl(t, "events recent", out, 3)
}

// Nothing on a row is a workflow's to write, but the row also carries what the
// event bus put there, and a payload field is trusted by its name and not by
// its bytes. So a control character anywhere in a frame, in the kind, the
// time, a trusted field's value or the cursor, is as inert as a name would be.
func TestEventsRecentPrintsEveryFieldOfAFrameWithoutControlCharacters(t *testing.T) {
	srv := recentServer(t, http.StatusOK, `{"items":[
 {"id":"k.1","kind":"job.updated`+shortHostileJSON+`","topic":"job:job_1","at":"2026-10-12T09:00:01Z`+shortHostileJSON+`",
  "data":{"id":"job_1","state":"queued\u001b[2K\u009b","code":"c\u202e","reason":"held`+shortHostileJSON+`"}}
],"next":"k.1`+shortHostileJSON+`","complete":true,"truncated":false,"timed_out":false}`, nil)

	out, _ := runCLI(t, "events", "recent", "--url", srv.URL)

	// A header, the frame and the cursor: no newline in any of them added a row.
	assertNoTerminalControl(t, "events recent", out, 3)
}

func TestEventsRecentPrintsAHostileCursorWithoutControlCharactersWhenNothingChanged(t *testing.T) {
	srv := recentServer(t, http.StatusOK,
		`{"items":[],"next":"k.1`+shortHostileJSON+`","complete":true,"truncated":false,"timed_out":true}`, nil)

	out, _ := runCLI(t, "events", "recent", "--since", "k.1", "--wait", "5", "--url", srv.URL)

	assertNoTerminalControl(t, "events recent", out, 2)
}

func TestEventsRecentSaysNothingChangedOnATimedOutWait(t *testing.T) {
	var asked url.Values
	srv := recentServer(t, http.StatusOK,
		`{"items":[],"next":"k3f9a2.1206","complete":true,"truncated":false,"timed_out":true}`, &asked)

	out, _ := runCLI(t, "events", "recent", "--since", "k3f9a2.1206", "--wait", "20", "--url", srv.URL)

	if got := asked.Get("wait"); got != "20s" {
		t.Errorf("the request carried wait=%q, want 20s: the route reads a duration, and a bare 20 is not one", got)
	}
	if !strings.Contains(out, "nothing changed in 20s") {
		t.Errorf("a wait that timed out does not say so:\n%s", out)
	}
	if !strings.Contains(out, "k3f9a2.1206") {
		t.Errorf("a poll loop needs the cursor back, and it was not printed:\n%s", out)
	}
}

func TestEventsRecentSaysInWordsThatEventsWereMissed(t *testing.T) {
	srv := recentServer(t, http.StatusOK, `{"items":[
 {"id":"k.9","kind":"job.updated","topic":"job:job_1","at":"2026-10-12T09:00:01Z","data":{"id":"job_1","state":"queued"}}
],"next":"k.9","complete":false,"truncated":false,"timed_out":false}`, nil)

	out, _ := runCLI(t, "events", "recent", "--since", "k.1", "--url", srv.URL)

	if !strings.Contains(out, "missed") || !strings.Contains(out, "afresh") {
		t.Errorf("an incomplete answer does not tell the person events were missed and to fetch the resources afresh:\n%s", out)
	}
	// The route says "incomplete" for three reasons, and the person can act on
	// each, so each is named: the cursor is older than what the controller holds,
	// it is from before a restart, or a held request fell behind.
	for _, cause := range []string{"no longer holds events back", "restarted", "fell behind"} {
		if !strings.Contains(out, cause) {
			t.Errorf("the sentence does not name the cause %q:\n%s", cause, out)
		}
	}
}

func TestEventsRecentSaysWhenMoreMatchedThanTheLimit(t *testing.T) {
	srv := recentServer(t, http.StatusOK, `{"items":[
 {"id":"k.9","kind":"job.updated","topic":"job:job_1","at":"2026-10-12T09:00:01Z","data":{"id":"job_1","state":"queued"}}
],"next":"k.9","complete":true,"truncated":true,"timed_out":false}`, nil)

	out, _ := runCLI(t, "events", "recent", "--limit", "1", "--url", srv.URL)

	if !strings.Contains(out, "More matched than --limit") || !strings.Contains(out, "--since k.9") {
		t.Errorf("a truncated answer does not say how to read on:\n%s", out)
	}
}

func TestEventsRecentSaysMoreIsWaitingWhenNoneOfItIsVisible(t *testing.T) {
	// Every frame of the batch was another audience's, so none is shown, but the
	// route still says more matched than the limit.
	srv := recentServer(t, http.StatusOK,
		`{"items":[],"next":"k.40","complete":true,"truncated":true,"timed_out":false}`, nil)

	out, _ := runCLI(t, "events", "recent", "--since", "k.1", "--url", srv.URL)

	if strings.Contains(out, "no events") {
		t.Errorf("a truncated answer with nothing visible says there are no events:\n%s", out)
	}
	for _, want := range []string{"More events are waiting", "--since k.40", "--limit"} {
		if !strings.Contains(out, want) {
			t.Errorf("the line does not say %q:\n%s", want, out)
		}
	}
}

func TestEventsRecentPrintsTheRoutesBodyAsItIsForJSON(t *testing.T) {
	srv := recentServer(t, http.StatusOK, recentBody, nil)

	out, _ := runCLI(t, "events", "recent", "--output", "json", "--url", srv.URL)

	// The names are in the body: this is the route's answer for a program to
	// read, not a page for a person, and it is not edited on the way.
	for _, want := range []string{`"next": "k3f9a2.1206"`, `"complete": true`, `"head_branch"`} {
		if !strings.Contains(out, want) {
			t.Errorf("--output json does not carry %s:\n%s", want, out)
		}
	}
}

func TestEventsRecentPrintsTheControllersSentenceWhenTheStreamSlotsAreFull(t *testing.T) {
	srv := recentServer(t, http.StatusConflict,
		`{"error":{"code":"limit_reached","message":"This instance is already serving 20 event streams, the most it allows. Close one, or raise limits.event_subscribers.\u001b[2K"}}`, nil)

	_, errOut, code := runCLIFailing(t, "events", "recent", "--since", "k.1", "--wait", "20", "--url", srv.URL)

	if code == exitOK {
		t.Fatal("a refused wait exited 0")
	}
	if !strings.Contains(errOut, "already serving 20 event streams") || !strings.Contains(errOut, "limits.event_subscribers") {
		t.Errorf("the controller's own sentence was not printed:\n%s", errOut)
	}
	if !strings.Contains(errOut, "limit_reached") {
		t.Errorf("the stable code a script goes by was not printed:\n%s", errOut)
	}
	if strings.ContainsRune(errOut, '\x1b') {
		t.Errorf("a terminal control sequence in the refusal reached the terminal:\n%q", errOut)
	}
}

func TestEventsRecentRefusesANegativeWaitBeforeAskingAnything(t *testing.T) {
	srv := recentServer(t, http.StatusOK, `{}`, nil)

	_, errOut, code := runCLIFailing(t, "events", "recent", "--wait", "-3", "--url", srv.URL)

	if code != exitUsage {
		t.Errorf("exit code = %d, want %d for a wait that cannot be", code, exitUsage)
	}
	if !strings.Contains(errOut, "--wait") {
		t.Errorf("the refusal does not name the flag:\n%s", errOut)
	}
}

func TestEventsRecentHoldsAWaitToWhatTheRouteWillHold(t *testing.T) {
	var asked url.Values
	srv := recentServer(t, http.StatusOK,
		`{"items":[],"next":"k.1","complete":true,"truncated":false,"timed_out":true}`, &asked)

	out, _ := runCLI(t, "events", "recent", "--since", "k.1", "--wait", "90", "--url", srv.URL)

	// The route holds a request for 25 seconds whatever it is asked for, so the
	// line says what was waited and not what was hoped for.
	if got := asked.Get("wait"); got != "25s" {
		t.Errorf("the request carried wait=%q, want 25s", got)
	}
	if !strings.Contains(out, "nothing changed in 25s") {
		t.Errorf("the line names a wait the controller did not give:\n%s", out)
	}
}

func TestEventsRecentRefusesALimitTheRouteWouldChange(t *testing.T) {
	srv := recentServer(t, http.StatusOK, `{}`, nil)
	for _, limit := range []string{"0", "-5", "201", "5000"} {
		_, errOut, code := runCLIFailing(t, "events", "recent", "--limit", limit, "--url", srv.URL)
		if code != exitUsage {
			t.Errorf("--limit %s: exit code = %d, want %d", limit, code, exitUsage)
		}
		if !strings.Contains(errOut, "--limit") || !strings.Contains(errOut, "1 to 200") {
			t.Errorf("--limit %s: the refusal does not name the flag and the range:\n%s", limit, errOut)
		}
	}
}

// The row's fields are the controller's own, and the MCP tool decides which
// they are. The command reads that tool's list rather than keeping a copy that
// a comment would have to keep in step, so a field added there shows here.
func TestEventsRecentShowsEveryFieldTheMCPToolTrusts(t *testing.T) {
	var data []string
	for _, name := range mcp.TrustedFrameFields() {
		data = append(data, `"`+name+`":"v_`+name+`"`)
	}
	srv := recentServer(t, http.StatusOK, `{"items":[{"id":"k.1","kind":"x.updated","topic":"x:1","at":"2026-10-12T09:00:01Z",
 "data":{`+strings.Join(data, ",")+`,"job_name":"a-name"}}],"next":"k.1","complete":true,"truncated":false,"timed_out":false}`, nil)

	out, _ := runCLI(t, "events", "recent", "--url", srv.URL)

	for _, name := range mcp.TrustedFrameFields() {
		if !strings.Contains(out, name+"=v_"+name) {
			t.Errorf("the row does not show the trusted field %s:\n%s", name, out)
		}
	}
	if strings.Contains(out, "a-name") {
		t.Errorf("the row shows a field the MCP tool does not trust:\n%s", out)
	}
}
