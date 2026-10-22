package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/eyupio/zoomies/internal/agent"
	"github.com/eyupio/zoomies/internal/backend"
	"github.com/eyupio/zoomies/internal/store"
)

// Tunables for the relay. A browser tab that has been backgrounded stops
// reading its SSE connection, and a runner compiling something noisy produces
// megabytes a minute; the queue depth bounds how much of that is held for a
// reader who may never come back.
const (
	logSubscriberQueue = 512
	logReadChunk       = 32 << 10
	// logStreamBytesPerSecond and logStreamBurst are one stream's byte budget.
	// The subscriber queue bounds what is held for a slow reader; this bounds
	// what one agent can push through the relay at all, which is otherwise as
	// fast as it can write and is copied once per viewer. A megabyte a second
	// is more than any runner's output a person can read, and the burst
	// covers the backlog a stream opened with --tail sends at once.
	logStreamBytesPerSecond = 1 << 20
	logStreamBurst          = 8 << 20
)

// ErrStreamUnknown is returned when an agent posts a log stream nobody is
// waiting for, which happens whenever the last viewer closed the tab between
// the task being issued and the agent acting on it.
var ErrStreamUnknown = errors.New("controller: no log stream is open with that ID")

// logRelay fans a runner's output out to everyone watching it.
//
// The direction is inverted compared with what one would expect, because the
// controller never dials an agent: a viewer asks the controller, the
// controller queues a stream_logs task, and the agent opens an outbound
// chunked POST that lands in AcceptLogStream.
type logRelay struct {
	c *Controller

	mu       sync.Mutex
	streams  map[string]*logStream // by stream ID
	byRunner map[string]string     // runner ID -> stream ID
}

type logStream struct {
	id       string
	runnerID string
	hostID   string

	mu     sync.Mutex
	subs   map[int]chan []byte
	nextID int
	closed bool

	// budget is the stream's remaining byte allowance and budgetAt when it
	// was last topped up, both under budgetMu. One goroutine is meant to read
	// the agent's POST, but an agent that opens the stream again while this
	// side is still blocked on a connection that has not failed yet puts a
	// second one on it, and the two have to share one allowance.
	budgetMu sync.Mutex
	budget   float64
	budgetAt time.Time
}

// spend reports whether n bytes fit in the stream's budget at now, and takes
// them if they do. A chunk that does not fit is dropped whole rather than cut:
// half a line is worse than a gap.
func (s *logStream) spend(n int, now time.Time) bool {
	s.budgetMu.Lock()
	defer s.budgetMu.Unlock()
	if s.budgetAt.IsZero() {
		s.budget = logStreamBurst
	} else if d := now.Sub(s.budgetAt); d > 0 {
		s.budget = min(logStreamBurst, s.budget+d.Seconds()*logStreamBytesPerSecond)
	}
	s.budgetAt = now
	if float64(n) > s.budget {
		return false
	}
	s.budget -= float64(n)
	return true
}

func newLogRelay(c *Controller) *logRelay {
	return &logRelay{c: c, streams: map[string]*logStream{}, byRunner: map[string]string{}}
}

