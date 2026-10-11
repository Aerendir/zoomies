package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/config"
	"github.com/eyupio/zoomies/internal/store"
)

const assistantConversations = "/api/v1/assistant/personal/conversations"

// A conversation kept once an answer has landed is there after a reload and
// on another device, is the person's alone, and is gone when they say so. A
// token pasted into a question is not in the database either: what is kept
// is what a model was shown, hidden the same way.
func TestConversationsAreKeptPerPersonRedactedAndDeletable(t *testing.T) {
	h := newHarness(t)
	_, alice := h.user("alice", store.RoleViewer)
	_, bob := h.user("bob", store.RoleViewer)
	body := map[string]any{"messages": []map[string]string{
		{"role": "user", "content": "Why did job_1 fail? My token is ghp_abcdefghijklmnopqrstuvwxyz0123456789"},
		{"role": "assistant", "content": "The runner ran out of memory."},
	}}
	created := h.do(request{method: http.MethodPost, path: assistantConversations, cookie: alice, body: body})
	created.mustStatus(t, http.StatusCreated, "keeping a conversation")
	saved := created.json(t)
	id, _ := saved["id"].(string)
	if !store.HasPrefix(id, store.PrefixAssistantConversation) || saved["message_count"] != float64(2) {
		t.Fatalf("saved = %v", saved)
	}
	if title, _ := saved["title"].(string); !strings.HasPrefix(title, "Why did job_1 fail?") || strings.Contains(title, "ghp_abc") {
		t.Errorf("title = %q", title)
	}
	got := h.do(request{method: http.MethodGet, path: assistantConversations + "/" + id, cookie: alice})
	got.mustStatus(t, http.StatusOK, "reading it back")
	if raw := string(got.body); strings.Contains(raw, "ghp_abc") || !strings.Contains(raw, "[redacted credential]") || !strings.Contains(raw, "out of memory") {
		t.Errorf("read back %s", raw)
	}
	// A list carries the summaries and not the words.
	list := h.do(request{method: http.MethodGet, path: assistantConversations, cookie: alice})
	list.mustStatus(t, http.StatusOK, "listing")
	items, _ := list.json(t)["items"].([]any)
	if len(items) != 1 || strings.Contains(string(list.body), "out of memory") {
		t.Errorf("list = %s", list.body)
	}
	// Replacing it with the next turn keeps the id.
	body["messages"] = append(body["messages"].([]map[string]string), map[string]string{"role": "user", "content": "What should I do?"})
	h.do(request{method: http.MethodPut, path: assistantConversations + "/" + id, cookie: alice, body: body}).mustStatus(t, http.StatusOK, "adding a turn")
	// Another person sees none of it.
	h.do(request{method: http.MethodGet, path: assistantConversations + "/" + id, cookie: bob}).mustStatus(t, http.StatusNotFound, "another person's read")
	h.do(request{method: http.MethodPut, path: assistantConversations + "/" + id, cookie: bob, body: body}).mustStatus(t, http.StatusNotFound, "another person's write")
	h.do(request{method: http.MethodDelete, path: assistantConversations + "/" + id, cookie: bob}).mustStatus(t, http.StatusNotFound, "another person's delete")
	if theirs, _ := h.do(request{method: http.MethodGet, path: assistantConversations, cookie: bob}).json(t)["items"].([]any); len(theirs) != 0 {
		t.Errorf("bob listed %d of alice's", len(theirs))
	}
	h.do(request{method: http.MethodDelete, path: assistantConversations + "/" + id, cookie: alice}).mustStatus(t, http.StatusNoContent, "deleting")
	h.do(request{method: http.MethodGet, path: assistantConversations + "/" + id, cookie: alice}).mustStatus(t, http.StatusNotFound, "gone")
}

// The transcript is bounded the way a chat request is, and a role the chat
// does not have is refused, so the stored document is always one the page
// could send back.
func TestAConversationIsBoundedLikeAChat(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.user("alice", store.RoleViewer)
	h.do(request{method: http.MethodPost, path: assistantConversations, cookie: cookie, body: map[string]any{"messages": []map[string]string{}}}).mustStatus(t, http.StatusUnprocessableEntity, "no messages")
	h.do(request{method: http.MethodPost, path: assistantConversations, cookie: cookie, body: map[string]any{"messages": []map[string]string{{"role": "system", "content": "x"}}}}).mustStatus(t, http.StatusUnprocessableEntity, "a system message")
	h.do(request{method: http.MethodPost, path: assistantConversations, cookie: cookie, body: map[string]any{"messages": []map[string]string{{"role": "user", "content": strings.Repeat("x", 9<<10)}}}}).mustStatus(t, http.StatusUnprocessableEntity, "a message over the size")
	// Ending with the answer is the ordinary shape of a kept conversation.
	h.do(request{method: http.MethodPost, path: assistantConversations, cookie: cookie, body: map[string]any{"messages": []map[string]string{{"role": "user", "content": "q"}, {"role": "assistant", "content": "a"}}}}).mustStatus(t, http.StatusCreated, "ending with the answer")
}

// With authentication off there is no account to keep a conversation under,
// so the list is empty and a save is refused rather than written to nobody.
func TestWithoutAnAccountConversationsAreNotKept(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.Security.DisableAuth = true })
	list := h.do(request{method: http.MethodGet, path: assistantConversations})
	list.mustStatus(t, http.StatusOK, "listing without an account")
	if items, _ := list.json(t)["items"].([]any); len(items) != 0 {
		t.Errorf("items = %v", items)
	}
	h.do(request{method: http.MethodPost, path: assistantConversations, body: map[string]any{"messages": []map[string]string{{"role": "user", "content": "q"}}}}).mustStatus(t, http.StatusForbidden, "saving without an account")
}
