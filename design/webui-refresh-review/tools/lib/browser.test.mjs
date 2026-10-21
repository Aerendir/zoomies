// Every capture and check in this pack starts a browser through launch(), so
// the one thing worth proving on its own is that it can start the preinstalled
// Chromium without any environment variable and render a page.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './browser.mjs';

test('launch() starts the preinstalled Chromium and renders a page', async () => {
  const browser = await launch();
  try {
    const page = await browser.newPage();
    await page.setContent('<h1>review</h1>');
    assert.equal(await page.locator('h1').textContent(), 'review');
  } finally {
    await browser.close();
  }
});
