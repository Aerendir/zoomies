import { test } from 'node:test';
import assert from 'node:assert/strict';
import { COMMITTED_TIGHT, committedNotice } from '../src/lib/hosts/committed.ts';

// A closed host card folds the Committed bars away. They are the one thing that
// explains a host with slots free that still takes nothing, because the slot
// line counts slots and the memory bar beside it is usage, not reservations.
// These are the rows HostCard builds for that block, and the cases in which the
// card must say so without being opened.

const cpu = (used: number, total = 14.2) => ({
  label: 'CPU',
  used,
  total,
  text: `${used} of ${total}`,
});
const memory = (used: number, total = 30_720) => ({
  label: 'Memory',
  used,
  total,
  text: `${(used / 1024).toFixed(0)} GB of ${(total / 1024).toFixed(0)} GB`,
});

test('a host with free slots and its memory all promised away says so, with the figures the Committed block shows', () => {
  const notice = committedNotice([cpu(4), memory(30_720)], 6);
  assert.match(notice, /memory 30 GB of 30 GB/);
  assert.doesNotMatch(notice, /CPU/, 'CPU is nowhere near its line, so it is not named');
  assert.match(notice, /may not take another runner/, 'it says what the free slots cannot do');
});

test('the notice starts at the line the Committed bars turn amber at, and not a megabyte before it', () => {
  // ResourceBar draws its bar and figure amber from COMMITTED_TIGHT; a card that
  // used another line would be amber in one place and silent in the other.
  assert.equal(COMMITTED_TIGHT, 0.9);
  assert.notEqual(committedNotice([memory(27_648)], 1), '', 'exactly 90% is tight');
  assert.equal(committedNotice([memory(27_647)], 1), '', 'a megabyte under it is not');
  assert.notEqual(committedNotice([memory(40_000)], 1), '', 'over-committed is more so, not less');
});

test('a host with no free slot says nothing, because the slot line already says why', () => {
  // A full host has everything promised away by design, so on a busy fleet this
  // would otherwise be on every card and explain none of them.
  assert.equal(committedNotice([cpu(14.2), memory(30_720)], 0), '');
});

test('both resources are named when both are close, CPU first as in the block', () => {
  const notice = committedNotice([cpu(13), memory(29_696)], 2);
  assert.match(notice, /CPU 13 of 14\.2, memory 29 GB of 30 GB/);
});

test('a host that has not reported its size has no committed figures to speak of', () => {
  // The block is absent for such a host (resources null in the card), and so is
  // its notice. An empty list, or a resource with nothing to place on, is the
  // same: a ratio over zero is not a full machine.
  assert.equal(committedNotice(null, 3), '');
  assert.equal(committedNotice([], 3), '');
  assert.equal(committedNotice([memory(100, 0)], 3), '');
});
