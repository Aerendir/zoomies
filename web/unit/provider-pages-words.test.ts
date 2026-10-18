import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const routes = join(import.meta.dirname, '..', 'src', 'routes');

// A new provider opens on a maximum of two and one warm machine, so a page that
// says nothing is rented until a ceiling is raised above zero describes a form
// that no longer exists, and sits above the one that contradicts it. Zero is
// still how an operator says rent nothing, and that is what these say instead.
for (const page of ['ProviderWizard.svelte', 'Providers.svelte']) {
  test(`${page} does not say a new provider rents nothing until its ceiling is raised`, () => {
    const source = readFileSync(join(routes, page), 'utf8');
    assert.doesNotMatch(source, /Nothing is rented until you raise (the|a) ceiling above zero/);
    assert.match(source, /A ceiling of zero rents nothing\./);
  });
}
