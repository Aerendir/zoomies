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

/** The values a property may take: its own enum, or the one its $ref names, or null. */
function enumOf(schema, property) {
  const start = components.indexOf(`\n    ${schema}:\n`);
  const rest = components.slice(start + 1);
  const end = rest.slice(1).search(/\n    [A-Z][A-Za-z0-9]+:\n/);
  const block = end < 0 ? rest : rest.slice(0, end + 1);
  const line = new RegExp(`\\n {8}${property}: ([^\\n]*)`).exec(block)?.[1] ?? '';
  const inline = /enum: \[([^\]]*)\]/.exec(line);
  if (inline) return inline[1].split(',').map((v) => v.trim());
  const ref = /schemas\/([A-Za-z]+)"/.exec(line)?.[1];
  if (!ref) return null;
  const named = components.slice(components.indexOf(`\n    ${ref}:\n`) + 1);
  const listed = /enum: \[([^\]]*)\]/.exec(named);
  return listed ? listed[1].split(',').map((v) => v.trim()) : null;
}

const SCHEMAS = {
  pools: 'Pool',
  hosts: 'Host',
  runners: 'Runner',
  jobs: 'Job',
  runs: 'WorkflowRun',
  installations: 'Installation',
  providers: 'Provider',
  machines: 'Machine',
  audit: 'AuditEvent',
  problems: 'Problem',
  kennelRepositories: 'KennelRepository',
  repairs: 'EliRepair',
  contexts: 'AIContextRepository',
  usage: 'UsageBucket',
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

test('every Kennel Club finding is one the API returns (KennelFinding)', () => {
  const declared = properties('KennelFinding');
  assert.ok(declared, 'api/openapi.yaml has no KennelFinding schema');
  const all = FLEET.kennelRepositories.flatMap((repository) => repository.findings);
  assert.ok(all.length >= 3);
  for (const finding of all) {
    for (const key of Object.keys(finding)) assert.ok(declared.has(key), `KennelFinding.${key} is not in api/openapi.yaml`);
  }
});

test("a Kennel Club repository's counts are its findings, by severity", () => {
  for (const repository of FLEET.kennelRepositories) {
    for (const severity of ['error', 'warning', 'info']) {
      assert.equal(repository.counts[severity], repository.findings.filter((f) => f.severity === severity).length, `${repository.name} ${severity}`);
    }
  }
});

// A key that exists is not a value that can occur: a mock showing `target_type: 'Organization'`
// draws a word the API never returns, on a page a person is asked to judge.
const ENUMS = [
  ['runners', 'Runner', 'state'],
  ['jobs', 'Job', 'state'],
  ['hosts', 'Host', 'connection'],
  ['installations', 'Installation', 'target_type'],
  ['machines', 'Machine', 'state'],
  ['audit', 'AuditEvent', 'actor_kind'],
  ['kennelRepositories', 'KennelRepository', 'state'],
];
for (const [collection, schema, property] of ENUMS) {
  test(`every ${collection} ${property} is a value the API can return (${schema}.${property})`, () => {
    const allowed = enumOf(schema, property);
    assert.ok(allowed && allowed.length > 1, `could not read the values of ${schema}.${property} from api/openapi.yaml`);
    for (const item of FLEET[collection]) assert.ok(allowed.includes(item[property]), `${item.name ?? item.id}: "${item[property]}" is not one of ${allowed}`);
  });
}

test('every AI context freshness state is one the API can return', () => {
  const block = components.slice(components.indexOf('\n    AIContextRepository:\n'));
  const allowed = /freshness:[\s\S]*?state: \{ type: string, enum: \[([^\]]*)\]/.exec(block)[1].split(',').map((v) => v.trim());
  for (const item of FLEET.contexts) assert.ok(allowed.includes(item.freshness.state), `${item.full_name}: ${item.freshness.state}`);
});

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
    const rows = FLEET.runners.filter((r) => r.pool_id === pool.id && LIVE.includes(r.state));
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

test('a workflow run counts the jobs that belong to it', () => {
  for (const run of FLEET.runs) {
    const own = FLEET.jobs.filter((j) => j.github_run_id === run.github_run_id);
    assert.equal(run.jobs, own.length, `${run.repo} ${run.workflow} #${run.run_number}`);
    assert.ok(own.every((j) => j.repo === run.repo && j.workflow === run.workflow), 'one run is one workflow of one repository');
  }
  assert.ok(FLEET.runs.some((run) => run.jobs > 1), 'at least one run has more than one job');
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
