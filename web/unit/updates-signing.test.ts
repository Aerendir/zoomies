import { test } from 'node:test';
import assert from 'node:assert/strict';
import { signingLine } from '../src/lib/updates/words.ts';

// The line a build with no release key shows: it does not verify a release, so
// an operator is told the checksums are taken on trust rather than being left to
// believe a signature was checked.
test('a build with no release key says its checksums are taken on trust', () => {
  assert.match(signingLine([]), /taken on trust/);
});

test('a build with one release key names it', () => {
  assert.match(signingLine(['0123456789abcdef']), /0123456789abcdef/);
});

// A rotation carries two keys for a release, so the line counts them and names
// each, and a reader can tell a transition build from a single-key one.
test('a build with more than one release key counts and names them', () => {
  const line = signingLine(['aaaaaaaaaaaaaaaa', 'bbbbbbbbbbbbbbbb']);
  assert.match(line, /2 keys/);
  assert.match(line, /aaaaaaaaaaaaaaaa/);
  assert.match(line, /bbbbbbbbbbbbbbbb/);
});
