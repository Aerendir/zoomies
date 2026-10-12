package api

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/auth"
	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/events"
	"github.com/eyupio/zoomies/internal/store"
)

func publicView(c *config.Config) { c.Security.PublicView = true }

// The visitor walk: every route in the table, asked by somebody with nothing
// to present, on a controller whose fleet is meant to be looked at.
//
// A route a visitor's scopes reach answers them; every other route refuses
// them with 403 and never 401, because a 401 would send the UI to a sign-in
// form the visitor did not ask for, and the 403 is where they are told why.
func TestAVisitorReadsTheFleetAndIsRefusedEverythingElse(t *testing.T) {
	h := newHarness(t, publicView)
	ids := h.fixtures()
	reach := auth.VisitorScopes()

	for _, rt := range routeTable(ids) {
		if rt.public {
			continue
		}
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			resp := h.do(request{method: rt.method, path: rt.path, body: rt.body})
			allowed := rt.path == "/api/v1/auth/session" ||
				(rt.action != "" && !rt.accountOnly && rt.role == store.RoleViewer && slices.Contains(reach, rt.action.Scope()))
			if allowed {
				if resp.status == http.StatusUnauthorized || resp.status == http.StatusForbidden {
					t.Fatalf("a visitor was refused %d: %s", resp.status, truncate(resp.body))
				}
				return
			}
			if resp.status != http.StatusForbidden {
				t.Fatalf("a visitor got %d, want 403: %s", resp.status, truncate(resp.body))
			}
			if msg := resp.errorMessage(t); !strings.Contains(msg, "public view") || !strings.Contains(msg, "sign in") {
				t.Errorf("the refusal does not tell a visitor to sign in: %q", msg)
			}
		})
	}
}

// Without the setting nothing changes: the same caller is told to sign in.
func TestWithoutAPublicViewNobodyIsAVisitor(t *testing.T) {
	h := newHarness(t)
	h.do(request{method: "GET", path: "/api/v1/pools"}).mustStatus(t, http.StatusUnauthorized, "an anonymous caller on a private controller")
	if got := h.do(request{method: "GET", path: "/api/v1/meta"}).json(t)["public_view"]; got != false {
		t.Errorf("meta public_view = %v on a private controller; want false", got)
	}
}

// The UI decides what to show from these two answers: the controller says it
// is public, and the session says who is looking.
func TestAPublicViewSaysSoAndNamesTheVisitor(t *testing.T) {
	h := newHarness(t, publicView)
	if got := h.do(request{method: "GET", path: "/api/v1/meta"}).json(t)["public_view"]; got != true {
		t.Errorf("meta public_view = %v; want true", got)
	}
	who := h.do(request{method: "GET", path: "/api/v1/auth/session"})
	who.mustStatus(t, http.StatusOK, "a visitor asking who they are")
	body := who.json(t)
	if body["kind"] != auth.KindPublic || body["role"] != string(store.RoleViewer) {
		t.Errorf("session = %v; want kind %q at the viewer role", body, auth.KindPublic)
	}
}

// Signing out writes an audit row. A visitor who could call it would let an
// anonymous loop fill the audit log with nothing.
func TestAVisitorCannotSignOutIntoTheAuditLog(t *testing.T) {
	h := newHarness(t, publicView)
	_, before, err := h.st.ListAudit(h.ctx, store.AuditFilter{}, store.Page{Limit: 1})
	if err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}
	for range 3 {
		h.do(request{method: "POST", path: "/api/v1/auth/logout"}).mustStatus(t, http.StatusForbidden, "a visitor signing out")
	}
	_, after, err := h.st.ListAudit(h.ctx, store.AuditFilter{}, store.Page{Limit: 1})
	if err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}
	if after != before {
		t.Errorf("the audit log grew from %d to %d rows while a visitor signed out", before, after)
	}
}

// A browser whose session lapsed keeps sending the cookie until it is told to
// forget it. On a public view the request is answered as a visitor and the
// cookie is cleared in the same response.
func TestALapsedSessionIsForgottenOnAPublicView(t *testing.T) {
	h := newHarness(t, publicView)
	resp := h.do(request{method: "GET", path: "/api/v1/auth/session", cookie: "a-session-that-ended-long-ago"})
	resp.mustStatus(t, http.StatusOK, "a lapsed session on a public view")
	if kind := resp.json(t)["kind"]; kind != auth.KindPublic {
		t.Errorf("a lapsed session resolved to %v; want the visitor", kind)
	}
	if resp.cookie == nil || resp.cookie.MaxAge >= 0 {
		t.Errorf("the dead cookie was not cleared: %+v", resp.cookie)
	}

	// A token is different: a broken automation must hear it was refused.
	h.do(request{method: "GET", path: "/api/v1/pools", token: auth.APITokenPrefix + "notarealtoken"}).
		mustStatus(t, http.StatusUnauthorized, "an unknown token on a public view")
}

