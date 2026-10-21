// Photograph the concepts, from the review page, as the pack's evidence.
//
// Every picture is a whole page: the window is made as tall as the page before
// the stage is photographed, so nothing is cut at a fold and a bar that sticks to
// the bottom of a phone rests where it would if the page were fully scrolled.
// Where a fold matters, the README says it with a measured number, because a
// picture cannot show where a screen ended.
//
//   node design/webui-refresh-review/tools/capture.mjs [--pack signed-in] [--only screen]
import { mkdir } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { formatHash } from '../public/review-state.js';
import { launch } from './lib/browser.mjs';
import { serve } from './serve.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
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

/** The state each signed-in page is photographed in: the one that has something to say. */
export const SIGNED_IN_STATE = {
  overview: 'attention',
  pools: 'rows',
  'pool-detail': 'working',
  runners: 'rows',
  'runner-detail': 'busy',
  queue: 'rows',
  workflows: 'rows',
  jobs: 'rows',
  hosts: 'rows',
  'host-detail': 'down',
  providers: 'rows',
  usage: 'normal',
  comparisons: 'normal',
  kennel: 'normal',
  repairs: 'normal',
  'ai-context': 'normal',
  installations: 'rows',
  migrate: 'normal',
  audit: 'rows',
  settings: 'unsaved',
  'pool-new': 'invalid',
  'host-new': 'step-2',
  'provider-new': 'step-1',
};

/** Three pictures of each signed-in page: Mission Control dark at 1440 and on a phone, and Classic light at 1440. */
export function signedInPlan(screens) {
  const shots = [];
  for (const screen of screens) {
    const state = SIGNED_IN_STATE[screen];
    if (!state) throw new Error(`no capture state for "${screen}"; add it to SIGNED_IN_STATE`);
    for (const [width, style, mode, look] of [
      [1440, 'mission-control', 'dark', ''],
      [375, 'mission-control', 'dark', ''],
      [1440, 'classic', 'light', '-classic-light'],
    ]) {
      shots.push({
        screen,
        concept: 'a',
        width,
        style,
        mode,
        hash: formatHash({ screen, concept: 'a', style, mode, state }),
        file: `concept-${screen}${look}-${width}.png`,
      });
    }
  }
  return shots;
}

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
  const signedIn = argv.includes('--pack') && argv[argv.indexOf('--pack') + 1] === 'signed-in';
  const only = argv.includes('--only') ? argv[argv.indexOf('--only') + 1] : undefined;
  const page_ = signedIn ? '/signed-in/mockups.html' : '/review/mockups.html';
  const out = join(root, signedIn ? 'signed-in' : 'public');
  const server = await serve(0);
  const browser = await launch();
  try {
    const survey = await browser.newPage();
    await survey.goto(`${server.url}${page_}`);
    await survey.waitForFunction(() => 'rendered' in document.documentElement.dataset);
    const screens = await survey.$$eval('#screen option', (options) => options.map((o) => o.value));
    await survey.close();
    if (only && !screens.includes(only)) throw new Error(`unknown screen "${only}"; the page offers: ${screens.join(', ')}`);

    await mkdir(out, { recursive: true });
    const chosen = only ? [only] : screens;
    for (const s of signedIn ? signedInPlan(chosen) : plan(chosen)) {
      const context = await browser.newContext({
        viewport: { width: s.width, height: 900 },
        colorScheme: s.mode,
      });
      const page = await context.newPage();
      await page.goto(`${server.url}${page_}${s.hash}`);
      await page.waitForFunction((h) => document.documentElement.dataset.rendered === h, s.hash);
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
