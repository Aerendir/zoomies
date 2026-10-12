package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/mcp"
)

// maxEventsWait is the longest the route holds a request, whatever it is asked.
// The command clamps to it so that the line it prints on a quiet wait names the
// time the controller actually waited.
const maxEventsWait = 25

// maxEventsLimit is the most events the route returns in one answer.
const maxEventsLimit = 200

// runEvents is `zoomies events ...`: what the controller published lately.
func runEvents(ctx context.Context, e *env, args []string) error {
	return runGroup(ctx, e, "events", "What happened lately, and a wait for the next change.", []*subcommand{
		{"recent", "[--since ID|DURATION] [--wait SECONDS]", "The events since a point, optionally holding for the next one", eventsRecent},
	}, args)
}

// recentFrame is one event as GET /events/recent renders it. Data stays raw: it
// is the resource's own shape, and only a few of its fields are the
// controller's to vouch for.
type recentFrame struct {
	ID    string          `json:"id"`
	Kind  string          `json:"kind"`
	Topic string          `json:"topic"`
	At    string          `json:"at"`
	Data  json.RawMessage `json:"data"`
}

type recentEventsResponse struct {
	Items     []recentFrame `json:"items"`
	Next      string        `json:"next"`
	Complete  bool          `json:"complete"`
	Truncated bool          `json:"truncated"`
	TimedOut  bool          `json:"timed_out"`
}

func eventsRecent(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "zoomies events recent [--since ID|DURATION] [--kinds a,b] [--topic prefix] [--limit N] [--wait SECONDS]",
		"What the controller published lately, oldest first, one row for each event. "+
			"Pass the cursor a call prints as --since to read on from there; with --wait, a call with nothing new to show is held for the next matching event.")
	cf := registerClientFlags(fs, true)
	since := fs.String("since", "", "an event id as a previous call printed it, or a duration such as 10m; leave out for everything the controller still holds")
	kinds := fs.String("kinds", "", "only these kinds of event, comma-separated, such as job.updated,runner.updated")
	topic := fs.String("topic", "", "only events about resources whose topic starts with this, such as job: or runner:")
	limit := fs.Int("limit", 50, fmt.Sprintf("how many events at most (1 to %d)", maxEventsLimit))
	wait := fs.Int("wait", 0, fmt.Sprintf("seconds to hold for the next matching event when there is none yet (the controller holds %d at most)", maxEventsWait))
	fs.example(
		"zoomies events recent --since 10m",
		"zoomies events recent --kinds job.updated --topic job:",
		"zoomies events recent --since k3f9a2.1204 --wait 20",
		"zoomies events recent --output json",
	)
	if err := fs.parse(args); err != nil {
		return err
	}
	if err := fs.noMoreArgs(); err != nil {
		return err
	}
	if *limit < 1 || *limit > maxEventsLimit {
		return usagef("events recent", "--limit %d: ask for 1 to %d events; the controller would otherwise change it without saying", *limit, maxEventsLimit)
	}
	if *wait < 0 {
		return usagef("events recent", "--wait %d: seconds to wait cannot be negative; use 0 to answer at once", *wait)
	}
	seconds := min(*wait, maxEventsWait)
	client, err := cf.client()
	if err != nil {
		return err
	}
	p, err := cf.printer(e)
	if err != nil {
		return err
	}

	q := url.Values{}
	if s := strings.TrimSpace(*since); s != "" {
		q.Set("since", s)
	}
	if s := strings.TrimSpace(*kinds); s != "" {
		q.Set("kinds", s)
	}
	if s := strings.TrimSpace(*topic); s != "" {
		q.Set("topic", s)
	}
	q.Set("limit", strconv.Itoa(*limit))
	if seconds > 0 {
		// The route reads a duration, and a bare number is not one.
		q.Set("wait", strconv.Itoa(seconds)+"s")
	}

	var out recentEventsResponse
	raw, err := client.get(ctx, "/events/recent", q, &out)
	if err != nil {
		// A held request can be refused when the stream slots are full, and the
		// sentence it comes with says what to close or raise.
		return plainAPIError(err)
	}
	if p.structured() {
		return p.emit(raw)
	}
	printRecentEvents(p, out, time.Duration(seconds)*time.Second)
	return nil
}

func printRecentEvents(p *printer, out recentEventsResponse, waited time.Duration) {
	next := plain(out.Next)
	if !out.Complete {
		// The route says this for three reasons, and the person can act on any of
		// them the same way, so each is named and the advice is one.
		fmt.Fprintln(p.out, p.paint(colourYellow,
			"Some events may have been missed: the controller no longer holds events back to the point you asked for, "+
				"that point is from before the controller last restarted, or this wait fell behind the events as they came. "+
				"Fetch the resources afresh (zoomies status, zoomies jobs list, zoomies runners list) rather than trusting what follows to be everything."))
	}
	switch {
	case len(out.Items) > 0:
		rows := make([][]string, 0, len(out.Items))
		for _, f := range out.Items {
			rows = append(rows, []string{plain(f.At), plain(f.Kind), plain(f.Topic), recentDetail(f.Data)})
		}
		p.table([]string{"at", "kind", "topic", "detail"}, rows)
	case out.TimedOut:
		p.note("nothing changed in %s", waited)
	case out.Truncated:
		// Everything in the batch was for another audience, so nothing is shown,
		// but "no events" would be untrue: more is waiting past the cursor.
	default:
		p.note("no events")
	}
	switch {
	case out.Truncated && len(out.Items) > 0:
		p.note("More matched than --limit allows, and the oldest are shown. Read on with --since %s", next)
	case out.Truncated:
		p.note("More events are waiting past this point, though none of this batch is one you can read. Read on with --since %s, or ask for more at once with a larger --limit.", next)
	default:
		p.note("next: %s   (pass it as --since to read on from here)", next)
	}
}

// recentDetail is the controller's own fields of an event, as name=value. A
// value holding a space or a quote is quoted, so one field cannot run into the
// next; an object or an array is not shown at all.
func recentDetail(data json.RawMessage) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return ""
	}
	var parts []string
	for _, name := range mcp.TrustedFrameFields() {
		raw := strings.TrimSpace(string(fields[name]))
		if raw == "" {
			continue
		}
		var value string
		switch raw[0] {
		case '{', '[', 'n':
			continue
		case '"':
			if json.Unmarshal(fields[name], &value) != nil {
				continue
			}
		default:
			value = raw
		}
		value = plain(value)
		if value == "" || strings.ContainsAny(value, " \"=") {
			value = strconv.Quote(value)
		}
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, " ")
}
