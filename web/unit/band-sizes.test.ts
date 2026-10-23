import { test } from 'node:test';
import assert from 'node:assert/strict';
import { BAND_PAGES, defaultBandSize, isBandSize } from '../src/lib/bands/pages.ts';

// The sizes are the only thing the first load shows differently from before, so they are
// held to what the pages looked like: the Overview's full stage, everything else slim.
test('the Overview opens on a standard band and every other page on a compact one', () => {
  assert.equal(defaultBandSize('overview'), 'standard');
  for (const page of BAND_PAGES) {
    if (page.key !== 'overview') assert.equal(defaultBandSize(page.key), 'compact', page.key);
  }
});

test('only the two sizes this build draws are sizes', () => {
  assert.ok(isBandSize('compact'));
  assert.ok(isBandSize('standard'));
  assert.ok(!isBandSize('huge'));
  assert.ok(!isBandSize(undefined));
});
