-- A person's past conversations with Eli, kept so a chat survives a reload
-- and can be read back or resumed. One row per conversation, the messages as
-- one JSON document the way the page sends them: the API bounds the document
-- (the same limits as a chat request) and redacts it, and the store keeps it
-- opaque so a turn's shape can grow without a migration. Rows belong to an
-- account and go with it; a retention window prunes the rest.
CREATE TABLE assistant_conversations (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    messages      TEXT NOT NULL,
    message_count INTEGER NOT NULL,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);
CREATE INDEX assistant_conversations_by_user ON assistant_conversations(user_id, updated_at DESC);
