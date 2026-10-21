// Measure where the concepts put things, as a reader's screen would hold them.
//
// The README compares these numbers with today's pages, which were photographed
// with nothing above them. The review page has a bar of controls above the
// stage, 79px tall at 1440 and 263px at 375 where its five selects wrap, and a
// concept that centres its content is moved by half of it. So the bar is hidden
// before anything is read, and every number is a position from the top of the
// page as the product would draw it. Everything measured is written down, and a
// test holds each number in the README to this file.
//
//   node design/webui-refresh-review/tools/measure.mjs
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { formatHash } from '../public/review-state.js';
import { launch } from './lib/browser.mjs';
import { serve } from './serve.mjs';

const out = resolve(dirname(fileURLToPath(import.meta.url)), '../public/concept-measurements.json');
const SCREENS = { 1440: 900, 375: 812 };

/**
 * What each README table is about. `first` is the field the page asks for first,
 * `action` the button that finishes it (by text, as a regular expression, or by
 * selector) and `last` the bottom-most of a group, such as the blocking causes.
 * The style is the one the table says it was measured in: the status page has
 * only Classic today, so it is compared in Classic.
 */
export const CASES = [
  { screen: 'signin', states: ['password'], style: 'mission-control', first: '#stage input', action: { text: '^sign in$' } },
  {
    screen: 'first-account',
    states: ['empty'],
    style: 'mission-control',
    first: '#f-setup-token',
    action: { selector: 'button[type="submit"]' },
  },
  { screen: 'connect-app', states: ['normal', 'loopback'], style: 'mission-control', action: { text: '^allow' } },
  { screen: 'fleet-status', states: ['blocked'], style: 'classic', last: '#stage .m-reason.sev-error' },
];

/** Every page to measure: each case, three concepts, both widths. */
export function plan(screens = CASES.map((c) => c.screen)) {
  const shots = [];
  for (const spec of CASES.filter((c) => screens.includes(c.screen))) {
    for (const concept of ['a', 'b', 'c']) {
      for (const state of spec.states) {
        for (const [width, height] of Object.entries(SCREENS)) {
          shots.push({
            screen: spec.screen,
            concept,
            state,
            width: Number(width),
            height,
            spec,
            hash: formatHash({ screen: spec.screen, concept, style: spec.style, mode: 'dark', state }),
          });
        }
      }
    }
  }
  return shots;
}

/**
 * Read the positions the spec names from a loaded page, in whole pixels from the
 * top of the page once the review bar is gone. `page` is the document's height,
 * and `fits` says whether it needs no scrolling. An action that sticks to the
 * bottom of the screen is flagged, because its edge is the screen's and not the
 * page's. A missing action is an error: a selector that quietly matches nothing
 * would write a table of nulls that looked like a finding.
 */
export async function measure(page, { first, action, last }) {
  await page.addStyleTag({ content: '.review-bar { display: none !important; }' });
  await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))));
  return page.evaluate(
    ({ first, action, last }) => {
      const at = (el) => {
        const box = el.getBoundingClientRect();
        return { top: Math.round(box.top + scrollY), bottom: Math.round(box.bottom + scrollY) };
      };
      const pinned = (el) => {
        for (let n = el; n && n.id !== 'stage'; n = n.parentElement) {
          if (['sticky', 'fixed'].includes(getComputedStyle(n).position)) return true;
        }
        return false;
      };
      let found = null;
      if (action) {
        const candidates = action.selector
          ? [...document.querySelectorAll(action.selector)]
          : [...document.querySelectorAll('#stage button, #stage a')].filter((el) =>
              new RegExp(action.text, 'i').test(el.textContent.trim()),
            );
        if (!candidates.length) throw new Error(`no action matching ${action.selector ?? action.text}`);
        found = { bottom: at(candidates[0]).bottom, pinned: pinned(candidates[0]) };
      }
      const field = first ? document.querySelector(first) : null;
      const group = last ? [...document.querySelectorAll(last)].filter((el) => el.checkVisibility()).at(-1) : null;
      return {
        first: field ? at(field) : null,
        action: found,
        last: group ? { bottom: at(group).bottom } : null,
        page: Math.round(document.documentElement.scrollHeight),
        // A page cannot be shorter than the window, so a page that needs no
        // scrolling reads as exactly the window tall; this says so outright.
        fits: document.documentElement.scrollHeight <= innerHeight,
      };
    },
    { first, action, last },
  );
}

async function main() {
  const server = await serve(0);
  const browser = await launch();
  try {
    const screens = {};
    for (const shot of plan()) {
      const context = await browser.newContext({
        viewport: { width: shot.width, height: shot.height },
        colorScheme: 'dark',
      });
      const page = await context.newPage();
      await page.goto(`${server.url}/review/mockups.html${shot.hash}`);
      await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
      await page.evaluate(() => document.fonts.ready);
      const got = await measure(page, shot.spec);
      await context.close();
      const slot = ((screens[shot.screen] ??= {})[shot.concept] ??= {});
      (slot[shot.state] ??= {})[shot.width] = got;
      console.log(`${shot.screen} ${shot.concept} ${shot.state} ${shot.width}`);
    }
    await mkdir(dirname(out), { recursive: true });
    await writeFile(
      out,
      `${JSON.stringify({ generated: new Date().toISOString(), reviewBar: 'hidden', screens }, null, 2)}\n`,
    );
  } finally {
    await browser.close();
    await server.close();
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    console.error(error.message);
    process.exitCode = 2;
  });
}
