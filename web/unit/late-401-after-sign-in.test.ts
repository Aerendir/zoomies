import assert from 'node:assert/strict';
import { test } from 'node:test';
import { api, noteSignedIn, onUnauthorized } from '../src/lib/api/client.ts';

// A request begun while nobody was signed in can come back 401 after the
// person has finished signing in. That answer is about the old credential, and
// treating it as "the session is gone" threw the fresh identity away and put
// the login form back over a cookie that was perfectly good, so a reload
// showed them signed in.
function unauthorizedAfter(release: Promise<void>): () => Promise<Response> {
  return async () => {
    await release;
    return Response.json({ error: { code: 'unauthorized', message: 'sign in' } }, { status: 401 });
  };
}

test('a 401 for a request begun before sign-in does not end the new session', async () => {
  const saved = globalThis.fetch;
  let hooked = 0;
  onUnauthorized(() => hooked++);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  globalThis.fetch = unauthorizedAfter(gate);
  try {
    const late = api.get('/runners').catch(() => undefined);
    noteSignedIn();
    release();
    await late;
    assert.equal(hooked, 0);
  } finally {
    globalThis.fetch = saved;
  }
});

test('a 401 for a request begun after sign-in still ends the session', async () => {
  const saved = globalThis.fetch;
  let hooked = 0;
  onUnauthorized(() => hooked++);
  globalThis.fetch = unauthorizedAfter(Promise.resolve());
  try {
    noteSignedIn();
    await api.get('/runners').catch(() => undefined);
    assert.equal(hooked, 1);
  } finally {
    globalThis.fetch = saved;
  }
});
