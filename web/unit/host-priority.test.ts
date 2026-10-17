import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  isPinned,
  pinError,
  priorityBody,
  prioritySourceWords,
} from '../src/lib/hosts/priority.ts';
import type { Host } from '../src/lib/api/types.ts';

const unpinned: Host = { priority: 43, priority_source: 'computed' } as unknown as Host;
const pinned: Host = {
  priority: 90,
  priority_override: 90,
  priority_source: 'overridden',
} as unknown as Host;

test('a host with an override is pinned, whatever its specs scored', () => {
  assert.equal(isPinned(pinned), true);
  assert.equal(prioritySourceWords(pinned), 'set by an operator');
  assert.equal(prioritySourceWords(unpinned), 'scored from its specs');
});

test('a body is sent only where the answer changes', () => {
  // Leaving the dialog alone sends nothing at all: a PATCH that changes
  // nothing should not look like a decision the operator made.
  assert.deepEqual(priorityBody(unpinned, 'auto', null), {});
  assert.deepEqual(priorityBody(pinned, 'pinned', 90), {});
  assert.deepEqual(priorityBody(unpinned, 'pinned', 90), {
    priority: { override: 90 },
  });
  // Clearing a pin is its own decision, and null is how it is said.
  assert.deepEqual(priorityBody(pinned, 'auto', null), {
    priority: { override: null },
  });
});

test('a pin outside the scale is refused with the scale named', () => {
  assert.equal(pinError(90), '');
  assert.equal(pinError(null), 'Write a whole number between 0 and 100.');
  assert.match(pinError(101) ?? '', /outside the 0..100 scale/);
  assert.match(pinError(-1) ?? '', /outside the 0..100 scale/);
});
