-- A person has seen this failure and does not need it in front of them again.
-- This is the triage record the Jobs page's attention view stops asking about:
-- a failure with acknowledged_at NULL, no later attempt and a recent queue
-- time is one nobody has dealt with. The name is kept beside the time so a
-- shared fleet can say who looked; both are empty on a failure nobody has
-- triaged yet, which is the ordinary state.
--
-- It is a column on the job rather than a table of its own because there is
-- exactly one per job and it dies with the job's retention. Unlike
-- problem_dismissals it is not per account: an acknowledgement here is the
-- fleet's, so whoever looks first stops the failure being news to everybody.
ALTER TABLE jobs ADD COLUMN acknowledged_at INTEGER;
ALTER TABLE jobs ADD COLUMN acknowledged_by TEXT NOT NULL DEFAULT '';
