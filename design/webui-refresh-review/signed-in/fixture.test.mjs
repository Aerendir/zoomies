// One fleet for every signed-in mock, and the two things that keep it honest.
//
// A mock that shows a column the API does not return is a promise nobody can
// keep, so every object in the fixture is held to the schema in
// api/openapi.yaml. And a number the Overview shows has to be the number of rows
// the Runners page lists, so the totals are computed from the data and never
// typed, and these tests hold that they agree.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { FLEET, NOW, fleetCounts, minutesAgo } from './fixture.js';

/** The RunnerState values that hold a slot on a host (api/openapi.yaml, RunnerState). */
const LIVE = ['provisioning', 'registering', 'idle', 'busy', 'draining'];

const spec = readFileSync(fileURLToPath(new URL('../../../api/openapi.yaml', import.meta.url)), 'utf8');
const components = spec.slice(spec.indexOf('\ncomponents:'));

/** The property names a schema declares, or null when there is no such schema. */
function properties(name) {
  const start = components.indexOf(`\n    ${name}:\n`);
  if (start < 0) return null;
  const rest = components.slice(start + 1);
  const end = rest.slice(1).search(/\n    [A-Z][A-Za-z0-9]+:\n/);
  const block = end < 0 ? rest : rest.slice(0, end + 1);
  return new Set([...block.matchAll(/\n {8}([a-z0-9_]+):/g)].map((match) => match[1]));
}

const SCHEMAS = {
  pools: 'Pool',
  hosts: 'Host',
  runners: 'Runner',
  jobs: 'Job',
  installations: 'Installation',
  providers: 'Provider',
  machines: 'Machine',
  audit: 'AuditEvent',
  problems: 'Problem',
};

for (const [collection, schema] of Object.entries(SCHEMAS)) {
  test(`every ${collection} field is one the API returns (${schema})`, () => {
    const declared = properties(schema);
    assert.ok(declared, `api/openapi.yaml has no ${schema} schema`);
    assert.ok(FLEET[collection].length > 0, `the fixture has no ${collection}`);
    for (const item of FLEET[collection]) {
      for (const key of Object.keys(item)) {
        assert.ok(declared.has(key), `${schema}.${key} is not in api/openapi.yaml (${item.name ?? item.id ?? item.code})`);
      }
    }
  });
}

test('the fleet is frozen, so a screen cannot change what another one reads', () => {
  assert.ok(Object.isFrozen(FLEET));
  assert.ok(Object.isFrozen(FLEET.runners));
  assert.throws(() => FLEET.runners.push({}), TypeError);
});

test('the counts the Overview shows are the rows the pages list', () => {
  const counts = fleetCounts(FLEET);
  assert.equal(counts.runners.live, FLEET.runners.filter((r) => LIVE.includes(r.state)).length);
  assert.equal(counts.jobs.queued, FLEET.jobs.filter((j) => j.state === 'queued').length);
  assert.equal(counts.jobs.running, FLEET.jobs.filter((j) => j.state === 'in_progress').length);
  for (const pool of FLEET.pools) {
    const rows = FLEET.runners.filter((r) => r.pool_id === pool.id && r.state !== 'removed');
    assert.equal(counts.pools[pool.id].runners, rows.length, `${pool.name}: runners listed against runners counted`);
    assert.equal(
      counts.pools[pool.id].queued,
      FLEET.jobs.filter((j) => j.pool_id === pool.id && j.state === 'queued').length,
      `${pool.name}: jobs queued`,
    );
  }
  assert.equal(counts.hosts.slots, FLEET.hosts.reduce((total, h) => total + h.capacity, 0));
  assert.equal(counts.hosts.used, FLEET.hosts.reduce((total, h) => total + h.active_runners, 0));
});

test('a runner is on a host and in a pool that exist, and a job names the runner that has it', () => {
  const pools = new Set(FLEET.pools.map((p) => p.id));
  const hosts = new Set(FLEET.hosts.map((h) => h.id));
  for (const runner of FLEET.runners) {
    assert.ok(pools.has(runner.pool_id), `${runner.name} is in no pool`);
    assert.ok(hosts.has(runner.host_id), `${runner.name} is on no host`);
  }
  for (const job of FLEET.jobs.filter((j) => j.state === 'in_progress')) {
    const runner = FLEET.runners.find((r) => r.id === job.runner_id);
    assert.ok(runner, `in-progress job ${job.id} has no runner`);
    assert.equal(runner.current_job_id, job.id, `${runner.name} does not say it has ${job.id}`);
  }
});

test('the fixture carries the two inputs that break a layout: a 300-character name and a 120-character label', () => {
  const longest = FLEET.jobs.reduce((n, j) => Math.max(n, j.repo.length, j.workflow.length, j.job_name.length), 0);
  assert.ok(longest >= 300, `the longest job name is ${longest}`);
  const unbroken = FLEET.runners.flatMap((r) => r.labels).filter((label) => label.length >= 120 && !/\s/.test(label));
  assert.ok(unbroken.length >= 1, 'no runner has a 120-character label with nothing to break on');
});

test('times are relative to one instant, so every page agrees what "20 minutes ago" is', () => {
  assert.equal(new Date(minutesAgo(20)).getTime(), new Date(NOW).getTime() - 20 * 60_000);
  for (const job of FLEET.jobs) assert.ok(new Date(job.created_at ?? NOW) <= new Date(NOW), job.id);
});
