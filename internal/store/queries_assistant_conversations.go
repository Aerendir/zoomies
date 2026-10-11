package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// AssistantConversation is one person's past chat with Eli: the messages as
// the page sent them, bounded and redacted by the API before they got here,
// under a title taken from the first question.
type AssistantConversation struct {
	ID     string
	UserID string
	Title  string
	// Messages is the conversation as JSON, opaque here. Nil on a row read
	// from the list, which carries the count instead.
	Messages     json.RawMessage
	MessageCount int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// MaxAssistantConversations is how many one person keeps. Saving one past
// that removes their oldest, so the list stays a list and not an archive;
// the retention window removes the rest by age.
const MaxAssistantConversations = 50

const assistantConversationCols = `id, user_id, title, message_count, created_at, updated_at`

func scanAssistantConversation(sc interface{ Scan(...any) error }, withMessages bool) (*AssistantConversation, error) {
	var c AssistantConversation
	var created, updated int64
	var err error
	if withMessages {
		var raw string
		err = sc.Scan(&c.ID, &c.UserID, &c.Title, &c.MessageCount, &created, &updated, &raw)
		c.Messages = json.RawMessage(raw)
	} else {
		err = sc.Scan(&c.ID, &c.UserID, &c.Title, &c.MessageCount, &created, &updated)
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt, c.UpdatedAt = at(created), at(updated)
	return &c, nil
}

// SaveAssistantConversation writes a conversation: a new row when it has no
// ID, otherwise a replacement of the row with that ID, which must be the same
// person's (ErrNotFound otherwise, so one account cannot overwrite another's
// by guessing an id). The person's oldest conversations past the cap go with
// the same write.
func (s *Store) SaveAssistantConversation(ctx context.Context, c *AssistantConversation) error {
	now := s.Now()
	return s.tx(ctx, func(tx *sql.Tx) error {
		if c.ID == "" {
			c.ID = NewID(PrefixAssistantConversation)
			c.CreatedAt, c.UpdatedAt = now, now
			if _, err := tx.ExecContext(ctx, `INSERT INTO assistant_conversations (`+assistantConversationCols+`, messages)
				VALUES (?, ?, ?, ?, ?, ?, ?)`, c.ID, c.UserID, c.Title, c.MessageCount, ms(now), ms(now), string(c.Messages)); err != nil {
				return wrapWrite(err)
			}
		} else {
			res, err := tx.ExecContext(ctx, `UPDATE assistant_conversations SET title = ?, messages = ?, message_count = ?, updated_at = ?
				WHERE id = ? AND user_id = ?`, c.Title, string(c.Messages), c.MessageCount, ms(now), c.ID, c.UserID)
			if err != nil {
				return wrapWrite(err)
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return ErrNotFound
			}
			c.UpdatedAt = now
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM assistant_conversations WHERE user_id = ? AND id NOT IN (
			SELECT id FROM assistant_conversations WHERE user_id = ? ORDER BY updated_at DESC, id DESC LIMIT ?)`,
			c.UserID, c.UserID, MaxAssistantConversations)
		return wrapWrite(err)
	})
}

// ListAssistantConversations is one person's conversations, newest first,
// without their messages.
func (s *Store) ListAssistantConversations(ctx context.Context, userID string) ([]*AssistantConversation, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+assistantConversationCols+` FROM assistant_conversations
		WHERE user_id = ? ORDER BY updated_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AssistantConversation{}
	for rows.Next() {
		c, err := scanAssistantConversation(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetAssistantConversation is one conversation with its messages, and
// ErrNotFound for another person's as much as for none.
func (s *Store) GetAssistantConversation(ctx context.Context, userID, id string) (*AssistantConversation, error) {
	c, err := scanAssistantConversation(s.read.QueryRowContext(ctx, `SELECT `+assistantConversationCols+`, messages
		FROM assistant_conversations WHERE id = ? AND user_id = ?`, id, userID), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// DeleteAssistantConversation removes one of a person's conversations.
func (s *Store) DeleteAssistantConversation(ctx context.Context, userID, id string) error {
	res, err := s.exec(ctx, `DELETE FROM assistant_conversations WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	return affected(res, "assistant conversation", id)
}

// PruneAssistantConversations removes every conversation not touched since
// before: the retention window, applied to everyone's.
func (s *Store) PruneAssistantConversations(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.exec(ctx, `DELETE FROM assistant_conversations WHERE updated_at < ?`, ms(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
