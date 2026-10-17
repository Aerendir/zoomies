package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// singleUseJoins is a transport whose join tokens work once, as the
// controller's do, and which remembers every token it was offered. The plain
// fake accepts any token any number of times, which is why nothing could show a
// host offering the one it had already spent.
type singleUseJoins struct {
	*fakeTransport

	joinsMu sync.Mutex
	spent   map[string]bool
	offered []string
}

func newSingleUseJoins() *singleUseJoins {
	return &singleUseJoins{fakeTransport: newFakeTransport(), spent: make(map[string]bool)}
}

func (s *singleUseJoins) Join(ctx context.Context, req JoinRequest) (*JoinResponse, error) {
	s.joinsMu.Lock()
	s.offered = append(s.offered, req.JoinToken)
	again := s.spent[req.JoinToken]
	s.spent[req.JoinToken] = true
	s.joinsMu.Unlock()
	if again {
		// What the HTTP transport makes of the controller's 422 for a spent token.
		return nil, fmt.Errorf("%w: HTTP 422: join token already used", ErrUnauthorized)
	}
	return s.fakeTransport.Join(ctx, req)
}

func (s *singleUseJoins) tokensOffered() []string {
	s.joinsMu.Lock()
	defer s.joinsMu.Unlock()
	return slices.Clone(s.offered)
}

// refuseForGood is the host row being deleted, by the operator or by the
// controller retiring it: every call made with the credentials the host holds
// is answered with a 404.
func (s *singleUseJoins) refuseForGood() {
	s.fakeTransport.mu.Lock()
	s.fakeTransport.beatErr = ErrHostGone
	s.fakeTransport.mu.Unlock()
}

// joinedWith enrols a host with token on a machine whose work directory is dir,
// as `zoomies agent join` leaves it.
func joinedWith(t *testing.T, dir string, tr Transport, token string) {
	t.Helper()
	a, _, _, _ := newAgent(t, 1, func(o *Options) { o.WorkDir = dir; o.Transport = tr })
	if err := a.Join(context.Background(), token); err != nil {
		t.Fatalf("Join with %q: %v", token, err)
	}
}

// startAgent is one start of the agent's service: a new process over the state
// the last one left, handed whatever join token the environment file holds.
func startAgent(t *testing.T, dir string, tr Transport, token string) error {
	t.Helper()
	a, _, _, _ := newAgent(t, 1, func(o *Options) {
		o.WorkDir = dir
		o.Transport = tr
		o.FallbackJoinToken = token
	})
	a.fallbackWait = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return a.Run(ctx)
}

// ZOOMIES_JOIN_TOKEN stays in the environment file after the first start spends
// it, so a host the operator deletes, or the controller retires, restarts with a
// token it can only be refused for. Offering it anyway on every restart is an
// anonymous POST to the join route every few seconds, against a limiter that
// allows a handful a minute per address: one stale host behind a shared address
// would shut out the enrolment of every machine beside it, including the clones
// whose fallback is the point of having a token to fall back on.
func TestAHostWhoseCredentialsAreRefusedDoesNotOfferTheTokenThatMadeThemAgain(t *testing.T) {
	dir := t.TempDir()
	tr := newSingleUseJoins()
	joinedWith(t, dir, tr, "zoojoin_spent")
	tr.refuseForGood()

	for start := 1; start <= 3; start++ {
		if err := startAgent(t, dir, tr, "zoojoin_spent"); !errors.Is(err, ErrHostGone) {
			t.Fatalf("start %d: Run error = %v, want one wrapping ErrHostGone", start, err)
		}
	}
	if got := tr.tokensOffered(); !slices.Equal(got, []string{"zoojoin_spent"}) {
		t.Fatalf("the controller was offered %q; the enrolment was the only use the token had", got)
	}
}

// A machine cloned from a template that was ever joined itself carries the
// template's identity, and the controller refuses it. The clone's own token,
// handed to it through its environment, is what replaces that identity, and it
// is offered once: after that the identity is the clone's own.
func TestAClonedHostOffersItsOwnTokenOnceWhenTheInheritedCredentialsAreRefused(t *testing.T) {
	dir := t.TempDir()
	tr := newSingleUseJoins()
	joinedWith(t, dir, tr, "zoojoin_template")
	tr.refuseForGood()

	for start := 1; start <= 3; start++ {
		if err := startAgent(t, dir, tr, "zoojoin_clone"); !errors.Is(err, ErrHostGone) {
			t.Fatalf("start %d: Run error = %v, want one wrapping ErrHostGone", start, err)
		}
	}
	if got, want := tr.tokensOffered(), []string{"zoojoin_template", "zoojoin_clone"}; !slices.Equal(got, want) {
		t.Fatalf("the controller was offered %q, want %q: the clone's token once, and nothing after it", got, want)
	}
}

