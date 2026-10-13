-- Evidence survives job retention, but remains an installation's bounded history.
CREATE TABLE comparison_evidence (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    installation_id TEXT NOT NULL REFERENCES installations(id) ON DELETE CASCADE,
    repository TEXT NOT NULL,
    initiating_user TEXT NOT NULL,
    version TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    payload BLOB NOT NULL CHECK(length(payload) > 0 AND length(payload) <= 2097152),
    created_at INTEGER NOT NULL
);
CREATE INDEX comparison_evidence_scope ON comparison_evidence(installation_id, repository, id);
