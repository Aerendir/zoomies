// The detail pages (a pool, a host, a runner): the thing as it usually is, as it
// is when something is wrong with it, and the three states that are not about the
// thing at all. And that what they say is what the lists say.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from '../tools/lib/browser.mjs';
import { serve } from '../tools/serve.mjs';
import { FLEET, fleetCounts } from './fixture.js';

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

async function open(screen, state, width = 1440) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const page = await context.newPage();
  const hash = `#screen=${screen}&concept=a&style=mission-control&mode=dark&state=${state}`;
  await page.goto(`${server.url}/signed-in/mockups.html${hash}`);
  await page.waitForFunction((h) => document.documentElement.dataset.rendered === h, hash);
  return { page, context };
}

const DETAILS = {
  'pool-detail': ['working', 'disconnected'],
  'host-detail': ['healthy', 'down'],
  'runner-detail': ['busy', 'failed'],
};

for (const [screen, variants] of Object.entries(DETAILS)) {
  test(`${screen}: each variant has an h1 and the sidebar keeps the page it belongs to`, async () => {
    for (const state of variants) {
      const { page, context } = await open(screen, state);
      assert.equal(await page.locator('#stage h1').count(), 1);
      assert.equal(await page.locator('#stage .k-side .k-nav-item[aria-current="page"]').count(), 1);
      await context.close();
    }
  });

  test(`${screen}: loading, error with a retry, and not found are each drawn`, async () => {
    const loading = await open(screen, 'loading');
    assert.equal(await loading.page.locator('#stage [aria-busy="true"]').count(), 1);
    await loading.context.close();
    const error = await open(screen, 'error');
    assert.equal(await error.page.locator('#stage [role="alert"]').getByRole('link', { name: 'Try again' }).count(), 1);
    await error.context.close();
    const missing = await open(screen, 'missing');
    assert.match(await missing.page.locator('#stage .k-empty-title').innerText(), /is not here/);
    assert.equal(await missing.page.locator('#stage .k-card a', { hasText: /^(Pools|Hosts|Runners)$/ }).count(), 1);
    await missing.context.close();
  });

  test(`${screen}: nothing scrolls the page sideways at 320px`, async () => {
    for (const state of [...variants, 'loading', 'error', 'missing']) {
      const { page, context } = await open(screen, state, 320);
      assert.ok((await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)) <= 0, `${screen} ${state}`);
      await context.close();
    }
  });
}

test('the pool page lists the runners the Runners page lists for that pool, and says how many', async () => {
  const { page, context } = await open('pool-detail', 'working');
  const own = FLEET.runners.filter((r) => r.pool_id === 'pool_build' && r.state !== 'removed');
  const names = await page.$$eval('.k-table tbody tr td:first-child', (els) => els.map((e) => e.textContent.trim()));
  assert.deepEqual(names, own.map((r) => r.name));
  assert.equal(await page.locator('.k-stat-value').first().innerText(), String(fleetCounts(FLEET).pools.pool_build.runners));
  await context.close();
});

test('a host that is not reporting says so in words, above the numbers', async () => {
  const { page, context } = await open('host-detail', 'down');
  assert.match(await page.locator('#stage [role="alert"]').innerText(), /last reported/);
  assert.equal(await page.locator('#stage .k-pill', { hasText: 'Not reporting' }).count(), 1);
  await context.close();
});

test('a failed runner says what happened and does not claim a job', async () => {
  const { page, context } = await open('runner-detail', 'failed');
  assert.match(await page.locator('#stage [role="alert"]').innerText(), /exited before it registered/);
  assert.match(await page.locator('#stage').innerText(), /never registered/);
  await context.close();
});
