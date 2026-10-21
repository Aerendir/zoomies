// What the kit promises that a page's own test would not think to ask: that a
// tile is a tile, that a sentence is not cut off, that a clipped name can be read
// without a mouse, that a thumb can hit what is on a phone, that the More sheet
// takes focus, and that the stylesheet uses the product's own breakpoint and layers.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { launch } from '../tools/lib/browser.mjs';
import { serve } from '../tools/serve.mjs';

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

const css = readFileSync(fileURLToPath(new URL('./kit.css', import.meta.url)), 'utf8');

test('an icon tile is a tile: bigger than the glyph it holds, with the glyph inside it', async () => {
  const { page, context } = await open('overview', 'attention');
  const { tile, glyph } = await page.evaluate(() => {
    const t = document.querySelector('.k-tile');
    return { tile: t.getBoundingClientRect().width, glyph: t.querySelector('svg').getBoundingClientRect().width };
  });
  assert.ok(tile >= 32, `the tile is ${tile}px`);
  assert.ok(tile > glyph + 8, `the tile is ${tile}px around a ${glyph}px glyph`);
  await context.close();
});

for (const screen of ['queue', 'repairs', 'kennel', 'installations', 'ai-context', 'providers']) {
  test(`${screen}: a sentence is not cut off at 1440 with room to spare`, async () => {
    const { page, context } = await open(screen, screen === 'repairs' || screen === 'kennel' || screen === 'ai-context' ? 'normal' : 'rows');
    const cut = await page.$$eval('.k-table .k-clip', (els) =>
      els
        .filter((el) => el.title.trim().split(/\s+/).length >= 6 && el.scrollWidth > el.clientWidth + 1)
        .map((el) => el.title),
    );
    assert.deepEqual(cut, [], 'sentences the page exists to give are clipped');
    await context.close();
  });
}

test('a clipped name is read in full when its row is focused, without a mouse', async () => {
  const { page, context } = await open('jobs');
  const row = page.locator('.k-table tbody tr', { has: page.locator('.k-clip[title^="acme-corp/a-repository"]') });
  const clip = row.locator('.k-clip[title^="acme-corp/a-repository"]');
  assert.ok((await clip.evaluate((el) => el.scrollWidth - el.clientWidth)) > 0, 'it is clipped at rest');
  await row.getByRole('link', { name: /^Open/ }).focus();
  assert.ok((await clip.evaluate((el) => el.scrollWidth - el.clientWidth)) <= 1, 'focusing the row shows all of it');
  await context.close();
});

test('on a phone, filters, page actions, list buttons and search are 44px or more', async () => {
  for (const [screen, state, selector] of [
    ['runners', 'rows', '.k-filter'],
    ['runners', 'rows', '.k-page-actions .m-btn'],
    ['overview', 'attention', '.k-list .m-btn'],
    ['overview', 'attention', '.k-search'],
  ]) {
    const { page, context } = await open(screen, state, 375);
    const heights = await page.$$eval(`#stage ${selector}`, (els) => els.map((el) => el.getBoundingClientRect().height));
    assert.ok(heights.length > 0, `${selector} on ${screen}`);
    for (const height of heights) assert.ok(height >= 44, `${selector} is ${height}px tall on ${screen}`);
    await context.close();
  }
});

test('the buttons in the Overview problem list are each named for their problem', async () => {
  const { page, context } = await open('overview', 'attention');
  const names = await page.$$eval('.k-list .m-btn', (els) => els.map((el) => el.getAttribute('aria-label')));
  assert.ok(names.length >= 2);
  assert.equal(new Set(names).size, names.length, 'every one has its own name');
  for (const name of names) assert.match(name, /^Open .{10,}/);
  await context.close();
});

test('opening the More sheet moves focus into it, and the page behind it is dimmed', async () => {
  const { page, context } = await open('overview', 'attention', 375);
  await page.getByRole('button', { name: 'More pages' }).click();
  await page.locator('#k-more').waitFor({ state: 'visible' });
  assert.equal(await page.evaluate(() => document.getElementById('k-more').contains(document.activeElement)), true);
  const backdrop = await page.evaluate(() => getComputedStyle(document.getElementById('k-more'), '::backdrop').backgroundColor);
  assert.notEqual(backdrop, 'rgba(0, 0, 0, 0)', 'a sheet over a page that looks unchanged is not a sheet');
  await context.close();
});

test('the stylesheet uses the product’s phone breakpoint and its layers, not numbers of its own', () => {
  assert.doesNotMatch(css, /max-width:\s*800px/, 'the product’s phone breakpoint is 768px');
  assert.doesNotMatch(css, /z-index:\s*\d/, 'a layer is one of the --z-layer tokens');
});
