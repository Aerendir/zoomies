// The set of pictures is a decision, so it is tested before anything is
// photographed: which files exist, what they are called, and that the one the
// README recommends for each screen is shown in the other style and mode too.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { CAPTURE_STATE, RECOMMENDED, SIGNED_IN_STATE, plan, signedInPlan } from './capture.mjs';

const SCREENS = ['signin', 'first-account', 'connect-app', 'fleet-status', 'not-found'];

test('every screen has a capture state and a recommendation', () => {
  for (const screen of SCREENS) {
    assert.ok(CAPTURE_STATE[screen], `${screen} has no capture state`);
    assert.ok(['a', 'b', 'c'].includes(RECOMMENDED[screen]), `${screen} has no recommendation`);
  }
});

test('the plan is three concepts at two widths per screen, plus the recommended one in Classic light', () => {
  const shots = plan(SCREENS);
  assert.equal(shots.length, SCREENS.length * 3 * 2 + SCREENS.length);
  assert.equal(new Set(shots.map((s) => s.file)).size, shots.length, 'every file name is unique');
});

test('files are named concept, screen, concept letter, and the look when it is not the usual one', () => {
  const names = plan(SCREENS).map((s) => s.file);
  assert.ok(names.includes('concept-signin-a-1440.png'));
  assert.ok(names.includes('concept-fleet-status-c-375.png'));
  assert.ok(names.includes('concept-signin-b-classic-light-1440.png'));
  assert.ok(names.every((n) => /^concept-[a-z-]+-[abc](-classic-light)?-(1440|375)\.png$/.test(n)), names.join(', '));
});

test('the usual look is Mission Control dark, and only the recommended concept is shown in Classic light', () => {
  const shots = plan(SCREENS);
  for (const shot of shots.filter((s) => !s.file.includes('classic-light'))) {
    assert.match(shot.hash, /style=mission-control&mode=dark/);
  }
  const light = shots.filter((s) => s.file.includes('classic-light'));
  assert.equal(light.length, SCREENS.length);
  for (const shot of light) {
    assert.match(shot.hash, /style=classic&mode=light/);
    assert.equal(shot.width, 1440);
    assert.equal(shot.concept, RECOMMENDED[shot.screen]);
  }
});

test('each picture is of the state that shows the design best, in the hash the page reads', () => {
  for (const shot of plan(SCREENS)) {
    assert.match(shot.hash, new RegExp(`screen=${shot.screen}&concept=${shot.concept}`));
    assert.match(shot.hash, new RegExp(`state=${CAPTURE_STATE[shot.screen]}$`));
  }
});

/* the second pack: one concept, so a screen is three pictures and not ten */

const SIGNED_IN = Object.keys(SIGNED_IN_STATE);

test('the signed-in pack photographs every screen three ways: dark and phone, and Classic light', () => {
  const shots = signedInPlan(SIGNED_IN);
  assert.equal(shots.length, SIGNED_IN.length * 3);
  assert.equal(new Set(shots.map((s) => s.file)).size, shots.length, 'every file name is unique');
  for (const screen of SIGNED_IN) {
    const own = shots.filter((s) => s.screen === screen).map((s) => s.file);
    assert.deepEqual(own, [`concept-${screen}-1440.png`, `concept-${screen}-375.png`, `concept-${screen}-classic-light-1440.png`]);
  }
});

test('the signed-in pictures are of the portal style, in the state that shows the page best', () => {
  for (const shot of signedInPlan(SIGNED_IN)) {
    assert.match(shot.hash, new RegExp(`screen=${shot.screen}&concept=a&`));
    assert.match(shot.hash, new RegExp(`state=${SIGNED_IN_STATE[shot.screen]}$`));
    assert.match(shot.hash, shot.file.includes('classic-light') ? /style=classic&mode=light/ : /style=mission-control&mode=dark/);
  }
});

test('a screen the pack offers that has no capture state is refused by name, not photographed in a guess', () => {
  assert.throws(() => signedInPlan(['no-such-screen']), /no capture state for "no-such-screen"/);
});
