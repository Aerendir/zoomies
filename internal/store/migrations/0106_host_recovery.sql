-- No cascading foreign key: retirement and cutover obligations must survive a
-- host re-enrolling, and a deleted record must not make recovery look complete.
CREATE TABLE host_recovery (
    host_id TEXT PRIMARY KEY,
    retired_at INTEGER,
    completed_at INTEGER,
    retired_by TEXT NOT NULL DEFAULT ''
);
