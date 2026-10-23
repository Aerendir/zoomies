import { test } from 'node:test';
import assert from 'node:assert/strict';
import { figuresPlacement } from '../src/lib/bands/placement.ts';

test('figures sit in the band on a desktop and alone only on a phone while the band is drawn', () => {
  assert.equal(figuresPlacement({ shown: true, hasLine: true, hasStats: true }), 'phone-only');
});

test('figures stand alone whenever the band is off', () => {
  assert.equal(figuresPlacement({ shown: false, hasLine: true, hasStats: true }), 'always');
  assert.equal(figuresPlacement({ shown: false, hasLine: false, hasStats: true }), 'always');
});

// The sentence is made from fleet figures and the strip from the page's own request, so the
// strip can arrive when the sentence never does. Hiding it then would lose numbers that loaded.
test('figures that loaded are not hidden because the sentence over them did not', () => {
  assert.equal(figuresPlacement({ shown: true, hasLine: false, hasStats: true }), 'always');
});

test('there is nothing to place without figures', () => {
  for (const shown of [true, false]) {
    for (const hasLine of [true, false]) {
      assert.equal(figuresPlacement({ shown, hasLine, hasStats: false }), 'none');
    }
  }
});
