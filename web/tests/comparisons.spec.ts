import { expect, test } from '@playwright/test';
import { browserOverride, goto } from './support/fixtures';

test.use(browserOverride);
const distribution = (zero = false) => ({
  samples: zero ? 1 : 0,
  missing: zero ? 0 : 1,
  min_ms: zero ? 0 : null,
  p50_ms: zero ? 0 : null,
  p95_ms: zero ? 0 : null,
  max_ms: zero ? 0 : null,
  total_ms: zero ? 0 : null,
});
const counts = {
  selected: 1,
  completed: 1,
  succeeded: 0,
  failed: 1,
  cancelled: 0,
  superseded: 0,
  unknown: 0,
};
const arm = (run: number, zero = false) => ({
  source_revision: 'a'.repeat(40),
  workflow_revision: '',
  inputs_id: '',
  test_scope_id: '',
  configuration_id: '',
  image_id: '',
  resource_id: '',
  selection_complete: false,
  attempts: [
    {
      source_revision: 'a'.repeat(40),
      workflow_id: 20,
      latency_ms: null,
      execution_ms: zero ? 0 : null,
      allocated_idle_ms: null,
      run_id: run,
      attempt: 1,
      observed_at: '2026-10-13T00:00:00Z',
      outcome: 'failure',
      cache: '',
      host_condition: '',
    },
  ],
});
const measures = (zero = false) => ({
  successful_latency: distribution(),
  all_attempt_execution: distribution(zero),
  all_attempt_allocated_idle: distribution(),
  failure_rate: 1,
});
const retained = {
  id: 'cmp_fixture',
  sha256: 'd'.repeat(64),
  created_at: '2026-10-13T00:00:00Z',
  report: {
    window_basis: 'github_attempt_started_at',
    execution_basis: 'sum_of_all_github_job_elapsed_times',
    snapshot: {
      rule_version: 'selected-runs-v2',
      kind: 'runner',
      repository: 'acme/repo',
      initiating_user: 'fixture',
      hypothesis: '<script>window.injected = true</script>',
      intended_delta: 'larger runner',
      evidence_id: 'cmp_fixture',
      revision_delta_id: '',
      since: '2026-10-12T00:00:00Z',
      until: '2026-10-14T00:00:00Z',
      baseline: arm(10, true),
      candidate: arm(11),
    },
    measurement: {
      version: 'selected-measures-v1',
      eligibility: {
        rule_version: 'selected-runs-v2',
        state: 'inconclusive',
        reasons: ['selection_incomplete', 'inputs_unknown', 'cache_unknown'],
        minimum_samples: 30,
        baseline: counts,
        candidate: counts,
      },
      baseline: measures(true),
      candidate: measures(),
      latency_change: 'not_enough',
      execution_mean_change: 'not_enough',
      allocated_idle_mean_change: 'not_enough',
      failure_rate_change: 'not_enough',
    },
  },
};

