import { test } from 'node:test';
import assert from 'node:assert/strict';
import { focusIsUnclaimed } from '../src/lib/keys';

// Stand-ins for the three places focus can be when an overlay finishes closing.
// The decision only compares identities and asks the closing panel whether it
// contains the focused element, so plain objects are enough to pin it down.
const body = { name: 'body' };
const insideTheClosingPanel = { name: 'a control that has not been removed yet' };
const elsewhere = { name: 'the message box of a panel that opened as this one closed' };
const closing = { contains: (other: object) => other === insideTheClosingPanel };

// A drawer gives focus back to its opener a frame after it unmounts. That is
// right when nothing else has asked for focus in the meantime, and wrong when it
// has: "Ask Eli" closes the Problems drawer and opens a panel that is not modal,
// and the restore took focus from it and left it on the bell.
test('a closing overlay gives focus back only while nobody else has claimed it', () => {
  assert.equal(focusIsUnclaimed(null, body, closing), true, 'nothing holds focus');
  assert.equal(focusIsUnclaimed(body, body, closing), true, 'the focused control was removed');
  assert.equal(focusIsUnclaimed(insideTheClosingPanel, body, closing), true, 'still in the panel');
  assert.equal(focusIsUnclaimed(elsewhere, body, closing), false, 'another panel took it');
});
