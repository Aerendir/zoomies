// The Overview mock and the shell around it: what the page says is what the
// fleet is, a phone can reach every page, and a state is never only a colour.
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

async function open(state, width = 1440, extra = '', screen = 'overview') {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const page = await context.newPage();
  const hash = `#screen=${screen}&concept=a&style=mission-control&mode=dark&state=${state}${extra}`;
  await page.goto(`${server.url}/signed-in/mockups.html${hash}`);
  await page.waitForFunction((h) => document.documentElement.dataset.rendered === h, hash);
  return { page, context };
}

test('the numbers on the Overview are the fleet the other pages list', async () => {
  const { page, context } = await open('attention');
  const counts = fleetCounts(FLEET);
  const values = await page.$$eval('.k-stat-value', (els) => els.map((el) => el.textContent.trim()));
  assert.deepEqual(values, [
    String(counts.runners.live),
    String(counts.jobs.running),
    String(counts.jobs.queued),
    `${counts.hosts.used}/${counts.hosts.slots}`,
  ]);
  const pools = await page.$$eval('.k-grid.wide .k-card-title', (els) => els.map((el) => el.textContent.trim()));
  assert.deepEqual(pools, FLEET.pools.map((p) => p.name));
  await context.close();
});

test('the Runners stat is the runners the pool cards add up to, and the Runners page says the same live number', async () => {
  const { page, context } = await open('attention');
  const stat = Number(await page.locator('.k-stat-value').first().innerText());
  const cards = await page.$$eval('.k-grid.wide .k-facts div:first-child dd', (els) => els.map((e) => Number(e.textContent.split(' of ')[0])));
  assert.equal(cards.reduce((a, b) => a + b, 0), stat, 'the pool cards and the stat count the same runners');
  await context.close();
  const runners = await open('rows', 1440, '', 'runners');
  assert.match(await runners.page.locator('.k-card-head .k-eyebrow').first().innerText(), new RegExp(`${stat} live`, 'i'));
  await runners.context.close();
  const link = await open('attention');
  assert.equal(await link.page.locator('.k-stat', { hasText: 'Jobs running' }).locator('a').getAttribute('href'), '#screen=jobs');
  await link.context.close();
});

test('the healthy state is the same fleet with what is wrong taken out, and says so', async () => {
  const { page, context } = await open('healthy');
  assert.match(await page.locator('.k-stage-headline').innerText(), /Everything is running/);
  assert.equal(await page.locator('.k-card-title', { hasText: 'Needs attention' }).count(), 0);
  const pools = await page.$$eval('.k-grid.wide .k-card-title', (els) => els.map((el) => el.textContent.trim()));
  assert.ok(pools.length < FLEET.pools.length, 'a pool on a down host or a disconnected installation is not healthy');
  await context.close();
});

test('every pill has a word and the six states draw six different shapes', async () => {
  const { page, context } = await open('attention');
  await page.evaluate(() => {
    const host = document.querySelector('.k-page');
    host.insertAdjacentHTML(
      'beforeend',
      ['idle', 'busy', 'pending', 'draining', 'danger', 'neutral']
        .map((state) => `<span class="k-pill" data-state="${state}" id="probe-${state}">${state}</span>`)
        .join(''),
    );
  });
  const shapes = await page.evaluate(() =>
    ['idle', 'busy', 'pending', 'draining', 'danger', 'neutral'].map((state) => {
      const style = getComputedStyle(document.getElementById(`probe-${state}`), '::before');
      return [style.clipPath, style.borderRadius, style.borderTopWidth, style.height].join('|');
    }),
  );
  assert.equal(new Set(shapes).size, 6, `the shapes are ${shapes.join(' / ')}`);
  const empty = await page.$$eval('.k-pill', (els) => els.filter((el) => !el.textContent.trim()).length);
  assert.equal(empty, 0, 'a pill with no word is a colour alone');
  await context.close();
});

test('on a phone the bar keeps four pages and the More sheet opens, closes on Escape and gives focus back', async () => {
  const { page, context } = await open('attention', 375);
  assert.equal(await page.locator('.k-bar .k-bar-item').count(), 5);
  assert.equal(await page.locator('.k-side').isVisible(), false);
  const more = page.getByRole('button', { name: 'More pages' });
  assert.ok((await more.boundingBox()).height >= 44, 'the bar is a touch target');
  await more.focus();
  await page.keyboard.press('Enter');
  const sheet = page.locator('#k-more');
  await sheet.waitFor({ state: 'visible' });
  assert.equal(await sheet.locator('a').count(), 14, 'every page is in the sheet');
  await page.keyboard.press('Escape');
  await sheet.waitFor({ state: 'hidden' });
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('aria-label')), 'More pages');
  await context.close();
});

test('a table-less page does not scroll sideways at 320px, in any state', async () => {
  for (const state of ['attention', 'healthy', 'first-run', 'loading', 'error', 'stale', 'collapsed']) {
    const { page, context } = await open(state, 320);
    const wide = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
    assert.ok(wide <= 0, `${state} is ${wide}px too wide`);
    await context.close();
  }
});

test('loading says it is loading and the failure offers a retry', async () => {
  const loading = await open('loading');
  assert.equal(await loading.page.locator('#stage [aria-busy="true"]').count(), 1);
  assert.match(await loading.page.locator('#stage [role="status"]').innerText(), /Loading the fleet/);
  await loading.context.close();
  const failed = await open('error');
  const alert = failed.page.locator('#stage [role="alert"]');
  assert.match(await alert.innerText(), /did not answer/);
  assert.equal(await alert.getByRole('link', { name: 'Try again' }).count(), 1);
  await failed.context.close();
});
