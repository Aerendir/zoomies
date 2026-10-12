package auth

import "github.com/eyupio/zoomies/internal/store"

// KindPublic is the identity a request with no credential at all resolves to
// when security.public_view is on: somebody looking at a fleet that was put on
// the internet to be looked at.
const KindPublic = "public"

// visitorActions is everything a visitor may do, and every entry is a read.
//
// The viewer role alone is too wide for a stranger. A viewer reads the audit
// log, which names the people who run the fleet and the addresses they came
// from; their own tokens and assistant conversations; and the AI context's
// source files. So a visitor carries the viewer role narrowed by these scopes,
// and Allowed refuses the rest the way it refuses a narrowed token. A read
// action added later is hidden from visitors until somebody decides it belongs
// here, which is the direction a mistake should fail in.
var visitorActions = []Action{
	ActionPoolsRead,
	ActionRunnersRead,
	ActionJobsRead,
	ActionUsageRead,
	ActionHostsRead,
	ActionProvidersRead,
	ActionMachinesRead,
	ActionInstallationsRead,
	ActionWebhooksRead,
	ActionUpdatesRead,
	ActionKennelRead,
	ActionEventsRead,
	ActionLogsRead,
	ActionStatsRead,
}

// VisitorScopes is the scope list a visitor carries, in the form an API token's
// scopes take.
func VisitorScopes() []string {
	out := make([]string, 0, len(visitorActions))
	for _, a := range visitorActions {
		out = append(out, a.Scope())
	}
	return out
}

// PublicIdentity is the visitor: no account, no token, nothing to attribute
// beyond the address it came from.
func PublicIdentity(ip string) *Identity {
	return &Identity{
		Kind:   KindPublic,
		ID:     "public",
		Name:   "public visitor",
		Role:   store.RoleViewer,
		Scopes: VisitorScopes(),
		IP:     ip,
	}
}

// IsVisitor reports whether id is the public view's anonymous visitor.
func IsVisitor(id *Identity) bool { return id != nil && id.Kind == KindPublic }

// visitorRefusal is what a visitor is told instead of a missing role or scope.
// "Your public view has viewer" would be true and useless: the person reading
// it has no account to change, only the choice to sign in.
func visitorRefusal(a Action) string {
	if a.Verb() == "read" {
		return "the public view does not show this; sign in to see it"
	}
	return "this is a read-only public view; sign in to do this"
}
