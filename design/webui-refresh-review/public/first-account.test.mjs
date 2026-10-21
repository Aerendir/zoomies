// The first account form is the first thing an operator meets after
// `docker compose up`, so its first sight matters most: the capture of today's
// page puts the Create button's bottom edge at 851px on a 1440x900 screen, and
// the phone's token field 511px down. Concept A adds the install commands under
// the token field, which costs height, and promised to take it back by cutting
// the introduction. This holds it to that.
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

/** Where an element sits within the stage, which is the page without the review bar. */
async function within(concept, width, height, selector) {
  const context = await browser.newContext({ viewport: { width, height }, colorScheme: 'dark' });
  const page = await context.newPage();
  await page.goto(
    `${server.url}/review/mockups.html#screen=first-account&concept=${concept}&style=mission-control&mode=dark&state=empty`,
  );
  await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
  const where = await page.evaluate((sel) => {
    const stage = document.getElementById('stage').getBoundingClientRect();
    const box = [...document.querySelectorAll(sel)].find((el) => el.checkVisibility()).getBoundingClientRect();
    return { top: box.top - stage.top, bottom: box.bottom - stage.top };
  }, selector);
  await context.close();
  return where;
}

test('concept A keeps the Create button inside a 1440x900 screen', async () => {
  const create = await within('a', 1440, 900, 'button[type="submit"]');
  assert.ok(create.bottom <= 900, `the button ends at ${Math.round(create.bottom)}px`);
});

test('on a phone, concept A asks for the token higher up than today does (511px)', async () => {
  const token = await within('a', 375, 812, '#f-setup-token');
  assert.ok(token.top < 511, `the token field starts at ${Math.round(token.top)}px`);
});

test('in concept A the commands that find the token are visible under the token field', async () => {
  const field = await within('a', 1440, 900, '#f-setup-token');
  const command = await within('a', 1440, 900, '.m-install-panel .m-command');
  assert.ok(command.top > field.bottom && command.top - field.bottom < 150, 'the command sits just below the field');
});
