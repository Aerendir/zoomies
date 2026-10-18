import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const source = readFileSync(
  join(import.meta.dirname, '..', 'src', 'lib', 'runners', 'StandardStatusIcon.svelte'),
  'utf8',
);

/**
 * What the component's own style block says a class gives its elements, with
 * later rules over earlier ones. Every rule read here is one class by itself, so
 * they are equal in specificity and the order they are written in decides.
 */
function declaredFor(classes: readonly string[]): Map<string, string> {
  const css = (/<style>([\s\S]*)<\/style>/.exec(source)?.[1] ?? '').replace(
    /\/\*[\s\S]*?\*\//g,
    '',
  );
  const wanted = new Set(classes.map((name) => `.${name}`));
  const declared = new Map<string, string>();
  for (const [, selectors, body] of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    if (!selectors!.split(',').some((selector) => wanted.has(selector.trim()))) continue;
    for (const declaration of body!.split(';')) {
      const [property, ...value] = declaration.split(':');
      if (property?.trim()) declared.set(property.trim(), value.join(':').trim());
    }
  }
  return declared;
}

// A presentation attribute loses to any rule that sets the same property, so the
// charge's fill="currentColor" was overridden by the fill: none every part of the
// dog shares, and the flash docs/ui-guidelines.md promises at maximum zoomies
// never painted: the path stayed a second outline drawn over the first.
test('the charge flash is drawn with a fill that the shared part rule does not take away', () => {
  assert.match(
    source,
    /class="part charge"/,
    'the charge is a part, and the element this is about',
  );
  const drawn = declaredFor(['part', 'charge']);
  assert.equal(drawn.get('fill'), 'currentColor');
  assert.equal(drawn.get('stroke'), 'none');
});

// The flash is the animation. With motion reduced the animation is switched off,
// and whatever the path is left as is all there is: a fill at full strength
// would be a solid chin on a dog that is meant to be still.
test('the charge is invisible until its animation raises it, so reduced motion shows no flash', () => {
  assert.equal(declaredFor(['part', 'charge']).get('fill-opacity'), '0');
});
