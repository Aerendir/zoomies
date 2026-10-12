import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  NO_REPAIR_FILTERS,
  filtered,
  isActive,
  listQuery,
  offsetAfterClear,
  selectionQuery,
  stateLabel,
} from '../src/lib/repairs/history';

// The server treats a missing parameter as "no filter" and a spelled-out
// default as a value it must recognise, so the page leaves the defaults out
// rather than send `group=all` and depend on the server agreeing about it.
test('the list query leaves out the defaults and trims the repository', () => {
  assert.deepEqual(listQuery({ group: 'all', repo: ' site ', trigger: 'any' }, 50, 0), {
    repo: 'site',
    limit: 50,
    offset: 0,
  });
  assert.deepEqual(listQuery(NO_REPAIR_FILTERS, 25, 75), { limit: 25, offset: 75 });
  assert.deepEqual(listQuery({ group: 'attention', repo: '', trigger: 'automatic' }, 100, 100), {
    group: 'attention',
    trigger: 'automatic',
    limit: 100,
    offset: 100,
  });
});

// A box holding only spaces is not a filter; sending repo=%20%20 would match
// nothing and leave an operator looking at an empty history with no idea why.
test('a repository box holding only spaces is not sent', () => {
  assert.deepEqual(listQuery({ group: 'all', repo: '   ', trigger: 'any' }, 50, 0), {
    limit: 50,
    offset: 0,
  });
});

// The selection endpoint takes the same filters as the list and nothing about
// paging, because a select-all means everything the filters match.
test('the selection query carries the filters and no paging', () => {
  assert.deepEqual(selectionQuery(NO_REPAIR_FILTERS), {});
  assert.deepEqual(selectionQuery({ group: 'passed', repo: ' widgets', trigger: 'requested' }), {
    group: 'passed',
    repo: 'widgets',
    trigger: 'requested',
  });
});

// "Filtered" decides between "No repairs yet" and "No repairs match these
// filters", which are different facts that an operator acts on differently.
test('a filter counts as set when any of the three is away from its default', () => {
  assert.equal(filtered(NO_REPAIR_FILTERS), false);
  assert.equal(filtered({ ...NO_REPAIR_FILTERS, group: 'active' }), true);
  assert.equal(filtered({ ...NO_REPAIR_FILTERS, repo: 'site' }), true);
  assert.equal(filtered({ ...NO_REPAIR_FILTERS, trigger: 'requested' }), true);
  assert.equal(filtered({ ...NO_REPAIR_FILTERS, repo: '  ' }), false);
});

// These are the states a repair cannot be cleared or retried from, so the set
// must match the server's idea of in progress exactly.
test('a repair is in progress in exactly the queued, working and checking states', () => {
  for (const state of ['queued', 'working', 'checking']) assert.equal(isActive(state), true);
  for (const state of [
    'succeeded',
    'checks_failed',
    'interrupted',
    'unverified',
    'superseded',
    'blocked',
    'failed',
    '',
    'something_new',
  ])
    assert.equal(isActive(state), false, state);
});

test('a state reads as a phrase and an unknown one comes back as it was given', () => {
  assert.equal(stateLabel('checks_failed'), 'Checks failed');
  assert.equal(stateLabel('working'), 'Investigating');
  assert.equal(stateLabel('a_state_from_a_newer_controller'), 'a_state_from_a_newer_controller');
});

// After a clear the page the operator was on may no longer exist. Stepping
// back to the last page that still has rows keeps them looking at repairs
// rather than at an empty list beside a total that says there are some.
test('after a clear the offset steps back only when its page would be empty', () => {
  // 102 matched, 1 cleared: 101 remain, and the page starting at 100 still has a row.
  assert.equal(offsetAfterClear(100, 50, 102, 1), 100);
  // 101 matched, 1 cleared: 100 remain, so the page starting at 100 is empty.
  assert.equal(offsetAfterClear(100, 50, 101, 1), 50);
  assert.equal(offsetAfterClear(100, 50, 101, 2), 50);
  // Everything cleared: the first page, never a negative offset.
  assert.equal(offsetAfterClear(0, 50, 3, 3), 0);
  assert.equal(offsetAfterClear(50, 50, 51, 51), 0);
  // Clearing more than the total says is as good as clearing all of it.
  assert.equal(offsetAfterClear(100, 50, 101, 500), 0);
});
