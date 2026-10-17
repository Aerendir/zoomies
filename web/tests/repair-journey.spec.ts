/**
 * How the repair card is entered: by pasting a pull request link, and by arriving
 * from a failure.
 *
 * Both used to cost more than they should. Pasting a link rewrote the address
 * with the pull request, and the page keyed the card on the address, so the card
 * that had just started a readiness check was thrown away with the check in
 * flight and a new one asked again. Arriving by run looked for a repair already
 * running by the run's id, which only the repairs Eli queues by itself carry, so
 * a repair a person had queued from this page was offered a second time.
 */
import { expect, test, type Page } from '@playwright/test';
import type { Schemas } from '../src/lib/api/types';

const repo = 'acme/widgets';
const head = 'reviewed-head';
const ready: Schemas['EliReadiness'] = {
  ready: true,
  repo,
  pull_number: 17,
  head_sha: head,
  provider: 'My provider',
  model: 'my-model',
  allow_workflows: false,
  remaining: 5,
  blockers: [],
};

/** A repair a person queued from this page: it knows its pull request and commit, not a run. */
const queuedFromThePage: Schemas['EliRepair'] = {
  id: 'repair-1',
  installation_id: 'inst-1',
  repo,
  head_sha: head,
  commit_sha: '',
  user_id: 'user-1',
  github_login: 'octocat',
  provider_id: 'provider-1',
  trigger: 'mention',
  state: 'working',
  message: '',
  pull_number: 17,
  job_id: 0,
  run_id: 0,
  github_user_id: 42,
  comment_id: 0,
  created_at: '2026-10-17T12:00:00Z',
  updated_at: '2026-10-17T12:01:00Z',
};

async function mockJourney(page: Page, history: Schemas['EliRepair'][] = []) {
  page.on('pageerror', (error) => {
    throw error;
  });
  // Quiet for everything the shell asks that this suite is not about.
  await page.route('**/api/v1/**', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/v1/assistant/repairs/repositories?*', (route) =>
    route.fulfill({
      json: { items: [repo], total: 1, missing_permissions: [], capped: false },
    }),
  );
  await page.route('**/api/v1/meta', (route) =>
    route.fulfill({
      json: { auth_disabled: false, bootstrap_required: false, version: 'dev' },
    }),
  );
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill({ json: { kind: 'session', id: 'user-1', name: 'Operator', role: 'admin' } }),
  );
  await page.route('**/api/v1/assistant/repairs/settings', (route) =>
    route.fulfill({ json: { identities: [], policies: [] } }),
  );
  const checks: URLSearchParams[] = [];
  const histories: URLSearchParams[] = [];
  await page.route('**/api/v1/assistant/repairs/readiness?*', (route) => {
    checks.push(new URL(route.request().url()).searchParams);
    return route.fulfill({ json: ready });
  });
  await page.route(/\/api\/v1\/assistant\/repairs(\?.*)?$/, (route) => {
    const url = new URL(route.request().url());
    if (route.request().method() !== 'GET') return route.fulfill({ json: queuedFromThePage });
    // The card reads twenty rows of a repository's history when it is entered
    // by a failure. The history table below it asks for its own page size.
    if (url.searchParams.get('limit') === '20') histories.push(url.searchParams);
    return route.fulfill({ json: { items: history, total: history.length } });
  });
  return { checks, histories };
}

test('pasting a pull request link checks its readiness once, in the card it was pasted into', async ({
  page,
}) => {
  const model = await mockJourney(page);
  await page.goto('/repairs');
  await page
    .getByRole('textbox', { name: 'Pull request' })
    .fill('https://github.com/acme/widgets/pull/17');
  await page.getByRole('button', { name: 'Check readiness' }).click();
  await expect(page.getByRole('button', { name: 'Fix with Eli', exact: true })).toBeVisible();

  // The address still names the pull request, so a reload or a shared link comes
  // back to it.
  await expect(page).toHaveURL(/repo=acme%2Fwidgets&pull=17/);
  // One question, asked by the card that was pasted into. A second one, and a
  // read of the repository's history before it, is a card that was rebuilt
  // under the first.
  expect(model.checks).toHaveLength(1);
  expect(model.checks[0]?.get('pull_number')).toBe('17');
  expect(model.histories).toEqual([]);
});

test('arriving by run follows the repair a person already queued for that pull request', async ({
  page,
}) => {
  await mockJourney(page, [queuedFromThePage]);
  await page.goto(`/repairs?repo=${repo}&run=1234`);

  await expect(page.getByRole('list', { name: 'What Eli is doing' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Fix with Eli', exact: true })).toHaveCount(0);
});

test('arriving by run still offers a repair when the one running is for another commit', async ({
  page,
}) => {
  await mockJourney(page, [{ ...queuedFromThePage, head_sha: 'an-older-head' }]);
  await page.goto(`/repairs?repo=${repo}&run=1234`);

  await expect(page.getByRole('button', { name: 'Fix with Eli', exact: true })).toBeVisible();
  await expect(page.getByRole('list', { name: 'What Eli is doing' })).toHaveCount(0);
});
