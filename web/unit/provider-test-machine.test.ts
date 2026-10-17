import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  isTestMachine,
  latestTest,
  stepStatusText,
  testProgress,
} from '../src/lib/providers/testmachine.ts';
import type { StepStatus } from '../src/lib/providers/testmachine.ts';
import type { Machine } from '../src/lib/api/types.ts';

function machine(over: Partial<Machine> & { phases?: string[] }): Machine {
  const { phases = ['planned'], ...rest } = over;
  return {
    id: 'mach_1',
    provider_id: 'prv_1',
    name: 'zoomies-mach-1',
    state: 'creating',
    labels: {},
    safe_to_delete: false,
    test: true,
    timeline: phases.map((phase) => ({ phase, at: '2026-10-17T03:00:00Z' })),
    created_at: '2026-10-17T03:00:00Z',
    updated_at: '2026-10-17T03:00:00Z',
    ...rest,
  };
}

test('a machine that has only been planned is on the first step', () => {
  const progress = testProgress(machine({ state: 'planned' }));
  assert.equal(progress.outcome, 'running');
  assert.deepEqual(
    progress.steps.map((s) => s.status),
    ['active', 'waiting', 'waiting', 'waiting', 'waiting'],
  );
});

test('a step the guest skipped is passed over by the one after it', () => {
  // A clone that leaves the guest running never records a start.
  const progress = testProgress(
    machine({ state: 'bootstrapping', phases: ['planned', 'creating', 'created', 'bootstrapped'] }),
  );
  assert.deepEqual(
    progress.steps.map((s) => s.status),
    ['done', 'done', 'done', 'active', 'waiting'],
  );
});

test('a machine that reached ready passed, whether or not it has been released yet', () => {
  const phases = ['planned', 'created', 'starting', 'bootstrapped', 'enrolled', 'ready'];
  const draining = testProgress(machine({ state: 'draining', phases }));
  assert.equal(draining.outcome, 'passed');
  assert.ok(draining.steps.every((s) => s.status === 'done'));
  const gone = testProgress(machine({ state: 'deleted', phases: [...phases, 'deleted'] }));
  assert.equal(gone.outcome, 'passed');
  assert.match(gone.summary, /released/);
});

test('a machine that cloned and never enrolled fails at the step after the last one reached', () => {
  const progress = testProgress(
    machine({
      state: 'failed',
      phases: ['planned', 'created', 'starting'],
      bootstrap_error: 'the agent could not reach the controller',
    }),
  );
  assert.equal(progress.outcome, 'failed');
  assert.deepEqual(
    progress.steps.map((s) => s.status),
    ['done', 'done', 'failed', 'waiting', 'waiting'],
  );
  assert.equal(progress.problem, 'the agent could not reach the controller');
});

test('the newest test machine is found among machines that are not tests', () => {
  const older = machine({ id: 'a', created_at: '2026-10-17T01:00:00Z' });
  const newer = machine({ id: 'b', created_at: '2026-10-17T02:00:00Z' });
  const demand = machine({ id: 'c', test: false, created_at: '2026-10-17T03:00:00Z' });
  assert.equal(latestTest([older, demand, newer])?.id, 'b');
  assert.equal(latestTest([demand]), null);
  assert.equal(isTestMachine(demand), false);
});

// The icon and the colour that say where a step stands are drawn, not read, so
// a screen reader gets five identical list items unless each carries a word of
// its own. A failed test is the reason the panel exists, and the one step that
// failed is what somebody listening has to be told.
test('every step says where it stands in words, and the failed one is not left to its icon', () => {
  const progress = testProgress(
    machine({
      state: 'failed',
      phases: ['planned', 'created', 'starting'],
      bootstrap_error: 'the agent could not reach the controller',
    }),
  );
  assert.deepEqual(
    progress.steps.map((step) => stepStatusText(step.status)),
    ['done', 'done', 'failed', 'not reached', 'not reached'],
  );
});

test('a step that is under way is told apart from one that has not been reached', () => {
  const progress = testProgress(
    machine({ state: 'bootstrapping', phases: ['planned', 'creating', 'created', 'starting'] }),
  );
  assert.deepEqual(
    progress.steps.map((step) => stepStatusText(step.status)),
    ['done', 'done', 'in progress', 'not reached', 'not reached'],
  );
});

test('no two states share a word, so the words can stand in for the icons', () => {
  const states: StepStatus[] = ['done', 'active', 'waiting', 'failed'];
  const words = states.map(stepStatusText);
  assert.equal(new Set(words).size, states.length);
  assert.ok(words.every((word) => word.trim() !== ''));
});