// A fallback that fails is still a use of the token: the controller refused it,
// or another clone spent it, or the answer was lost. Either way the next start
// holds the same token and would get the same answer, so it is not asked again;
// the operator mints a fresh one, as the error says. What the file remembers is
// a hash, because the token itself is a credential.
func TestAFailedFallbackIsNotRetriedWithTheSameTokenOnTheNextStart(t *testing.T) {
	dir := t.TempDir()
	tr := newSingleUseJoins()
	joinedWith(t, dir, tr, "zoojoin_template")
	tr.refuseForGood()
	tr.spent["zoojoin_clone"] = true

	err := startAgent(t, dir, tr, "zoojoin_clone")
	if !errors.Is(err, ErrHostGone) || !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("first start: Run error = %v, want the refusal and the join's own failure", err)
	}
	for start := 2; start <= 3; start++ {
		if err := startAgent(t, dir, tr, "zoojoin_clone"); !errors.Is(err, ErrHostGone) {
			t.Fatalf("start %d: Run error = %v, want one wrapping ErrHostGone", start, err)
		}
	}
	if got, want := tr.tokensOffered(), []string{"zoojoin_template", "zoojoin_clone"}; !slices.Equal(got, want) {
		t.Fatalf("the controller was offered %q, want %q: a token that failed once is not offered again", got, want)
	}

	path := StatePath(dir)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"zoojoin_clone", "zoojoin_template"} {
		if strings.Contains(string(raw), token) {
			t.Fatalf("the state file holds the join token %q itself:\n%s", token, raw)
		}
	}
	// The write is Save's, so the file keeps the mode the credentials beside it have.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("state file is mode %04o after the failed fallback, want 0600", perm)
		}
	}
}

// A host upgraded in place has a state file from before tokens were remembered,
// which cannot say which token made its credentials. It must still load, and the
// worst it costs is one offer of a token that may be spent, remembered from then
// on like any other.
func TestAStateFileFromBeforeTokensWereRememberedStillLoadsAndFallsBackOnce(t *testing.T) {
	dir := t.TempDir()
	old := `{"host_id":"host-1","agent_token":"agent-token","controller_url":"https://controller:8080"}` + "\n"
	if err := os.WriteFile(StatePath(dir), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	creds, err := Load(StatePath(dir))
	if err != nil {
		t.Fatalf("Load of a file with no token hashes: %v", err)
	}
	if creds.JoinTokenHash != "" || creds.FallbackTokenHash != "" {
		t.Fatalf("hashes appeared from nowhere: %+v", creds)
	}

	tr := newSingleUseJoins()
	tr.refuseForGood()
	tr.spent["zoojoin_spent"] = true
	for start := 1; start <= 3; start++ {
		if err := startAgent(t, dir, tr, "zoojoin_spent"); !errors.Is(err, ErrHostGone) {
			t.Fatalf("start %d: Run error = %v, want one wrapping ErrHostGone", start, err)
		}
	}
	if got := tr.tokensOffered(); !slices.Equal(got, []string{"zoojoin_spent"}) {
		t.Fatalf("the controller was offered %q, want the one token once", got)
	}
}

// The token hashes are part of what Save writes and Load reads back.
func TestTokenHashesSurviveTheStateFile(t *testing.T) {
	path := StatePath(t.TempDir())
	want := Credentials{
		HostID: "host-1", AgentToken: "secret-token", Controller: "https://controller:8080",
		JoinTokenHash: "aaaa", FallbackTokenHash: "bbbb",
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
}

// A host that cannot use its join token waits before it gives the refusal back,
// because the service manager starts it again at once, and a stale host that
// asks the controller about a deleted row every few seconds is a loop nobody has
// to be watching to start.
func TestAHostThatCannotUseItsJoinTokenWaitsBeforeGivingTheRefusalBack(t *testing.T) {
	for _, tc := range []struct{ name, madeBy, handed string }{
		{"a token that made the credentials", "zoojoin_spent", "zoojoin_spent"},
		{"a token the controller refuses", "zoojoin_template", "zoojoin_clone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				dir := t.TempDir()
				tr := newSingleUseJoins()
				tr.spent["zoojoin_clone"] = true
				joinedWith(t, dir, tr, tc.madeBy)
				tr.refuseForGood()

				a, _, _, _ := newAgent(t, 1, func(o *Options) {
					o.WorkDir = dir
					o.Transport = tr
					o.FallbackJoinToken = tc.handed
				})
				done := make(chan error, 1)
				began := time.Now()
				go func() { done <- a.Run(context.Background()) }()

				synctest.Wait()
				time.Sleep(fallbackBackoff - time.Second)
				synctest.Wait()
				select {
				case err := <-done:
					t.Fatalf("Run returned %v after %s, before the wait was over", err, time.Since(began))
				default:
				}

				time.Sleep(time.Second)
				synctest.Wait()
				select {
				case err := <-done:
					if !errors.Is(err, ErrHostGone) {
						t.Fatalf("Run error = %v, want one wrapping ErrHostGone", err)
					}
					if waited := time.Since(began); waited != fallbackBackoff {
						t.Fatalf("Run waited %s, want %s", waited, fallbackBackoff)
					}
				default:
					t.Fatal("Run was still waiting after the whole backoff")
				}
			})
		})
	}
}

