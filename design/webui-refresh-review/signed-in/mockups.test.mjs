// The second pack uses the same review page code under its own registry, and
// the thing to hold is that it did not change what the first one does: a pack
// names its screens and its concepts, the page offers exactly those, and an
// address naming a concept the pack does not have lands on the one it does.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
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

async function open(hash = '') {
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto(`${server.url}/signed-in/mockups.html${hash}`);
  await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
  return { page, context, errors };
}

const options = (page, id) =>
  page.$$eval(`#${id} option`, (list) => list.map((o) => [o.value, o.textContent]));

test('the page offers Current and the portal style, and the screens its registry names', async () => {
  const { page, context, errors } = await open();
  assert.deepEqual(await options(page, 'concept'), [
    ['current', 'Current (capture)'],
    ['a', 'Portal style'],
  ]);
  const screens = (await options(page, 'screen')).map(([id]) => id);
  assert.ok(screens.includes('overview'), `the screens are ${screens}`);
  assert.deepEqual(errors, []);
  await context.close();
});

test('an address naming a concept this pack does not have lands on the portal style', async () => {
  const { page, context } = await open('#screen=overview&concept=c');
  assert.equal(await page.locator('#concept').inputValue(), 'a');
  assert.equal(await page.evaluate(() => document.documentElement.dataset.concept), 'a');
  await context.close();
});

test("pack 1's page still offers its own three concepts and Current", async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(`${server.url}/review/mockups.html`);
  await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
  assert.deepEqual(
    (await options(page, 'concept')).map(([id]) => id),
    ['current', 'a', 'b', 'c'],
  );
  await context.close();
});

test('the review page offers exactly the screens the capture script knows a state for', async () => {
  const { SIGNED_IN_STATE } = await import('../tools/capture.mjs');
  const { page, context } = await open();
  const screens = (await options(page, 'screen')).map(([id]) => id);
  assert.deepEqual([...screens].sort(), Object.keys(SIGNED_IN_STATE).sort());
  await context.close();
});
