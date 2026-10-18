import { test } from 'node:test';
import assert from 'node:assert/strict';
import { eliCanRepairRun } from '../src/lib/repairs/workspace.ts';

// Eli repairs a pull request's branch and nothing else, so a failed push,
// merge or scheduled run is not something it can be asked to fix.
test('a failure of a run no pull request started is not offered to Eli', () => {
  for (const run_event of ['push', 'schedule', 'workflow_dispatch', 'release'])
    assert.equal(eliCanRepairRun({ run_event }), false, run_event);
});

test('a pull request run is offered, and so is a run whose trigger is not known yet', () => {
  for (const run_event of ['pull_request', 'pull_request_target', '', undefined])
    assert.equal(eliCanRepairRun({ run_event }), true, String(run_event));
});
