import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseCheckoutSelection, checkoutSeconds } from '../src/lib/optimisations/input.ts';

const ok = { runId: '9007199254740993', attempt: '2', jobId: '100' };

test('GitHub identities stay exact strings and are never rounded through a number', () => {
  const s = parseCheckoutSelection(ok);
  assert.equal(s.run_id, '9007199254740993');
  assert.equal(typeof s.run_id, 'string');
  assert.equal(s.attempt, 2);
  assert.equal(
    'job_index' in s,
    false,
    'the server finds the step; the page never sends positions',
  );
});

test('a selection the server would refuse is refused before any request', () => {
  for (const bad of [
    { runId: '' },
    { runId: '0' },
    { runId: '010' },
    { runId: '1.5' },
    { runId: '9223372036854775808' },
    { jobId: '-1' },
    { attempt: '0' },
    { attempt: 'x' },
  ])
    assert.throws(() => parseCheckoutSelection({ ...ok, ...bad }), Error, JSON.stringify(bad));
});

test('a checkout time is shown as a measurement in seconds', () => {
  assert.equal(checkoutSeconds(4200), '4.2 s');
});
