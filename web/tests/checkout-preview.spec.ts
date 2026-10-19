import { expect, test } from '@playwright/test';
import { goto } from './support/fixtures';

const source =
  'name: <img src=x onerror=alert(1)>\njobs:\n  build:\n    steps:\n      - uses: actions/checkout@' +
  'b'.repeat(40) +
  '\n        with:\n          fetch-depth: 0\n';
const inspection = {
  installation_id: 'ins_fixture',
  repository: 'acme/repo',
  selection: {
    run_id: '9007199254740993',
    attempt: 2,
    github_job_id: '100',
    job_index: 3,
    step_index: 1,
  },
  job_name: 'Build',
  step_name: 'Checkout',
  head_sha: 'a'.repeat(40),
  path: '.github/workflows/ci.yml',
  mode: '100644',
  content: source,
  source_sha256: 'c'.repeat(64),
  step_number: 42,
  checkout_ms: 4200,
  evidence_id: 'checkout:10:2:100:42',
  basis: 'push_source_unique_literal_names',
  inspected_at: '2026-10-19T09:00:00Z',
};
const retained = {
  id: 'opv_fixture',
  installation_id: 'ins_fixture',
  repository: 'acme/repo',
  reviewed_by: 'usr_operator',
  sha256: 'd'.repeat(64),
  created_at: '2026-10-19T09:01:00Z',
  currentness: 'not_checked',
  collection: {
    version: 'checkout-evidence-v1',
    basis: 'push_source_unique_literal_names',
    step_number: 42,
    blob_sha: 'e'.repeat(40),
    source_funding: 'installation_github_read',
  },
  preview: {
    version: 'checkout-preview-v1',
    kind: 'checkout_depth',
    state: 'preview_ready',
    head_sha: 'a'.repeat(40),
    source_sha256: 'c'.repeat(64),
    after_sha256: 'f'.repeat(64),
    evidence_id: 'checkout:10:2:100:42',
    run_id: '9007199254740993',
    attempt: 2,
    job_index: 0,
    step_index: 0,
    line: 8,
    checkout_ms: 4200,
    path: '.github/workflows/ci.yml',
    before: '          fetch-depth: 0\n',
    after: '          fetch-depth: 1\n',
    verification: 'inconclusive',
    explanation: 'Reduce this checkout to one commit. Savings are unverified.',
  },
};

const enablement = (over: object = {}) => ({
  source_reads: true,
  workflow_edits: true,
  enabled: true,
  can_change: true,
  ...over,
});
async function previewsOn(page: import('@playwright/test').Page) {
  await page.route('**/api/v1/optimisations/enablement', (route) =>
    route.fulfill({ json: enablement() }),
  );
}

const job = {
  id: 'job_1',
  github_job_id: 100,
  github_run_id: 9007199254740,
  repo: 'acme/repo',
  workflow: 'CI',
  job_name: 'Build',
  labels: [],
  state: 'completed',
  conclusion: 'success',
  installation_id: 'ins_fixture',
  head_branch: 'main',
  run_attempt: 2,
  run_event: 'push',
  completed_at: '2026-10-19T09:00:00Z',
  matched: true,
  hosted: false,
  provisioning: '',
  provision_now: false,
};
async function listRuns(page: import('@playwright/test').Page) {
  await page.route('**/api/v1/jobs/facets', (route) =>
    route.fulfill({ json: { repos: ['acme/repo'], workflows: [], pools: [], branches: [] } }),
  );
  await page.route('**/api/v1/jobs?*', (route) =>
    route.fulfill({
      json: { items: [job, { ...job, id: 'job_2', run_event: 'pull_request' }], total: 2 },
    }),
  );
}
async function chooseRun(page: import('@playwright/test').Page) {
  await page.getByRole('combobox', { name: 'Repository' }).selectOption('acme/repo');
  await page.getByRole('row', { name: /Build/ }).getByRole('button', { name: 'Review' }).click();
}

