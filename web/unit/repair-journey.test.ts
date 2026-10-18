import { test } from 'node:test';
import assert from 'node:assert/strict';
import { REPAIR_TRACK, stateOf, trackIndex } from '../src/lib/repairs/journey';

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
