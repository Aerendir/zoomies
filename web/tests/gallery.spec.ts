/**
 * The component gallery, in every style and mode combination.
 *
 * The gallery is development-only (web/gallery.html) and the suite's other
 * specs drive a built binary, which does not contain it. This project drives
 * the Vite dev server instead; see tests/support/serve-gallery.mjs.
 *
 * The point of the spec is not that the page loads. N-T4 asks for the states a
 * healthy fleet never shows to be reviewable across both styles and both modes,
 * so every test below either visits all four combinations or is about the
 * behaviour a combination must not change: focus rings, readable refusal
 * reasons, live contrast, and the phone width with no page-level overflow.
 */
import { expect, test, type Page } from '@playwright/test';
const COMBINATIONS = [
  { style: 'classic', mode: 'light' },
  { style: 'classic', mode: 'dark' },
  { style: 'mission-control', mode: 'light' },
  { style: 'mission-control', mode: 'dark' },
] as const;

/** The regions a reviewer should find on the page, whatever the combination. */
const SECTIONS = [
  'Controls',
  'Form controls',
  'Status vocabulary',
  'Loading',
  'Empty states',
  'Errors',
  'Loading boundary',
  'Toasts',
  'Surfaces and text',
  'Status tones',
  'Type and density',
  'Overlays',
  'Notices',
  'Data grid',
];

function open(page: Page, style: string, mode: string): Promise<unknown> {
  return page.goto(`/gallery.html?style=${style}&mode=${mode}`);
}

function parseColour(value: string): [number, number, number] {
  const parts =
    /rgba?\(([^)]+)\)/
      .exec(value)?.[1]
      ?.split(/[,\s/]+/)
      .map(Number) ?? [];
  return [parts[0] ?? 0, parts[1] ?? 0, parts[2] ?? 0];
}

