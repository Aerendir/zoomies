// The review page is driven by its address, so a combination is something a
// capture, a check or a colleague can name. The address is also something
// anybody can type, which is why most of these tests are about junk: whatever
// is in the hash, the page must land on a combination that exists.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { formatHash, parseHash } from './review-state.js';

const SCREENS = [
  { id: 'signin', states: ['password', 'sso'] },
  { id: 'status', states: ['healthy', 'blocked', 'stale'] },
];

const DEFAULTS = {
  screen: 'signin',
  concept: 'a',
  style: 'mission-control',
  mode: 'dark',
  state: 'password',
};

test('an empty hash is the first screen, concept A, Mission Control, dark', () => {
  assert.deepEqual(parseHash('', SCREENS), DEFAULTS);
  assert.deepEqual(parseHash('#', SCREENS), DEFAULTS);
});

test('the mode a browser asked for is the default mode, until the hash says otherwise', () => {
  assert.equal(parseHash('', SCREENS, { mode: 'light' }).mode, 'light');
  assert.equal(parseHash('#mode=dark', SCREENS, { mode: 'light' }).mode, 'dark');
  // A default that is not a mode is not trusted either.
  assert.equal(parseHash('', SCREENS, { mode: 'sepia' }).mode, 'dark');
});

test('a combination survives a round trip through formatHash', () => {
  const chosen = { screen: 'status', concept: 'c', style: 'classic', mode: 'light', state: 'stale' };
  assert.deepEqual(parseHash(formatHash(chosen), SCREENS), chosen);
});

test('formatHash writes all five keys, in the order the checker reads them', () => {
  assert.equal(
    formatHash(DEFAULTS),
    '#screen=signin&concept=a&style=mission-control&mode=dark&state=password',
  );
});

test('the hash may omit the leading # and any key', () => {
  assert.deepEqual(parseHash('screen=status', SCREENS), {
    ...DEFAULTS,
    screen: 'status',
    state: 'healthy',
  });
});

test('a screen, concept, style or mode nobody offers falls back, without throwing', () => {
  assert.deepEqual(parseHash('#screen=nope&concept=z&style=x&mode=y&state=q', SCREENS), DEFAULTS);
});

test('a state that belongs to another screen is not carried over', () => {
  // `stale` is real, but not on the sign-in screen.
  assert.equal(parseHash('#screen=signin&state=stale', SCREENS).state, 'password');
  // And a state the chosen screen does have is kept.
  assert.equal(parseHash('#screen=status&state=blocked', SCREENS).state, 'blocked');
});

test('every concept the page offers is accepted, current included', () => {
  for (const concept of ['current', 'a', 'b', 'c']) {
    assert.equal(parseHash(`#concept=${concept}`, SCREENS).concept, concept);
  }
});

test('malformed percent-encoding and repeated keys do not throw', () => {
  assert.doesNotThrow(() => parseHash('#screen=%E0%A4%A&concept=%', SCREENS));
  assert.deepEqual(parseHash('#screen=status&screen=signin', SCREENS).screen, 'status');
});

test('a hash that tries to be markup is only ever compared, never echoed', () => {
  const state = parseHash('#screen=<img src=x onerror=alert(1)>&state=<script>', SCREENS);
  assert.deepEqual(state, DEFAULTS);
});

test('with no screens there is nothing to land on, which is a programming error', () => {
  assert.throws(() => parseHash('', []), /no screens/);
});
