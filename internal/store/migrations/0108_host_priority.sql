-- Speed figures beside the size ones a host reports: what placement priority
-- is scored from, and the operator's override of that score (NULL while the
-- score stays worked out from the figures).
ALTER TABLE hosts ADD COLUMN cpu_mhz INTEGER NOT NULL DEFAULT 0;
ALTER TABLE hosts ADD COLUMN memory_mts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE hosts ADD COLUMN disk_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE hosts ADD COLUMN priority_override INTEGER;
