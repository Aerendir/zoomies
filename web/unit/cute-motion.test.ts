import { test } from 'node:test';
import assert from 'node:assert/strict';
import { cuteMotion, cutePose } from '../src/lib/runners/cute-motion';

test('Cute keeps its existing gait and speed tiers when Standard changes style', () => {
  for (const seed of ['runner-one', 'runner-two', 'workflow-left', 'workflow-right']) {
    const maximum = cuteMotion('maximum_zoomies', seed);
    const extra = cuteMotion('zoomies', seed);
    const busy = cuteMotion('busy', seed);
    assert.ok(maximum.stride >= 0.56 && maximum.stride < 0.58);
    assert.ok(extra.stride >= 0.8 && extra.stride < 0.82);
    assert.ok(busy.stride >= 1.6 && busy.stride < 1.72);
    assert.ok(busy.walking && !extra.walking && !maximum.walking);
    assert.ok(maximum.spinning && extra.spinning && !busy.spinning);
    assert.deepEqual(cuteMotion('maximum_zoomies', seed), maximum);
  }
  for (const state of [
    'idle',
    'provisioning',
    'registering',
    'throttled',
    'draining',
    'failed',
    'removed',
    'unknown',
  ]) {
    assert.equal(cuteMotion(state, 'runner').walking, false);
    assert.equal(cuteMotion(state, 'runner').spinning, false);
  }
});

test('every state holds a pose that separates it at grid size', () => {
  // Posture and ears carry the state when the props have stepped aside, so
  // states that share a posture must differ in the ears or the eyes, and the
  // four sitting states must not all wear the same ears.
  const states = [
    'idle',
    'busy',
    'provisioning',
    'registering',
    'throttled',
    'draining',
    'failed',
    'removed',
    'unknown',
    'zoomies',
    'maximum_zoomies',
  ];
  const poses = states.map((state) => cutePose(state));
  assert.equal(
    new Set(poses.map((p) => `${p.pose}/${p.ears}/${p.eyes}/${p.mouth}/${p.props}`)).size,
    states.length,
  );
  // The collar tag and props borrow the status tone, the same fixed mapping
  // the rest of the UI uses.
  assert.equal(cutePose('idle').tone, 'idle');
  assert.equal(cutePose('zoomies').tone, 'busy');
  assert.equal(cutePose('throttled').tone, 'pending');
  assert.equal(cutePose('draining').tone, 'draining');
  assert.equal(cutePose('failed').tone, 'danger');
  assert.equal(cutePose('removed').tone, 'neutral');
  // Transitional states carry one prop each; the boosts carry their dust.
  assert.equal(cutePose('provisioning').props, 'trail');
  assert.equal(cutePose('registering').props, 'ring');
  assert.equal(cutePose('draining').props, 'leash');
  assert.equal(cutePose('throttled').props, 'leash');
  assert.ok(cutePose('maximum_zoomies').props.includes('sparkle'));
});

test('a state this build does not recognise falls back to the questioning pose', () => {
  assert.deepEqual(cutePose('future-state'), cutePose('unknown'));
});
