// Photograph the concepts, from the review page, as the pack's evidence.
//
// Every picture is a whole page: the window is made as tall as the page before
// the stage is photographed, so nothing is cut at a fold and a bar that sticks to
// the bottom of a phone rests where it would if the page were fully scrolled.
// Where a fold matters, the README says it with a measured number, because a
// picture cannot show where a screen ended.
//
//   node design/webui-refresh-review/tools/capture.mjs [--only screen]
import { mkdir } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { formatHash } from '../public/review-state.js';
import { launch } from './lib/browser.mjs';
import { serve } from './serve.mjs';

const out = resolve(dirname(fileURLToPath(import.meta.url)), '../public');
const WIDTHS = [1440, 375];

/** The state each screen is photographed in: the one that shows the design best. */
export const CAPTURE_STATE = {
  signin: 'password',
  'first-account': 'empty',
  'connect-app': 'normal',
  // The case the page must handle, with twelve reasons, and not the easy one.
  'fleet-status': 'blocked',
  'not-found': 'not-found',
};

/** The concept the README recommends for each screen, which is also shown in the other style and mode. */
export const RECOMMENDED = {
  signin: 'b',
  'first-account': 'b',
  'connect-app': 'b',
  'fleet-status': 'a',
  'not-found': 'b',
};

function shot(screen, concept, width, style, mode) {
  const look = style === 'classic' && mode === 'light' ? '-classic-light' : '';
  return {
    screen,
    concept,
    width,
    style,
    mode,
    hash: formatHash({ screen, concept, style, mode, state: CAPTURE_STATE[screen] }),
    file: `concept-${screen}-${concept}${look}-${width}.png`,
  };
}

/** Every picture to take: three concepts at two widths in Mission Control dark, and the recommended one in Classic light. */
export function plan(screens) {
  const shots = [];
  for (const screen of screens) {
    for (const concept of ['a', 'b', 'c']) {
      for (const width of WIDTHS) shots.push(shot(screen, concept, width, 'mission-control', 'dark'));
    }
    shots.push(shot(screen, RECOMMENDED[screen], 1440, 'classic', 'light'));
  }
  return shots;
}

async function main(argv) {
  const only = argv[0] === '--only' ? argv[1] : undefined;
  const server = await serve(0);
  const browser = await launch();
  try {
    const survey = await browser.newPage();
    await survey.goto(`${server.url}/review/mockups.html`);
    await survey.waitForFunction(() => 'rendered' in document.documentElement.dataset);
    const screens = await survey.$$eval('#screen option', (options) => options.map((o) => o.value));
    await survey.close();
    if (only && !screens.includes(only)) throw new Error(`unknown screen "${only}"; the page offers: ${screens.join(', ')}`);

    await mkdir(out, { recursive: true });
    for (const s of plan(only ? [only] : screens)) {
      const context = await browser.newContext({
        viewport: { width: s.width, height: 900 },
        colorScheme: s.mode,
      });
      const page = await context.newPage();
      await page.goto(`${server.url}/review/mockups.html${s.hash}`);
      await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
      await page.evaluate(() => document.fonts.ready);
      // As tall as the page, so nothing is cut and nothing sticks across the content.
      const whole = await page.evaluate(() => document.documentElement.scrollHeight);
      await page.setViewportSize({ width: s.width, height: whole });
      await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))));
      await page.locator('#stage').screenshot({ path: join(out, s.file) });
      await context.close();
      console.log(s.file);
    }
  } finally {
    await browser.close();
    await server.close();
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2)).catch((error) => {
    console.error(error.message);
    process.exitCode = 2;
  });
}
