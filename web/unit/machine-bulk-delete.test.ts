/**
 * Clearing out a pile of machines at once: which of them can be ticked, what
 * the confirmation says it is about to do, and what to make of the answer.
 *
 * Pure, for the reason `machines.ts` is. The sentence "this destroys three VMs"
 * is the one that has to be right, because the operator reads it to decide
 * whether to type the count, and it is checkable here without a browser.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { MACHINE_STATES } from '../src/lib/api/types.ts';
import type { Machine } from '../src/lib/api/types.ts';
import {
  cannotDeleteBecause,
  deletable,
  deletionWords,
  outcomeWords,
  planDeletion,
  readOutcome,
} from '../src/lib/providers/bulkDelete.ts';

function machine(fields: Partial<Machine> = {}): Machine {
  return {
    id: 'mach_k3f9qz2mx7ab',
    provider_id: 'prv_k3f9qz2mx7ab',
    name: 'zoomies-mach-k3f9q',
    state: 'failed',
    labels: {},
    safe_to_delete: false,
    timeline: [],
    created_at: '2026-09-14T11:00:00Z',
    updated_at: '2026-09-14T11:30:00Z',
    ...fields,
  };
}

test('a machine can be ticked when the controller would take a delete for it, and the rest say why', () => {
  // The store's allow-list is what decides, and a row the controller will only
  // refuse is a row that should not invite a click: planned has nothing to
  // delete yet, deleting and deleted are already on their way or done, and a
  // quarantined one is never deleted from here, forced or not.
  const ticked = MACHINE_STATES.filter((state) => cannotDeleteBecause(machine({ state })) === '');
  assert.deepEqual(ticked, [
    'creating',
    'starting',
    'bootstrapping',
    'enrolling',
    'ready',
    'draining',
    'failed',
  ]);

  assert.match(cannotDeleteBecause(machine({ state: 'planned' })), /not created yet/i);
  assert.match(cannotDeleteBecause(machine({ state: 'deleting' })), /already being deleted/i);
  assert.match(cannotDeleteBecause(machine({ state: 'deleted' })), /already deleted/i);
  assert.match(
    cannotDeleteBecause(machine({ state: 'quarantined' })),
    /release/i,
    'the reason points at the one thing that can be done with it',
  );
});

test('a machine whose state is not known yet is not offered for deletion', () => {
  // A row the API sent without a state is a row nobody has checked the rules
  // against, and the safe answer to "may this be destroyed?" is no.
  assert.notEqual(cannotDeleteBecause(machine({ state: undefined })), '');
});

test('deletable keeps the machines that can be ticked, in the order they were listed', () => {
  const rows = [
    machine({ id: 'a', state: 'failed' }),
    machine({ id: 'b', state: 'quarantined' }),
    machine({ id: 'c', state: 'ready' }),
    machine({ id: 'd', state: 'deleting' }),
    machine({ id: 'e', state: 'planned' }),
  ];
  assert.deepEqual(
    deletable(rows).map((m) => m.id),
    ['a', 'c'],
  );
  assert.deepEqual(deletable([]), []);
});

test('a machine that never got a VM costs nothing to delete, and one that has a VM destroys it', () => {
  // This is the line between a confirmation that is a click and one that is
  // typed, so it is the thing to get right: a failed machine with a resource
  // behind it is still a VM, and a confirmed-gone one is not.
  assert.deepEqual(
    planDeletion([
      machine({ id: 'a', state: 'failed' }),
      machine({ id: 'b', state: 'failed', resource_id: '143' }),
      machine({ id: 'c', state: 'ready', resource_id: '144' }),
    ]),
    { total: 3, withVM: 2, withoutVM: 1 },
  );
  assert.deepEqual(
    planDeletion([
      machine({ state: 'failed', resource_id: '143', deleted_at: '2026-09-14T11:45:00Z' }),
    ]),
    { total: 1, withVM: 0, withoutVM: 1 },
    'a resource the provider has confirmed gone is not one to destroy',
  );
  assert.deepEqual(planDeletion([]), { total: 0, withVM: 0, withoutVM: 0 });
});

test('a pile of machines with nothing behind them is confirmed with a click, and says nothing is touched', () => {
  const words = deletionWords({ total: 4, withVM: 0, withoutVM: 4 }, false);
  assert.equal(words.title, 'Delete 4 machines');
  assert.equal(words.confirmWord, null, 'nothing is destroyed, so nothing is typed');
  assert.match(words.description, /never got as far as a VM/);
  assert.match(words.description, /Nothing at the provider is touched/);
});

test('machines with a VM are confirmed by typing how many there are, and the sentence counts the VMs', () => {
  const words = deletionWords({ total: 5, withVM: 3, withoutVM: 2 }, false);
  assert.equal(words.title, 'Delete 5 machines');
  assert.equal(words.confirmWord, '5', 'the count of machines selected, not of VMs');
  assert.match(words.description, /3 VMs are destroyed at the provider/);
  assert.match(words.description, /2 never got as far as a VM/);

  const one = deletionWords({ total: 1, withVM: 1, withoutVM: 0 }, false);
  assert.equal(one.title, 'Delete 1 machine');
  assert.match(one.description, /1 VM is destroyed at the provider/);
  assert.doesNotMatch(
    one.description,
    /never got as far/,
    'nothing to say about the ones that are not there',
  );
});

test('the confirmation says what happens to a machine that still has runners, according to the box', () => {
  const plan = { total: 2, withVM: 2, withoutVM: 0 };
  const skipped = deletionWords(plan, false).consequences.join(' ');
  assert.match(skipped, /still has runners on it is skipped/);
  assert.doesNotMatch(skipped, /job .* fails/);

  const forced = deletionWords(plan, true).consequences.join(' ');
  assert.match(forced, /Runners on them stop/);
  assert.match(forced, /job they are running fails/);
  assert.doesNotMatch(forced, /is skipped/);
});

test('the confirmation never uses a dash where a comma would do', () => {
  // The house voice has no em dashes anywhere, and the UI is where a stray one
  // is most visible.
  for (const plan of [
    { total: 4, withVM: 0, withoutVM: 4 },
    { total: 5, withVM: 3, withoutVM: 2 },
  ]) {
    for (const force of [false, true]) {
      const words = deletionWords(plan, force);
      const all = [words.title, words.description, words.confirmLabel, ...words.consequences];
      for (const text of all) assert.doesNotMatch(text, /\u2014| -- /, text);
    }
  }
});

test('the answer is read machine by machine, and a machine nobody answered for is not taken as deleted', () => {
  const outcome = readOutcome(['a', 'b', 'c', 'd'], {
    results: [
      { id: 'a', ok: true },
      { id: 'b', ok: false, error: 'machine b is quarantined: nothing has proved it ours' },
      { id: 'c', ok: true },
    ],
  });
  assert.deepEqual(outcome.deleted, ['a', 'c']);
  assert.deepEqual([...outcome.refused.keys()], ['b', 'd'], 'in the order they were asked');
  assert.match(outcome.refused.get('b') ?? '', /quarantined/);
  assert.match(
    outcome.refused.get('d') ?? '',
    /did not answer/,
    'silence is a refusal, because the alternative is telling an operator a VM is going when it is not',
  );
});

test('a refusal with no words still says that it was a refusal', () => {
  const outcome = readOutcome(['a'], { results: [{ id: 'a', ok: false }] });
  assert.ok((outcome.refused.get('a') ?? '') !== '');
});

test('an answer with nothing in it refuses every machine rather than deleting none quietly', () => {
  const outcome = readOutcome(['a', 'b'], {});
  assert.deepEqual(outcome.deleted, []);
  assert.equal(outcome.refused.size, 2);
});

test('the toast says how it went in the tone that matches', () => {
  const all = outcomeWords(3, 0);
  assert.equal(all.tone, 'success');
  assert.equal(all.title, 'Deleting 3 machines');
  assert.match(all.message, /not gone until the provider cannot find/);

  const some = outcomeWords(2, 1);
  assert.equal(some.tone, 'warning', 'a partial failure is a warning, announced as one');
  assert.equal(some.title, 'Deleting 2 machines, 1 refused');
  assert.match(some.message, /stays selected/);

  const none = outcomeWords(0, 2);
  assert.equal(none.tone, 'error');
  assert.equal(none.title, 'No machine was deleted');
  assert.match(none.message, /2 were refused/);

  assert.equal(outcomeWords(1, 0).title, 'Deleting 1 machine');
  assert.match(outcomeWords(2, 2).message, /refused ones stay selected/);
});
