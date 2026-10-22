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
  // At 31px and below CuteStatusIcon hides the props, the brows, the freckles
  // and the highlights, so posture, ears, eyes and mouth are all that tell one
  // state from another. Two states that agree on all four are the same picture
  // in a grid whatever their props say, so only the pairs named here may, and
  // each has to be there for a reason.
  const mayCoincide: { pair: [string, string]; why: string }[] = [
    {
      pair: ['zoomies', 'maximum_zoomies'],
      why: 'the sparkle that sets maximum zoomies apart steps aside at grid size, so its faster run, which the first test pins, carries the tier',
    },
  ];
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
  const atGridSize = (state: string) => {
    const p = cutePose(state);
    return `${p.pose}/${p.ears}/${p.eyes}/${p.mouth}`;
  };
  for (const [i, a] of states.entries()) {
    for (const b of states.slice(i + 1)) {
      const listed = mayCoincide.find(({ pair }) => pair.includes(a) && pair.includes(b));
      if (listed) {
        assert.equal(
          atGridSize(a),
          atGridSize(b),
          `${a} and ${b} no longer look alike at grid size; take the pair off mayCoincide (it was there because ${listed.why})`,
        );
      } else {
        assert.notEqual(
          atGridSize(a),
          atGridSize(b),
          `${a} and ${b} are the same picture at grid size (${atGridSize(a)}); give one a different posture, ears, eyes or mouth, or name the pair in mayCoincide with the reason`,
        );
      }
    }
  }
  // Posture cannot tell the sitting states from one another, so their ears
  // have to vary: they must not all wear the same pair.
  const sitting = states.filter((state) => cutePose(state).pose === 'sit');
  assert.ok(
    new Set(sitting.map((state) => cutePose(state).ears)).size > 1,
    `the sitting states (${sitting.join(', ')}) all wear the same ears`,
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
