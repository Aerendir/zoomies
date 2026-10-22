import { test } from 'node:test';
import assert from 'node:assert/strict';
import { allExpanded, isIdList, setAllExpanded, setExpanded } from '../src/lib/hosts/card-fold.ts';

test('a stored choice is trusted only while it is a list of host ids', () => {
  // A value left by an older build, or edited by hand, must fall back to every
  // card closed rather than throw on the first card that asks.
  assert.equal(isIdList([]), true);
  assert.equal(isIdList(['host_a', 'host_b']), true);
  for (const bad of [null, undefined, 'host_a', 7, {}, [1], ['host_a', 2], [null]]) {
    assert.equal(isIdList(bad), false, JSON.stringify(bad));
  }
});

test('opening a card remembers it once, however many times it is opened', () => {
  const known = ['host_a', 'host_b', 'host_c'];
  const once = setExpanded([], 'host_b', true, known);
  assert.deepEqual(once, ['host_b']);
  assert.deepEqual(setExpanded(once, 'host_b', true, known), ['host_b']);
  assert.deepEqual(setExpanded(once, 'host_a', true, known), ['host_b', 'host_a']);
});

test('closing a card forgets it and leaves the others open', () => {
  const known = ['host_a', 'host_b', 'host_c'];
  assert.deepEqual(setExpanded(['host_a', 'host_b'], 'host_a', false, known), ['host_b']);
  assert.deepEqual(
    setExpanded(['host_b'], 'host_c', false, known),
    ['host_b'],
    'not open: no change',
  );
});

test('hosts that no longer exist are forgotten the next time a choice is made', () => {
  // Machines come and go as pools scale, so without this the list would keep an
  // id for every host anyone ever opened.
  const remembered = ['host_gone', 'host_a'];
  assert.deepEqual(setExpanded(remembered, 'host_b', true, ['host_a', 'host_b']), [
    'host_a',
    'host_b',
  ]);
  assert.deepEqual(setExpanded(remembered, 'host_a', false, ['host_a']), []);
});

test('the list it is handed is never changed', () => {
  // The page replaces its state with the answer. A function that edited the old
  // array in place would hand back the very array already held, and nothing
  // would repaint.
  const held = ['host_a'];
  const next = setExpanded(held, 'host_b', true, ['host_a', 'host_b']);
  assert.deepEqual(held, ['host_a']);
  assert.notEqual(next, held);
});

test('every card on show is open only when each of them is on the list', () => {
  const shown = ['host_a', 'host_b'];
  assert.equal(allExpanded(['host_a', 'host_b'], shown), true);
  assert.equal(
    allExpanded(['host_b', 'host_a', 'host_z'], shown),
    true,
    'a card the filter hides, open or not, does not change the answer',
  );
  assert.equal(allExpanded(['host_a'], shown), false);
  assert.equal(allExpanded([], shown), false);
});

test('no card on show is never "all open", so the button cannot offer to close what is not there', () => {
  assert.equal(allExpanded([], []), false);
  assert.equal(allExpanded(['host_a'], []), false);
});

test('opening all opens the cards on show and leaves the others as they were', () => {
  const known = ['host_a', 'host_b', 'host_c'];
  assert.deepEqual(setAllExpanded(['host_c'], ['host_a', 'host_b'], true, known), [
    'host_c',
    'host_a',
    'host_b',
  ]);
  // A card that was already open is kept where it was and not listed twice.
  assert.deepEqual(setAllExpanded(['host_b'], ['host_a', 'host_b'], true, known), [
    'host_b',
    'host_a',
  ]);
});

test('closing all closes the cards on show and leaves the others open', () => {
  const known = ['host_a', 'host_b', 'host_c'];
  assert.deepEqual(
    setAllExpanded(['host_a', 'host_b', 'host_c'], ['host_a', 'host_b'], false, known),
    ['host_c'],
  );
  assert.deepEqual(setAllExpanded([], ['host_a'], false, known), [], 'nothing open: no change');
});

test('hosts that no longer exist are forgotten when every card is opened or closed together', () => {
  assert.deepEqual(setAllExpanded(['host_gone'], ['host_a'], true, ['host_a']), ['host_a']);
  assert.deepEqual(setAllExpanded(['host_gone', 'host_a'], ['host_a'], false, ['host_a']), []);
  // A card that is not in the known list is not a card, so it is not opened.
  assert.deepEqual(setAllExpanded([], ['host_a', 'host_ghost'], true, ['host_a']), ['host_a']);
});

test('the lists it is handed are never changed, however many cards it opens', () => {
  const held = ['host_a'];
  const shown = ['host_b', 'host_c'];
  const next = setAllExpanded(held, shown, true, ['host_a', 'host_b', 'host_c']);
  assert.deepEqual(held, ['host_a']);
  assert.deepEqual(shown, ['host_b', 'host_c']);
  assert.notEqual(next, held);
});

test('the lists it is handed are not changed when it prunes or closes either', () => {
  // The test above has nothing to prune and only opens, so a version that tidied
  // the stored list in place, or closed cards in place and handed the same array
  // back, would pass it and then repaint nothing. Frozen lists make any edit in
  // place throw, in both directions and with a host that has gone in the way.
  const held = Object.freeze(['host_gone', 'host_a', 'host_b']);
  const shown = Object.freeze(['host_a']);
  const known = Object.freeze(['host_a', 'host_b']);
  assert.deepEqual(setAllExpanded(held, shown, false, known), ['host_b']);
  assert.deepEqual(setAllExpanded(held, shown, true, known), ['host_a', 'host_b']);
  assert.deepEqual(held, ['host_gone', 'host_a', 'host_b']);
});
