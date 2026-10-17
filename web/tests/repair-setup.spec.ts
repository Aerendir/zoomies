import { expect, test, type Page } from '@playwright/test';
import type { Schemas } from '../src/lib/api/types';

const repo = 'acme/widgets';
const identity: Schemas['EliIdentity'] = {
  user_id: 'user-1',
  github_user_id: 42,
  github_login: 'octocat',
  confirmed: true,
};
const ready: Schemas['EliReadiness'] = {
  ready: true,
  repo,
  pull_number: 17,
  head_sha: 'reviewed-head',
  provider: 'My provider',
  model: 'my-model',
  allow_workflows: false,
  remaining: 5,
  blockers: [],
};

async function mockFailure(page: Page, role: 'admin' | 'operator' = 'admin') {
  page.on('pageerror', (error) => {
    throw error;
  });
  // Keep unrelated shell requests quiet; this suite exercises the repair UI's
  // request contracts, including the writes that must never happen on a read.
  await page.route('**/api/v1/**', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/v1/assistant/repairs/repositories?*', (route) =>
    route.fulfill({
      json: {
        items: [repo],
        total: 1,
        missing_permissions: [],
        capped: false,
      },
    }),
  );
  await page.route('**/api/v1/meta', (route) =>
    route.fulfill({
      json: {
        auth_disabled: false,
        bootstrap_required: false,
        version: 'dev',
      },
    }),
  );
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill({
      json: {
        kind: 'session',
        id: 'user-1',
        name: 'Operator',
        role,
      },
    }),
  );
  await page.route('**/api/v1/installations', (route) =>
    route.fulfill({ json: { items: [{ id: 'inst-1', target: 'acme', target_type: 'org' }] } }),
  );
  await page.route('**/api/v1/assistant/repairs/settings', (route) =>
    route.fulfill({
      json: {
        identities: [identity],
        policies: [],
      },
    }),
  );
  const checks: URLSearchParams[] = [];
  const writes: unknown[] = [];
  let enabled = false;
  await page.route('**/api/v1/assistant/repairs/readiness?*', (route) => {
    checks.push(new URL(route.request().url()).searchParams);
    return route.fulfill({
      json: enabled
        ? ready
        : {
            ...ready,
            ready: false,
            blockers: [{ code: 'policy', message: 'Enable repository repairs.' }],
          },
    });
  });
  await page.route('**/api/v1/assistant/repairs/policy', async (route) => {
    const body = route.request().postDataJSON();
    writes.push(body);
    enabled = true;
    await route.fulfill({ json: body });
  });
  const repairs: unknown[] = [];
  await page.route(/\/api\/v1\/assistant\/repairs$/, async (route) => {
    if (route.request().method() !== 'POST') return route.fulfill({ json: { items: [] } });
    repairs.push(route.request().postDataJSON());
    await route.fulfill({ json: { id: 'repair-1' } });
  });
  return { checks, writes, repairs };
}

test('failure setup enables only its repository, rechecks the run and repairs the reviewed head', async ({
  page,
}) => {
  const model = await mockFailure(page);
  await page.goto(`/repairs?repo=${repo}&run=1234`);
  const setup = page.getByRole('region', { name: 'Set up Eli for this failure' });
  await expect(setup).toBeVisible();
  await expect(setup.getByLabel('GitHub installation')).toHaveValue('inst-1');
  expect(model.writes).toEqual([]);
  expect(model.repairs).toEqual([]);
  await setup.getByRole('button', { name: 'Save setup and check this failure' }).click();
  await expect(page.getByRole('button', { name: 'Fix with Eli', exact: true })).toBeVisible();
  expect(model.writes).toEqual([
    {
      repo,
      installation_id: 'inst-1',
      provider_id: '',
      enabled: true,
      automatic: false,
      ask_first: false,
      allow_workflows: false,
      daily_limit: 5,
    },
  ]);
  expect(model.checks.length).toBeGreaterThanOrEqual(2);
  for (const check of model.checks) {
    expect(check.get('repo')).toBe(repo);
    expect(check.get('run_id')).toBe('1234');
  }
  expect(model.repairs).toEqual([]);
  await page.getByRole('button', { name: 'Fix with Eli', exact: true }).click();
  await expect(
    page.getByText('Repair queued. Follow its progress in recent repairs below.'),
  ).toBeVisible();
  expect(model.repairs).toEqual([
    { repo, pull_number: 17, head_sha: 'reviewed-head', instruction: '' },
  ]);
});

