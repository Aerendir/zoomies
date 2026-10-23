-- The identity provider's own session that a browser session here was created
-- from (the sid claim of its ID token), so that a back-channel logout for one
-- of that person's sessions at the provider ends the sessions it started and
-- not the ones the same person holds from another browser. Empty for a session
-- made by a password and for one made before this column existed, which a
-- logout that names a session therefore leaves alone.
ALTER TABLE sessions ADD COLUMN oidc_sid TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_sessions_oidc_sid ON sessions(user_id, oidc_sid) WHERE oidc_sid <> '';
