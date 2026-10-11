package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// A conversation is one person's. Another account cannot list it, read it,
// overwrite it by guessing its id, or delete it; and when the account goes,
// its conversations go with it.
func TestAssistantConversationsBelongToOnePerson(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	alice, bob := &User{Username: "alice", Role: RoleViewer}, &User{Username: "bob", Role: RoleViewer}
	for _, u := range []*User{alice, bob} {
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	c := &AssistantConversation{UserID: alice.ID, Title: "Why is the queue slow?", Messages: json.RawMessage(`[{"role":"user","content":"Why is the queue slow?"}]`), MessageCount: 1}
	if err := s.SaveAssistantConversation(ctx, c); err != nil {
		t.Fatal(err)
	}
	if c.ID == "" || !HasPrefix(c.ID, PrefixAssistantConversation) {
		t.Fatalf("id = %q", c.ID)
	}
	got, err := s.GetAssistantConversation(ctx, alice.ID, c.ID)
	if err != nil || got.Title != c.Title || string(got.Messages) != string(c.Messages) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := s.GetAssistantConversation(ctx, bob.ID, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob read alice's conversation: %v", err)
	}
	theirs, _ := s.ListAssistantConversations(ctx, bob.ID)
	if len(theirs) != 0 {
		t.Errorf("bob listed %d of alice's", len(theirs))
	}
	stolen := &AssistantConversation{ID: c.ID, UserID: bob.ID, Title: "mine now", Messages: json.RawMessage(`[]`)}
	if err := s.SaveAssistantConversation(ctx, stolen); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob overwrote alice's conversation: %v", err)
	}
	if err := s.DeleteAssistantConversation(ctx, bob.ID, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob deleted alice's conversation: %v", err)
	}
	// Saving again with the id replaces the messages and moves it to the top.
	c.Messages, c.MessageCount = json.RawMessage(`[{"role":"user","content":"q"},{"role":"assistant","content":"a"}]`), 2
	if err := s.SaveAssistantConversation(ctx, c); err != nil {
		t.Fatal(err)
	}
	mine, _ := s.ListAssistantConversations(ctx, alice.ID)
	if len(mine) != 1 || mine[0].MessageCount != 2 || mine[0].Messages != nil {
		t.Errorf("list = %+v", mine[0])
	}
	if err := s.DeleteUser(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAssistantConversation(ctx, alice.ID, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the conversation outlived its account: %v", err)
	}
}

// Fifty is a list; the fifty-first saved removes the oldest, and the retention
// window removes by age whatever the cap kept.
func TestAssistantConversationsAreCappedPerPersonAndPrunedByAge(t *testing.T) {
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	s := newTestStoreAt(t, func() time.Time { return now })
	ctx := context.Background()
	u := &User{Username: "alice", Role: RoleViewer}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	var first string
	for i := 0; i <= MaxAssistantConversations; i++ {
		c := &AssistantConversation{UserID: u.ID, Title: "q", Messages: json.RawMessage(`[]`)}
		if err := s.SaveAssistantConversation(ctx, c); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = c.ID
		}
		now = now.Add(time.Minute)
	}
	list, _ := s.ListAssistantConversations(ctx, u.ID)
	if len(list) != MaxAssistantConversations {
		t.Errorf("kept %d, want %d", len(list), MaxAssistantConversations)
	}
	if _, err := s.GetAssistantConversation(ctx, u.ID, first); !errors.Is(err, ErrNotFound) {
		t.Errorf("the oldest survived the cap: %v", err)
	}
	n, err := s.PruneAssistantConversations(ctx, now.Add(-10*time.Minute))
	if err != nil || n != int64(MaxAssistantConversations-10) {
		t.Errorf("pruned %d, %v", n, err)
	}
}
