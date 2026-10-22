import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  hostsLine,
  windowPhrase,
  jobsLine,
  poolsLine,
  providersLine,
  queueLine,
  workflowsLine,
} from '../src/lib/bands/lines.ts';

test('pools say how many are enabled, how full they are and which are at the ceiling', () => {
  const line = poolsLine({ enabled: 3, total: 4, live: 8, max: 20, atCeiling: 1 });
  assert.equal(line.headline, '3 of 4 pools are enabled.');
  assert.equal(line.accent, '8 of 20 runners live.');
  assert.equal(line.lede, '1 is at its ceiling.');
  assert.equal(
    poolsLine({ enabled: 2, total: 2, live: 0, max: 8, atCeiling: 0 }).headline,
    '2 pools are enabled.',
  );
  assert.equal(
    poolsLine({ enabled: 1, total: 1, live: 0, max: 8, atCeiling: 0 }).lede,
    'None is at its ceiling.',
  );
  assert.equal(
    poolsLine({ enabled: 0, total: 0, live: 0, max: 0, atCeiling: 0 }).headline,
    'No pools yet.',
  );
});

test('the queue names how many jobs wait, and says so when none does', () => {
  const waiting = queueLine({ queued: 6, running: 3, medianWait: '19s' });
  assert.equal(waiting.headline, '6 jobs are waiting for a runner.');
  assert.equal(waiting.accent, '3 running.');
  assert.equal(waiting.lede, 'The median wait is 19s.');
  assert.equal(
    queueLine({ queued: 1, running: 0, medianWait: null }).headline,
    '1 job is waiting for a runner.',
  );
  const calm = queueLine({ queued: 0, running: 2, medianWait: '4s' });
  assert.equal(calm.headline, 'Nothing is waiting.');
  assert.equal(calm.lede, 'Every job has a runner.');
});

test('jobs say what is running and what finished in the window, by the numbers', () => {
  const line = jobsLine({
    running: 3,
    queued: 6,
    succeeded: 40,
    failed: 2,
    windowText: 'the last hour',
  });
  assert.equal(line.headline, '3 jobs are running.');
  assert.equal(line.accent, '6 waiting for a runner.');
  assert.equal(line.lede, '2 failed and 40 succeeded in the last hour.');
  assert.equal(
    jobsLine({ running: 1, queued: 0, succeeded: 0, failed: 0, windowText: 'the last hour' })
      .headline,
    '1 job is running.',
  );
});

test('workflows count the jobs in flight, running and waiting', () => {
  const line = workflowsLine({ running: 3, queued: 6 });
  assert.equal(line.headline, '9 jobs are in flight.');
  assert.equal(line.accent, '3 running, 6 waiting.');
  assert.equal(workflowsLine({ running: 0, queued: 1 }).headline, '1 job is in flight.');
});

test('hosts say how many are connected and how many slots are in use', () => {
  const line = hostsLine({ connected: 2, total: 3, inUse: 4, capacity: 12 });
  assert.equal(line.headline, '2 of 3 hosts are connected.');
  assert.equal(line.accent, '4 of 12 slots in use.');
  assert.equal(line.lede, '1 is not sending heartbeats.');
  const all = hostsLine({ connected: 3, total: 3, inUse: 0, capacity: 12 });
  assert.equal(all.headline, '3 hosts are connected.');
  assert.equal(all.lede, 'Every host is sending heartbeats.');
  assert.equal(
    hostsLine({ connected: 0, total: 0, inUse: 0, capacity: 0 }).headline,
    'No hosts yet.',
  );
});

test('providers say how many machines they own and what needs review', () => {
  const line = providersLine({ providers: 2, owned: 3, ceiling: 10, paused: 1, review: 2 });
  assert.equal(line.headline, '3 machines are owned.');
  assert.equal(line.accent, 'Up to 10 at once.');
  assert.equal(line.lede, '2 need review.');
  assert.equal(
    providersLine({ providers: 1, owned: 1, ceiling: 4, paused: 0, review: 0 }).headline,
    '1 machine is owned.',
  );
  assert.equal(
    providersLine({ providers: 1, owned: 0, ceiling: 4, paused: 1, review: 0 }).lede,
    '1 provider is paused.',
  );
  assert.equal(
    providersLine({ providers: 0, owned: 0, ceiling: 0, paused: 0, review: 0 }).headline,
    'No providers yet.',
  );
});

test('a window is worded as a period, never as "the last 1 hour"', () => {
  assert.equal(windowPhrase('1 hour'), 'the last hour');
  assert.equal(windowPhrase('24 hours'), 'the last 24 hours');
  assert.equal(windowPhrase('2 days'), 'the last 2 days');
  assert.equal(windowPhrase(''), 'the reported window');
});
