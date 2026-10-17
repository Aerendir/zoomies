import { expect, test, type Page } from '@playwright/test';
import { dataRows, goto, grid } from './support/fixtures';

async function openJob(page: Page) {
  await goto(page, '/jobs', 'Jobs');
  await dataRows(grid(page, 'Jobs')).first().click();
  const drawer = page.getByRole('dialog');
  await expect(drawer.getByRole('heading', { name: 'PR check delay', exact: true })).toBeVisible();
  return drawer;
}

// The real controller serves navigation and jobs; the diagnostic is shaped
// here for states which need a live GitHub installation in production.
async function diagnostic(page: Page, status = 200) {
  let calls = 0;
  await page.route('**/api/v1/jobs/*/pr-delay?*', async (route) => {
    calls++;
    if (status === 429) {
      await route.fulfill({
        status,
        headers: { 'Retry-After': '60' },
        json: {
          error: {
            code: 'rate_limited',
            message: 'The installation diagnostic allowance is exhausted.',
          },
        },
      });
      return;
    }
    const jobID = new URL(route.request().url()).pathname.split('/').at(-2)!;
    const job = await (await page.request.get(`/api/v1/jobs/${jobID}`)).json();
    await route.fulfill({
      json: {
        job_id: jobID,
        repository: job.repo,
        installation_id: job.installation_id,
        pull_number: 42,
        window_basis: 'earliest_observed_job',
        result: {
          version: 'v1',
          head_sha: job.head_sha || 'pinned',
          target_branch: 'release',
          observed_at: '2026-10-12T12:00:00Z',
          window_start: '2026-10-12T11:00:00Z',
          protection_coverage: 'complete',
          observation_coverage: 'partial',
          state: 'incomplete',
          last_finishing_check_id: 'chk_001',
          ignored_other_heads: 0,
          checks: [
            {
              check_id: 'chk_001',
              state: 'passed',
              observation_id: '40',
              completion_delay_ms: 180000,
              timing: {
                pre_start_ms: 60000,
                queue_ms: null,
                startup_ms: null,
                execution_ms: 120000,
              },
            },
            {
              check_id: 'chk_002',
              state: 'missing',
              observation_id: '',
              completion_delay_ms: null,
              timing: { pre_start_ms: null, queue_ms: null, startup_ms: null, execution_ms: null },
            },
          ],
        },
      },
    });
  });
  return () => calls;
}

test('the real diagnostic is read only after a valid explicit request', async ({ page }) => {
  let calls = 0;
  page.on('request', (request) => {
    if (request.url().includes('/pr-delay?')) calls++;
  });
  const drawer = await openJob(page);
  expect(calls).toBe(0);
  await drawer.getByLabel('PR number', { exact: true }).fill('0');
  await drawer.getByRole('button', { name: 'Read PR delay', exact: true }).click();
  await expect(drawer.getByRole('alert')).toContainText('positive PR number');
  expect(calls).toBe(0);
  await drawer.getByLabel('PR number', { exact: true }).fill('42');
  const reply = page.waitForResponse((response) => response.url().includes('/pr-delay?'));
  await drawer.getByRole('button', { name: 'Read PR delay', exact: true }).click();
  expect((await reply).status()).toBeGreaterThanOrEqual(400);
  await expect(drawer.getByRole('alert')).toBeVisible();
  await expect(drawer.getByRole('button', { name: 'Read PR delay', exact: true })).toBeEnabled();
  expect(calls).toBe(1);
});

test('the snapshot shows partial coverage and unknown phases', async ({ page }, testInfo) => {
  const calls = await diagnostic(page);
  const drawer = await openJob(page);
  expect(calls()).toBe(0);
  await drawer.getByLabel('PR number', { exact: true }).fill('42');
  await drawer.getByRole('button', { name: 'Read PR delay', exact: true }).click();
  const panel = drawer
    .locator('section')
    .filter({ has: page.getByRole('heading', { name: 'PR check delay', exact: true }) });
  await expect(panel).toContainText('Evidence is incomplete');
  await expect(panel).toContainText('partial');
  await expect(panel).toContainText('chk_002: missing');
  await expect(panel.getByText('Unknown', { exact: true })).toHaveCount(7);
  await expect(panel).toContainText('do not establish full merge readiness');
  await expect(panel.getByRole('button', { name: 'Ask Eli', exact: true })).toBeVisible();
  expect(calls()).toBe(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await panel.getByRole('button', { name: 'Ask Eli', exact: true }).scrollIntoViewIfNeeded();
  await expect(panel.getByRole('button', { name: 'Ask Eli', exact: true })).toBeInViewport();
  await page.screenshot({ path: testInfo.outputPath('pr-delay.png') });
  await drawer.getByRole('button', { name: 'Close', exact: true }).click();
  await dataRows(grid(page, 'Jobs')).first().click();
  await expect(page.getByRole('dialog').getByLabel('PR number', { exact: true })).toHaveValue('');
  await expect(page.getByRole('dialog').getByText('chk_002: missing', { exact: true })).toHaveCount(
    0,
  );
  expect(calls()).toBe(1);
});

test('Retry-After holds the manual button without scheduling another read', async ({ page }) => {
  const calls = await diagnostic(page, 429);
  const drawer = await openJob(page);
  await drawer.getByLabel('PR number', { exact: true }).fill('42');
  await drawer.getByRole('button', { name: 'Read PR delay', exact: true }).click();
  await expect(drawer.getByRole('alert')).toContainText('allowance is exhausted');
  await expect(drawer.getByText('No retry has been scheduled.', { exact: false })).toBeVisible();
  await expect(drawer.getByRole('button', { name: 'Read PR delay', exact: true })).toBeDisabled();
  expect(calls()).toBe(1);
});
