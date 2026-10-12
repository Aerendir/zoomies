import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const src = join(import.meta.dirname, '..', 'src', 'lib', 'assistant');
const button = readFileSync(join(src, 'AskEli.svelte'), 'utf8');
const avatar = readFileSync(join(src, 'EliAvatar.svelte'), 'utf8');

// The Ask Eli button on a page and the dog in the corner are the same way to
// reach Eli, so they wear the same blue: the avatar's tokens, not a colour of
// the button's own that one theme would get wrong.
test('the Ask Eli button wears the tokens the corner avatar wears', () => {
  for (const token of ['--z-accent', '--z-accent-subtle', '--z-accent-border']) {
    assert.ok(avatar.includes(`var(${token})`), `${token} is the avatar's`);
    assert.ok(button.includes(`var(${token})`), `${token} is the button's`);
  }
  assert.doesNotMatch(button, /#[0-9a-f]{3,8}\b/i, 'no colour of its own');
});

// The button is meant to be noticed: it wiggles once when the page first
// shows it and again under the pointer. Those are two animation names on
// purpose, because a hover that re-declares the arrival's name does not
// restart it, and a person who asked for less motion gets neither.
test('the Ask Eli button wiggles on arrival and on hover, unless motion is reduced', () => {
  assert.match(button, /\.btn\.ask-eli\)\s*\{[^}]*animation:\s*arrive\b/);
  assert.match(
    button,
    /\.btn\.ghost\.ask-eli:hover:not\(:disabled\):not\(\[aria-disabled='true'\]\)\)\s*\{[^}]*animation:\s*wiggle\b/,
  );
  assert.match(button, /@keyframes arrive\b/);
  assert.match(button, /@keyframes wiggle\b/);
  const reduced = button.slice(button.indexOf('prefers-reduced-motion: reduce'));
  assert.match(reduced, /:global\(\.btn\.ask-eli\),[\s\S]*animation:\s*none/);
});