// OpenLogStream subscribes to a runner's output and returns the channel the
// API's SSE handler reads, plus the function that unsubscribes.
//
// The channel is closed when the stream ends, so a handler can range over it.
// Bytes are dropped rather than queued without limit when a reader falls
// behind: a wedged browser tab must not hold a runner's output in memory.
func (c *Controller) OpenLogStream(ctx context.Context, runnerID string, opts backend.LogOptions) (<-chan []byte, func(), error) {
	r, err := c.st.GetRunner(ctx, runnerID)
	if err != nil {
		return nil, nil, err
	}
	if r.HostID == "" {
		return nil, nil, fmt.Errorf("runner %s is not on a host, so it has no logs to stream", runnerID)
	}
	if r.State == store.RunnerRemoved {
		// The container is gone and so is its output; saying so beats an empty
		// stream that looks like a runner producing nothing.
		return nil, nil, fmt.Errorf("runner %s has been removed; an ephemeral runner's output only exists while its container does", runnerID)
	}
	// host_id references hosts(id) ON DELETE CASCADE, so a runner row cannot
	// outlive its host: this is a real query failure, not a deletion race.
	h, err := c.st.GetHost(ctx, r.HostID)
	if err != nil {
		return nil, nil, err
	}
	if !h.Healthy(c.Now()) {
		// Nothing would ever answer this stream: no agent is going to poll for
		// the task it enqueues. Refusing here, rather than opening one that
		// waits forever, is what keeps a repeatedly reopened stream from
		// piling up queued tasks for a host that is never coming back.
		return nil, nil, fmt.Errorf("runner %s's host has not been heard from recently, so it cannot stream logs right now", runnerID)
	}
	return c.relay.subscribe(r, opts)
}

// subscribe attaches a viewer, starting the stream if this is the first one.
func (lr *logRelay) subscribe(r *store.Runner, opts backend.LogOptions) (<-chan []byte, func(), error) {
	lr.mu.Lock()
	var s *logStream
	var streamID string
	fresh := true
	if opts.Follow {
		if curID, ok := lr.byRunner[r.ID]; ok {
			s = lr.streams[curID]
			if s != nil && !s.isClosed() {
				streamID = curID
				fresh = false
			}
		}
	}
	// A viewer joining a stream that is already running only gets output from
	// now on, because the live stream's subscribers are fed by the agent's
	// current POST. If it asked for a tail, open a second, short-lived relay of
	// its own: the agent answers it with the backlog and then closes it. It is
	// deliberately not in byRunner, since it is not the stream a later viewer
	// should join.
	var tail *logStream
	var tailTask agent.Task
	if !fresh && opts.Tail > 0 {
		tailOpts := backend.LogOptions{Follow: false, Tail: opts.Tail}
		tailID := "log_" + store.NewSecret(8)
		tail = &logStream{id: tailID, runnerID: r.ID, hostID: r.HostID, subs: map[int]chan []byte{}}
		lr.streams[tailID] = tail
		tailTask = agent.Task{
			Kind:       agent.TaskStreamLogs,
			RunnerID:   r.ID,
			StreamID:   tailID,
			LogOptions: &tailOpts,
		}
	}
	if fresh {
		streamID = "log_" + store.NewSecret(8)
		s = &logStream{id: streamID, runnerID: r.ID, hostID: r.HostID, subs: map[int]chan []byte{}}
		lr.streams[streamID] = s
		if opts.Follow {
			lr.byRunner[r.ID] = streamID
		}
	}
	// The join happens under the same hold of lr.mu that chose the stream. If
	// it were made after the lock was released, a teardown deciding "last
	// viewer" in between would empty the stream and close it a moment later,
	// and the viewer that had just joined would watch its channel close under
	// it with nothing said. The lock order is lr.mu then s.mu, the same order
	// the teardown takes, so the two cannot deadlock.
	ch, subID, err := s.add()
	if err != nil {
		lr.mu.Unlock()
		if tail != nil {
			lr.discard(tail)
		}
		return nil, nil, err
	}

	var tailCh chan []byte
	var tailSubID int
	if tail != nil {
		tailCh, tailSubID, err = tail.add()
		if err != nil {
			lr.mu.Unlock()
			lr.unsubscribe(s, subID)
			lr.discard(tail)
			return nil, nil, err
		}
	}
	lr.mu.Unlock()

	if fresh {
		lr.c.enqueue(r.HostID, agent.Task{
			Kind:       agent.TaskStreamLogs,
			RunnerID:   r.ID,
			StreamID:   streamID,
			LogOptions: &opts,
		})
		lr.c.log.Debug("opened a log relay", "runner", r.ID, "stream", streamID, "host", r.HostID)
	}

	if tail == nil {
		var once sync.Once
		cancel := func() { once.Do(func() { lr.unsubscribe(s, subID) }) }
		return ch, cancel, nil
	}

	lr.c.enqueue(r.HostID, tailTask)
	lr.c.log.Debug("opened a tail relay for a later viewer", "runner", r.ID, "stream", tail.id, "host", r.HostID)

	merged := make(chan []byte, logSubscriberQueue)
	done := make(chan struct{})
	go func() {
		defer lr.c.recoverDetached("log-merge")
		mergeLogRelays(merged, ch, tailCh, done)
	}()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			// Stop the merge before the channels close under it, and let the
			// tail's own stream finish rather than leaving it open for a
			// viewer that has gone.
			close(done)
			lr.unsubscribe(s, subID)
			lr.unsubscribe(tail, tailSubID)
		})
	}
	return merged, cancel, nil
}

