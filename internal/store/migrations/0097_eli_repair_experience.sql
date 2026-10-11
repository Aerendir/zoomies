-- Manual policies do not need an unattended payer. Existing modes are retained.
CREATE TABLE eli_repair_policies_next (
 repo TEXT PRIMARY KEY,
 installation_id TEXT NOT NULL REFERENCES installations(id) ON DELETE CASCADE,
 provider_id TEXT REFERENCES assistant_providers(id) ON DELETE SET NULL,
 enabled INTEGER NOT NULL DEFAULT 0,
 automatic INTEGER NOT NULL DEFAULT 0,
 ask_first INTEGER NOT NULL DEFAULT 0,
 allow_workflows INTEGER NOT NULL DEFAULT 0,
 daily_limit INTEGER NOT NULL DEFAULT 5 CHECK(daily_limit BETWEEN 1 AND 50),
 updated_at INTEGER NOT NULL
);
INSERT INTO eli_repair_policies_next SELECT repo,installation_id,provider_id,enabled,automatic,0,allow_workflows,daily_limit,updated_at FROM eli_repair_policies;
DROP TABLE eli_repair_policies;
ALTER TABLE eli_repair_policies_next RENAME TO eli_repair_policies;
ALTER TABLE eli_repairs ADD COLUMN details TEXT NOT NULL DEFAULT '{}';
CREATE TABLE eli_repair_opportunities (
 id TEXT PRIMARY KEY,
 repo TEXT NOT NULL,
 installation_id TEXT NOT NULL REFERENCES installations(id) ON DELETE CASCADE,
 pull_number INTEGER NOT NULL,
 head_sha TEXT NOT NULL,
 run_id INTEGER NOT NULL,
 title TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'offered',
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL,
 UNIQUE(repo,head_sha,pull_number)
);
CREATE TABLE eli_repair_dismissals (
 opportunity_id TEXT NOT NULL REFERENCES eli_repair_opportunities(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 PRIMARY KEY(opportunity_id,user_id)
);
CREATE INDEX eli_repair_opportunity_state ON eli_repair_opportunities(state,updated_at);
