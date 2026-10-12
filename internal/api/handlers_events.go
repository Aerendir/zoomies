package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/events"
)

// maxRecentWait is the longest GET /events/recent will hold a request, however
// much a client asks for.
//
// It sits inside the idle timeout of the proxies people put in front of this,
// the way the stream's heartbeat does, so a held request is answered by us with
// timed_out rather than cut by a middlebox that then looks like an outage. A
// client that wants longer asks again with the cursor it was given.
const maxRecentWait = 25 * time.Second

const (
	defaultRecentLimit = 50
	maxRecentLimit     = 200
)

// recentFrame is one event as the polling route renders it. The id is the wire
// id the stream sends, so a cursor taken from either is good for both.
type recentFrame struct {
	ID    string          `json:"id"`
	Kind  events.Kind     `json:"kind"`
	Topic string          `json:"topic"`
	At    time.Time       `json:"at"`
	Data  json.RawMessage `json:"data"`
}

// recentView is the body of GET /events/recent.
type recentView struct {
	Items     []recentFrame `json:"items"`
	Next      string        `json:"next"`
	Complete  bool          `json:"complete"`
	Truncated bool          `json:"truncated"`
	TimedOut  bool          `json:"timed_out"`
}

// recentSince is where a caller asked to read from.
type recentSince struct {
	// id is the sequence number to read after; zero is the whole ring.
	id uint64
	// fromStart says the caller named this run's very start (an id of zero in
	// this epoch), which is a claim about the ring and not a request for
	// "everything": the ring covers it only while it still holds the first event.
	fromStart bool
	// foreign says the id belongs to another run of the controller, so it names
	// nothing here and the read starts from the ring's start, incomplete.
	foreign bool
	// window, when positive, selects by age instead: events at or after now minus
	// window.
	window time.Duration
}

// parseRecentSince reads `since`. A value with a dot in it, or made only of
// digits, is an event id as the stream sends it; anything else is a duration.
// A dotted value whose sequence is not a number is tried as a duration before it
// is refused, because "1.5s" has a dot in it and means what it says.
func parseRecentSince(bus *events.Bus, raw string) (recentSince, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return recentSince{}, true
	}
	_, seq, dotted := strings.Cut(raw, ".")
	if !dotted {
		seq = raw
	}
	if _, err := strconv.ParseUint(seq, 10, 64); err == nil {
		id, same := bus.ParseWireID(raw)
		return recentSince{id: id, foreign: !same, fromStart: same && id == 0}, true
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return recentSince{}, false
	}
	return recentSince{window: d}, true
}

// handleRecentEvents answers "what happened since this event id" as plain JSON,
// optionally holding the request briefly for the next change.
//
// It exists for the caller that cannot keep a stream open: an agent, a script, a
// tool whose requests are all short. It reads the same ring the stream replays
// from, with the same ids, the same filters and the same narrowing by audience,
// and says whether the ring still reached back to the caller's cursor, so a hole
// is something it is told about rather than something it shows quietly.
func (s *Server) handleRecentEvents(w http.ResponseWriter, r *http.Request) {
	bus := s.ctrl.Events()
	q := r.URL.Query()

	since, ok := parseRecentSince(bus, q.Get("since"))
	if !ok {
		badRequestField(w, "since", "since must be an event id as the stream sends it, or a duration such as 10m")
		return
	}
	limit := queryInt(r, "limit", defaultRecentLimit)
	if limit < 1 {
		limit = defaultRecentLimit
	}
	limit = clamp(limit, 1, maxRecentLimit)

	var wait time.Duration
	if raw := strings.TrimSpace(q.Get("wait")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 {
			badRequestField(w, "wait", "wait must be a duration such as 20s, or 0 to answer at once")
			return
		}
		wait = min(d, s.recentWaitCap)
	}
	opts := events.SubscribeOptions{
		Kinds:       parseKinds(q.Get("kinds")),
		TopicPrefix: strings.TrimSpace(q.Get("topic")),
	}

	// The cursor, taken once. A timed-out wait hands this back unchanged, which
	// is what makes a poll loop that finds nothing idempotent.
	view := s.readRecent(r, since, opts, limit)
	// A truncated read is answered at once even when none of it is for this
	// caller: more is waiting in the ring after the cursor it carries, and holding
	// the request would let a newer frame overtake it.
	if len(view.Items) > 0 || view.Truncated || wait <= 0 {
		writeJSON(w, http.StatusOK, view)
		return
	}

	// Nothing yet. Holding the request is holding a subscription, so it is one of
	// the streams the instance allows, refused before anything is sent.
	subCtx, unsubscribe := context.WithCancel(r.Context())
	defer unsubscribe()
	ceiling := s.cfg().Limits.EventSubscribers
	sub, ok := bus.SubscribeWithin(subCtx, opts, ceiling)
	if !ok {
		limitReached(w, &controller.LimitError{
			Setting: "limits.event_subscribers", Limit: ceiling, What: "live-update streams",
			Free: "close a browser tab that is no longer needed",
		})
		return
	}
	// An event published between the read above and the subscription taking
	// effect is in the ring but was never offered to the subscriber. Reading
	// again, now that it is registered, closes that gap: anything later arrives
	// on the channel.
	if again := s.readRecent(r, since, opts, limit); len(again.Items) > 0 || again.Truncated {
		writeJSON(w, http.StatusOK, again)
		return
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-r.Context().Done():
			// The caller has gone; there is nobody to answer.
			return
		case <-timer.C:
			view.TimedOut = true
			writeJSON(w, http.StatusOK, view)
			return
		case ev, open := <-sub.C:
			if !open {
				// The bus ended this feed for falling behind. What is in the ring
				// is the answer now, whether or not it is what was waited for.
				writeJSON(w, http.StatusOK, s.readRecent(r, since, opts, limit))
				return
			}
			first, ok := s.recentFrame(r, ev)
			if !ok {
				continue
			}
			items := []recentFrame{first}
			// Whatever queued up behind it goes in the same answer, without
			// waiting for more. One past the limit is read so that truncated is
			// known rather than guessed; the extra frame is still in the ring for
			// the call that follows the cursor.
		drain:
			for len(items) <= limit {
				select {
				case more, open := <-sub.C:
					if !open {
						break drain
					}
					if f, ok := s.recentFrame(r, more); ok {
						items = append(items, f)
					}
				default:
					break drain
				}
			}
			truncated := len(items) > limit
			if truncated {
				items = items[:limit]
			}
			writeJSON(w, http.StatusOK, recentView{
				Items: items, Next: items[len(items)-1].ID, Complete: view.Complete, Truncated: truncated,
			})
			return
		}
	}
}