test('pick a run, review the source, declare, keep: no IDs typed, the server finds the step', async ({
  page,
}) => {
  await previewsOn(page);
  await listRuns(page);
  let inspectBody: { selection: Record<string, unknown> } | undefined;
  let retainBody: Record<string, unknown> | undefined;
  await page.route('**/api/v1/optimisations/checkout/inspections', async (route) => {
    inspectBody = route.request().postDataJSON();
    await route.fulfill({ json: inspection });
  });
  await page.route('**/api/v1/optimisations/checkout/previews', async (route) => {
    retainBody = route.request().postDataJSON();
    await route.fulfill({ status: 201, json: retained });
  });
  await goto(page, '/optimisations/checkout', 'Checkout preview');
  // Only the push-started job is offered; the pull-request one is not.
  await page.getByRole('combobox', { name: 'Repository' }).selectOption('acme/repo');
  await expect(page.getByRole('row', { name: /Build/ })).toHaveCount(1);
  await page.getByRole('row', { name: /Build/ }).getByRole('button', { name: 'Review' }).click();
  await expect(page.getByRole('heading', { name: '2. Review the source' })).toBeVisible();
  await expect(page.getByText('Checkout', { exact: true }).first()).toBeVisible();
  expect(inspectBody?.selection).toEqual({
    run_id: '9007199254740',
    attempt: 2,
    github_job_id: '100',
  });
  const code = page.getByLabel('Workflow source, untrusted text');
  await expect(code).toContainText('<img src=x onerror=alert(1)>');
  expect(await page.locator('main img').count()).toBe(0);

  const keep = page.getByRole('button', { name: 'Keep a preview for 14 days' });
  await expect(keep).toBeDisabled();
  await page.getByLabel(/I reviewed this file/).check();
  await keep.click();
  await expect(page.getByRole('heading', { name: 'Retained preview', exact: true })).toBeVisible();
  // The positions the server found and the reviewer was shown are what is kept.
  expect(retainBody?.selection).toEqual(inspection.selection);
  expect(retainBody?.history_reviewed).toBe(true);
  expect(retainBody?.source_sha256).toBe('c'.repeat(64));
  expect(retainBody?.head_sha).toBe('a'.repeat(40));
  await expect(page.getByText('Nothing was published.')).toBeVisible();
  await expect(page.getByText(/no measurement shows this change helps/)).toBeVisible();
  await expect(page.getByLabel('Workflow source, untrusted text')).toHaveCount(0);
});

test('a link from a job prefills the run and reads nothing until it is pressed', async ({
  page,
}) => {
  await previewsOn(page);
  await listRuns(page);
  let reads = 0;
  await page.route('**/api/v1/optimisations/checkout/inspections', async (route) => {
    reads++;
    await route.fulfill({ json: inspection });
  });
  await goto(
    page,
    '/optimisations/checkout?installation_id=ins_fixture&repo=acme%2Frepo&run_id=9007199254740993&attempt=2&job_id=100',
    'Checkout preview',
  );
  await expect(page.getByText(/Ready to read/)).toBeVisible();
  expect(reads).toBe(0);
  await page.getByRole('button', { name: 'Read this workflow' }).click();
  await expect(page.getByRole('heading', { name: '2. Review the source' })).toBeVisible();
  expect(reads).toBe(1);
});

test('a refusal names the setting an administrator must turn on', async ({ page }) => {
  await previewsOn(page);
  await listRuns(page);
  await page.route('**/api/v1/optimisations/checkout/inspections', (route) =>
    route.fulfill({
      status: 403,
      json: {
        error: {
          code: 'forbidden',
          message:
            'Reviewed workflow optimisations are off. An administrator turns on optimisation.source_reads and optimisation.workflow_edits in Settings.',
        },
      },
    }),
  );
  await goto(page, '/optimisations/checkout', 'Checkout preview');
  await chooseRun(page);
  await expect(page.getByRole('alert')).toContainText('optimisation.source_reads');
});