function luminance(value: string): number {
  const [r, g, b] = parseColour(value).map((channel) => {
    const c = channel / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(foreground: string, background: string): number {
  const [hi, lo] = [luminance(foreground), luminance(background)].sort((a, b) => b - a) as [
    number,
    number,
  ];
  return (hi + 0.05) / (lo + 0.05);
}

/**
 * Read the colours the browser actually resolved, not the strings the CSS
 * wrote. A component that hard-codes a colour passes a token lint and fails
 * here, which is the test the plan's contrast contract needs.
 *
 * Badges and state cells are presentation rather than controls, so there is no
 * role to reach for; the class each carries is the only thing that says which
 * element is which.
 */
async function assertReadableText(page: Page): Promise<void> {
  const surface = await page.evaluate(() => {
    const panel = document.querySelector('.panel');
    return panel ? getComputedStyle(panel).backgroundColor : 'rgb(255, 255, 255)';
  });

  const badges = await page.locator('.badge').evaluateAll((els) =>
    els.map((el) => {
      const style = getComputedStyle(el);
      return { text: el.textContent?.trim() ?? '', fg: style.color, bg: style.backgroundColor };
    }),
  );
  expect(badges.length).toBeGreaterThan(20);
  for (const badge of badges) {
    if (badge.bg === 'rgba(0, 0, 0, 0)' || badge.bg === 'transparent') continue;
    expect(
      contrast(badge.fg, badge.bg),
      `${badge.text || 'a badge'}: ${badge.fg} on ${badge.bg}`,
    ).toBeGreaterThanOrEqual(4.5);
  }

  const cells = await page
    .locator('[data-gallery="runner-states"] .cell')
    .evaluateAll((els) => els.map((el) => getComputedStyle(el).color));
  expect(cells.length).toBeGreaterThan(5);
  for (const colour of cells) {
    expect(contrast(colour, surface), `${colour} on the panel surface`).toBeGreaterThanOrEqual(4.5);
  }
}

test.describe('every style and mode combination', () => {
  for (const { style, mode } of COMBINATIONS) {
    test(`${style} ${mode} carries its own root attributes and renders every section`, async ({
      page,
    }) => {
      await open(page, style, mode);

      const root = page.locator('html');
      if (style === 'mission-control') {
        await expect(root).toHaveAttribute('data-ui-style', 'mission-control');
      } else {
        expect(await root.getAttribute('data-ui-style')).toBeNull();
      }
      await expect(root).toHaveAttribute('data-theme', mode);

      for (const name of SECTIONS) {
        await expect(page.getByRole('region', { name, exact: true })).toBeVisible();
      }

      // The refusal keeps its reason in the accessible description, and stays
      // a button: `aria-disabled`, never the native attribute.
      const refused = page.locator('button[aria-disabled="true"]', { hasText: 'Delete pool' });
      await expect(refused).toHaveCount(1);
      expect(await refused.getAttribute('disabled')).toBeNull();
      const describedBy = await refused.getAttribute('aria-describedby');
      expect(describedBy).toBeTruthy();
      await expect(page.locator(`#${describedBy}`)).toContainText('Only administrators can delete');

      // A plain disabled control is the other half: it is really disabled and
      // carries no reason.
      await expect(page.getByRole('button', { name: 'Publish' })).toBeDisabled();

      // The keyboard reaches the refused control and draws a ring on it.
      await refused.focus();
      const ring = await refused.evaluate((el) => {
        const style = getComputedStyle(el);
        return {
          visible: el.matches(':focus-visible'),
          style: style.outlineStyle,
          width: Number.parseFloat(style.outlineWidth),
        };
      });
      expect(ring.visible).toBe(true);
      expect(ring.style).not.toBe('none');
      expect(ring.width).toBeGreaterThan(0);

      // Retry actions are present, enabled and named.
      await expect(page.getByRole('button', { name: 'Try again' }).first()).toBeEnabled();

      await assertReadableText(page);
    });

    test(`${style} ${mode} does not overflow at 1440 or 375 pixels`, async ({ page }) => {
      await open(page, style, mode);
      for (const width of [1440, 375]) {
        await page.setViewportSize({ width, height: 900 });
        // A frame for the grid's own measurements to settle after the reflow.
        await page.waitForTimeout(150);
        const { scrollWidth, inner } = await page.evaluate(() => ({
          scrollWidth: document.documentElement.scrollWidth,
          inner: window.innerWidth,
        }));
        expect(scrollWidth, `page width at ${width}px`).toBeLessThanOrEqual(inner);
      }
    });
  }
});

test('an unknown style and mode fall back to Classic and System', async ({ page }) => {
  await open(page, 'neon', 'twilight');
  const root = page.locator('html');
  expect(await root.getAttribute('data-ui-style')).toBeNull();
  expect(await root.getAttribute('data-theme')).toBeNull();
});

test('a fresh phone grid follows the style fallback: Cards in Mission Control, Rows in Classic', async ({
  page,
}) => {
  // The gallery sets the root attribute without saving a preference, so this
  // is also the check that the store the grid reads is hearing the same
  // combination the tokens are.
  for (const [style, view] of [
    ['mission-control', 'Cards'],
    ['classic', 'Rows'],
  ] as const) {
    await open(page, style, 'light');
    await page.setViewportSize({ width: 375, height: 900 });
    const region = page.getByRole('region', { name: 'Data grid' });
    await expect(
      region.getByRole('button', { name: new RegExp(`^${view}`) }),
      `${style} starts a fresh phone grid as ${view}`,
    ).toHaveAttribute('aria-pressed', 'true');
  }
});

test('System follows the operating system without changing the chosen style', async ({ page }) => {
  await open(page, 'mission-control', 'system');
  const root = page.locator('html');
  await expect(root).toHaveAttribute('data-ui-style', 'mission-control');
  expect(await root.getAttribute('data-theme')).toBeNull();

  await page.emulateMedia({ colorScheme: 'dark' });
  const dark = await page.evaluate(() =>
    getComputedStyle(document.documentElement).getPropertyValue('--z-bg').trim(),
  );
  await page.emulateMedia({ colorScheme: 'light' });
  const light = await page.evaluate(() =>
    getComputedStyle(document.documentElement).getPropertyValue('--z-bg').trim(),
  );
  expect(dark).not.toBe(light);
  await expect(root).toHaveAttribute('data-ui-style', 'mission-control');
});

test('choosing a style applies it in place and writes no preference', async ({ page }) => {
  await open(page, 'classic', 'light');
  const styleGroup = page.getByRole('group', { name: 'Visual style', exact: true });

  await styleGroup.getByRole('button', { name: /^Mission Control/ }).click();
  await expect(page.locator('html')).toHaveAttribute('data-ui-style', 'mission-control');
  expect(new URL(page.url()).searchParams.get('style')).toBe('mission-control');

  await page
    .getByRole('group', { name: 'Colour mode', exact: true })
    .getByRole('button', { name: 'Dark' })
    .click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

  // The gallery is a review tool: it must not touch the preferences a real
  // Appearance change owns.
  const stored = await page.evaluate(() => ({
    style: localStorage.getItem('zoomies.ui-style'),
    theme: localStorage.getItem('zoomies.theme'),
  }));
  expect(stored).toEqual({ style: null, theme: null });

  await styleGroup.getByRole('button', { name: 'Classic' }).click();
  expect(await page.locator('html').getAttribute('data-ui-style')).toBeNull();
  expect(new URL(page.url()).searchParams.get('style')).toBe('classic');
});

test('the drawer traps focus, closes on Escape and returns focus to its trigger', async ({
  page,
}) => {
  await open(page, 'mission-control', 'dark');
  const trigger = page.getByRole('button', { name: 'Open the runner drawer' });
  await trigger.click();

  const drawer = page.getByRole('dialog', { name: 'run_demo00' });
  await expect(drawer).toBeVisible();
  await expect(drawer.locator('header').getByRole('button', { name: 'Close' })).toBeVisible();

  // Tab never leaves the open drawer, however many times it is pressed.
  for (let i = 0; i < 8; i += 1) {
    await page.keyboard.press('Tab');
    const inside = await drawer.evaluate((el) => el.contains(document.activeElement));
    expect(inside, `focus after Tab ${i + 1}`).toBe(true);
  }

  // Exactly one layer closes per Escape. The last Tab may have landed on the
  // refused control, whose reason opens the shared tooltip, so the presses
  // unwind whatever the loop left open and then the drawer.
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: 'run_demo00' })).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test('the destructive confirmation names the thing and gates the confirm', async ({ page }) => {
  await open(page, 'classic', 'light');
  await page.getByRole('button', { name: 'Delete a pool' }).click();

  const dialog = page.getByRole('dialog', { name: 'Delete pool' });
  await expect(dialog).toBeVisible();
  const confirm = dialog.getByRole('button', { name: 'Delete', exact: true });
  await expect(confirm).toBeDisabled();

  await dialog.getByLabel('Type linux-x64 to confirm').fill('linux-x64');
  await expect(confirm).toBeEnabled();
  await confirm.click();
  await expect(dialog).toHaveCount(0);
});

test('the grid shows ready, empty, loading and failed states', async ({ page }) => {
  await open(page, 'mission-control', 'light');
  const region = page.getByRole('region', { name: 'Data grid' });
  const table = region.getByRole('grid', { name: 'Gallery runners' });

  await expect(table).toBeVisible();
  await expect(region.getByRole('row', { name: /run_demo00/ })).toBeVisible();

  await region.getByRole('button', { name: /^Empty/ }).click();
  await expect(region.getByText('No runners match these filters')).toBeVisible();

  await region.getByRole('button', { name: /^Error/ }).click();
  await expect(region.getByRole('alert')).toContainText('The controller did not answer');

  await region.getByRole('button', { name: /^Loading/ }).click();
  // Nothing in the accessibility tree distinguishes a skeleton row from a data
  // row, so this one reaches for the class the grid gives it.
  await expect(region.locator('tr.skeleton-row').first()).toBeVisible();
});

test('a toast lands in the region with the tone it was raised with', async ({ page }) => {
  await open(page, 'classic', 'dark');
  await page.getByRole('button', { name: 'Success' }).click();
  await expect(page.getByText('Pool created')).toBeVisible();

  await page.getByRole('button', { name: 'Error with an action' }).click();
  const refusal = page.getByText('The drain was refused');
  await expect(refusal).toBeVisible();
  await expect(page.getByRole('button', { name: 'Open your account' })).toBeVisible();
});
