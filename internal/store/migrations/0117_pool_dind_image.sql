-- The image a pool's docker-in-docker sidecar runs, for a pool that needs a
-- different one from the host's: a Docker version of its own, or the mirrors a
-- single registry cache needs. Empty means the host's own choice
-- (agent.dind_image, and docker:27-dind where that is empty), which is what
-- every pool already meant, so an upgrade changes no sidecar.
ALTER TABLE pools ADD COLUMN dind_image TEXT NOT NULL DEFAULT '';
