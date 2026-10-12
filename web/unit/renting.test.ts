import assert from 'node:assert/strict';
import test from 'node:test';
import {
  clampCeiling,
  poolRenting,
  refusalText,
  rentingNotice,
  settingsFor,
} from '../src/lib/providers/renting.ts';
import type { PoolRenting, ProviderRenting } from '../src/lib/api/types.ts';

const base: ProviderRenting = {
  state: 'on',
  summary: '',
  enabled: false,
  paused: false,
  max_machines: 0,
  owned: 0,
  pools: [],
};

function pool(id: string, state: PoolRenting['state']): PoolRenting {
  return {
    pool_id: id,
    pool_name: id,
    state,
    summary: `${id} is ${state}`,
    queued: 1,
    short: 1,
    machines: 0,
  };
}

test('renting off asks an administrator for one click that sets both settings', () => {
  const n = rentingNotice(
    { ...base, state: 'off', summary: 'S', fix: 'F', suggested_max_machines: 2 },
    true,
  );
  assert.equal(n?.title, 'Renting is switched off');
  assert.equal(n?.body, 'S');
  assert.equal(n?.fix, 'F');
  assert.equal(n?.action?.label, 'Turn on renting');
  assert.deepEqual(settingsFor(n!, 2), { 'provider.enabled': true, 'provider.max_machines': 2 });
});

test('someone who cannot change settings is told who can', () => {
  const n = rentingNotice({ ...base, state: 'off' }, false);
  assert.equal(n?.action, null);
  assert.equal(n?.hint, 'An administrator can change this under Settings, Rent machines.');
});

test('no ceiling offers the ceiling alone', () => {
  const pools = [pool('a', 'ceiling'), pool('b', 'waiting')];
  const n = rentingNotice(
    { ...base, state: 'no_ceiling', fix: 'Set a ceiling.', suggested_max_machines: 2, pools },
    true,
  );
  assert.equal(n?.title, 'Renting is on, but the fleet may rent no machines');
  assert.equal(n?.fix, 'Set a ceiling.');
  assert.deepEqual(
    n?.pools.map((p) => p.pool_id),
    ['a'],
  );
  assert.equal(n?.action?.label, 'Allow 2 machines');
  assert.deepEqual(settingsFor(n!, 2), { 'provider.max_machines': 2 });
  assert.equal(
    rentingNotice({ ...base, state: 'no_ceiling' }, false)?.hint,
    'An administrator can change this under Settings, Machines the fleet may rent.',
  );
});

test('a suggestion of one machine is worded in the singular', () => {
  const n = rentingNotice({ ...base, state: 'no_ceiling', suggested_max_machines: 1 }, true);
  assert.equal(n?.action?.label, 'Allow 1 machine');
});

test('a missing suggestion falls back to one machine', () => {
  const n = rentingNotice({ ...base, state: 'no_ceiling' }, true);
  assert.equal(n?.action?.ceiling, 1);
  assert.equal(n?.action?.label, 'Allow 1 machine');
});

test('there is nothing to say when nothing is wrong', () => {
  for (const state of ['none', 'on'] as const)
    assert.equal(rentingNotice({ ...base, state }, true), null);
  assert.equal(rentingNotice(null, true), null);
});

test('only pools renting cannot help are listed', () => {
  const pools = [
    pool('a', 'off'),
    pool('b', 'ceiling'),
    pool('c', 'waiting'),
    pool('d', 'renting'),
    pool('e', 'held'),
  ];
  const n = rentingNotice({ ...base, state: 'off', pools }, true);
  assert.deepEqual(
    n?.pools.map((p) => p.pool_id),
    ['a', 'b'],
  );
});

test('a ceiling is a whole number of at least one', () => {
  assert.equal(clampCeiling(0, 2), 2);
  assert.equal(clampCeiling(Number.NaN, 2), 2);
  assert.equal(clampCeiling(3.9, 2), 3);
  assert.equal(clampCeiling(Infinity, 2), 2);
  assert.equal(clampCeiling(-3, 2), 2);
  // A ceiling above 100000 is refused to the fallback: no real fleet rents that
  // many machines, so a figure that large is a mistake, not a wish.
  assert.equal(clampCeiling(1e21, 2), 2);
  assert.equal(clampCeiling(100_000, 2), 100_000);
});

test('a pool is found by its id, and a missing view finds nothing', () => {
  const renting = { ...base, pools: [pool('a', 'off')] };
  assert.equal(poolRenting(renting, 'a')?.state, 'off');
  assert.equal(poolRenting(renting, 'z'), null);
  assert.equal(poolRenting(null, 'a'), null);
});

/*
 * The settings route refuses an environment-pinned key with a general message
 * and the reason under the key. The reason is the one that says where the
 * setting lives, so it is what the notice shows.
 */
test("a refusal shows the server's per-key reasons, and its message when there are none", () => {
  const pinned = 'ZOOMIES_PROVIDER_ENABLED is setting this in the environment.';
  assert.equal(
    refusalText('these settings could not be changed', [
      { field: 'provider.enabled', message: pinned },
    ]),
    pinned,
  );
  assert.equal(refusalText('that was refused', []), 'that was refused');
  assert.equal(refusalText('that was refused', undefined), 'that was refused');
});
