-- A bounded non-secret observation survives runner pruning for historical joins.
-- Installation removal still removes its observations. Current pool/host state
-- cannot reconstruct an issued task or an agent's past report.
CREATE TABLE runner_creation_observations (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    runner_id TEXT NOT NULL UNIQUE,
    installation_id TEXT NOT NULL REFERENCES installations(id) ON DELETE CASCADE,
    sha256 TEXT NOT NULL,
    payload BLOB NOT NULL CHECK(length(payload) <= 4096),
    received_at INTEGER NOT NULL
);
CREATE INDEX idx_runner_creation_observations_scope ON runner_creation_observations(installation_id, runner_id, received_at);