// A frame is the resource's GET in miniature. Before, holding events.read was
// enough for every frame, so a token scoped to events:read alone received the
// audit log a row at a time, and a visitor would have too.
func TestAStreamSendsAFrameOnlyToSomebodyWhoCouldReadIt(t *testing.T) {
	h := newHarness(t, publicView)
	scoped := h.token("events only", store.RoleAdmin, "events:read", "pools:read")
	viewer, _ := h.user("viewer", store.RoleViewer)

	callers := []struct {
		name      string
		cookie    string
		headers   map[string]string
		wantAudit bool
	}{
		{name: "visitor"},
		{name: "token scoped to events and pools", headers: map[string]string{"Authorization": "Bearer " + scoped}},
		{name: "viewer", cookie: h.session(viewer), wantAudit: true},
	}
	for _, c := range callers {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			frames, resp := h.openStream(t, ctx, "/api/v1/events", c.cookie, c.headers)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("opening the stream answered %d", resp.StatusCode)
			}
			await(t, frames, "the opening comment", func(f sseFrame) bool { return f.comment != "" })

			h.ctrl.Events().Publish(events.KindAudit, "audit:1", store.AuditEvent{ID: "aud_1", Action: "auth.login", ActorName: "alice", IP: "198.51.100.4"})
			h.ctrl.Events().Publish(events.KindPoolCreated, "pool:p", map[string]any{"id": "p"})

			sawAudit := false
			await(t, frames, "the pool frame", func(f sseFrame) bool {
				if f.event == string(events.KindAudit) {
					sawAudit = true
				}
				return f.event == string(events.KindPoolCreated)
			})
			if sawAudit != c.wantAudit {
				t.Errorf("audit frame delivered = %v; want %v", sawAudit, c.wantAudit)
			}
		})
	}
}

// A new kind of frame reaches nobody until somebody decides who may read it.
// This walks every kind the bus declares, read from its source so a new one
// cannot be added without a row here.
func TestEveryKindOfFrameSaysWhoMayReadIt(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "../events/bus.go", nil, 0)
	if err != nil {
		t.Fatalf("reading the bus's kinds: %v", err)
	}
	found := 0
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if id, ok := spec.Type.(*ast.Ident); !ok || id.Name != "Kind" {
			return true
		}
		for i, name := range spec.Names {
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok {
				continue
			}
			found++
			k := events.Kind(strings.Trim(lit.Value, `"`))
			if _, mapped := frameReads[k]; !mapped && !framesForEveryone[k] {
				t.Errorf("%s (%s) has no read action, so no stream will ever carry it", name.Name, k)
			}
		}
		return true
	})
	if found == 0 {
		t.Fatal("found no kinds in internal/events/bus.go, so this checked nothing")
	}
}

// Signing in from a browser still holding a dead cookie must answer with the
// new session alone. A clearing header beside the one that sets it would leave
// the browser to settle which wins.
func TestSigningInOverALapsedSessionSetsOneCookie(t *testing.T) {
	h := newHarness(t, publicView)
	h.user("alice", store.RoleAdmin)
	resp := h.do(request{method: "POST", path: "/api/v1/auth/login", cookie: "a-session-that-ended-long-ago",
		body: map[string]any{"username": "alice", "password": testPassword}})
	resp.mustStatus(t, http.StatusOK, "signing in over a lapsed session")
	set := 0
	for _, c := range resp.header.Values("Set-Cookie") {
		if strings.HasPrefix(c, SessionCookie+"=") {
			set++
		}
	}
	if set != 1 {
		t.Errorf("the sign-in set the session cookie %d times; want once", set)
	}
	if resp.cookie == nil || resp.cookie.Value == "" || resp.cookie.MaxAge < 0 {
		t.Errorf("the session cookie set was %+v; want the new session", resp.cookie)
	}
}

// A public demo invites people nobody can ask to close a tab. Their streams
// have a ceiling of their own, and however full it is, somebody who runs the
// fleet still gets the stream the ordinary ceiling keeps for them.
func TestVisitorsCannotTakeTheStreamsStaffNeed(t *testing.T) {
	h := newHarness(t, publicView, func(c *config.Config) {
		c.Limits.VisitorEventSubscribers = 1
		c.Limits.EventSubscribers = 1
	})
	admin, _ := h.user("admin", store.RoleAdmin)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frames, first := h.openStream(t, ctx, "/api/v1/events", "", nil)
	if first.StatusCode != http.StatusOK {
		t.Fatalf("the first visitor's stream answered %d", first.StatusCode)
	}
	await(t, frames, "the first visitor's opening comment", func(f sseFrame) bool { return f.comment != "" })

	second := h.do(request{method: "GET", path: "/api/v1/events"})
	second.mustStatus(t, http.StatusConflict, "a second visitor past the visitors' ceiling")
	if msg := second.errorMessage(t); !strings.Contains(msg, "limits.visitor_event_subscribers") {
		t.Errorf("the refusal does not name the visitors' ceiling: %q", msg)
	}

	_, staff := h.openStream(t, ctx, "/api/v1/events", h.session(admin), nil)
	if staff.StatusCode != http.StatusOK {
		t.Fatalf("an administrator's stream answered %d while visitors were at their ceiling", staff.StatusCode)
	}
}
