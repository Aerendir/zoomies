import { expect, test } from '@playwright/test';
import { browserOverride, goto } from './support/fixtures';

test.use(browserOverride);

const root = (page: import('@playwright/test').Page) => page.locator('html');

test('style selection keeps colour mode, logo, dog vocabulary and saved grid preferences', async ({
  page,
}) => {
  await page.addInitScript(() => {
    if (sessionStorage.getItem('style-test-seeded')) return;
    localStorage.setItem('zoomies.theme', 'dark');
    localStorage.setItem(
      'zoomies.prefs',
      JSON.stringify({ statusStyle: 'cute', logoTreatment: 'circle', gridView: 'cards' }),
    );
    sessionStorage.setItem('style-test-seeded', '1');
  });
  await goto(page, '/settings/appearance', 'Appearance');
  const saved = await page.evaluate(() => localStorage.getItem('zoomies.prefs'));
  await expect(page.getByLabel('Logo style', { exact: true })).toHaveValue('circle');
  const mission = page.getByRole('radio', { name: /^Mission Control/ });
  await mission.check();
  await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
  await expect(root(page)).toHaveAttribute('data-theme', 'dark');
  await expect(mission).toBeFocused();
  expect(await page.evaluate(() => localStorage.getItem('zoomies.prefs'))).toBe(saved);
  await expect(
    page
      .getByRole('group', { name: 'Zoomies vocabulary', exact: true })
      .getByRole('button', { name: /^Cute/ }),
  ).toHaveAttribute('aria-pressed', 'true');
  await page.reload();
  await expect(mission).toBeChecked();
  await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
  await page.getByRole('button', { name: 'Reset visual style to Classic' }).click();
  await expect(root(page)).not.toHaveAttribute('data-ui-style');
  await expect(root(page)).toHaveAttribute('data-theme', 'dark');
  expect(await page.evaluate(() => localStorage.getItem('zoomies.ui-style'))).toBeNull();
  expect(await page.evaluate(() => localStorage.getItem('zoomies.prefs'))).toBe(saved);
});

test('visual-style cards support native radio keyboard navigation', async ({ page }) => {
  await goto(page, '/settings/appearance', 'Appearance');
  const classic = page.getByRole('radio', { name: /^Classic/ });
  await classic.focus();
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('radio', { name: /^Mission Control/ })).toBeChecked();
  await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
  await page.keyboard.press('ArrowLeft');
  await expect(classic).toBeChecked();
  await expect(root(page)).not.toHaveAttribute('data-ui-style');
});

test('system colour changes retain Mission Control and update browser chrome', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('zoomies.ui-style', 'mission-control'));
  await page.emulateMedia({ colorScheme: 'light' });
  await goto(page, '/settings/appearance', 'Appearance');
  const background = () =>
    page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--z-bg').trim(),
    );
  expect(await background()).toBe('#f4f7fb');
  await expect(root(page)).not.toHaveAttribute('data-theme');
  await expect(page.locator('meta[data-theme-colour="light"]')).toHaveAttribute(
    'content',
    '#f4f7fb',
  );
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect.poll(background).toBe('#090e18');
  await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
  await expect(page.locator('meta[data-theme-colour="dark"]')).toHaveAttribute(
    'content',
    '#090e18',
  );
  await expect(page.locator('meta[data-theme-colour="dark"]')).toHaveAttribute(
    'media',
    '(prefers-color-scheme: dark)',
  );
});

test('a phone can switch the same visual-style preference through More', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 900 });
  await goto(page, '/', 'Overview');
  await page.getByRole('button', { name: 'More', exact: true }).click();
  const menu = page.getByRole('dialog', { name: 'All sections' });
  await menu
    .getByRole('group', { name: 'Visual style' })
    .getByRole('button', { name: /^Mission Control/ })
    .click();
  await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
  await menu.getByRole('button', { name: 'Close the menu' }).click();
  await goto(page, '/settings/appearance', 'Appearance');
  await expect(page.getByRole('radio', { name: /^Mission Control/ })).toBeChecked();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(375);
});

for (const mode of ['light', 'dark'] as const) {
  test(`Mission Control ${mode} fits desktop and phone monitoring routes`, async ({ page }) => {
    test.setTimeout(90_000);
    await page.addInitScript((mode) => {
      localStorage.setItem('zoomies.ui-style', 'mission-control');
      localStorage.setItem('zoomies.theme', mode);
    }, mode);
    const routes = [
      ['/', 'Overview'],
      ['/pools', 'Pools'],
      ['/runners', 'Runners'],
      ['/workflows', 'Workflows'],
      ['/jobs', 'Jobs'],
      ['/queue', 'Queue'],
      ['/usage', 'Usage'],
      ['/hosts', 'Hosts'],
      ['/providers', 'Providers'],
      ['/repairs', 'Eli repairs'],
      ['/kennel', 'Kennel Club'],
      ['/migrate', 'Migrate repositories'],
      ['/audit', 'Audit'],
      ['/settings/appearance', 'Appearance'],
    ];
    for (const width of [1440, 375]) {
      await page.setViewportSize({ width, height: 900 });
      for (const [path, heading] of routes) {
        await goto(page, path!, heading!);
        await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
        await expect
          .poll(
            () => page.evaluate(() => document.documentElement.scrollWidth),
            `${path} at ${width}px`,
          )
          .toBe(width);
      }
    }
  });
}

test('changing style from More preserves an unfinished pool form', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 900 });
  await goto(page, '/pools/new', 'Create a pool');
  const name = page.getByRole('textbox', { name: 'Pool name' });
  await name.fill('zoomies-style-draft');
  await page.getByRole('button', { name: 'More', exact: true }).click();
  const menu = page.getByRole('dialog', { name: 'All sections' });
  await menu
    .getByRole('group', { name: 'Visual style' })
    .getByRole('button', { name: /^Mission Control/ })
    .click();
  await menu.getByRole('button', { name: 'Close the menu' }).click();
  await expect(page).toHaveURL(/\/pools\/new$/);
  await expect(name).toHaveValue('zoomies-style-draft');
});

test('blocked browser storage still allows an in-page visual-style choice', async ({ page }) => {
  await page.addInitScript(() =>
    Object.defineProperty(window, 'localStorage', {
      get() {
        throw new Error('Blocked storage');
      },
    }),
  );
  await goto(page, '/settings/appearance', 'Appearance');
  await expect(page.getByRole('radio', { name: /^Classic/ })).toBeChecked();
  await page.getByRole('radio', { name: /^Mission Control/ }).check();
  await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
});

test('the first-account screen honours stored style and keeps the original Zoomies artwork', async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem('zoomies.ui-style', 'mission-control');
    localStorage.setItem('zoomies.theme', 'dark');
  });
  for (const width of [1440, 375]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto('http://127.0.0.1:8098/');
    await expect(page.getByRole('heading', { name: 'Create the first account' })).toBeVisible();
    await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
    await expect(
      page.locator('img[src="/brand/mark-black-transparent.svg"]').first(),
    ).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width);
  }
});
