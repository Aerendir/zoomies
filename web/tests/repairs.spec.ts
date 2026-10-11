import { expect, test } from '@playwright/test';
import { browserOverride, goto } from './support/fixtures';

test.use(browserOverride);

test('repair setup discovers repositories and saves a manual policy without a shared payer', async ({
  page,
}) => {
  await goto(page, '/repairs', 'Eli repairs');
  await page.locator('#repair-setup > summary').click();
  const policies = page
    .locator('details')
    .filter({ has: page.locator('#policy-installation') })
    .last();
  if (!(await page.locator('#policy-installation').isVisible()))
    await policies.locator('summary').first().click();
  await page.locator('#repair-mode').selectOption('manual');
  const repositories = page.getByRole('group', { name: 'Select repositories', exact: true });
  await repositories.getByRole('checkbox', { name: 'acme/site', exact: true }).check();
  await repositories.getByRole('checkbox', { name: 'acme/widgets', exact: true }).check();
  await expect(page.locator('#policy-provider')).toHaveCount(0);
  await page.getByRole('button', { name: 'Save for 2 repositories' }).click();
  await expect(page.getByText('acme/site: saved', { exact: true })).toBeVisible();
  await expect(page.getByText('acme/widgets: saved', { exact: true })).toBeVisible();
  const response = await page.request.get('/api/v1/assistant/repairs/settings');
  const settings = await response.json();
  for (const repo of ['acme/site', 'acme/widgets']) {
    const policy = settings.policies.find((p: { repo: string }) => p.repo === repo);
    expect(policy.provider_id).toBe('');
    expect(policy.automatic).toBe(false);
    expect(policy.ask_first).toBe(false);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
