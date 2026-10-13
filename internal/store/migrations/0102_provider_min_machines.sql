-- How many machines a provider keeps ready whether or not anything is waiting.
--
-- Until now a provider owned exactly as many machines as the queue needed and
-- gave each one up when it had been idle for the provider's idle timeout, so
-- the first job after a quiet spell always waited for a clone. A minimum keeps
-- that many machines rented through the quiet, which is paid for in return for
-- a job starting at once. Zero, the default, is what every provider did before.
ALTER TABLE providers ADD COLUMN min_machines INTEGER NOT NULL DEFAULT 0;
