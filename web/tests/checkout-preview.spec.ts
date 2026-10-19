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
    job_index: 0,
    step_index: 0,
  },
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

async function fill(page: import('@playwright/test').Page) {
  await page.getByLabel('Installation ID').first().fill('ins_fixture');
  await page.getByLabel('Repository').first().fill('acme/repo');
  await page.getByLabel(/^Run ID/).fill('9007199254740993');
  await page.getByLabel('Run attempt').fill('2');
  await page.getByLabel(/^GitHub job ID/).fill('100');
}

test('review comes before retention: source is escaped, the declaration gates the button, exact IDs travel as strings', async ({
  page,
}) => {
  let inspectBody: unknown, retainBody: Record<string, unknown> | undefined;
  await page.route('**/api/v1/optimisations/checkout/inspections', async (route) => {
    inspectBody = route.request().postDataJSON();
    await route.fulfill({ json: inspection });
  });
  await page.route('**/api/v1/optimisations/checkout/previews', async (route) => {
    retainBody = route.request().postDataJSON();
    await route.fulfill({ status: 201, json: retained });
  });
  await goto(page, '/optimisations/checkout', 'Checkout preview');
  await fill(page);
  await page.getByRole('button', { name: 'Read workflow source' }).click();
  await expect(page.getByRole('heading', { name: '2. Review the source' })).toBeVisible();
  const code = page.getByLabel('Workflow source, untrusted text');
  await expect(code).toContainText('<img src=x onerror=alert(1)>');
  expect(await page.locator('main img').count()).toBe(0);
  expect((inspectBody as { selection: { run_id: unknown } }).selection.run_id).toBe(
    '9007199254740993',
  );

  const keep = page.getByRole('button', { name: 'Keep a preview for 14 days' });
  await expect(keep).toBeDisabled();
  await page.getByLabel(/I reviewed this file/).check();
  await keep.click();
  await expect(page.getByRole('heading', { name: 'Retained preview', exact: true })).toBeVisible();
  expect(retainBody?.history_reviewed).toBe(true);
  expect(retainBody?.source_sha256).toBe('c'.repeat(64));
  expect(retainBody?.head_sha).toBe('a'.repeat(40));
  await expect(page.getByText('Nothing was published.')).toBeVisible();
  await expect(page.getByText(/no measurement shows this change helps/)).toBeVisible();
  // The source was for review only; the retained card must not show it.
  await expect(page.getByLabel('Workflow source, untrusted text')).toHaveCount(0);
});

test('editing the selection drops the review so it cannot ride onto other source', async ({
  page,
}) => {
  await page.route('**/api/v1/optimisations/checkout/inspections', (route) =>
    route.fulfill({ json: inspection }),
  );
  await goto(page, '/optimisations/checkout', 'Checkout preview');
  await fill(page);
  await page.getByRole('button', { name: 'Read workflow source' }).click();
  await page.getByLabel(/I reviewed this file/).check();
  await page.getByLabel(/^GitHub job ID/).fill('101');
  await expect(page.getByRole('heading', { name: '2. Review the source' })).toHaveCount(0);
});

test('a refusal names the setting an administrator must turn on', async ({ page }) => {
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
  await fill(page);
  await page.getByRole('button', { name: 'Read workflow source' }).click();
  await expect(page.getByRole('alert')).toContainText('optimisation.source_reads');
});

test('a retained preview opens from its link without any GitHub request and the same page works on a phone', async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 700 });
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
