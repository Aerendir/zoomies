import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  JOURNEY_STEPS,
  REPAIR_TRACK,
  isTerminal,
  outcomeTone,
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
