-- A machine an operator asked for to prove a provider works, rather than one
-- demand asked for. It is built, enrolled and released by the same loop as any
-- other, and counts against max_machines; the flag only says that it is a trial,
-- so it is not kept warm, is released once it reports ready, and is not mistaken
-- for supply by the planner.
ALTER TABLE machines ADD COLUMN is_test INTEGER NOT NULL DEFAULT 0;