// Stopping the service, which is what a restart from the operator does, must
// not wait out the backoff.
func TestCancellingAHostThatIsBackingOffEndsTheWaitAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		tr := newSingleUseJoins()
		joinedWith(t, dir, tr, "zoojoin_spent")
		tr.refuseForGood()

		a, _, _, _ := newAgent(t, 1, func(o *Options) {
			o.WorkDir = dir
			o.Transport = tr
			o.FallbackJoinToken = "zoojoin_spent"
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- a.Run(ctx) }()
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			if !errors.Is(err, ErrHostGone) {
				t.Fatalf("Run error = %v, want one wrapping ErrHostGone", err)
			}
		default:
			t.Fatal("a cancelled agent went on waiting out its backoff")
		}
	})
}

// A host with no join token to fall back on, which is the embedded agent and
// every host nobody handed one, still ends its run when the controller refuses
// it for good. What it must not do is end it with a create half done or a log
// stream open: a stop finishes what the agent admitted first, and so does this.
// The one thing it skips is the last report, which would travel on the
// credential that was just refused.
func TestARefusedHostWithNoJoinTokenStillDrainsItsTasksAndStopsItsLogs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, tr, b := startupAgent(t)
		pr, pw := io.Pipe()
		defer pw.Close()
		b.fakeBackend.mu.Lock()
		b.fakeBackend.logs = func() io.ReadCloser { return pr }
		b.fakeBackend.mu.Unlock()
		if err := a.Join(context.Background(), "join-token"); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- a.Run(context.Background()) }()

		// A runner whose log somebody is watching...
		tr.tasks <- []Task{createTask("create-1", "runner-1")}
		synctest.Wait()
		startupEntered(t, b, "runner-1")
		b.finish <- nil
		synctest.Wait()
		tr.tasks <- []Task{{ID: "logs", Kind: TaskStreamLogs, RunnerID: "runner-1", StreamID: "stream-1"}}
		synctest.Wait()
		if s := tr.stream("stream-1"); s == nil || s.isClosed() {
			t.Fatal("the log stream was not open before the refusal")
		}

		// ...and a create still in flight when the controller refuses the host.
		tr.tasks <- []Task{createTask("create-2", "runner-2")}
		synctest.Wait()
		startupEntered(t, b, "runner-2")
		for len(tr.reports) > 0 {
			<-tr.reports
		}
		tr.mu.Lock()
		tr.beatErr = ErrHostGone
		tr.mu.Unlock()
		time.Sleep(time.Minute)
		synctest.Wait()
		select {
		case err := <-done:
			b.finish <- nil
			t.Fatalf("Run returned %v with a create unfinished", err)
		default:
		}

		b.finish <- nil
		synctest.Wait()
		if err := <-done; !errors.Is(err, ErrHostGone) {
			t.Fatalf("Run error = %v, want one wrapping ErrHostGone", err)
		}
		created := map[string]bool{}
		for len(tr.results) > 0 {
			if res := <-tr.results; res.OK && res.TaskID == "create-2" {
				created[res.TaskID] = true
			}
		}
		if !created["create-2"] {
			t.Fatal("Run returned before the create it had admitted was reported")
		}
		if !tr.stream("stream-1").isClosed() {
			t.Fatal("Run returned with a log stream still open")
		}
		if len(tr.reports) != 0 {
			t.Fatalf("a final report was sent on the credential that had just been refused: %v", <-tr.reports)
		}
	})
}
