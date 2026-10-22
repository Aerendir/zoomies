import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  aiContextInSetup,
  aiContextItemState,
  aiContextNeedsAttention,
  aiContextReady,
} from '../src/lib/aicontext/state.ts';

const config = (over: Record<string, unknown> = {}) => ({
  id: 'aic_1',
  full_name: 'acme/context',
  config: { source_branch: 'main', destination: 'both', exclude: [], keep_snapshots: 7 },
  available: false,
  ...over,
});

// The reader's copy has no config, and the only thing true of it is that its
// source is available: calling it a draft would name a state they cannot have.
test('a source reader is its own state, not a draft they cannot own', () => {
  assert.equal(aiContextItemState({ id: 'aic_reader', full_name: 'acme/context' }), 'available');
});

// The badge and the summary count read the same function, so a freshness state
// on the record always wins over the guess made from the flags beside it.
test('a known freshness state is the state, whatever the flags say', () => {
  assert.equal(aiContextItemState(config({ freshness: { state: 'stale' } })), 'stale');
  assert.equal(aiContextItemState(config({ available: true })), 'available');
});

test('a setup that has not published yet says which step it is on', () => {
  assert.equal(aiContextItemState(config({ setup_state: 'awaiting_merge' })), 'awaiting_merge');
  assert.equal(aiContextItemState(config({ setup_state: 'pending' })), 'working');
});

test('a diagnosis with no freshness is something to act on, and a bare draft is not', () => {
  assert.equal(aiContextItemState(config({ diagnosis: { cause: 'unknown' } })), 'attention');
  assert.equal(aiContextItemState(config()), 'draft');
});

test('the three groups are disjoint and cover the states a person acts on', () => {
  assert.ok(aiContextNeedsAttention('stale'));
  assert.ok(aiContextNeedsAttention('unavailable'));
  assert.ok(aiContextNeedsAttention('attention'));
  assert.ok(!aiContextNeedsAttention('ready'));
  assert.ok(aiContextReady('ready'));
  assert.ok(aiContextReady('available'));
  assert.ok(aiContextInSetup('draft'));
  assert.ok(aiContextInSetup('review'));
  assert.ok(aiContextInSetup('awaiting_merge'));
  assert.ok(aiContextInSetup('working'));
  assert.ok(!aiContextInSetup('ready'));
});
