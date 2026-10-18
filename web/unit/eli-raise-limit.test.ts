import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import test from 'node:test';

const view = readFileSync(
  join(import.meta.dirname, '..', 'src', 'lib/assistant/ConversationView.svelte'),
  'utf8',
).replace(/\s+/g, ' ');

// The button that raises the limit after a cut answer writes a number to the
// provider, and a repair keeps a number exactly where it would otherwise raise
// the room itself. Pressed once to read one long answer, it switches that off
// for every repair on the provider, so the page says so before the button is
// pressed and does not leave a person to find it out from a failed repair.
test('the button that raises the output limit says that a repair then keeps the number', () => {
  const beforeTheButton = view.split('{#if raisable(turn)')[1]?.split('<button')[0] ?? '';
  assert.match(beforeTheButton, /Output limit/);
  assert.match(beforeTheButton, /\brepair\b/);
  assert.match(beforeTheButton, /exact/);
});
