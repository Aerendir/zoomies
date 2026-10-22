import { test } from 'node:test';
import assert from 'node:assert/strict';
import { runnersLine } from '../src/lib/bands/runners-line.ts';

const counts = { provisioning: 0, registering: 0, idle: 2, busy: 3, draining: 0 };

test('the headline counts the live runners and the accent splits them into busy and idle', () => {
  const line = runnersLine(counts);
  assert.equal(line.headline, '5 runners are live.');
  assert.equal(line.accent, '3 busy, 2 idle.');
  assert.equal(line.lede, 'Every one is working or ready for work.');
});

test('runners that are starting up or draining are named, and counted as live', () => {
  const line = runnersLine({ provisioning: 1, registering: 1, idle: 0, busy: 2, draining: 1 });
  assert.equal(line.headline, '5 runners are live.');
  assert.equal(line.lede, '2 are starting up and 1 is draining.');
});

test('one runner is singular', () => {
  const line = runnersLine({ provisioning: 0, registering: 0, idle: 0, busy: 1, draining: 0 });
  assert.equal(line.headline, '1 runner is live.');
});

test('a fleet with no runners says so and does not invent a split', () => {
  const line = runnersLine({ provisioning: 0, registering: 0, idle: 0, busy: 0, draining: 0 });
  assert.equal(line.headline, 'No runners are live.');
  assert.equal(line.accent, '');
  assert.equal(line.lede, 'The next job that needs one will start it.');
});
