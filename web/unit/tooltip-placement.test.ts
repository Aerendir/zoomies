import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  ARROW_INSET,
  GAP,
  MARGIN,
  narrowPlacement,
  place,
  type Box,
} from '../src/lib/tooltip-place';

const viewport = { width: 1000, height: 800 };
const size = { width: 200, height: 60 };

/** A 40 by 20 trigger whose top left corner is at (left, top). */
const trigger = (left: number, top: number): Box => ({
  left,
  top,
  right: left + 40,
  bottom: top + 20,
});

test('a bubble sits above its trigger, centred on it, with the caret in the middle', () => {
  const t = trigger(480, 400);
  const p = place({ anchor: t, size, viewport, preferred: 'top' });
  assert.equal(p.side, 'top');
  assert.equal(p.left, 500 - 100);
  assert.equal(p.top, 400 - 60 - GAP);
  assert.equal(p.arrow, 100, 'the caret is at the middle of a bubble that is centred');
});

test('it goes below when the preference is bottom', () => {
  const p = place({ anchor: trigger(480, 400), size, viewport, preferred: 'bottom' });
  assert.equal(p.side, 'bottom');
  assert.equal(p.top, 420 + GAP);
});

test('it flips to the other side when there is no room and the other side has more', () => {
  // 60 + GAP + MARGIN = 76 wanted above; only 50 there.
  const t = trigger(480, 50);
  const p = place({ anchor: t, size, viewport, preferred: 'top' });
  assert.equal(p.side, 'bottom');
  assert.equal(p.top, 70 + GAP);
});

test('the room it asks for includes the margin, not just the gap', () => {
  const need = size.height + GAP + MARGIN;
  const fits = place({ anchor: trigger(480, need), size, viewport, preferred: 'top' });
  assert.equal(fits.side, 'top', 'exactly enough room stays put');
  const short = place({ anchor: trigger(480, need - 1), size, viewport, preferred: 'top' });
  assert.equal(short.side, 'bottom', 'one pixel less flips');
});

test('it stays on its preferred side when the opposite side is no better', () => {
  // A bubble taller than either side: flipping would only move the problem.
  const tall = { width: 200, height: 500 };
  const t = trigger(480, 300); // 300 above, 480 below... below is more
  const flipped = place({ anchor: t, size: tall, viewport, preferred: 'top' });
  assert.equal(flipped.side, 'bottom');
  const worse = place({ anchor: trigger(480, 560), size: tall, viewport, preferred: 'top' });
  assert.equal(worse.side, 'top', 'only 220 below against 560 above');
});

test('it is held inside the margin, and the caret still points at the trigger', () => {
  // A trigger near the left edge: the bubble cannot centre on it, so it is
  // held at the margin, and the caret has to slide to keep pointing at it.
  const t = trigger(20, 400); // centre x = 40
  const p = place({ anchor: t, size, viewport, preferred: 'top' });
  assert.equal(p.left, MARGIN, 'clamped to the margin');
  assert.equal(p.left + p.arrow, 40, 'the caret is still under the trigger\u2019s centre');
});

test('the caret keeps its distance from the ends of the bubble', () => {
  // The trigger is so near the corner that its centre is inside the rounded end.
  const t = trigger(0, 400); // centre x = 20, bubble held at left = 8, so 12 in
  const p = place({ anchor: t, size, viewport, preferred: 'top' });
  assert.equal(p.arrow, ARROW_INSET, 'held 14px in from the start');

  const farRight = trigger(viewport.width - 40, 400); // centre x = 980
  const q = place({ anchor: farRight, size, viewport, preferred: 'top' });
  assert.equal(q.left, viewport.width - size.width - MARGIN);
  assert.equal(q.arrow, size.width - ARROW_INSET, 'and 14px in from the end');
});

test('a bubble narrower than two caret insets puts the caret in the middle', () => {
  const tiny = { width: 20, height: 20 };
  const p = place({ anchor: trigger(480, 400), size: tiny, viewport, preferred: 'top' });
  assert.equal(p.arrow, 10);
});

test('a side bubble is centred on the trigger vertically and clamped the same way', () => {
  const t = trigger(100, 400);
  const p = place({ anchor: t, size, viewport, preferred: 'right' });
  assert.equal(p.side, 'right');
  assert.equal(p.left, 140 + GAP);
  assert.equal(p.top, 410 - 30);
  assert.equal(p.arrow, 30);

  const top = place({ anchor: trigger(100, 0), size, viewport, preferred: 'right' });
  assert.equal(top.top, MARGIN);
  assert.equal(top.arrow, ARROW_INSET, 'the trigger\u2019s centre is 2px in, so the inset wins');
});

test('a side bubble flips across the trigger when the window has no room', () => {
  const t = trigger(viewport.width - 60, 400);
  const p = place({ anchor: t, size, viewport, preferred: 'right' });
  assert.equal(p.side, 'left');
  assert.equal(p.left, t.left - size.width - GAP);
});

test('an entry in the sidebar anchors to the sidebar’s edge so the bubble clears the rail', () => {
  // The entry is inset in a rail that ends at x = 240. The bubble must start
  // past the rail, not past the entry, or it sits on top of the rail.
  const entry: Box = { left: 12, top: 300, right: 228, bottom: 332 };
  const rail: Box = { left: 0, top: 0, right: 240, bottom: 800 };
  const p = place({ anchor: entry, edge: rail, size, viewport, preferred: 'right' });
  assert.equal(p.side, 'right');
  assert.equal(p.left, 240 + GAP);
  assert.equal(p.top, 316 - 30, 'vertical still follows the entry itself');
  assert.equal(p.arrow, 30);
});

test('the sidebar’s edge matters only to a side placement', () => {
  const entry: Box = { left: 12, top: 300, right: 228, bottom: 332 };
  const rail: Box = { left: 0, top: 0, right: 240, bottom: 800 };
  const withEdge = place({ anchor: entry, edge: rail, size, viewport, preferred: 'top' });
  const without = place({ anchor: entry, size, viewport, preferred: 'top' });
  assert.deepEqual(withEdge, without);
});

test('on a narrow screen a right preference becomes bottom', () => {
  assert.equal(narrowPlacement('right', true), 'bottom');
  assert.equal(narrowPlacement('right', false), 'right');
  for (const side of ['top', 'bottom', 'left'] as const) {
    assert.equal(narrowPlacement(side, true), side, `${side} is not touched`);
  }
});

test('a bubble bigger than the window is pinned to the margin rather than off-screen', () => {
  const huge = { width: 2000, height: 2000 };
  const p = place({ anchor: trigger(480, 400), size: huge, viewport, preferred: 'top' });
  assert.equal(p.left, MARGIN);
  assert.equal(p.top, MARGIN);
});
