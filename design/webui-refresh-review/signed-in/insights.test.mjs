// The insight and repair pages (Usage, Comparisons, Kennel Club, Eli repairs, AI
// context): one answer in a stage, the evidence under it, and the same states a
// list has. And that the answer is the evidence: the sentence in the stage is
// computed from the same rows the table below it lists.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from '../tools/lib/browser.mjs';
import { serve } from '../tools/serve.mjs';
import { FLEET } from './fixture.js';

const PAGES = ['usage', 'comparisons', 'kennel', 'repairs', 'ai-context'];
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

async function open(screen, state = 'normal', width = 1440, look = 'style=mission-control&mode=dark') {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const page = await context.newPage();
  const hash = `#screen=${screen}&concept=a&${look}&state=${state}`;
  await page.goto(`${server.url}/signed-in/mockups.html${hash}`);
  await page.waitForFunction((h) => document.documentElement.dataset.rendered === h, hash);
  return { page, context };
}

for (const screen of PAGES) {
  test(`${screen}: the stage answers, and loading, empty, error with a retry and stale are each drawn`, async () => {
    const normal = await open(screen);
    assert.equal(await normal.page.locator('#stage .k-stage').count(), 1);
    assert.equal(await normal.page.locator('#stage h1').count(), 1);
    assert.equal(await normal.page.locator('#stage .k-stage h1').count(), 0, 'the stage is not the page heading');
    await normal.context.close();

    const loading = await open(screen, 'loading');
    assert.equal(await loading.page.locator('#stage [aria-busy="true"]').count(), 1);
    assert.equal(await loading.page.locator('#stage .k-stage').count(), 0);
    await loading.context.close();

    const empty = await open(screen, 'empty');
    assert.equal(await empty.page.locator('#stage .k-empty').count(), 1);
    await empty.context.close();

    const error = await open(screen, 'error');
    assert.equal(await error.page.locator('#stage [role="alert"]').getByRole('link', { name: 'Try again' }).count(), 1);
    await error.context.close();

    const stale = await open(screen, 'stale');
    assert.equal(await stale.page.locator('#stage .k-stage').count(), 1, 'stale means the answer is still there');
    assert.match(await stale.page.locator('#stage .k-stale').innerText(), /last refresh/);
    await stale.context.close();
  });

  test(`${screen}: no state scrolls the page sideways at 320px, in either style`, async () => {
    for (const look of ['style=mission-control&mode=dark', 'style=classic&mode=light']) {
      for (const state of ['normal', 'loading', 'empty', 'error', 'stale']) {
        const { page, context } = await open(screen, state, 320, look);
        assert.ok((await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)) <= 0, `${screen} ${state} ${look}`);
        await context.close();
      }
    }
  });
}

test('the Kennel Club stage counts the repositories that need you, as the cards under it show them', async () => {
  const { page, context } = await open('kennel');
  const needing = FLEET.kennelRepositories.filter((r) => r.state === 'attention').length;
  assert.match(await page.locator('.k-stage-headline').innerText(), new RegExp(`^${needing} of ${FLEET.kennelRepositories.length} repositories`));
  const cards = await page.locator('.k-grid.wide .k-card').count();
  assert.equal(cards, FLEET.kennelRepositories.length);
  const rows = await page.locator('.k-table tbody tr').count();
  assert.equal(rows, FLEET.kennelRepositories.reduce((n, r) => n + r.findings.length, 0));
  const firstSeverity = await page.locator('.k-table tbody tr:first-child .k-pill').innerText();
  assert.equal(firstSeverity, 'Error', 'errors come first');
  await context.close();
});

test('the Usage page adds up: the stat is the sum of the days under it', async () => {
  const { page, context } = await open('usage');
  const started = FLEET.usage.reduce((n, d) => n + d.started, 0);
  assert.equal(await page.locator('.k-stat-value').first().innerText(), String(started));
  assert.equal(await page.locator('.k-table tbody tr').count(), FLEET.usage.length);
  await context.close();
});

test('the Eli repairs stage counts what is in flight and what failed, from the table below', async () => {
  const { page, context } = await open('repairs');
  const active = FLEET.repairs.filter((r) => ['working', 'checking'].includes(r.state)).length;
  const failed = FLEET.repairs.filter((r) => r.state === 'failed').length;
  const text = await page.locator('.k-stage-headline').innerText();
  assert.ok(text.includes(`${active} repairs are in flight`), text);
  assert.ok(text.includes(`${failed} has failed`), text);
  assert.equal(await page.locator('.k-table tbody tr').count(), FLEET.repairs.length);
  await context.close();
});

test('every AI context row names the commit it was prepared from, in full', async () => {
  const { page, context } = await open('ai-context');
  const commits = await page.$$eval('.k-table tbody tr td:nth-child(3)', (els) => els.map((e) => e.textContent.trim()));
  assert.deepEqual(commits, FLEET.contexts.map((c) => c.freshness.published_commit));
  await context.close();
});
