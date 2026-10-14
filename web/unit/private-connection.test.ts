import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  connectionIntro,
  isPrivate,
  lostConnection,
  reconnectSteps,
} from '../src/lib/providers/private-connection.ts';
import type { ProviderCheck } from '../src/lib/api/types.ts';

const check = (reachable: boolean): ProviderCheck =>
  ({ reachable, findings: [] }) as unknown as ProviderCheck;

// The guidance is private-provider business: a direct provider's page has
// nothing to say about gateways, and must never show what it has no need of.
test('only a provider reached through a gateway shows connection guidance', () => {
  assert.equal(isPrivate({ connection: 'tailcat' }), true);
  assert.equal(isPrivate({ connection: 'direct' }), false);
  assert.equal(isPrivate({}), false);
  assert.equal(isPrivate({ connection: undefined }), false);
});

test('a lost connection is what the last check says, and only that', () => {
  assert.equal(lostConnection(check(false)), true);
  assert.equal(lostConnection(check(true)), false);
  // No check yet, or one from a build that kept no report: nothing is lost
  // until something says so.
  assert.equal(lostConnection(null), false);
  assert.equal(lostConnection(undefined), false);
});

test('the steps ask nobody to reconnect and name commands to run elsewhere', () => {
  const steps = reconnectSteps();
  assert.ok(steps.length >= 4);
  for (const step of steps) {
    // Reconnection is automatic at both ends; a step that asked the operator
    // to do it would be the wrong instruction next to a recovery already
    // under way.
    assert.doesNotMatch(step, /reconnect|restart zoomies/i);
  }
  const joined = steps.join(' ');
  // The commands are on the provider's machine, so they arrive as commands a
  // person can run there, not as descriptions of one.
  assert.match(joined, /systemctl status 'zoomies-proxmox-\*'/);
  assert.match(joined, /journalctl -u 'zoomies-proxmox-\*'/);
  // Reinstalling the machine is the case rerunning setup exists for; the
  // stored credential surviving it is the reason to say so.
  assert.match(joined, /reinstalled/);
  assert.match(joined, /credential/);
});

test('the intro says what the connection is without borrowing the error', () => {
  assert.doesNotMatch(connectionIntro, /could not|unreachable|cannot/);
});
