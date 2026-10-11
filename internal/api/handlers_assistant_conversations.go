package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eyupio/zoomies/internal/auth"
	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/redact"
	"github.com/eyupio/zoomies/internal/store"
)

// A person's saved conversations with Eli. The chat itself keeps nothing
// between requests; the page holds the conversation and, once an answer has
// landed, asks for it to be kept here, under the account, so a reload or
// another device finds it. What is kept is the transcript the page would send
// back, bounded the same way and redacted the same way, so nothing is in the
// database that was not already in front of a model. Only a signed-in account
// has somewhere to keep one: a token or an auth-disabled session reads an empty
// list and may not write.

// assistantConversationTitleRunes bounds the title: the first question, cut
// to what a list row can show.
const assistantConversationTitleRunes = 80

type assistantConversationInput struct {
	Title    string                            `json:"title"`
	Messages []controller.AssistantChatMessage `json:"messages"`
}

// assistantConversationView is one conversation as the API renders it: the
// summary alone in a list, the messages too when one is read or written.
type assistantConversationView struct {
	ID           string                            `json:"id"`
	Title        string                            `json:"title"`
	MessageCount int                               `json:"message_count"`
	CreatedAt    time.Time                         `json:"created_at"`
	UpdatedAt    time.Time                         `json:"updated_at"`
	Messages     []controller.AssistantChatMessage `json:"messages,omitempty"`
}

func conversationView(c *store.AssistantConversation) assistantConversationView {
	v := assistantConversationView{ID: c.ID, Title: c.Title, MessageCount: c.MessageCount, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
	if len(c.Messages) > 0 {
		// A row this process wrote parses; one that does not is shown empty
		// rather than failing the read, so a person can still delete it.
		_ = json.Unmarshal(c.Messages, &v.Messages)
	}
	return v
}

// conversationOwner is the account that may keep conversations, or empty.
func conversationOwner(r *http.Request) string {
	id := Identity(r.Context())
	if id == nil || id.Kind != auth.KindUser || !store.HasPrefix(id.ID, store.PrefixUser) {
		return ""
	}
	return id.ID
}

func (s *Server) handleListAssistantConversations(w http.ResponseWriter, r *http.Request) {
	owner := conversationOwner(r)
	items := []assistantConversationView{}
	if owner == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	rows, err := s.ctrl.Store().ListAssistantConversations(r.Context(), owner)
	if err != nil {
		s.internal(w, r, "listing the conversations", err)
		return
	}
	for _, row := range rows {
		items = append(items, conversationView(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleGetAssistantConversation(w http.ResponseWriter, r *http.Request) {
	owner := conversationOwner(r)
	if owner == "" {
		notFound(w, "only a signed-in account keeps conversations")
		return
	}
	row, err := s.ctrl.Store().GetAssistantConversation(r.Context(), owner, chiURLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "reading the conversation", err)
		return
	}
	writeJSON(w, http.StatusOK, conversationView(row))
}

// handleSaveAssistantConversation is both POST (a new one) and PUT (a
// replacement of one of the person's own): the page does not know whether an
// answer is the first it is keeping until it has been told an id.
func (s *Server) handleSaveAssistantConversation(w http.ResponseWriter, r *http.Request) {
	owner := conversationOwner(r)
	if owner == "" {
		forbidden(w, "only a signed-in account can keep conversations")
		return
	}
	var in assistantConversationInput
	if !decode(w, r, &in) {
		return
	}
	messages, err := controller.ValidateAssistantTranscript(in.Messages)
	var invalid *controller.AssistantChatInvalid
	if errors.As(err, &invalid) {
		unprocessable(w, invalid.Message, []fieldError{{invalid.Field, invalid.Message}})
		return
	}
	// Nothing is kept that was not already shown to a model: the same hiding
	// the chat applies, so a token pasted into a question is not in the
	// database either.
	for i := range messages {
		messages[i].Content, _ = redact.Text(messages[i].Content)
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		for _, m := range messages {
			if m.Role == "user" {
				title = m.Content
				break
			}
		}
	}
	title, _ = redact.Text(title)
	if r := []rune(title); len(r) > assistantConversationTitleRunes {
		title = string(r[:assistantConversationTitleRunes-1]) + "\u2026"
	}
	if !utf8.ValidString(title) {
		unprocessable(w, "the title must be text", []fieldError{{"title", "must be text"}})
		return
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		s.internal(w, r, "encoding the conversation", err)
		return
	}
	row := &store.AssistantConversation{ID: chiURLParam(r, "id"), UserID: owner, Title: title, Messages: raw, MessageCount: len(messages)}
	if err := s.ctrl.Store().SaveAssistantConversation(r.Context(), row); err != nil {
		s.fail(w, r, "keeping the conversation", err)
		return
	}
	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusCreated
	}
	writeJSON(w, status, conversationView(row))
}

func (s *Server) handleDeleteAssistantConversation(w http.ResponseWriter, r *http.Request) {
	owner := conversationOwner(r)
	if owner == "" {
		notFound(w, "only a signed-in account keeps conversations")
		return
	}
	if err := s.ctrl.Store().DeleteAssistantConversation(r.Context(), owner, chiURLParam(r, "id")); err != nil {
		s.fail(w, r, "removing the conversation", err)
		return
	}
	noContent(w)
}
