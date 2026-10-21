// The four monitoring pages (Runners, Queue, Jobs, Workflows) and the cases a
// table has to survive: a 300-character name, a 120-character label, a phone,
// and a list that is loading, empty, failed, or showing rows from before a
// failed refresh.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from '../tools/lib/browser.mjs';
import { serve } from '../tools/serve.mjs';
import { FLEET } from './fixture.js';

const PAGES = ['runners', 'queue', 'jobs', 'workflows'];
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

async function open(screen, state = 'rows', width = 1440) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const page = await context.newPage();
  const hash = `#screen=${screen}&concept=a&style=mission-control&mode=dark&state=${state}`;
  await page.goto(`${server.url}/signed-in/mockups.html${hash}`);
  await page.waitForFunction((h) => document.documentElement.dataset.rendered === h, hash);
  return { page, context };
}

for (const screen of PAGES) {
  test(`${screen} draws all five states: rows, loading, empty, error with a retry, stale rows`, async () => {
    const rows = await open(screen, 'rows');
    assert.equal(await rows.page.locator('#stage table').count(), 1);
    await rows.context.close();

    const loading = await open(screen, 'loading');
    assert.equal(await loading.page.locator('#stage [aria-busy="true"]').count(), 1);
    assert.equal(await loading.page.locator('#stage table').count(), 0);
    await loading.context.close();

    const empty = await open(screen, 'empty');
    assert.equal(await empty.page.locator('#stage .k-empty').count(), 1);
    assert.equal(await empty.page.locator('#stage table').count(), 0);
    await empty.context.close();

    const error = await open(screen, 'error');
    const alert = error.page.locator('#stage [role="alert"]');
    assert.equal(await alert.count(), 1);
    assert.equal(await alert.getByRole('link', { name: 'Try again' }).count(), 1);
    await error.context.close();

    const stale = await open(screen, 'stale');
    assert.equal(await stale.page.locator('#stage table').count(), 1, 'stale means the rows are still there');
    assert.match(await stale.page.locator('#stage .k-stale').innerText(), /did not answer the last refresh/);
    await stale.context.close();
  });

  test(`${screen}: the table scrolls inside its card on a phone, never the page, and the region can be reached by keyboard`, async () => {
    const { page, context } = await open(screen, 'rows', 375);
    const measure = await page.evaluate(() => {
      const region = document.querySelector('.k-scroll');
      return {
        page: document.documentElement.scrollWidth - innerWidth,
        scrolls: region.scrollWidth > region.clientWidth,
        tabindex: region.getAttribute('tabindex'),
        name: region.getAttribute('aria-label'),
        role: region.getAttribute('role'),
      };
    });
    assert.ok(measure.page <= 0, `the page is ${measure.page}px wider than the window`);
    assert.ok(measure.scrolls, 'the table is wider than its card, so the card scrolls');
    assert.deepEqual([measure.tabindex, measure.role], ['0', 'region']);
    assert.ok(measure.name, 'a scrolling region needs a name');
    await context.close();
  });

  test(`${screen}: every row action is a button 44px tall on a phone and is named for its row`, async () => {
    const { page, context } = await open(screen, 'rows', 375);
    const actions = await page.$$eval('.k-table .m-btn', (els) =>
      els.map((el) => ({ height: el.getBoundingClientRect().height, name: el.getAttribute('aria-label') })),
    );
    assert.ok(actions.length > 0);
    for (const action of actions) {
      assert.ok(action.height >= 44, `${action.name} is ${action.height}px tall`);
      assert.match(action.name, /^Open .+/);
    }
    assert.equal(new Set(actions.map((a) => a.name)).size, actions.length, 'every action has its own name');
    await context.close();
  });
}

test('a 300-character job name is held to a line and every character of it is in the title', async () => {
  const { page, context } = await open('jobs', 'rows', 320);
  const longest = FLEET.jobs.reduce((a, j) => (j.repo.length > a.length ? j.repo : a), '');
  const clipped = await page.$$eval('.k-clip', (els, text) => {
    const el = els.find((e) => e.title === text);
    if (!el) return null;
    const style = getComputedStyle(el);
    return { wraps: style.whiteSpace, ellipsis: style.textOverflow, height: el.getBoundingClientRect().height };
  }, longest);
  assert.ok(clipped, 'the 300-character repository is in the table with its full text as the title');
  assert.equal(clipped.wraps, 'nowrap');
  assert.equal(clipped.ellipsis, 'ellipsis');
  assert.ok(clipped.height < 40, `it is ${clipped.height}px tall, so it did not wrap`);
  assert.ok((await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)) <= 0);
  await context.close();
});

test('the 120-character runner label is in the table and its full text is reachable', async () => {
  const { page, context } = await open('runners', 'rows', 320);
  const label = FLEET.runners.flatMap((r) => r.labels).find((l) => l.length >= 120);
  const titles = await page.$$eval('.k-clip', (els) => els.map((e) => e.title));
  assert.ok(titles.some((t) => t.includes(label)), 'the label is the title of its cell');
  await context.close();
});

test('every queued job says why it is waiting, from the pool it is waiting for', async () => {
  const { page, context } = await open('queue');
  const queued = FLEET.jobs.filter((j) => j.state === 'queued');
  const reasons = await page.$$eval('.k-table tbody tr td:nth-child(5) .k-clip', (els) => els.map((e) => e.title));
  assert.equal(reasons.length, queued.length);
  for (const reason of reasons) assert.match(reason, /\.$/, `"${reason}" is a sentence`);
  const gpu = queued.find((j) => j.pool_id === 'pool_gpu');
  assert.ok(gpu);
  assert.ok(reasons.some((r) => /starting in linux-gpu/.test(r)), reasons.join(' | '));
  await context.close();
});

test('the workflow run count on a row is the jobs Jobs lists for that run', async () => {
  const { page, context } = await open('workflows');
  const counts = await page.$$eval('.k-table tbody tr td:nth-child(5)', (els) => els.map((e) => Number(e.textContent)));
  assert.deepEqual(counts, FLEET.runs.map((r) => r.jobs));
  assert.equal(counts.reduce((a, b) => a + b, 0), FLEET.jobs.length);
  await context.close();
});
