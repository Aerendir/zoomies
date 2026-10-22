import { test } from 'node:test';
import assert from 'node:assert/strict';
import { statusLine } from '../src/lib/overview/status-line.ts';

const calm = { problems: [], running: 4, slotsUsed: 9, slots: 22, setupPending: false };

test('a fleet with nothing wrong says everything is running, with the real numbers', () => {
  const line = statusLine(calm);
  assert.equal(line.headline, 'Everything is running.');
  assert.equal(line.accent, '4 jobs are running.');
  assert.match(line.lede, /^9 of 22 host slots are in use/);
  assert.equal(line.needsYou, false);
});

test('problems are counted by severity, worst first, and never quoted', () => {
  const one = statusLine({ ...calm, problems: [{ severity: 'error' }] });
  assert.equal(one.headline, '1 thing needs you.');
  assert.equal(one.lede, '1 error needs your attention.');
  assert.equal(one.needsYou, true);
  const many = statusLine({
    ...calm,
    problems: [
      { severity: 'warning' },
      { severity: 'error' },
      { severity: 'warning' },
      { severity: 'info' },
    ],
  });
  assert.equal(many.headline, '4 things need you.');
  assert.equal(many.lede, '1 error, 2 warnings and 1 note need your attention.');
});

test('a single running job is "job is", not "jobs are"', () => {
  assert.equal(statusLine({ ...calm, running: 1 }).accent, '1 job is running.');
  assert.equal(statusLine({ ...calm, running: 0 }).accent, '0 jobs are running.');
});

test('a fleet with nothing set up says so, and does not claim it is running', () => {
  const line = statusLine({ ...calm, problems: [{ severity: 'error' }], setupPending: true });
  assert.equal(line.headline, 'Your fleet is empty.');
  assert.equal(line.needsYou, false);
  assert.doesNotMatch(line.lede, /running/);
});
