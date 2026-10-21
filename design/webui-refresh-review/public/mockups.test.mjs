// The review page in a real browser: what a person sees when they open it, and
// what the checker relies on when it drives it. The page is plain HTML over the
// product's own tokens, so the failure that matters most is the quiet one:
// opening it the wrong way and getting an unstyled page with no explanation.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { launch } from '../tools/lib/browser.mjs';
import { serve } from '../tools/serve.mjs';

const file = fileURLToPath(new URL('./mockups.html', import.meta.url));
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

async function open(hash = '', { colorScheme = 'light', width = 1440 } = {}) {
  const context = await browser.newContext({ colorScheme, viewport: { width, height: 900 } });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (message) => message.type() === 'error' && errors.push(message.text()));
  await page.goto(`${server.url}/review/mockups.html${hash}`);
  await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
  return { page, context, errors };
}

const choice = (page) =>
  page.evaluate(() => ({
    screen: document.getElementById('screen').value,
    concept: document.getElementById('concept').value,
    style: document.getElementById('style').value,
    mode: document.getElementById('mode').value,
    state: document.getElementById('state').value,
    theme: document.documentElement.getAttribute('data-theme'),
    uiStyle: document.documentElement.getAttribute('data-ui-style'),
    dataConcept: document.documentElement.getAttribute('data-concept'),
    hash: location.hash,
    rendered: document.documentElement.dataset.rendered,
  }));

test('opened from file:// the page explains how to open it, instead of showing nothing', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(`file://${file}`);
  const banner = page.locator('#assets-banner');
  await banner.waitFor({ state: 'visible' });
  assert.match(await banner.innerText(), /serve\.mjs/);
  assert.match(await banner.innerText(), /127\.0\.0\.1:8101/);
  await context.close();
});

test('served, the page has no banner and draws a screen with one h1', async () => {
  const { page, context, errors } = await open();
  assert.equal(await page.locator('#assets-banner').isVisible(), false);
  assert.equal(await page.locator('#stage h1').count(), 1);
  assert.deepEqual(errors, []);
  await context.close();
});

test('the controls, the root attributes and the hash all agree on the combination', async () => {
  const { page, context } = await open('#screen=signin&concept=b&style=classic&mode=light&state=sso');
  const now = await choice(page);
  assert.equal(now.concept, 'b');
  assert.equal(now.style, 'classic');
  assert.equal(now.state, 'sso');
  assert.equal(now.theme, 'light');
  // Classic is the absence of the attribute, exactly as the app does it.
  assert.equal(now.uiStyle, null);
  assert.equal(now.dataConcept, 'b');
  assert.equal(now.rendered, now.hash);
  await context.close();
});

test('Mission Control sets data-ui-style, as the app does', async () => {
  const { page, context } = await open('#style=mission-control&mode=dark');
  const now = await choice(page);
  assert.equal(now.uiStyle, 'mission-control');
  assert.equal(now.theme, 'dark');
  await context.close();
});

test('a junk hash lands on the defaults and raises no error', async () => {
  const { page, context, errors } = await open(
    '#screen=nope&concept=z&style=x&mode=y&state=%E0%A4%A<img src=x onerror=alert(1)>',
  );
  const now = await choice(page);
  assert.equal(now.screen, 'signin');
  assert.equal(now.concept, 'a');
  assert.equal(now.style, 'mission-control');
  assert.equal(await page.locator('#stage img[onerror]').count(), 0);
  assert.deepEqual(errors, []);
  await context.close();
});

test('with no hash the mode follows the browser, and the hash overrides it', async () => {
  const light = await open('', { colorScheme: 'light' });
  assert.equal((await choice(light.page)).mode, 'light');
  await light.context.close();
  const dark = await open('', { colorScheme: 'dark' });
  assert.equal((await choice(dark.page)).mode, 'dark');
  await dark.context.close();
  const forced = await open('#mode=light', { colorScheme: 'dark' });
  assert.equal((await choice(forced.page)).mode, 'light');
  await forced.context.close();
});

test('choosing in a control rewrites the hash and redraws the stage', async () => {
  const { page, context } = await open('#concept=a');
  await page.locator('#concept').selectOption('c');
  await page.waitForFunction(() => document.documentElement.dataset.concept === 'c');
  const now = await choice(page);
  assert.equal(now.concept, 'c');
  assert.match(now.hash, /concept=c/);
  assert.equal(now.rendered, now.hash);
  await context.close();
});

test('the current concept shows the capture as an image with a text alternative, and says so', async () => {
  const { page, context } = await open('#concept=current');
  const image = page.locator('#stage img');
  assert.match(await image.getAttribute('src'), /current-signin-1440\.png$/);
  assert.ok((await image.getAttribute('alt')).length > 20, 'the capture needs a real description');
  assert.match(await page.locator('#stage').innerText(), /capture/i);
  assert.equal(await page.locator('#stage h1').count(), 1);
  await context.close();
});

test('on a phone the current concept shows the phone capture', async () => {
  const { page, context } = await open('#concept=current', { width: 375 });
  assert.match(await page.locator('#stage img').evaluate((img) => img.currentSrc), /current-signin-375\.png$/);
  await context.close();
});

test('every control in the review bar has an accessible name, and the bar has no h1', async () => {
  const { page, context } = await open();
  for (const name of ['Screen', 'Concept', 'Style', 'Mode', 'State']) {
    assert.equal(await page.getByLabel(name, { exact: true }).count(), 1, name);
  }
  assert.equal(await page.locator('header h1').count(), 0);
  await context.close();
});

test('the page offers every concept and both styles and modes', async () => {
  const { page, context } = await open();
  const values = (id) => page.$$eval(`#${id} option`, (os) => os.map((o) => o.value));
  assert.deepEqual(await values('concept'), ['current', 'a', 'b', 'c']);
  assert.deepEqual(await values('style'), ['mission-control', 'classic']);
  assert.deepEqual(await values('mode'), ['light', 'dark']);
  await context.close();
});
