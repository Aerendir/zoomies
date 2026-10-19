-- Whether a host keeps its capacity and reserve at what Zoomies recommends for
-- its machine and the fleet's pools, and when the controller last moved them.
-- Its own table, so the central host row and every query that reads it stay as
-- they are: a host with no row follows the fleet's default.
CREATE TABLE host_auto_adjust (
    host_id     TEXT    PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    mode        TEXT    NOT NULL DEFAULT '',
    adjusted_at INTEGER NOT NULL DEFAULT 0
);
