-- The event that started a job's workflow run (push, pull_request, schedule...).
-- workflow_job deliveries do not carry it, so it is filled in from the run when
-- the controller reads one. Eli repairs a pull request's branch and nothing
-- else, so a failure of any other kind of run must not be offered to it.
ALTER TABLE jobs ADD COLUMN run_event TEXT NOT NULL DEFAULT '';
