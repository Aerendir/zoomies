import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  Activity,
  ChevronsUp,
  CircleDot,
  CircleMinus,
  CircleSlash,
  CircleX,
  Clock,
  Handshake,
  Hourglass,
  LoaderCircle,
  OctagonPause,
  TimerOff,
  TrendingDown,
  TrendingUp,
  Zap,
} from '@lucide/svelte';
import { offMotion } from '../src/lib/components/off-motion';

test('only work still in progress moves, and each glyph moves its own way', () => {
  // Seven glyphs, seven motions: being set up turns, work executing pulses
  // along its trace, being accepted breathes, finishing turns its hourglass
  // over, extra CPU hops its trend line, maximum CPU buzzes its bolt and CPU
  // being held back sags, so the three CPU states are told apart by how they
  // move and not only by which way their line points.
  const moving = [LoaderCircle, Activity, Handshake, Hourglass, TrendingUp, Zap, TrendingDown].map(
    (icon) => offMotion(icon),
  );
  assert.deepEqual(moving, ['spin', 'trace', 'breathe', 'turn', 'lift', 'buzz', 'sag']);
});

test('whatever is waiting or finished holds still', () => {
  // A failure that moved would draw the eye the way a healthy runner should
  // not, and a page of idle runners should be a calm one.
  for (const icon of [
    CircleDot,
    Clock,
    ChevronsUp,
    OctagonPause,
    CircleSlash,
    TimerOff,
    CircleX,
    CircleMinus,
  ])
    assert.equal(offMotion(icon), 'none');
});

test('a busy glyph missing from the map falls back to motion, not stillness', () => {
  // A new in-progress glyph nobody mapped yet must not look finished; a
  // waiting or finished one should stay calm, so only the busy tone falls back.
  assert.equal(offMotion(CircleDot, 'busy'), 'breathe');
  assert.equal(offMotion(CircleDot), 'none');
  assert.equal(offMotion(CircleDot, 'idle'), 'none');
});
