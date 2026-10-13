-- The size a provider's machines turned out to be, when its row names none.
--
-- A provider whose machine size is left empty builds whatever its template is,
-- and the fit check behind renting judged that machine as one that had measured
-- nothing, which fits every pool by slots alone. A fleet bought 2 vCPU, 3.9 GB
-- machines for a docker-in-docker pool whose runner needs 6.7 GB, and then sat
-- at the provider's ceiling with two machines that could never run a job.
--
-- The columns below are observations, written by the controller: the template's
-- own size when a connection check reads it, and what a machine it built
-- measured itself as once its agent joined. seen_at is when the evidence was
-- true, so that an older machine never overrules a check made after the
-- template changed. They are kept rather than worked out from the machines
-- that exist, because the machines that prove a shape too small are the ones
-- released for it, and a fleet that forgot the reason would buy them again.
ALTER TABLE providers ADD COLUMN seen_cpus INTEGER NOT NULL DEFAULT 0;
ALTER TABLE providers ADD COLUMN seen_memory_mb INTEGER NOT NULL DEFAULT 0;
ALTER TABLE providers ADD COLUMN seen_from TEXT NOT NULL DEFAULT '';
ALTER TABLE providers ADD COLUMN seen_at INTEGER;
