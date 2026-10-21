-- What an agent found when the update helper on a host was not reading requests,
-- as the controller wrote it down: a small JSON document of numbers and fixed
-- words (which stall, how long the request had waited, the units' states, what
-- root repaired). Empty on an attempt that was not stalled. It rides the attempt
-- because the problem it raises has to survive a restart, and a sentence in
-- the error column cannot be told from any other failure's.
ALTER TABLE update_attempts ADD COLUMN stuck TEXT NOT NULL DEFAULT '';
