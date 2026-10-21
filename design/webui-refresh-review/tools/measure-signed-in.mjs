// Measure the signed-in mocks, so the README's comparisons with today's pages are
// numbers a script recorded and not numbers anyone remembered.
//
// Each page is measured in the state its picture shows, at 1440 and on a phone,
// the way the capture script frames it: the stage alone, with the review bar
// out of the sum. What it records is what the README cites: how tall the page is
// (the same figure as its picture, which a test holds), how many columns its
// table has, and whether the table scrolls inside its card.
//
//   node design/webui-refresh-review/tools/measure-signed-in.mjs
import { writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { SIGNED_IN_STATE, signedInPlan } from './capture.mjs';
import { launch } from './lib/browser.mjs';
import { serve } from './serve.mjs';

const out = resolve(dirname(fileURLToPath(import.meta.url)), '../signed-in/concept-measurements.json');

/** What the README cites, read from the page as drawn. */
function read() {
  const stage = document.getElementById('stage');
  const table = stage.querySelector('table');
  const region = stage.querySelector('.k-scroll');
  return {
    height: Math.round(stage.getBoundingClientRect().height),
    columns: table ? table.querySelectorAll('thead th').length : 0,
    tables: stage.querySelectorAll('table').length,
    cards: stage.querySelectorAll('.k-card, .k-stat, .k-stage').length,
    scrollsInCard: region ? region.scrollWidth > region.clientWidth : false,
    scrollsSideways: document.documentElement.scrollWidth > innerWidth,
  };
}

export async function measure() {
  const server = await serve(0);
  const browser = await launch();
  const screens = {};
  try {
    const survey = await browser.newPage();
    await survey.goto(`${server.url}/signed-in/mockups.html`);
    await survey.waitForFunction(() => 'rendered' in document.documentElement.dataset);
    const titles = Object.fromEntries(await survey.$$eval('#screen option', (os) => os.map((o) => [o.value, o.textContent])));
    await survey.close();
    for (const shot of signedInPlan(Object.keys(SIGNED_IN_STATE))) {
      if (shot.file.includes('classic-light')) continue;
      const context = await browser.newContext({ viewport: { width: shot.width, height: 900 }, colorScheme: shot.mode });
      const page = await context.newPage();
      await page.goto(`${server.url}/signed-in/mockups.html${shot.hash}`);
      await page.waitForFunction((h) => document.documentElement.dataset.rendered === h, shot.hash);
      await page.evaluate(() => document.fonts.ready);
      screens[shot.screen] ??= { title: titles[shot.screen] };
      screens[shot.screen][String(shot.width)] = await page.evaluate(read);
      await context.close();
    }
  } finally {
    await browser.close();
    await server.close();
  }
  return { generated: new Date().toISOString(), look: 'mission-control dark', screens };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  measure()
    .then((result) => writeFile(out, `${JSON.stringify(result, null, 2)}\n`))
    .then(() => console.log(`wrote ${join('signed-in', 'concept-measurements.json')}`))
    .catch((error) => {
      console.error(error.message);
      process.exitCode = 2;
    });
}