// The real controller serves the application and role/session state. The
// comparison response is a fixture for GitHub evidence unavailable in CI.
test('operator capture is explicit, immutable replay is shareable and unknown stays distinct from zero', async ({
  page,
}) => {
  let captures = 0,
    reads = 0;
  await page.route('**/api/v1/comparisons', async (route) => {
    captures++;
    expect(route.request().method()).toBe('POST');
    const b = route.request().postDataJSON();
    expect(b.baseline).toEqual([{ run_id: 10, attempt: 1 }]);
    expect(b.candidate).toEqual([{ run_id: 11, attempt: 1 }]);
    await route.fulfill({ status: 201, json: retained });
  });
  await page.route('**/api/v1/comparisons/*?*', async (route) => {
    reads++;
    const q = new URL(route.request().url()).searchParams;
    expect(q.get('installation_id')).toBe('ins_fixture');
    expect(q.get('repo')).toBe('acme/repo');
    await route.fulfill({ json: retained });
  });
  await goto(page, '/jobs', 'Jobs');
  await page.getByRole('link', { name: 'Compare runs' }).click();
  await expect(page.getByRole('heading', { name: 'Comparisons', exact: true })).toBeVisible();
  expect(captures).toBe(0);
  await page.getByLabel('Installation ID').fill('ins_fixture');
  await page.getByLabel('Repository').fill('acme/repo');
  await page.getByLabel('Hypothesis').fill('lower latency');
  await page.getByLabel('Intended change').fill('larger runner');
  await page.getByLabel('Baseline run attempts').fill('10:1');
  await page.getByLabel('Candidate run attempts').fill('11:1');
  await page.getByRole('button', { name: 'Capture and retain evidence' }).click();
  await expect(page.getByRole('heading', { name: 'Retained report' })).toBeVisible();
  expect(captures).toBe(1);
  expect(reads).toBe(0);
  await expect(page.getByRole('heading', { name: 'Retained report' })).toBeFocused();
  await page.screenshot({
    path: test.info().outputPath('comparison.png'),
  });
  await expect(page.getByText('Inconclusive', { exact: true })).toBeVisible();
  await expect(page.getByText('selection incomplete', { exact: true })).toBeVisible();
  await expect(page.getByText(retained.report.snapshot.hypothesis, { exact: true })).toBeVisible();
  expect(await page.evaluate(() => Object.hasOwn(window, 'injected'))).toBe(false);
  const baseline = page.getByRole('region', { name: 'Baseline measurements', exact: true });
  await expect(baseline.getByText('0 ms', { exact: true }).first()).toBeVisible();
  await expect(baseline.getByText('Unknown', { exact: true }).first()).toBeVisible();
  await page.getByText('Baseline identities and attempts', { exact: true }).click();
  await expect(
    page
      .getByRole('region', { name: 'Baseline attempts', exact: true })
      .getByText('failure', { exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Retained report' })).toBeVisible();
  expect(reads).toBe(1);
  expect(captures).toBe(1);
});

test('missing history and rate limiting do not capture or automatically retry', async ({
  page,
}) => {
  let calls = 0;
  let limited = false;
  await page.route('**/api/v1/comparisons/*?*', async (route) => {
    calls++;
    await route.fulfill({
      status: limited ? 429 : 404,
      headers: limited ? { 'Retry-After': '60' } : {},
      json: {
        error: { code: limited ? 'rate_limited' : 'not_found', message: 'History unavailable' },
      },
    });
  });
  await goto(
    page,
    '/comparisons?id=cmp_fixture&installation_id=ins_fixture&repo=acme%2Frepo',
    'Comparisons',
  );
  await expect(page.getByRole('alert')).toContainText('No replacement was captured');
  expect(calls).toBe(1);
  limited = true;
  await page.getByRole('button', { name: 'Open report' }).click();
  await expect(page.getByRole('button', { name: 'Open report' })).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Capture and retain evidence' })).toBeDisabled();
  expect(calls).toBe(2);
});

test('viewer can read but cannot capture', async ({ page }) => {
  await page.route('**/api/v1/auth/session', async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.role = 'viewer';
    if (body.identity) body.identity.role = 'viewer';
    await route.fulfill({ response, json: body });
  });
  // Auth-disabled sessions bypass /auth/session, so use the real identity path
  // by reporting auth enabled while the fixture controller serves viewer data.
  await page.route('**/api/v1/meta', async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.auth_disabled = false;
    body.bootstrap_required = false;
    await route.fulfill({ response, json: body });
  });
  await page.route('**/api/v1/comparisons/*?*', (route) => route.fulfill({ json: retained }));
  await goto(
    page,
    '/comparisons?id=cmp_fixture&installation_id=ins_fixture&repo=acme%2Frepo',
    'Comparisons',
  );
  await expect(page.getByRole('heading', { name: 'Retained report' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Capture and retain evidence' })).toHaveCount(0);
});

test('public visitors make no history request', async ({ page }) => {
  let reads = 0;
  await page.route('**/api/v1/meta', async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.auth_disabled = false;
    body.bootstrap_required = false;
    body.public_view = true;
    await route.fulfill({ response, json: body });
  });
  await page.route('**/api/v1/auth/session', async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.role = 'viewer';
    body.kind = 'public';
    await route.fulfill({ response, json: body });
  });
  await page.route('**/api/v1/comparisons/*?*', async (route) => {
    reads++;
    await route.fulfill({ json: retained });
  });
  await goto(
    page,
    '/comparisons?id=cmp_fixture&installation_id=ins_fixture&repo=acme%2Frepo',
    'Comparisons',
  );
  await expect(page.getByText('Sign in to read comparisons', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Open report' })).toHaveCount(0);
  expect(reads).toBe(0);
});
