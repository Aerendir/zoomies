-- Private source-bearing previews have their own bounded, installation-scoped history.
CREATE TABLE optimisation_previews (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    installation_id TEXT NOT NULL REFERENCES installations(id) ON DELETE CASCADE,
    repository TEXT NOT NULL,
    initiating_user TEXT NOT NULL,
    version TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    payload BLOB NOT NULL CHECK(length(payload) > 0 AND length(payload) <= 262144),
    created_at INTEGER NOT NULL
);
CREATE INDEX optimisation_preview_scope ON optimisation_previews(installation_id, repository, id);
