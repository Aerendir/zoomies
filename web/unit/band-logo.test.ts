import { test } from 'node:test';
import assert from 'node:assert/strict';
import { DEFAULT_BAND_LOGO_SIZE, isBandLogoSize } from '../src/lib/bands/band-logo.ts';

test('a dog size this build does not draw is not accepted, and auto is the default', () => {
  for (const size of ['auto', 'small', 'medium', 'large']) {
    assert.equal(isBandLogoSize(size), true, size);
  }
  assert.equal(isBandLogoSize('huge'), false);
  assert.equal(isBandLogoSize(undefined), false);
  assert.equal(DEFAULT_BAND_LOGO_SIZE, 'auto');
});