test('an operator gets an actionable setup explanation without repository writes', async ({
  page,
}) => {
  const model = await mockFailure(page, 'operator');
  await page.goto(`/repairs?repo=${repo}&run=1234`);
  const setup = page.getByRole('region', { name: 'Set up Eli for this failure' });
  await expect(
    setup.getByText(`An administrator needs to enable repairs for ${repo}.`),
  ).toBeVisible();
  await expect(
    setup.getByRole('button', { name: 'Save setup and check this failure' }),
  ).toBeDisabled();
  expect(model.writes).toEqual([]);
});

test('a run without an eligible PR explains the limit and never offers to repair', async ({
  page,
}) => {
  const model = await mockFailure(page);
  await page.route('**/api/v1/assistant/repairs/readiness?*', (route) =>
    route.fulfill({
      json: {
        ...ready,
        ready: false,
        pull_number: 0,
        blockers: [{ code: 'pull', message: 'No eligible PR.' }],
      },
    }),
  );
  await page.goto(`/repairs?repo=${repo}&run=1234`);
  await expect(
    page.getByText('Eli repairs open PR branches in this repository.', { exact: false }),
  ).toBeVisible();
  await expect(page.getByRole('button', { name: 'Fix with Eli', exact: true })).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Set up Eli for this failure' })).toHaveCount(0);
  expect(model.repairs).toEqual([]);
});

test('setup asks only for a missing GitHub username, then links and confirms it', async ({
  page,
}) => {
  const model = await mockFailure(page);
  await page.route('**/api/v1/assistant/repairs/settings', (route) =>
    route.fulfill({
      json: {
        identities: [],
        policies: [],
      },
    }),
  );
  const links: unknown[] = [];
  const consents: unknown[] = [];
  await page.route('**/api/v1/assistant/repairs/identity', (route) => {
    links.push(route.request().postDataJSON());
    return route.fulfill({ json: { ...identity, confirmed: false } });
  });
  await page.route('**/api/v1/assistant/repairs/consent', (route) => {
    consents.push(route.request().postDataJSON());
    return route.fulfill({ status: 204 });
  });
  await page.goto(`/repairs?repo=${repo}&run=1234`);
  const setup = page.getByRole('region', { name: 'Set up Eli for this failure' });
  const save = setup.getByRole('button', { name: 'Save setup and check this failure' });
  await expect(save).toBeDisabled();
  await setup.getByLabel('Your GitHub username').fill('octocat');
  await save.click();
  await expect(page.getByRole('button', { name: 'Fix with Eli', exact: true })).toBeVisible();
  expect(links).toEqual([
    { user_id: 'user-1', github_login: 'octocat', installation_id: 'inst-1', repo },
  ]);
  expect(consents).toEqual([{ github_user_id: 42, enabled: true }]);
  expect(model.repairs).toEqual([]);
});

test('a refused setup keeps the failure context and offers a retry without starting repair', async ({
  page,
}) => {
  const model = await mockFailure(page);
  await page.route('**/api/v1/assistant/repairs/policy', (route) =>
    route.fulfill({
      status: 403,
      json: { code: 'forbidden', message: 'Administrator access is required.' },
    }),
  );
  await page.goto(`/repairs?repo=${repo}&run=1234`);
  const setup = page.getByRole('region', { name: 'Set up Eli for this failure' });
  await setup.getByRole('button', { name: 'Save setup and check this failure' }).click();
  await expect(setup.getByRole('alert')).toContainText('You do not have permission to do that.');
  await setup.getByRole('button', { name: 'Reload setup' }).click();
  await expect(
    setup.getByRole('button', { name: 'Save setup and check this failure' }),
  ).toBeEnabled();
  await expect(page).toHaveURL(/repo=acme\/widgets&run=1234/);
  expect(model.repairs).toEqual([]);
});
