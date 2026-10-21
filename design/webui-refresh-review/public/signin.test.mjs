// Sign in is the page the most people see, and the one that carries the most
// states, so what each concept promises about it is held here and not left to the
// checker's sweep alone: the sweep proves nothing overflows, and these prove the
// things that are the point of each concept still happen.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from '../tools/lib/browser.mjs';
import { checkPage, watch } from '../tools/check.mjs';
import { serve } from '../tools/serve.mjs';
import screen from './screens/signin.js';

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

async function open(concept, state, width = 1440, height = 900) {
  const context = await browser.newContext({ viewport: { width, height }, colorScheme: 'dark' });
  const page = await context.newPage();
  watch(page);
  await page.goto(
    `${server.url}/review/mockups.html#screen=signin&concept=${concept}&style=mission-control&mode=dark&state=${state}`,
  );
  await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
  return { page, context };
}

test('every state of every concept has exactly one level one heading', () => {
  assert.equal(screen.states.length, 10, 'the spec asks for ten states');
  for (const concept of ['a', 'b', 'c']) {
    for (const state of screen.states) {
      const headings = screen.render(concept, state).match(/<h1[\s>]/g) ?? [];
      assert.equal(headings.length, 1, `${concept} ${state} has ${headings.length} headings`);
    }
  }
});

test('a host name of a hundred and twenty-eight characters wraps in every concept at 320px', async () => {
  // The two states that name the host on a bad day: it is exactly when a name
  // this long is on the screen, and when the page can least afford to break.
  // "Wraps" means neither scrolls sideways nor is cut off: concept C's root
  // clips, so the page's own width says nothing about whether the host was seen.
  for (const concept of ['a', 'b', 'c']) {
    for (const state of ['rate-limited', 'unreachable']) {
      const { page, context } = await open(concept, state, 320, 800);
      assert.match(await page.locator('#stage').innerText(), /controller0123456789/, `${concept} ${state} shows the long host`);
      const found = (await checkPage(page, { layoutOnly: true })).filter((f) => f.kind === 'overflow' || f.kind === 'clipped');
      assert.deepEqual(found, [], `${concept} ${state}: ${found.map((f) => f.detail).join('; ')}`);
      await context.close();
    }
  }
});

test('A: a phone gets one compact mark in place of the black stage, and a desktop the other way round', async () => {
  const phone = await open('a', 'password', 375, 812);
  const onPhone = await phone.page.evaluate(() => ({
    stage: document.querySelector('.a-stage').checkVisibility(),
    mark: document.querySelector('.a-mark').checkVisibility(),
  }));
  assert.deepEqual(onPhone, { stage: false, mark: true });
  await phone.context.close();

  const desktop = await open('a', 'password', 1440, 900);
  const onDesktop = await desktop.page.evaluate(() => ({
    stage: document.querySelector('.a-stage').checkVisibility(),
    mark: document.querySelector('.a-mark').checkVisibility(),
  }));
  assert.deepEqual(onDesktop, { stage: true, mark: false });
  await desktop.context.close();
});

test('the password can be revealed with a labelled toggle that says whether it is pressed, in every concept', async () => {
  for (const concept of ['a', 'b', 'c']) {
    const { page, context } = await open(concept, 'password');
    const toggle = page.locator('#stage button[aria-label="Show password"]');
    assert.equal(await toggle.count(), 1, `${concept} has one reveal toggle`);
    assert.equal(await toggle.getAttribute('aria-pressed'), 'false', concept);
    await context.close();
  }
});

test('the Sign in button is inside a phone screen in every concept', async () => {
  // Today's button ends at 839px of 812 on a phone; no concept may bring that back.
  for (const concept of ['a', 'b', 'c']) {
    const { page, context } = await open(concept, 'password', 375, 812);
    const bottom = await page.evaluate(() => {
      const stage = document.getElementById('stage').getBoundingClientRect().top + scrollY;
      const button = [...document.querySelectorAll('#stage button')].find((b) => /^sign in$/i.test(b.textContent.trim()));
      return button.getBoundingClientRect().bottom + scrollY - stage;
    });
    assert.ok(bottom <= 812, `${concept}: the button ends at ${Math.round(bottom)}px`);
    await context.close();
  }
});
