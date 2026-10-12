package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/store"
)

func newPublicService(t *testing.T) (*Service, *store.Store, *clock) {
	t.Helper()
	cfg := config.Default()
	cfg.Security.RateLimitLogins = 0
	cfg.Security.SessionTTL = time.Hour
	cfg.Security.PublicView = true
	return newServiceWith(t, cfg)
}

// The whole point of the setting: somebody who followed a link from the
// landing page has nothing to present and is let in to look.
func TestAPublicViewTurnsACallerWithNothingIntoAVisitor(t *testing.T) {
	s, _, _ := newPublicService(t)
	id, err := s.Authenticate(t.Context(), AuthInput{IP: "203.0.113.7"})
	if err != nil {
		t.Fatalf("Authenticate with nothing on a public view = %v; want a visitor", err)
	}
	if !IsVisitor(id) || id.Role != store.RoleViewer || id.IP != "203.0.113.7" {
		t.Fatalf("identity = %+v; want the visitor, as a viewer, from the caller's address", id)
	}
	if id.UserID != "" {
		t.Errorf("a visitor acts for account %q; it has none, and a token it minted would be somebody's", id.UserID)
	}
}

// A revoked token that quietly became a visitor would keep a broken
// automation reading as a stranger, and nobody would learn the token is dead.
func TestABadTokenIsRefusedEvenOnAPublicView(t *testing.T) {
	s, _, _ := newPublicService(t)
	_, err := s.Authenticate(t.Context(), AuthInput{Authorization: "Bearer " + APITokenPrefix + "notarealtokenatall", IP: "203.0.113.7"})
	if err == nil {
		t.Fatal("a token that does not exist was accepted on a public view")
	}
}

// A session that ran out leaves a cookie the browser keeps sending. On a
// public view that browser is a visitor again rather than a sign-in form it
// cannot leave; without the setting nothing changes.
func TestALapsedSessionIsAVisitorOnlyOnAPublicView(t *testing.T) {
	for _, public := range []bool{true, false} {
		cfg := config.Default()
		cfg.Security.RateLimitLogins = 0
		cfg.Security.SessionTTL = time.Hour
		cfg.Security.PublicView = public
		s, st, c := newServiceWith(t, cfg)
		addUser(t, st, "alice", store.RoleAdmin, nil)
		_, cookie, err := s.Login(t.Context(), "alice", testPassword, "10.0.0.1", "")
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		c.Advance(2 * time.Hour)

		id, err := s.Authenticate(t.Context(), AuthInput{SessionCookie: cookie})
		if public {
			if err != nil || !IsVisitor(id) {
				t.Errorf("public view: lapsed session = %v, %v; want the visitor", id, err)
			}
			continue
		}
		if !errors.Is(err, ErrSessionExpired) {
			t.Errorf("private: lapsed session = %v; want ErrSessionExpired", err)
		}
	}
}

// Somebody who runs the demo signs in and gets their own role back: the
// public view is a floor, never a ceiling.
func TestASignedInPersonIsThemselvesOnAPublicView(t *testing.T) {
	s, st, _ := newPublicService(t)
	addUser(t, st, "alice", store.RoleAdmin, nil)
	_, cookie, err := s.Login(t.Context(), "alice", testPassword, "10.0.0.1", "")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	id, err := s.Authenticate(t.Context(), AuthInput{SessionCookie: cookie})
	if err != nil || IsVisitor(id) || id.Role != store.RoleAdmin {
		t.Fatalf("signed in on a public view = %+v, %v; want alice as an administrator", id, err)
	}
}

// The scope list is the boundary between a stranger and the fleet. Every entry
// has to be something a viewer may already do, and only ever a read: a write
// or an administrator's read slipped into it would be handed to the internet.
func TestEveryActionAVisitorMayTakeIsAViewersRead(t *testing.T) {
	for _, a := range visitorActions {
		if role, ok := actionRoles[a]; !ok || role != store.RoleViewer {
			t.Errorf("%s needs %q; a visitor may only take a viewer's actions", a, role)
		}
		if a.Verb() != "read" {
			t.Errorf("%s is not a read; a visitor changes nothing", a)
		}
	}
}

// What a viewer reads but a stranger must not: the people who run the fleet,
// where they connect from, their credentials and conversations, and source
// files from the AI context.
func TestAVisitorReadsNothingAboutThePeopleWhoRunTheFleet(t *testing.T) {
	v := PublicIdentity("203.0.113.7")
	for _, a := range []Action{
		ActionAuditRead, ActionTokensOwn, ActionAssistantOwn, ActionAssistantChat,
		ActionContextRead, ActionContextManage, ActionContextPublish,
		ActionUsersRead, ActionSettingsRead, ActionMetricsRead, ActionMigrationsRead,
	} {
		if Allowed(v, a) {
			t.Errorf("a visitor is allowed %s", a)
		}
	}
	for _, a := range []Action{ActionPoolsRead, ActionJobsRead, ActionLogsRead, ActionEventsRead} {
		if !Allowed(v, a) {
			t.Errorf("a visitor is refused %s, which the public view exists to show", a)
		}
	}
}

// Every change a viewer could not make, a visitor cannot either.
func TestAVisitorChangesNothing(t *testing.T) {
	v := PublicIdentity("203.0.113.7")
	for a := range actionRoles {
		if a.Verb() != "read" && Allowed(v, a) {
			t.Errorf("a visitor is allowed %s", a)
		}
	}
}

// "Your public view has viewer" would be true and useless; the person reading
// a refusal on the demo has no account to change, only the choice to sign in.
func TestAVisitorIsToldToSignInRatherThanWhichRoleIsMissing(t *testing.T) {
	v := PublicIdentity("203.0.113.7")
	if got := Explain(v, ActionPoolsWrite); !strings.Contains(got, "read-only public view") || !strings.Contains(got, "sign in") {
		t.Errorf("a refused change explains %q; want it to say the view is read-only and to sign in", got)
	}
	if got := Explain(v, ActionAuditRead); !strings.Contains(got, "does not show this") {
		t.Errorf("a refused read explains %q; want it to say the public view does not show this", got)
	}
	if got := Explain(v, ActionPoolsRead); got != "" {
		t.Errorf("an allowed read explains %q; want nothing", got)
	}
}
