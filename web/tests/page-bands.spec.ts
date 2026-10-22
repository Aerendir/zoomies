import { expect, test } from '@playwright/test';
import { goto } from './support/fixtures';

// A band is the dark stage at the top of a page that says its answer in a sentence. Each
// is on by default and each can be switched off in Appearance, per page, in this browser.
test.use({ viewport: { width: 1440, height: 900 } });

const PAGES = [
  { path: '/', heading: 'Overview', label: 'Overview', band: 'fleet-status' },
  { path: '/runners', heading: 'Runners', label: 'Runners', band: 'band-runners' },
  { path: '/pools', heading: 'Pools', label: 'Pools', band: 'band-pools' },
  { path: '/queue', heading: 'Queue', label: 'Queue', band: 'band-queue' },
  { path: '/jobs', heading: 'Jobs', label: 'Jobs', band: 'band-jobs' },
  { path: '/workflows', heading: 'Workflows', label: 'Workflows', band: 'band-workflows' },
  { path: '/hosts', heading: 'Hosts', label: 'Hosts', band: 'band-hosts' },
  { path: '/providers', heading: 'Providers', label: 'Providers', band: 'band-providers' },
] as const;

for (const item of PAGES) {
  test(`the ${item.label} page opens on its band`, async ({ page }) => {
    await goto(page, item.path, item.heading);
    await expect(page.getByTestId(item.band)).toBeVisible();
  });
}

test('Appearance has a switch for each page, all on, and switching one off hides only that band', async ({
  page,
}) => {
  await goto(page, '/settings/appearance', 'Appearance');
  const group = page.getByRole('group', { name: 'Page bands' });
  for (const item of PAGES) {
    await expect(group.getByRole('switch', { name: item.label, exact: true })).toBeChecked();
  }
  await group.getByRole('switch', { name: 'Pools', exact: true }).click();

  await goto(page, '/pools', 'Pools');
  await expect(page.getByTestId('band-pools')).toHaveCount(0);
  // The others are untouched.
  await goto(page, '/queue', 'Queue');
  await expect(page.getByTestId('band-queue')).toBeVisible();

  // The choice is remembered, and the switch is where it was left.
  await page.reload();
  await goto(page, '/settings/appearance', 'Appearance');
  await expect(
    page
      .getByRole('group', { name: 'Page bands' })
      .getByRole('switch', { name: 'Pools', exact: true }),
  ).not.toBeChecked();
});

test("switching the Overview's band off puts its problems line back", async ({ page }) => {
  await page.addInitScript(() =>
    localStorage.setItem('zoomies.prefs', JSON.stringify({ hiddenBands: ['overview'] })),
  );
  await goto(page, '/', 'Overview');
  await expect(page.getByTestId('fleet-status')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Overview', level: 1 })).toBeVisible();
});

test('a band is not drawn on a phone, switch or no switch', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 800 });
  await goto(page, '/runners', 'Runners');
  await expect(page.getByTestId('band-runners')).toBeHidden();
});
