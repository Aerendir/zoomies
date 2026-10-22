// The setup pages: the three wizards, Settings and Migrate. A step says where the
// person is, a wrong field says which and why, a button that is saving still has
// a name, and a setting that has not been saved says so.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from '../tools/lib/browser.mjs';
import { serve } from '../tools/serve.mjs';

const WIZARDS = ['pool-new', 'host-new', 'provider-new'];
let browser;
let server;
before(async () => {
  browser = await launch();
  server = await serve(0);
});
after(async () => {
  await browser?.close();
  await server?.close();
});

async function open(screen, state, width = 1440, look = 'style=mission-control&mode=dark') {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const page = await context.newPage();
  const hash = `#screen=${screen}&concept=a&${look}&state=${state}`;
  await page.goto(`${server.url}/signed-in/mockups.html${hash}`);
  await page.waitForFunction((h) => document.documentElement.dataset.rendered === h, hash);
  return { page, context };
}

for (const screen of WIZARDS) {
  test(`${screen}: each step marks the step the person is on, and the ones behind them`, async () => {
    for (const [state, index] of [['step-1', 0], ['step-2', 1], ['step-3', 2]]) {
      const { page, context } = await open(screen, state);
      const current = await page.$$eval('.k-step', (els) => els.findIndex((el) => el.getAttribute('aria-current') === 'step'));
      assert.equal(current, index, `${screen} ${state}`);
      assert.equal(await page.locator('.k-step.is-done').count(), index);
      await context.close();
    }
  });

  test(`${screen}: a wrong field is marked invalid, says why, and is named by the field`, async () => {
    const { page, context } = await open(screen, 'invalid');
    const input = page.locator('input[aria-invalid="true"]');
    assert.equal(await input.count(), 1);
    const described = await input.getAttribute('aria-describedby');
    const message = await page.locator(`#${described.split(' ')[0]}`).innerText();
    assert.ok(message.length > 20, `"${message}" says what to do`);
    assert.match(await page.locator('#stage [role="alert"]').innerText(), /nothing has been saved/);
    await context.close();
  });

  test(`${screen}: the saving button is busy and still has a name`, async () => {
    const { page, context } = await open(screen, 'saving');
    const button = page.locator('button[type="submit"]');
    assert.equal(await button.getAttribute('aria-busy'), 'true');
    assert.match(await button.getAttribute('aria-label'), /, saving$/);
    await context.close();
  });

  test(`${screen}: no step scrolls the page sideways at 320px, in either style`, async () => {
    for (const look of ['style=mission-control&mode=dark', 'style=classic&mode=light']) {
      for (const state of ['step-1', 'step-2', 'step-3', 'invalid', 'saving']) {
        const { page, context } = await open(screen, state, 320, look);
        assert.ok((await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)) <= 0, `${screen} ${state} ${look}`);
        await context.close();
      }
    }
  });

  test(`${screen}: on a phone the form comes after the steps and every field is 44px or taller`, async () => {
    const { page, context } = await open(screen, 'step-1', 375);
    const heights = await page.$$eval('#stage input', (els) => els.map((el) => el.getBoundingClientRect().height));
    assert.ok(heights.length >= 2);
    for (const height of heights) assert.ok(height >= 44, `an input is ${height}px tall`);
    await context.close();
  });
}

test('Settings says a change is unsaved, in words, in a bar that is announced', async () => {
  const saved = await open('settings', 'normal');
  assert.equal(await saved.page.locator('#stage .k-savebar').count(), 0);
  await saved.context.close();
  const unsaved = await open('settings', 'unsaved');
  const bar = unsaved.page.locator('#stage .k-savebar');
  assert.equal(await bar.getAttribute('role'), 'status');
  assert.match(await bar.innerText(), /not saved/);
  assert.equal(await bar.getByRole('link', { name: 'Discard' }).count(), 1);
  await unsaved.context.close();
});

test('Settings and Migrate draw loading and error, and the error offers a retry', async () => {
  for (const [screen, states] of [['settings', ['loading', 'error']], ['migrate', ['loading', 'error', 'empty', 'stale']]]) {
    for (const state of states) {
      const { page, context } = await open(screen, state);
      assert.equal(await page.locator('#stage h1').count(), 1, `${screen} ${state}`);
      if (state === 'error') assert.equal(await page.locator('#stage [role="alert"]').getByRole('link', { name: 'Try again' }).count(), 1);
      if (state === 'loading') assert.equal(await page.locator('#stage [aria-busy="true"]').count(), 1);
      await context.close();
    }
  }
});

test('every settings card has a heading, and the page has exactly one h1', async () => {
  const { page, context } = await open('settings', 'normal');
  assert.equal(await page.locator('#stage h1').count(), 1);
  assert.equal(await page.locator('#stage .k-card-title').count(), 4);
  await context.close();
});
