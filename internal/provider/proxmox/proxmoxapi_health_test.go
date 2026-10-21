package proxmox

import (
	"context"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// silenceableProxy forwards TCP connections to a target until silence is
// called. From then on the connections it already holds stay open and swallow
// everything in both directions, while a connection opened afterwards is
// forwarded as before: one path dead without a reset or a FIN, a fresh one
// fine. That is what a Proxmox connection looks like when something on the
// path starts dropping its packets, such as a NAT or firewall that has
// forgotten the flow.
type silenceableProxy struct {
	ln       net.Listener
	target   string
	accepted atomic.Int32

	mu    sync.Mutex
	pairs []*proxiedPair
}

type proxiedPair struct {
	client, server net.Conn
	silent         atomic.Bool
}

func newSilenceableProxy(t *testing.T, target string) *silenceableProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	p := &silenceableProxy{ln: ln, target: target}
	go p.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, pair := range p.pairs {
			_ = pair.client.Close()
			_ = pair.server.Close()
		}
	})
	return p
}

func (p *silenceableProxy) serve() {
	for {
		client, err := p.ln.Accept()
		if err != nil {
			return
		}
		p.accepted.Add(1)
		server, err := net.Dial("tcp", p.target)
		if err != nil {
			_ = client.Close()
			continue
		}
		pair := &proxiedPair{client: client, server: server}
		p.mu.Lock()
		p.pairs = append(p.pairs, pair)
		p.mu.Unlock()
		go pair.forward(server, client)
		go pair.forward(client, server)
	}
}

// forward keeps reading after the pair falls silent, so the sender sees its
// bytes accepted and never learns they went nowhere.
func (pair *proxiedPair) forward(dst, src net.Conn) {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 && !pair.silent.Load() {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// silence kills every connection open now, without telling either end.
func (p *silenceableProxy) silence() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, pair := range p.pairs {
		pair.silent.Store(true)
	}
}

func (p *silenceableProxy) url() string { return "https://" + p.ln.Addr().String() }

// A request deadline only resets its HTTP/2 stream. The shared connection must
// be closed too, or the machine loop keeps sending calls down the same dead path.
func TestTheClientLeavesAProxmoxConnectionThatStopsAnswering(t *testing.T) {
	var sawHTTP2 atomic.Bool
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 {
			sawHTTP2.Store(true)
		}
		_, _ = w.Write([]byte(`{"data":{"version":"8.0"}}`))
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	proxy := newSilenceableProxy(t, srv.Listener.Addr().String())
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	c, err := New(Options{Endpoint: proxy.url(), TokenID: "test@pve!test", Secret: "test", CAPEM: string(ca)})
	if err != nil {
		t.Fatal(err)
	}
	transport := c.http.Transport.(*http.Transport)
	t.Cleanup(transport.CloseIdleConnections)
	// Let a loaded race build finish TLS and the healthy first call before a
	// missing ping response can close it; keep recovery inside the test's bound.
	if transport.HTTP2 != nil {
		transport.HTTP2.SendPingTimeout = 200 * time.Millisecond
		transport.HTTP2.PingTimeout = 2 * time.Second
	}
	call := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := c.Version(ctx)
		return err
	}
	if err := call(); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if !sawHTTP2.Load() {
		t.Fatal("the call did not use HTTP/2, so it did not exercise a multiplexed connection")
	}
	proxy.silence()
	deadline := time.Now().Add(8 * time.Second)
	for {
		err := call()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("every call still fails on the dead connection: %v", err)
		}
	}
	if got := proxy.accepted.Load(); got < 2 {
		t.Fatalf("connections opened = %d, want a fresh connection after the first went silent", got)
	}
}

// Test timings above are shorter; hold production to the same explicit budget
// as the agent, comfortably before an ordinary Proxmox call's deadline.
func TestTheProxmoxConnectionHasAnExplicitHealthCheckBudget(t *testing.T) {
	c, err := New(Options{Endpoint: "https://proxmox.example:8006", TokenID: "test@pve!test", Secret: "test"})
	if err != nil {
		t.Fatal(err)
	}
	tr := c.http.Transport.(*http.Transport)
	t.Cleanup(tr.CloseIdleConnections)
	if tr.HTTP2 == nil {
		t.Fatal("the Proxmox connection has no HTTP/2 health check")
	}
	if got := tr.HTTP2.SendPingTimeout; got != 15*time.Second {
		t.Errorf("send ping timeout = %s, want 15s", got)
	}
	if got := tr.HTTP2.PingTimeout; got != 10*time.Second {
		t.Errorf("ping timeout = %s, want 10s", got)
	}
	if budget := tr.HTTP2.SendPingTimeout + tr.HTTP2.PingTimeout; budget >= defaultCallTimeout {
		t.Errorf("health check budget %s reaches the %s call deadline", budget, defaultCallTimeout)
	}
}
