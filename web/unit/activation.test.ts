import { test } from 'node:test';
import assert from 'node:assert/strict';
import { activationActionsUrl, activationRepository } from '../src/lib/overview/activation.ts';

test('activation requires a repository within the selected installation target', () => {
  const org = { target: 'Acme', target_type: 'org' as const };
  assert.equal(activationRepository(' acme/widgets ', org), 'acme/widgets');
  for (const value of [
    'other/widgets',
    'https://github.com/acme/widgets',
    'acme/widgets/actions',
    'acme/..',
    'acme/.',
    'acme/a?x=1',
    '',
  ]) {
    assert.equal(activationRepository(value, org), null);
  }
  assert.equal(
    activationRepository('acme/other', { target: 'acme/widgets', target_type: 'repo' }),
    null,
  );
  assert.equal(
    activationRepository('ACME/widgets', { target: 'acme/widgets', target_type: 'repo' }),
    'ACME/widgets',
  );
});

test('activation uses the configured enterprise web origin and refuses unsafe or unknown origins', () => {
  assert.equal(
    activationActionsUrl('acme/widgets', {}),
    'https://github.com/acme/widgets/actions/workflows/zoomies-test.yml',
  );
  assert.equal(
    activationActionsUrl('acme/widgets', {
      enterprise: true,
      web_url: 'https://git.example.test/',
    }),
    'https://git.example.test/acme/widgets/actions/workflows/zoomies-test.yml',
  );
  for (const web_url of [
    'javascript:alert(1)',
    'http://git.example.test',
    'https://user:secret@git.example.test',
  ]) {
    assert.equal(activationActionsUrl('acme/widgets', { web_url }), null);
  }
  assert.equal(activationActionsUrl('acme/widgets', { enterprise: true }), null);
});
