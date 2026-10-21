// The one rule every concept of this page keeps: it never says what the address
// was. A path taken from the URL is text anybody can put in a link, so repeating
// it turns a wrong address into a way to write words on this controller's page.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { launch } from '../tools/lib/browser.mjs';
import { serve } from '../tools/serve.mjs';
import screen from './screens/not-found.js';

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

async function open(concept, hash = '') {
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: 'dark' });
  const page = await context.newPage();
  await page.goto(`${server.url}/review/mockups.html#screen=not-found&concept=${concept}&style=mission-control&mode=dark${hash}`);
  await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
  return { page, context };
}

test('no concept reads the address, in its markup or on the page it draws', async () => {
  const MARKER = 'SECRET-PATH-FROM-A-LINK';
  for (const concept of ['a', 'b', 'c']) {
    // The markup never reaches for the address...
    assert.doesNotMatch(screen.render(concept, 'not-found'), /location|pathname|document\.URL|referrer/i, concept);
    // ...and a hash that carries a marker never reaches the page.
    const { page, context } = await open(concept, `&state=${MARKER}&extra=${MARKER}`);
    assert.doesNotMatch(await page.locator('#stage').innerText(), new RegExp(MARKER), concept);
    assert.equal(await page.locator('#stage').evaluate((el, m) => el.innerHTML.includes(m), MARKER), false, concept);
    await context.close();
  }
});

test('A keeps the way forward and adds the way back', async () => {
  const { page, context } = await open('a');
  assert.equal(await page.getByRole('link', { name: 'Go to the overview' }).count(), 1);
  assert.equal(await page.getByRole('button', { name: 'Go back' }).count(), 1);
  await context.close();
});

test("B offers the three destinations the phone's bottom bar keeps, with the navigation's own sentences", async () => {
  const sections = readFileSync(fileURLToPath(new URL('../../../web/src/lib/shell/sections.ts', import.meta.url)), 'utf8');
  const { page, context } = await open('b');
  const nav = page.getByRole('navigation', { name: 'Where to go next' });
  assert.equal(await nav.getByRole('link').count(), 3);
  for (const label of ['Overview', 'Pools', 'Runners']) {
    const text = await nav.getByRole('link', { name: new RegExp(`^${label}`) }).innerText();
    const hint = /hint: '([^']+)'/g;
    const known = [...sections.matchAll(hint)].map((m) => m[1]);
    assert.ok(known.some((sentence) => text.includes(sentence)), `${label}'s sentence is not the navigation's (${text})`);
  }
  await context.close();
});

test('C is the mark and one action, with a single heading and nothing else to press', async () => {
  const { page, context } = await open('c');
  assert.equal(await page.locator('#stage h1').count(), 1);
  // The appearance toggle is the shell's, in every frame; the page's own actions are the hero's.
  assert.equal(await page.locator('#stage .nf-hero .m-btn').count(), 1);
  const pressable = await page.locator('#stage .nf-hero').locator('a, button').evaluateAll((els) => els.filter((e) => e.checkVisibility()).length);
  assert.equal(pressable, 1);
  await context.close();
});
