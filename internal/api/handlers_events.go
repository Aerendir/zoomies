package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/zoomies/internal/auth"
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
	// now asks for no events at all, only a cursor at the present, so that a
	// caller who wants to hear about what happens next is not first handed the
	// past to page through. On a busy fleet the ring holds five pages of it,
	// and a wait asked with a cursor from the start answers at once with each.
	now bool
}

// parseRecentSince reads `since`. The word now is the present; a value with a
// dot in it, or made only of digits, is an event id as the stream sends it;
// anything else is a duration.
// A dotted value whose sequence is not a number is tried as a duration before it
// is refused, because "1.5s" has a dot in it and means what it says.
func parseRecentSince(bus *events.Bus, raw string) (recentSince, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return recentSince{}, true
	}
	if raw == "now" {
		return recentSince{now: true}, true
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
		badRequestField(w, "since", "since must be an event id as the stream sends it, a duration such as 10m, or now")
		return
	}
	// The present is the bus's last id, taken before anything else is read, so
	// an event published while the rest of the request is parsed is after the
	// cursor and is the caller's to be told about, not behind it and lost.
	var nowMark uint64
	if since.now {
		nowMark = bus.LastID()
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
	if since.now {
		if wait <= 0 {
			// Nothing has happened since now, and the cursor is the whole answer.
			writeJSON(w, http.StatusOK, recentView{Items: []recentFrame{}, Next: bus.WireID(nowMark), Complete: true})
			return
		}
		// A wait from now is a wait from that id: the ring re-read after
		// subscribing then covers a frame published in between, as it does for
		// any cursor.
		since = recentSince{id: nowMark}
	}

	// Who the frames of this answer are narrowed for. Resolved once per
	// request, which lasts at most a wait, as the live stream resolves it
	// once per heartbeat: a page of frames is narrowed for one identity, and
	// each resolution of a session is a database read. A resolution that
	// fails is nobody, and nobody's frames are narrowed as a visitor's.
	id, _ := s.resolveIdentity(r)

	// The cursor, taken once. A timed-out wait hands this back unchanged, which
	// is what makes a poll loop that finds nothing idempotent.
	view := s.readRecent(r, id, since, opts, limit)
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
	sub, refused := s.subscribeStream(subCtx, Identity(r.Context()), opts)
	if refused != nil {
		limitReached(w, refused)
		return
	}
	// Closed here as well as by the context: the context's release happens on
	// the bus's own goroutine, after this response has gone, so a client that
	// reopens a stream the moment it has its answer could still be refused.
	defer sub.Close()
	// An event published between the read above and the subscription taking
	// effect is in the ring but was never offered to the subscriber. Reading
	// again, now that it is registered, closes that gap: anything later arrives
	// on the channel.
	if again := s.readRecent(r, id, since, opts, limit); len(again.Items) > 0 || again.Truncated {
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
				again := s.readRecent(r, id, since, opts, limit)
				// Frames may have been dropped with the feed, and the ring is
				// all there is to say what they were.
				again.Complete = false
				writeJSON(w, http.StatusOK, again)
				return
			}
			first, ok := s.recentFrame(r, ev, id)
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
					if f, ok := s.recentFrame(r, more, id); ok {
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
// The frames are narrowed for id, the identity the request resolved.
func (s *Server) readRecent(r *http.Request, id *auth.Identity, since recentSince, opts events.SubscribeOptions, limit int) recentView {
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
		cutoff := time.Now().Add(-since.window)
		var ring []events.Event
		items, ring, truncated = fromRing(bus, opts, limit, func(ev events.Event) bool { return !ev.At.Before(cutoff) })
		// Incomplete when the oldest event the ring holds is itself inside the
		// window and is not the first this run ever numbered: something older,
		// perhaps inside the window too, has been pushed out. An id taken by a
		// transient frame, which is never ringed, looks the same, so this errs
		// towards telling the caller to fetch afresh. An empty ring is whole
		// only while nothing has been numbered: an unkept frame, such as the
		// resync after many runner rows are pruned, is otherwise passed unseen.
		complete = ringIsWhole(ring, mark) || (len(ring) > 0 && !ring[0].At.After(cutoff))
	case since.foreign:
		items, _, truncated = bus.Recent(0, opts, limit)
	case since.fromStart:
		var ring []events.Event
		items, ring, truncated = fromRing(bus, opts, limit, nil)
		// A cursor of zero claims the ring still holds everything this run ever
		// numbered. Like the window read it errs towards false when a transient
		// frame took the first id, or any id while the ring is empty.
		complete = ringIsWhole(ring, mark)
	default:
		items, complete, truncated = bus.Recent(since.id, opts, limit)
	}

	out := recentView{Items: make([]recentFrame, 0, len(items)), Complete: complete, Truncated: truncated}
	for _, ev := range items {
		if f, ok := s.recentFrame(r, ev, id); ok {
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

// ringIsWhole says whether the ring still holds every frame this run has
// numbered, given mark, the newest id taken before the ring was read. A ring
// that is empty holds everything only if no id has been taken, because a
// transient frame takes one and is never kept.
func ringIsWhole(ring []events.Event, mark uint64) bool {
	if len(ring) == 0 {
		return mark == 0
	}
	return ring[0].ID <= 1
}

// fromRing reads the whole ring once and selects from that one read, so what it
// reports about the ring and what it returns cannot disagree about an event
// published between two reads. keep, when not nil, narrows by something the bus
// cannot filter on, such as age; the kinds and topic are the bus's own rule.
func fromRing(bus *events.Bus, opts events.SubscribeOptions, limit int, keep func(events.Event) bool) (items, ring []events.Event, truncated bool) {
	ring, _, _ = bus.Recent(0, events.SubscribeOptions{}, 0)
	for _, ev := range ring {
		if !opts.Matches(ev) || (keep != nil && !keep(ev)) {
			continue
		}
		items = append(items, ev)
	}
	if len(items) > limit {
		items, truncated = items[:limit], true
	}
	return items, ring, truncated
}

// recentFrame renders one event for this caller, or reports that it is not
// theirs to see.
//
// id is who the frame is narrowed for: the identity the request resolved,
// once, so every frame of a page is narrowed for the same caller.
func (s *Server) recentFrame(r *http.Request, ev events.Event, id *auth.Identity) (recentFrame, bool) {
	// The stream's rule: a frame goes only to a caller who could have fetched
	// it from its GET route. Resolved once per request, which lasts at most a
	// wait, as the stream resolves it once per heartbeat.
	if !frameReadable(Identity(r.Context()), ev.Kind) {
		return recentFrame{}, false
	}
	data, ok := s.frameFor(r, ev, id)
	if !ok {
		return recentFrame{}, false
	}
	if len(data) == 0 {
		// What the stream's data line carries for an event with no payload.
		data = []byte("{}")
	}
	return recentFrame{
		ID: s.ctrl.Events().WireID(ev.ID), Kind: ev.Kind, Topic: ev.Topic, At: ev.At, Data: data,
	}, true
}