// mergeLogRelays fans a live stream and a later viewer's tail into one channel.
//
// The tail is a backlog that arrives once and the live output is what follows
// it, so the tail goes first: a viewer that asked for the backlog must see the
// history before the present, not interleaved with it. The live output is not
// held back until the tail ends, though, because a tail that never ends would
// then silence the runner's output for as long as it lasted.
//
// The live stream is the one the viewer's page is attached to, so its end ends
// the merged channel; a tail that ends first simply stops contributing. done is
// closed by cancel, so a handler that goes away cannot leave this goroutine
// blocked on a send nobody will receive.
func mergeLogRelays(merged chan<- []byte, live, tail <-chan []byte, done <-chan struct{}) {
	defer close(merged)
	for {
		// The viewer goes first, checked before anything else: a merge that
		// keeps draining a tail nobody is reading is a goroutine that outlives
		// the page it was serving.
		select {
		case <-done:
			return
		default:
		}
		// The tail first, whenever it has something to say.
		var keep bool
		tail, keep = drainTail(merged, tail, done)
		if !keep {
			return
		}
		select {
		case <-done:
			return
		case chunk, ok := <-live:
			if !ok {
				return
			}
			// The tail may have arrived in the same moment as this chunk, and
			// whichever the select picked, the backlog is the history: it goes
			// first, so the two are never interleaved.
			tail, keep = drainTail(merged, tail, done)
			if !keep {
				return
			}
			if !forwardLogChunk(merged, chunk, done) {
				return
			}
		case chunk, ok := <-tail:
			if !ok {
				tail = nil
				continue
			}
			if !forwardLogChunk(merged, chunk, done) {
				return
			}
		}
	}
}

// drainTail hands over what the tail holds without waiting for more, and
// returns the tail to read from next, which is nil once the tail has ended.
// It reports whether the merge should carry on.
func drainTail(merged chan<- []byte, tail <-chan []byte, done <-chan struct{}) (<-chan []byte, bool) {
	for {
		select {
		case chunk, ok := <-tail:
			if !ok {
				return nil, true
			}
			if !forwardLogChunk(merged, chunk, done) {
				return nil, false
			}
		default:
			return tail, true
		}
	}
}

// forwardLogChunk hands one chunk to the merged channel and reports whether the
// merge should carry on. It reports false once the viewer has gone.
func forwardLogChunk(merged chan<- []byte, chunk []byte, done <-chan struct{}) bool {
	select {
	case merged <- chunk:
		return true
	case <-done:
		return false
	}
}

// discard removes a stream that never got a viewer, so a failed join does not
// leave its ID behind for an agent to POST into.
func (lr *logRelay) discard(s *logStream) {
	lr.mu.Lock()
	if lr.streams[s.id] == s {
		delete(lr.streams, s.id)
	}
	lr.mu.Unlock()
	s.close()
}

