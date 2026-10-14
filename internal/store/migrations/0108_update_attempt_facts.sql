-- What the update helper reported about the engine's run: the copy it took
-- before it changed anything, and whether it put the previous release back.
-- Both are for the page, so an operator can restore from the copy and can tell
-- a failure that was undone from one left in place. Empty and zero are an
-- attempt from before this, or one whose result carried neither.
ALTER TABLE update_attempts ADD COLUMN backup TEXT NOT NULL DEFAULT '';
ALTER TABLE update_attempts ADD COLUMN rolled_back INTEGER NOT NULL DEFAULT 0;
