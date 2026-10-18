import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  JOURNEY_STEPS,
  REPAIR_TRACK,
  isTerminal,
  outcomeTone,
  runningRepair,
  stateOf,
  stepState,
  trackIndex,
} from '../src/lib/repairs/journey';

// The stepper is the journey's promise: nothing is painted as behind you until
// it has happened, and a readiness check that came back blocked is still the
// check, not a step somebody finished.
test('the stepper holds a blocked readiness on the first step', () => {
  for (const step of JOURNEY_STEPS.map((s) => s.id)) {
    assert.equal(stepState(step, 'blocked'), step === 'check' ? 'current' : 'todo');
    assert.equal(stepState(step, 'idle'), 'todo');
  }
  assert.equal(stepState('check', 'checking'), 'current');
  assert.equal(stepState('check', 'ready'), 'done');
  assert.equal(stepState('review', 'ready'), 'current');
  assert.equal(stepState('review', 'running'), 'done');
  assert.equal(stepState('repair', 'running'), 'current');
  assert.equal(stepState('repair', 'done'), 'done');
  assert.equal(stepState('outcome', 'done'), 'current');
});

// The track is what a waiting operator reads, so a row caught between two
// writes may not move the mark backwards: the commit is the proof, and an
// absent state is the queued state rather than a finished one.
test('the track moves forwards only, and an absent state is queued', () => {
  assert.equal(stateOf(null), 'queued');
  assert.equal(stateOf({}), 'queued');
  assert.equal(trackIndex({ state: 'queued' }), 0);
  assert.equal(trackIndex({ state: 'working', details: { stage: 'evidence' } }), 1);
  assert.equal(trackIndex({ state: 'working', details: { stage: 'source' } }), 1);
  assert.equal(trackIndex({ state: 'working', details: { stage: 'prepare_fix' } }), 2);
  assert.equal(trackIndex({ state: 'working', details: { stage: 'publish' } }), 3);
  assert.equal(trackIndex({ state: 'checking', details: { stage: 'watch_ci' } }), 4);
  // A stale stage beside a published commit: the push happened, so the mark
  // sits at the watch and not at whatever the worker last wrote.
  assert.equal(
    trackIndex({ state: 'working', commit_sha: 'abc', details: { stage: 'evidence' } }),
    4,
  );
  // No state at all, as a row read mid-write can be: queued, not finished.
  assert.equal(trackIndex({}), 0);
  assert.equal(REPAIR_TRACK.length, 5);
});

// Following stops at a terminal state and not before, or the card would either
// poll for ever or announce an outcome the worker has not reached.
test('following stops exactly at the states the worker leaves', () => {
  for (const state of ['queued', 'working', 'checking']) assert.equal(isTerminal(state), false);
  for (const state of [
    'succeeded',
    'checks_failed',
    'failed',
    'blocked',
    'interrupted',
    'unverified',
    'superseded',
  ])
    assert.equal(isTerminal(state), true);
});

// Success and failure are the two outcomes an operator acts on differently;
// the in-betweens are news, not verdicts, and must not borrow either colour.
test('the outcome colour is reserved for verdicts', () => {
  assert.equal(outcomeTone('succeeded'), 'success');
  assert.equal(outcomeTone('checks_failed'), 'danger');
  assert.equal(outcomeTone('failed'), 'danger');
  assert.equal(outcomeTone('interrupted'), 'danger');
  assert.equal(outcomeTone('superseded'), 'neutral');
  assert.equal(outcomeTone('unverified'), 'neutral');
});

// A failure is reached by its run, but only a repair Eli queued itself records a
// run: one a person queued from the page is known by its pull request and commit.
// Looking only by run offered that repair a second time to whoever came back to
// the failure.
test('a repair a person queued is found by its pull request and commit, not by a run', () => {
  const row = (over: { run_id?: number } = {}) => ({
    state: 'working',
    repo: 'Acme/Widgets',
    run_id: 0,
    pull_number: 17,
    head_sha: 'abc123',
    ...over,
  });
  const automatic = row({ run_id: 1234 });
  const queuedByAPerson = row();

  // By the run it arrived with: the automatic repair, and not the person's.
  assert.equal(
    runningRepair([queuedByAPerson, automatic], { repo: 'acme/widgets', run: 1234 }),
    automatic,
  );
  assert.equal(runningRepair([queuedByAPerson], { repo: 'acme/widgets', run: 1234 }), undefined);

  // Once readiness has resolved the run to its pull request and commit.
  const resolved = { repo: 'acme/widgets', pull: 17, head: 'abc123' };
  assert.equal(runningRepair([queuedByAPerson], resolved), queuedByAPerson);

  // Arriving by pull request alone is as it was: any repair on that pull request.
  assert.equal(
    runningRepair([queuedByAPerson], { repo: 'acme/widgets', pull: 17 }),
    queuedByAPerson,
  );
});

// A repair for another commit is a different task, and one that has finished is
// not running: neither may take the place of the offer.
test('a repair on another commit, another pull request or a finished one is not the one running', () => {
  const base = { repo: 'acme/widgets', run_id: 0, pull_number: 17, head_sha: 'abc123' };
  const want = { repo: 'acme/widgets', pull: 17, head: 'abc123' };
  assert.equal(runningRepair([{ ...base, state: 'working', head_sha: 'older' }], want), undefined);
  assert.equal(runningRepair([{ ...base, state: 'working', pull_number: 18 }], want), undefined);
  assert.equal(runningRepair([{ ...base, state: 'working', repo: 'acme/other' }], want), undefined);
  for (const state of ['succeeded', 'failed', 'checks_failed', 'interrupted', 'superseded']) {
    assert.equal(runningRepair([{ ...base, state }], want), undefined, state);
  }
  for (const state of ['queued', 'working', 'checking']) {
    assert.ok(runningRepair([{ ...base, state }], want), state);
  }
});
