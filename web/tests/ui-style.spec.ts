import { expect, test } from '@playwright/test';
import { goto } from './support/fixtures';

const root = (page: import('@playwright/test').Page) => page.locator('html');

test('style selection keeps colour mode, logo, dog vocabulary and saved grid preferences', async ({
  page,
}) => {
  await page.addInitScript(() => {
    if (sessionStorage.getItem('style-test-seeded')) return;
    localStorage.setItem('zoomies.theme', 'dark');
    localStorage.setItem('zoomies.ui-style', 'classic');
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
  // Mission Control is the default, so choosing it stores nothing; Classic is
  // the deliberate choice and is remembered.
  expect(await page.evaluate(() => localStorage.getItem('zoomies.ui-style'))).toBeNull();
  await page.getByRole('radio', { name: /^Classic/ }).check();
  await expect(root(page)).not.toHaveAttribute('data-ui-style');
  await expect(root(page)).toHaveAttribute('data-theme', 'dark');
  expect(await page.evaluate(() => localStorage.getItem('zoomies.ui-style'))).toBe('classic');
  await page.getByRole('button', { name: 'Reset visual style to Mission Control' }).click();
  await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
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
  await page.addInitScript(() => {
    localStorage.setItem('zoomies.ui-style', 'mission-control');
    // Dark is the default; this test is about following the system.
    localStorage.setItem('zoomies.theme', 'system');
  });
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

// The plan asks that a style switch not remount an editor or reset an open
// conversation: only root tokens change. Eli's panel and its draft live in app
// state, so both have to survive the change. The route is walked through the
// app's own links, since a fresh page load is a reload and the conversation is
// meant to be forgotten on reload.
test('switching style leaves an open assistant conversation alone', async ({ page }) => {
  // Wide enough that the sidebar shows its labels: a collapsed sidebar empties
  // the accessible name of every entry.
  await page.setViewportSize({ width: 1440, height: 900 });
  await goto(page, '/', 'Overview');
  await page.getByRole('button', { name: 'Ask Eli' }).click();
  const message = page.getByRole('dialog', { name: 'Eli assistant' }).getByLabel('Message');
  await message.fill('zoomies-style-draft');
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Settings' })
    .click();
  await page
    .getByRole('navigation', { name: 'Settings' })
    .getByRole('link', { name: 'Appearance' })
    .click();
  // Switched from the keyboard, because the open panel sits over the cards and
  // would intercept a pointer click; the style change is the same either way.
  await page.getByRole('radio', { name: /^Classic/ }).focus();
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('radio', { name: /^Mission Control/ })).toBeChecked();
  await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
  await expect(page.getByRole('dialog', { name: 'Eli assistant' })).toBeVisible();
  await expect(message).toHaveValue('zoomies-style-draft');
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
  // Nothing can be read, so nothing is chosen: the default, Mission Control.
  await expect(page.getByRole('radio', { name: /^Mission Control/ })).toBeChecked();
  await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
  await page.getByRole('radio', { name: /^Classic/ }).check();
  await expect(root(page)).not.toHaveAttribute('data-ui-style');
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

// The plan's width range: full desktop, the narrow-desktop edges either side of
// its breakpoint, the phone breakpoint itself, and the two smallest phones the
// UI has to survive. Local grid and code scrolling is deliberate, so only the
// document is allowed to stay put; one pixel more is a sideways page.
const PLAN_WIDTHS = [1440, 1180, 1179, 768, 375, 360];

for (const style of ['classic', 'mission-control'] as const) {
  test(`${style} has no page-level overflow across the plan's width range`, async ({ page }) => {
    test.setTimeout(120_000);
    await page.addInitScript((style) => {
      localStorage.setItem('zoomies.theme', 'dark');
      localStorage.setItem('zoomies.ui-style', style);
    }, style);
    const routes = [
      ['/', 'Overview'],
      ['/pools', 'Pools'],
      ['/jobs', 'Jobs'],
      ['/settings/appearance', 'Appearance'],
    ] as const;
    for (const width of PLAN_WIDTHS) {
      await page.setViewportSize({ width, height: 900 });
      for (const [path, heading] of routes) {
        await goto(page, path, heading);
        // The test is about this style, so the page has to be in it: an empty
        // browser is Mission Control, and a run that stored nothing for Classic
        // measured Mission Control twice.
        if (style === 'classic') await expect(root(page)).not.toHaveAttribute('data-ui-style');
        else await expect(root(page)).toHaveAttribute('data-ui-style', 'mission-control');
        await expect
          .poll(
            () => page.evaluate(() => document.documentElement.scrollWidth),
            `${style} ${path} at ${width}px`,
          )
          .toBe(width);
      }
    }
  });
}

// A 200% zoom lays the same page out in half the CSS pixels, so it is checked
// by halving the plan's widths rather than by trusting a scale transform. The
// enlarged-text case is separate on purpose: zoom changes the pixel the
// viewport is measured in, while root text growth changes the type inside it.
test('Mission Control reflows under a 200% zoom instead of pushing the page sideways', async ({
  page,
}) => {
  test.setTimeout(120_000);
  await page.addInitScript(() => localStorage.setItem('zoomies.ui-style', 'mission-control'));
  const routes = [
    ['/', 'Overview'],
    ['/jobs', 'Jobs'],
    ['/settings/appearance', 'Appearance'],
  ] as const;
  for (const width of [720, 590, 384]) {
    await page.setViewportSize({ width, height: 900 });
    for (const [path, heading] of routes) {
      await goto(page, path, heading);
      await expect
        .poll(
          () => page.evaluate(() => document.documentElement.scrollWidth),
          `${path} at 200% of ${width * 2}px`,
        )
        .toBe(width);
    }
  }
});

// 200% text is asserted where a browser's 200% zoom leaves at least the 320
// CSS pixels WCAG's reflow rule names as the floor. At 360 or 375 pixels the
// same text size lays the page out in under 190 CSS pixels, and Classic scrolls
// there too; the phone widths are covered at default size by the test above.
test('Mission Control reflows enlarged text instead of clipping it', async ({ page }) => {
  test.setTimeout(120_000);
  await page.addInitScript(() => localStorage.setItem('zoomies.ui-style', 'mission-control'));
  const routes = [
    ['/', 'Overview'],
    ['/jobs', 'Jobs'],
    ['/settings/appearance', 'Appearance'],
  ] as const;
  for (const width of [1440, 1180, 768]) {
    await page.setViewportSize({ width, height: 900 });
    for (const [path, heading] of routes) {
      await goto(page, path, heading);
      await page.evaluate(() => {
        document.documentElement.style.fontSize = '200%';
      });
      await expect
        .poll(
          () => page.evaluate(() => document.documentElement.scrollWidth),
          `${path} at ${width}px with 200% text`,
        )
        .toBe(width);
    }
  }
});
