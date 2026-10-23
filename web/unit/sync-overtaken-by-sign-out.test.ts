import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { test } from 'node:test';

// The stores are Svelte runes classes. Outside the compiler a rune is its
// value, which is all these tests need of them.
const runes = globalThis as unknown as Record<string, unknown>;
runes.$state = <T>(v: T) => v;
runes.$derived = <T>(v: T) => v;
runes.$effect = Object.assign(() => {}, { root: () => () => {} });
const { prefs } = await import('../src/lib/state/prefs.svelte.ts');

// A sign-in no longer waits for the account's saved layouts to arrive, so a
// slow answer can land after the person has signed out, or after somebody else
// has signed in. It describes the previous account and must be dropped, not
// applied to whoever is there now.
function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

test('layouts that arrive after a sign-out are not applied', async () => {
  const saved = globalThis.fetch;
  const answer = deferred<Response>();
  globalThis.fetch = () => answer.promise;
  try {
    const sync = prefs.syncAccount('usr_previous');
    prefs.disconnectAccount();
    answer.resolve(Response.json({ table_layouts: { runners: { widths: { name: 321 } } } }));
    await sync;
    assert.equal(prefs.columnWidth('runners', 'name'), undefined);
  } finally {
    globalThis.fetch = saved;
  }
});

test('layouts that arrive for the account still signed in are applied', async () => {
  const saved = globalThis.fetch;
  globalThis.fetch = async () =>
    Response.json({ table_layouts: { hosts: { widths: { name: 222 } } } });
  try {
    await prefs.syncAccount('usr_current');
    assert.equal(prefs.columnWidth('hosts', 'name'), 222);
  } finally {
    globalThis.fetch = saved;
  }
});

// The dismissals store imports the fleet store by an extensionless path that
// Node cannot load, so its guard is held by its shape: a read of the account
// is checked against the sync it belongs to after every request, and a
// disconnect ends the syncs in flight.
test('the dismissals sync is dropped when a sign-out or another sign-in overtakes it', () => {
  const src = readFileSync(
    join(import.meta.dirname, '..', 'src/lib/state/notifications.svelte.ts'),
    'utf8',
  );
  const body = src.slice(
    src.indexOf('async syncAccount('),
    src.indexOf('disconnectAccount(): void'),
  );
  assert.match(body, /const gen = \+\+this\.#syncGen;/);
  for (const request of ['await getProblemDismissals()', 'await patchProblemDismissals(']) {
    const at = body.indexOf(request);
    assert.ok(at >= 0, `${request} is still made`);
    assert.match(
      body.slice(at, at + 160),
      /if \(gen !== this\.#syncGen\) return;/,
      `${request} is followed by the check`,
    );
  }
  assert.match(
    src.slice(src.indexOf('disconnectAccount(): void')),
    /^disconnectAccount\(\): void \{\s*this\.#syncGen\+\+;/,
  );
});
