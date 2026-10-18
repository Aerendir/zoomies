package docs

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/auth"
	"github.com/eyupio/zoomies/internal/config"
)

// hooks/auth.md is served verbatim at zoomies.sh/auth.md, to agents that act on
// what it says, and its header promises to keep it true to internal/auth. These
// are the parts of that promise a machine can hold: the form a REST token's
// scopes are written in, and the default of the setting that decides whether an
// MCP client can register itself.

func readAuthMD(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../hooks/auth.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// auth.Action's string form is resource.verb, and the scope a token carries is
// resource:verb. An agent that writes the first into a token request gets a 422
// naming the scopes that exist, after reading a page that told it otherwise.
func TestAuthMdWritesRestScopesInTheFormATokenCarries(t *testing.T) {
	text := readAuthMD(t)
	if strings.Contains(text, "resource.verb") {
		t.Error("hooks/auth.md writes a REST scope as resource.verb; a token carries resource:verb (hosts:read), and auth.ValidateScopes refuses the other form")
	}
	if !strings.Contains(text, "`resource:verb`") {
		t.Error("hooks/auth.md does not say that REST scopes are written resource:verb")
	}
	// Every scope the page gives as an example is one the server accepts. The
	// mcp: scopes are a different vocabulary, the OAuth one, and are held by the
	// MCP tests.
	example := regexp.MustCompile("`([a-z][a-z-]*:(?:[a-z-]+|\\*))`")
	for _, m := range example.FindAllStringSubmatch(text, -1) {
		scope := m[1]
		if strings.HasPrefix(scope, "mcp:") || scope == "resource:verb" {
			continue
		}
		if err := auth.ValidateScopes([]string{scope}); err != nil {
			t.Errorf("hooks/auth.md gives %q as a scope: %v", scope, err)
		}
	}
}

// The page says what a controller does when security.mcp_open_registration is
// left alone, and it said off for a setting that is on by default, which sent an
// operator to create a client by hand that Claude would have registered itself.
func TestAuthMdSaysWhatOpenRegistrationDefaultsTo(t *testing.T) {
	text := strings.Join(strings.Fields(readAuthMD(t)), " ")
	_, after, ok := strings.Cut(text, "security.mcp_open_registration")
	if !ok {
		t.Fatal("hooks/auth.md no longer names security.mcp_open_registration; the controller section is where an agent learns whether it can register a client itself")
	}
	if len(after) > 200 {
		after = after[:200]
	}
	want := "the default is off"
	if config.Default().Security.MCPOpenRegistration {
		want = "the default is on"
	}
	if !strings.Contains(after, want) {
		t.Errorf("hooks/auth.md does not say %q within a sentence of security.mcp_open_registration, but config.Default() says it is %v", want, config.Default().Security.MCPOpenRegistration)
	}
}