// unsubscribe drops one viewer and tears the stream down when it was the last.
func (lr *logRelay) unsubscribe(s *logStream, subID int) {
	// Decide "last viewer" and empty the maps under one hold of lr.mu. If the
	// two happened separately, a viewer arriving in between would see a stream
	// still in byRunner that is already being torn down, join it, and have its
	// channel closed moments later. Lock order is lr.mu then s.mu everywhere
	// (subscribe already takes them in that order), so this cannot deadlock
	// with add or close.
	lr.mu.Lock()
	last := s.remove(subID)
	if last {
		if lr.streams[s.id] == s {
			delete(lr.streams, s.id)
		}
		if lr.byRunner[s.runnerID] == s.id {
			delete(lr.byRunner, s.runnerID)
		}
	}
	lr.mu.Unlock()
	if !last {
		return
	}

	// Nobody is watching, so tell the agent to stop reading the container's
	// output and close its outbound POST.
	lr.c.enqueue(s.hostID, agent.Task{
		Kind:     agent.TaskCancelLogs,
		RunnerID: s.runnerID,
		StreamID: s.id,
	})
	s.close()
	lr.c.log.Debug("closed a log relay; the last viewer went away", "runner", s.runnerID, "stream", s.id)
}

// fail ends a stream whose relay the agent could not open, after handing every
// viewer the agent's explanation as output.
//
// Without it the viewer waits on a stream no POST will ever feed, and a second
// Follow viewer attaches to the same dead stream rather than queuing a task of
// its own. No cancel task is queued: the agent never opened anything to close.
// The explanation travels as a log chunk because that is the one channel the
// API layer already renders, so the api/controller boundary stays as it is.
func (lr *logRelay) fail(streamID, reason string) {
	lr.mu.Lock()
	s := lr.streams[streamID]
	if s != nil {
		delete(lr.streams, streamID)
		if lr.byRunner[s.runnerID] == streamID {
			delete(lr.byRunner, s.runnerID)
		}
	}
	lr.mu.Unlock()
	if s == nil {
		return
	}
	if reason != "" {
		s.send([]byte("zoomies: could not open this runner's log stream: " + reason + "\n"))
	}
	s.close()
	lr.c.log.Debug("closed a log relay the agent could not open", "runner", s.runnerID, "stream", streamID, "reason", reason)
}