test('the manual entry takes only a run, an attempt and a job, with exact IDs as strings', async ({
  page,
}) => {
  await previewsOn(page);
  await listRuns(page);
  let body: { selection: Record<string, unknown> } | undefined;
  await page.route('**/api/v1/optimisations/checkout/inspections', async (route) => {
    body = route.request().postDataJSON();
    await route.fulfill({ json: inspection });
  });
  await goto(page, '/optimisations/checkout', 'Checkout preview');
  await page.getByText('Enter a run by its IDs instead').click();
  const form = page.locator('details', { hasText: 'Enter a run by its IDs instead' });
  await form.getByLabel('Installation ID').fill('ins_fixture');
  await form.getByLabel('Repository').fill('acme/repo');
  await form.getByLabel(/^Run ID/).fill('9007199254740993');
  await form.getByLabel('Run attempt').fill('2');
  await form.getByLabel(/^GitHub job ID/).fill('100');
  await form.getByRole('button', { name: 'Read workflow source' }).click();
  await expect(page.getByRole('heading', { name: '2. Review the source' })).toBeVisible();
  expect(body?.selection).toEqual({ run_id: '9007199254740993', attempt: 2, github_job_id: '100' });
});

test('an administrator turns the previews on in one press, from the page that needs them', async ({
  page,
}) => {
  await listRuns(page);
  let on = false;
  let put: unknown;
  await page.route('**/api/v1/optimisations/enablement', async (route) => {
    if (route.request().method() === 'PUT') {
      put = route.request().postDataJSON();
      on = true;
    }
    await route.fulfill({
      json: on
        ? enablement()
        : enablement({ source_reads: false, workflow_edits: false, enabled: false }),
    });
  });
  await goto(page, '/optimisations/checkout', 'Checkout preview');
  await page.getByRole('combobox', { name: 'Repository' }).selectOption('acme/repo');
  const review = page.getByRole('row', { name: /Build/ }).getByRole('button', { name: 'Review' });
  await expect(review).toBeDisabled();
  await expect(page.getByText('no model sees the file', { exact: false }).first()).toBeVisible();
  await page.getByRole('button', { name: 'Turn on workflow previews' }).click();
  await expect(page.getByRole('heading', { name: 'Turn on workflow previews' })).toHaveCount(0);
  expect(put).toEqual({ enabled: true });
  await expect(review).toBeEnabled();
});

test('someone who cannot turn them on is told whom to ask and offered the words', async ({
  page,
}) => {
  await listRuns(page);
  await page.route('**/api/v1/optimisations/enablement', (route) =>
    route.fulfill({
      json: enablement({
        source_reads: false,
        workflow_edits: false,
        enabled: false,
        can_change: false,
        reason: 'Only an administrator can turn this on. Ask one to press the button on this page.',
      }),
    }),
  );
  await goto(page, '/optimisations/checkout', 'Checkout preview');
  await expect(page.getByText('Only an administrator can turn this on.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Turn on workflow previews' })).toHaveCount(0);
  await expect(
    page.getByRole('button', { name: /Copy a request for an administrator/ }),
  ).toBeVisible();
});

test('a kept preview opens from its link without any GitHub request and the page fits a phone', async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 700 });
  await previewsOn(page);
  await listRuns(page);
  await page.route('**/api/v1/optimisations/checkout/previews/*?*', (route) =>
    route.fulfill({ json: retained }),
  );
  await goto(
    page,
    '/optimisations/checkout?id=opv_fixture&installation_id=ins_fixture&repo=acme%2Frepo',
    'Checkout preview',
  );
  await expect(page.getByRole('heading', { name: 'Retained preview', exact: true })).toBeVisible();
  await expect(page.getByText('fetch-depth: 1')).toBeVisible();
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
  expect(overflow).toBe(false);
});
