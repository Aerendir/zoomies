// The fleet status page is read by people with no account, often on a phone,
// by somebody whose job has not started. What is worth holding is that the
// state survives without colour, that the cause is reachable, and that nothing
// on the page is a number the status endpoint does not send.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { launch } from '../tools/lib/browser.mjs';
import { serve } from '../tools/serve.mjs';
import screen from './screens/fleet-status.js';

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

const SHAPES = { healthy: 'hollow', degraded: 'dashed', blocked: 'triangle' };

async function open(concept, state, { width = 1440, height = 900, forcedColors = 'none' } = {}) {
  const context = await browser.newContext({ viewport: { width, height }, colorScheme: 'dark', forcedColors });
  const page = await context.newPage();
  await page.goto(
    `${server.url}/review/mockups.html#screen=fleet-status&concept=${concept}&style=classic&mode=dark&state=${state}`,
  );
  await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
  return { page, context };
}

/** What a reader has to tell the three states apart by, read from the page as drawn. */
const read = (page) =>
  page.evaluate(() => {
    const svg = document.querySelector('#stage .m-shape');
    const word = document.querySelector('#stage .m-state-word');
    const style = getComputedStyle(svg);
    const box = svg.getBoundingClientRect();
    const mark = svg.querySelector('circle, path');
    return {
      shape: svg.dataset.shape,
      word: word.textContent.trim().toLowerCase(),
      wordVisible: word.checkVisibility(),
      width: box.width,
      drawn: style.stroke !== 'none' || style.fill !== 'none',
      // What differs between the three outlines without a colour to tell them by.
      signature: `${svg.dataset.shape}|${getComputedStyle(mark).strokeDasharray}|${style.fill === 'none'}`,
    };
  });

for (const forcedColors of ['none', 'active']) {
  for (const concept of ['a', 'b', 'c']) {
    test(`concept ${concept}${forcedColors === 'active' ? ' in forced colours' : ''}: the state is a word and a shape, and the three shapes differ`, async () => {
      const seen = new Set();
      for (const [state, shape] of Object.entries(SHAPES)) {
        const { page, context } = await open(concept, state, { forcedColors });
        if (forcedColors === 'active') {
          assert.equal(await page.evaluate(() => matchMedia('(forced-colors: active)').matches), true);
        }
        const got = await read(page);
        assert.equal(got.shape, shape, `${concept} ${state}`);
        assert.equal(got.word, state);
        assert.ok(got.wordVisible, 'the word is on the page');
        assert.ok(got.width >= 20 && got.drawn, `the shape is ${got.width}px and drawn: ${got.drawn}`);
        seen.add(got.signature);
        await context.close();
      }
      assert.equal(seen.size, 3, 'a triangle, a dashed ring and a hollow ring are told apart without colour');
    });
  }
}

test('the screen draws only what the status endpoint returns', () => {
  const spec = readFileSync(fileURLToPath(new URL('../../../api/openapi.yaml', import.meta.url)), 'utf8');
  const block = (name) => {
    const start = spec.indexOf(`\n    ${name}:`);
    const end = spec.slice(start + 1).search(/\n    [A-Za-z]+:|\n\s*# ---/);
    return spec.slice(start, start + 1 + end);
  };
  const properties = (text) => [...text.matchAll(/^ {8}([a-z0-9_]+):/gm)].map((m) => m[1]);
  const status = properties(block('FleetStatus'));
  const reason = properties(block('FleetStatusReason'));
  for (const field of screen.uses) assert.ok(status.includes(field), `${field} is not in FleetStatus (${status})`);
  for (const field of screen.reasonUses) assert.ok(reason.includes(field), `${field} is not in FleetStatusReason (${reason})`);
  // And the three states the page offers are the three the API can say.
  const enumLine = /state:\n\s+type: string\n\s+enum: \[([^\]]+)\]/.exec(block('FleetStatus'))[1];
  assert.deepEqual(enumLine.split(',').map((x) => x.trim()).sort(), Object.keys(SHAPES).sort());
});

test('A: on a phone a blocked fleet shows its blocking causes on the first screen, in under half the height of today', async () => {
  // Today's page is 2080px at 375: its two blocking causes and ten lesser ones are one undivided list.
  const { page, context } = await open('a', 'blocked', { width: 375, height: 812 });
  const m = await page.evaluate(() => {
    const stage = document.getElementById('stage').getBoundingClientRect();
    const blocking = [...document.querySelectorAll('#stage .m-reason.sev-error')];
    return { height: stage.height, lastBlockingBottom: blocking.at(-1).getBoundingClientRect().bottom - stage.top, blocking: blocking.length };
  });
  assert.equal(m.blocking, 2);
  assert.ok(m.lastBlockingBottom <= 812, `the last blocking cause ends at ${Math.round(m.lastBlockingBottom)}px`);
  assert.ok(m.height < 1040, `the page is ${Math.round(m.height)}px tall`);
  await context.close();
});

test('A: the long group is folded and the short one is not', async () => {
  const blocked = await open('a', 'blocked');
  assert.equal(await blocked.page.locator('details.fs-group').count(), 1);
  assert.equal(await blocked.page.locator('details.fs-group[open]').count(), 0);
  await blocked.context.close();
  const degraded = await open('a', 'degraded');
  assert.equal(await degraded.page.locator('details.fs-group').count(), 0, 'three reasons need no folding');
  await degraded.context.close();
});

test('C: on a desktop the state and the queue are held in view beside the reasons', async () => {
  const { page, context } = await open('c', 'blocked');
  assert.equal(await page.locator('.fs-c-hold').evaluate((el) => getComputedStyle(el).position), 'sticky');
  await context.close();
  const phone = await open('c', 'blocked', { width: 375, height: 812 });
  assert.equal(await phone.page.locator('.fs-c-hold').evaluate((el) => getComputedStyle(el).position), 'static');
  await phone.context.close();
});

test('B: the cause is behind a disclosure, which is its trade-off and says how many there are', async () => {
  const { page, context } = await open('b', 'blocked');
  assert.equal(await page.locator('details.fs-b-more[open]').count(), 0);
  assert.match(await page.locator('details.fs-b-more > summary').innerText(), /12/);
  await context.close();
});

test('stale shows the last thing the fleet said and says so, in a status role', async () => {
  for (const concept of ['a', 'b', 'c']) {
    const { page, context } = await open(concept, 'stale');
    assert.match(await page.getByRole('status').innerText(), /Could not refresh/);
    assert.equal((await read(page)).word, 'healthy');
    await context.close();
  }
});