// AcceptLogStream consumes the agent's outbound chunked POST and fans it out.
// It returns when the agent closes the body or the stream is torn down.
//
// hostID is the host the request authenticated as, and it must be the host the
// stream was opened for: whatever arrives here is shown to operators as that
// runner's output, so a stream another agent could write into would let one
// host put words in another's mouth. Every other agent endpoint checks the same
// thing against the runner's host; this one checks it against the stream's.
func (c *Controller) AcceptLogStream(hostID, streamID string, r io.Reader) error {
	c.relay.mu.Lock()
	s := c.relay.streams[streamID]
	c.relay.mu.Unlock()
	if s == nil {
		return fmt.Errorf("%w: %s", ErrStreamUnknown, streamID)
	}
	if s.hostID != hostID {
		c.log.Warn("a host tried to write into another host's log stream",
			"host", hostID, "stream", streamID, "owner", s.hostID)
		// Deliberately the same answer a closed stream gets: an agent probing
		// for streams it does not own learns nothing from the difference.
		return fmt.Errorf("%w: %s", ErrStreamUnknown, streamID)
	}

	// Clean up registration on any exit path (error, viewer cancellation, EOF)
	// so closed streams are never leaked in relay maps.
	defer func() {
		c.relay.mu.Lock()
		if c.relay.streams[streamID] == s {
			delete(c.relay.streams, streamID)
		}
		if c.relay.byRunner[s.runnerID] == streamID {
			delete(c.relay.byRunner, s.runnerID)
		}
		c.relay.mu.Unlock()
		s.close()
	}()

	buf := make([]byte, logReadChunk)
	dropped := 0
	// The agent's own buffer says what it refused in band; the relay owes the
	// same sentence, or an operator reading a downloaded file cannot tell a
	// quiet build from a truncated one. The marker travels the ordinary send
	// path, so every viewer and the download alike see it.
	flushDropped := func() bool {
		if dropped == 0 {
			return true
		}
		notice := []byte(fmt.Sprintf("\n[the controller's log relay fell behind and dropped %d bytes; read the log live, or download it again with a tail]\n", dropped))
		if !s.spend(len(notice), c.Now()) {
			// The notice is bytes like any other, and spending them is what
			// stops a stream that has already spent its allowance from
			// spending it again on its own apology. Leave the count set: the
			// next marker still tells the truth about what was lost.
			return true
		}
		if !s.send(notice) {
			return false
		}
		dropped = 0
		return true
	}
	for {
		n, err := r.Read(buf)
		if n > 0 && !s.spend(n, c.Now()) {
			// Over budget: the bytes are read and dropped, never waited on.
			// Blocking would hold the agent's POST open and push the backlog
			// back into the agent's memory, which is the same wedge the
			// subscriber queue exists to avoid, one hop earlier.
			c.metrics.logRelayDropped.Add(float64(n))
			dropped += n
			n = 0
			if s.isClosed() {
				// Nobody is left to drop it for; send would have said so.
				return nil
			}
		}
		if n > 0 {
			if !flushDropped() {
				return nil
			}
			// The buffer is reused, so each subscriber gets its own copy.
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if !s.send(chunk) {
				// The last viewer left mid-stream; the cancel task is already
				// queued, and returning ends the agent's POST.
				return nil
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				// The agent's connection dropped mid-stream -- the job ended
				// and the container with it, or the network went. The output
				// up to here was delivered, the viewer sees the stream end,
				// and nothing needs a 500 in the log every time a job
				// finishes under a watcher.
				c.log.Debug("a log stream ended before its EOF", "stream", streamID, "runner", s.runnerID, "error", err)
			}
			// The stream is ending, so this is the last chance to say what it
			// refused; a viewer must always be told what it lost, even when it
			// was lost in the last moments of the output.
			flushDropped()
			break
		}
	}
	return nil
}

// streamIDFor returns the stream currently open for a runner, or "".
func (lr *logRelay) streamIDFor(runnerID string) string {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	return lr.byRunner[runnerID]
}

// closeAll ends every stream, used at shutdown so no SSE response is left open.
func (lr *logRelay) closeAll() {
	lr.mu.Lock()
	streams := make([]*logStream, 0, len(lr.streams))
	for _, s := range lr.streams {
		streams = append(streams, s)
	}
	lr.streams = map[string]*logStream{}
	lr.byRunner = map[string]string{}
	lr.mu.Unlock()
	for _, s := range streams {
		s.close()
	}
}

// add registers a viewer.
func (s *logStream) add() (chan []byte, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, 0, ErrStreamUnknown
	}
	s.nextID++
	ch := make(chan []byte, logSubscriberQueue)
	s.subs[s.nextID] = ch
	return ch, s.nextID, nil
}

// remove drops a viewer and reports whether it was the last one.
func (s *logStream) remove(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.subs[id]
	if !ok {
		return false
	}
	delete(s.subs, id)
	close(ch)
	return len(s.subs) == 0
}

// send fans one chunk out, dropping for subscribers that are not keeping up.
// It reports whether anybody is still listening.
func (s *logStream) send(chunk []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(s.subs) == 0 {
		return false
	}
	for _, ch := range s.subs {
		select {
		case ch <- chunk:
		default:
			// Dropped rather than blocked. A slow reader loses lines; every
			// other viewer, and the agent writing into the relay, do not stall.
		}
	}
	return true
}

// close ends the stream for everyone still attached.
func (s *logStream) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for id, ch := range s.subs {
		delete(s.subs, id)
		close(ch)
	}
}

// isClosed reports whether the stream has already ended.
func (s *logStream) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
