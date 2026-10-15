import { test } from 'node:test';
import assert from 'node:assert/strict';
import { heldLink, heldLinkMessage } from '../src/lib/repairs/identity.ts';
import type { Schemas, User } from '../src/lib/api/types.ts';

/*
  The held-link branch of the link form: which refusals it answers, whose row
  it names, and the sentence it shows. Anything else keeps the toast, and a
  refusal nobody can act on is not turned into a move button by accident.
*/

const held = (detail = 'usr_old') => ({ code: 'assistant.identity_held', detail });

const row = (over: Partial<Schemas['EliIdentity']> = {}): Schemas['EliIdentity'] => ({
  user_id: 'usr_old',
  github_user_id: 42,
  github_login: 'octo',
  confirmed: true,
  ...over,
});

const users: User[] = [
  { id: 'usr_old', username: 'previous-owner' },
  { id: 'usr_new', username: 'fresh-account' },
];

test('only the held-link refusal becomes a move offer', () => {
  assert.equal(heldLink({ code: 'conflict', detail: 'usr_old' }, 'octo', [row()], users), null);
  assert.equal(heldLink({ code: 'assistant.provider_not_yours' }, 'octo', [row()], users), null);
  assert.equal(heldLink(held(), 'octo', [row()], users)?.holderId, 'usr_old');
});

test('the refusal carries the holder, and the rows only agree with it', () => {
  // The user list was drawn before the link landed where it now sits: the
  // refusal, read as this call's truth, wins over a stale row.
  const moved = heldLink(held('usr_new'), 'octo', [row()], users);
  assert.equal(moved?.holderId, 'usr_new');
  assert.equal(moved?.user?.username, 'fresh-account');
  assert.equal(heldLink(held('usr_old'), 'Octo', [row()], users)?.holderId, 'usr_old');
  // The refusal is the truth about the row: a holder the page's list has no
  // name for stays the holder, and the message falls back instead of
  // pointing at the typed row only.
  const unknown = heldLink(held('usr_other'), 'octo', [row()], users);
  assert.equal(unknown?.holderId, 'usr_other');
  assert.equal(unknown?.user, null);
});

test('a page whose records predate the row names the holder by the detail alone', () => {
  assert.equal(heldLink(held('usr_old'), 'octo', [], users)?.holderId, 'usr_old');
  assert.equal(heldLink(held('usr_old'), 'octo', [], users)?.user?.username, 'previous-owner');
});

test('a refusal with nothing to name keeps its move button and loses its at-sign', () => {
  const unknown = heldLink({ code: 'assistant.identity_held' }, 'octo', [], users);
  assert.equal(unknown?.holderId, '');
  assert.equal(unknown?.user, null);
  const said = heldLinkMessage({ holderId: '', user: null });
  assert.match(said, /another Zoomies user/);
  assert.match(said, /Move the link/);
});

test('the sentence names the holder by username, then id', () => {
  assert.match(
    heldLinkMessage({ holderId: 'usr_old', user: users.find((u) => u.id === 'usr_old') }),
    /@previous-owner already holds/,
  );
  assert.match(heldLinkMessage({ holderId: 'usr_gone', user: null }), /@usr_gone already holds/);
});