// readRecent is one read of the ring for the caller's cursor and filters.
func (s *Server) readRecent(r *http.Request, since recentSince, opts events.SubscribeOptions, limit int) recentView {
	bus := s.ctrl.Events()
	// Taken before the ring is read, so that the cursor handed back when nothing
	// matched can only be behind what was read, never past an event published in
	// between. It is the bus's last id rather than the ring's newest: a transient
	// frame uses an id without being ringed, and a cursor short of it would be
	// read later as a gap.
	mark := bus.LastID()

	var (
		items     []events.Event
		complete  bool
		truncated bool
	)
	switch {
	case since.window > 0:
		items, complete, truncated = recentWithin(bus, since.window, time.Now(), opts, limit)
	case since.foreign:
		items, _, truncated = bus.Recent(0, opts, limit)
	case since.fromStart:
		items, _, truncated = bus.Recent(0, opts, limit)
		complete = ringReachesStart(bus)
	default:
		items, complete, truncated = bus.Recent(since.id, opts, limit)
	}

	out := recentView{Items: make([]recentFrame, 0, len(items)), Complete: complete, Truncated: truncated}
	for _, ev := range items {
		if f, ok := s.recentFrame(r, ev); ok {
			out.Items = append(out.Items, f)
		}
	}
	if len(items) > 0 {
		// The last event looked at, shown or not: a frame this caller may not
		// see still has to be stepped over, or the next call would meet it again.
		out.Next = bus.WireID(items[len(items)-1].ID)
	} else {
		out.Next = bus.WireID(mark)
	}
	return out
}

// ringReachesStart reports whether the ring still holds everything this run ever
// numbered, which is what a cursor of zero claims. Like the window read it errs
// towards false when a transient frame took the first id.
func ringReachesStart(bus *events.Bus) bool {
	all, _, _ := bus.Recent(0, events.SubscribeOptions{}, 0)
	return len(all) == 0 || all[0].ID <= 1
}

// recentWithin selects the ringed events no older than window. The ring says
// nothing about time, so it is asked for everything and filtered on At.
//
// now is the wall clock, not the controller's, because the bus stamps events
// with the wall clock.
//
// It reports incomplete when the oldest event the ring holds is itself inside the
// window and is not the first this run ever numbered: something older than it,
// perhaps inside the window too, has been pushed out. An id taken by a transient
// frame, which is never ringed, looks the same, so this errs towards telling the
// caller to fetch afresh.
func recentWithin(bus *events.Bus, window time.Duration, now time.Time, opts events.SubscribeOptions, limit int) (items []events.Event, complete, truncated bool) {
	cutoff := now.Add(-window)
	all, _, _ := bus.Recent(0, events.SubscribeOptions{}, 0)
	complete = len(all) == 0 || all[0].ID <= 1 || !all[0].At.After(cutoff)

	matched, _, _ := bus.Recent(0, opts, 0)
	for _, ev := range matched {
		if !ev.At.Before(cutoff) {
			items = append(items, ev)
		}
	}
	if len(items) > limit {
		items, truncated = items[:limit], true
	}
	return items, complete, truncated
}

// recentFrame renders one event for this caller, or reports that it is not
// theirs to see.
func (s *Server) recentFrame(r *http.Request, ev events.Event) (recentFrame, bool) {
	data, ok := s.frameFor(r, ev)
	if !ok {
		return recentFrame{}, false
	}
	if len(data) == 0 {
		data = []byte("{}")
	}
	return recentFrame{
		ID: s.ctrl.Events().WireID(ev.ID), Kind: ev.Kind, Topic: ev.Topic, At: ev.At, Data: data,
	}, true
}
