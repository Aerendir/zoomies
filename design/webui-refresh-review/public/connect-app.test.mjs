// Connect an app is the one decision a person is sent to from somebody else's
// app, so what is worth holding is what makes it hard to decide: the buttons
// out of reach, a warning that is always on or never on, a name so long it
// takes the page with it, and a bar of buttons that covers the field a person
// is looking at.
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

async function open(concept, state, width = 1440, height = 900) {
  const context = await browser.newContext({ viewport: { width, height }, colorScheme: 'dark' });
  const page = await context.newPage();
  await page.goto(
    `${server.url}/review/mockups.html#screen=connect-app&concept=${concept}&style=mission-control&mode=dark&state=${state}`,
  );
  await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
  return { page, context };
}

/** The bottom of the Allow button, measured from the top of the page without the review bar. */
async function allowBottom(page) {
  return page.evaluate(() => {
    const stage = document.getElementById('stage').getBoundingClientRect();
    const allow = [...document.querySelectorAll('#stage button, #stage a')].find(
      (el) => /^Allow/.test(el.textContent.trim()) && el.checkVisibility(),
    );
    return allow.getBoundingClientRect().bottom - stage.top;
  });
}

for (const concept of ['a', 'b', 'c']) {
  for (const state of ['normal', 'loopback']) {
    test(`concept ${concept}, ${state}: Allow is inside a 1440x900 screen`, async () => {
      // Today's page puts it at 820px, and at 905px with the warning showing.
      const { page, context } = await open(concept, state);
      const bottom = await allowBottom(page);
      assert.ok(bottom <= 900, `Allow ends at ${Math.round(bottom)}px`);
      await context.close();
    });
  }
}

test('the warning about this computer appears only when the app returns there', async () => {
  for (const concept of ['a', 'b', 'c']) {
    const normal = await open(concept, 'normal');
    assert.doesNotMatch(await normal.page.locator('#stage').innerText(), /this computer/i, `${concept} normal`);
    await normal.context.close();
    const loopback = await open(concept, 'loopback');
    assert.match(await loopback.page.locator('#stage').innerText(), /this computer/i, `${concept} loopback`);
    await loopback.context.close();
  }
});

test('concept A names the app on its primary button, and cuts a 300-character name short', async () => {
  const normal = await open('a', 'normal');
  assert.equal(await normal.page.getByRole('button', { name: 'Allow Claude' }).count(), 1);
  await normal.context.close();

  const stress = await open('a', 'stress');
  const label = (await stress.page.getByRole('button', { name: /^Allow/ }).innerText()).trim();
  assert.ok(label.length <= 40 && label.endsWith('…'), `the button says "${label}"`);
  // The heading still carries the whole name.
  const heading = await stress.page.locator('#stage h1').innerText();
  assert.ok(heading.length > 300);
  await stress.context.close();
});

test('every concept has one checked role, and it is the operator the product suggests', async () => {
  for (const concept of ['a', 'b', 'c']) {
    const { page, context } = await open(concept, 'normal');
    assert.equal(await page.locator('input[name="connection-role"]:checked').getAttribute('value'), 'operator', concept);
    await context.close();
  }
});

test('B: the buttons stick to the bottom of a phone and never cover what is focused', async () => {
  const { page, context } = await open('b', 'loopback', 375, 812);
  const bar = page.locator('.cb-actions');
  assert.equal(await bar.evaluate((el) => getComputedStyle(el).position), 'sticky');

  const covered = [];
  for (let stop = 0; stop < 30; stop++) {
    await page.keyboard.press('Tab');
    const hit = await page.evaluate(() => {
      const el = document.activeElement;
      const barBox = document.querySelector('.cb-actions')?.getBoundingClientRect();
      if (!el || !barBox || !document.getElementById('stage').contains(el)) return null;
      if (el.closest('.cb-actions')) return null;
      const box = el.getBoundingClientRect();
      const overlap = Math.min(box.bottom, barBox.bottom) - Math.max(box.top, barBox.top);
      return overlap > 0 ? `${el.tagName.toLowerCase()} overlaps the bar by ${Math.round(overlap)}px` : null;
    });
    if (hit) covered.push(hit);
  }
  assert.deepEqual(covered, []);
  await context.close();
});

test('on a phone the decision is reachable: A and C put Allow above their Deny, as the product does', async () => {
  for (const concept of ['a', 'c']) {
    const { page, context } = await open(concept, 'normal', 375, 812);
    const order = await page.evaluate(() => {
      const top = (label) => [...document.querySelectorAll('#stage button')].find((b) => b.textContent.trim().startsWith(label)).getBoundingClientRect().top;
      return top('Allow') < top('Deny');
    });
    assert.equal(order, true, concept);
    await context.close();
  }
});

test('B: a radio scrolled beneath the buttons is brought back into view when it is focused', async () => {
  // Tabbing from the top never lands one there, which is why the loop above
  // cannot fail. This is the case that can: somebody has scrolled, a radio sits
  // behind the bar, and they focus it.
  const { page, context } = await open('b', 'loopback', 375, 812);
  const radio = page.locator('input[name="connection-role"]').first();
  await radio.evaluate((el) => window.scrollBy(0, el.getBoundingClientRect().bottom - (innerHeight - 20)));
  const before = await radio.evaluate((el) => el.getBoundingClientRect().bottom);
  assert.ok(before > 812 - 72, `the radio starts inside the bar's zone (${Math.round(before)}px)`);
  await radio.focus();
  const overlap = await page.evaluate(() => {
    const box = document.querySelector('input[name="connection-role"]').getBoundingClientRect();
    const bar = document.querySelector('.cb-actions').getBoundingClientRect();
    return Math.min(box.bottom, bar.bottom) - Math.max(box.top, bar.top);
  });
  assert.ok(overlap <= 0, `the focused radio is covered by ${Math.round(overlap)}px of the bar`);
  await context.close();
});
