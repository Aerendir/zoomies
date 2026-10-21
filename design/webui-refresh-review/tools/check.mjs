// The checker for the review pack.
//
// A mockup that looks right at one width and one colour mode is the easy case.
// This drives the review page through every screen, state, concept, style,
// mode and width and reports what a person would hit: a page that scrolls
// sideways, a control with no name, a focus ring that has gone, a colour pair
// below the contrast minimum. It reports; it never edits a concept to pass.
//
//   node design/webui-refresh-review/tools/check.mjs [--screen id] [--out file]
//
// It exits 1 on any finding. What it cannot see is listed in each README under
// "Not claimed": it is no substitute for a person with a screen reader.
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { launch } from './lib/browser.mjs';
import { serve } from './serve.mjs';

export const WIDTHS = [320, 375, 1440];

/** Below this a phone is only asked to reflow; the slow rules run from here up. */
const FULL_CHECK_FROM = 375;

/** The pairs every concept inherits from tokens.css: 4.5 for text, 3 for a control's edge. */
export const BASE_PAIRS = [
  { fg: '--z-text', bg: '--z-surface', min: 4.5 },
  { fg: '--z-text-muted', bg: '--z-surface', min: 4.5 },
  { fg: '--z-accent', bg: '--z-surface', min: 4.5 },
  { fg: '--z-danger', bg: '--z-danger-subtle', min: 4.5 },
  { fg: '--z-border-strong', bg: '--z-surface', min: 3 },
];

/**
 * What the product's own tokens already get wrong, and identically for every
 * concept built from them, so no concept can fix it. A finding that matches an
 * entry is reported as known and does not fail the run. It stops matching if
 * the ratio falls below `atLeast`, so a change for the worse is still caught,
 * and it never covers another style. Delete an entry to make the pack fail on
 * it again.
 */
export const KNOWN = [
  {
    style: 'classic',
    fg: '--z-border-strong',
    bg: '--z-surface',
    atLeast: 1.6,
    reason:
      "Classic's control edge is about 1.62:1 against the surface in light and in dark, under the 3:1 minimum for the edge of a control; Mission Control's is past 3:1. Raising Classic's --z-border-strong is a token change independent of any concept.",
  },
];

const CONCEPT_PAIRS = fileURLToPath(new URL('../public/concept-c-pairs.json', import.meta.url));
const MAX_TAB_STOPS = 60;

/* colour */

function channels(text) {
  const m = /^rgba?\(\s*([\d.]+)[\s,]+([\d.]+)[\s,]+([\d.]+)\s*(?:[,/]\s*[\d.]+%?\s*)?\)$/i.exec(
    String(text).trim(),
  );
  if (!m) throw new Error(`cannot read colour "${text}"`);
  return [m[1], m[2], m[3]].map(Number);
}

function luminance(text) {
  const [r, g, b] = channels(text).map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/**
 * The WCAG contrast ratio of two opaque `rgb()` colours. Alpha is ignored: a
 * translucent colour has to be composited over what is behind it first, which
 * only the browser can do (see `checkPairs`).
 */
export function contrast(fg, bg) {
  const [light, dark] = [luminance(fg), luminance(bg)].sort((a, b) => b - a);
  return (light + 0.05) / (dark + 0.05);
}

/* one page */

const pending = new WeakMap();

/**
 * Start gathering console errors and uncaught exceptions from `page`. Call it
 * before navigating: `checkPage` is handed a page that has already loaded, and
 * a listener attached then would have missed everything that mattered.
 */
export function watch(page) {
  const seen = [];
  pending.set(page, seen);
  page.on('console', (message) => {
    if (message.type() === 'error') seen.push(message.text());
  });
  page.on('pageerror', (error) => seen.push(error.message));
}

async function label(locator) {
  return locator.evaluate((el) => {
    const text = (el.getAttribute('aria-label') || el.textContent || '').trim().slice(0, 30);
    return `${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : ''}${text ? ` "${text}"` : ''}`;
  });
}

/**
 * Check what is on `page` now. Everything is scoped to `#stage` (the concept
 * itself) when it exists, so the review bar around it is not judged as though
 * it were part of a design. `layoutOnly` stops after overflow and console
 * errors, which is all a 320px pass needs to prove.
 */
export async function checkPage(page, { layoutOnly = false } = {}) {
  const findings = [];
  const add = (kind, detail) => findings.push({ kind, detail });

  const { scrollWidth, innerWidth } = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }));
  if (scrollWidth > innerWidth) {
    add('overflow', `the page is ${scrollWidth}px wide in a ${innerWidth}px window`);
  }
  // A page can fit its window and still cut a sentence off at the edge: an
  // overflow rule hides the spill from scrollWidth. Only asked when the page
  // does not already report overflow, so one cause is one finding. Anything
  // inside aria-hidden is decoration (a scene hung past the edge on purpose).
  if (scrollWidth <= innerWidth) {
    const cut = await page.evaluate(() => {
      const stage = document.querySelector('#stage') ?? document.body;
      const limit = window.innerWidth;
      const found = [];
      for (const el of stage.querySelectorAll('*')) {
        if (el.closest('[aria-hidden="true"]')) continue;
        const box = el.getBoundingClientRect();
        if (box.width === 0 || box.height === 0) continue;
        if (box.right > limit + 1 || box.left < -1) {
          found.push(`${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : ''}`);
        }
      }
      return [...new Set(found)].slice(0, 3);
    });
    for (const name of cut) add('clipped', `${name} extends past the ${innerWidth}px window and is cut off, not scrolled`);
  }
  // Drained, so an error is reported against the combination that caused it.
  for (const message of pending.get(page)?.splice(0) ?? []) add('console', message);
  if (layoutOnly) return findings;

  const scope = (await page.locator('#stage').count()) ? page.locator('#stage') : page.locator('body');

  const structure = await page.evaluate(() => {
    const stage = document.querySelector('#stage') ?? document.body;
    return {
      h1: [...stage.querySelectorAll('h1')].filter((el) => el.checkVisibility()).length,
      mains: document.querySelectorAll('main, [role="main"]').length,
      positive: [...document.querySelectorAll('[tabindex]')]
        .filter((el) => Number(el.getAttribute('tabindex')) > 0)
        .map((el) => el.tagName.toLowerCase()),
    };
  });
  if (structure.h1 !== 1) add('heading', `${structure.h1} visible h1 elements; a screen has exactly one`);
  if (structure.mains !== 1) add('landmark', `${structure.mains} main landmarks; a page has exactly one`);
  for (const tag of structure.positive) add('tabindex', `a ${tag} has a positive tabindex, which reorders the page`);

  // Playwright computes the accessible name; a role with no quoted name after
  // it in the snapshot's first line has none.
  const controls = scope.locator('button, a[href], input:not([type="hidden"]), select, textarea');
  for (let i = 0, n = await controls.count(); i < n; i++) {
    const control = controls.nth(i);
    if (!(await control.isVisible())) continue;
    const first = (await control.ariaSnapshot()).split('\n')[0];
    if (!/^- [\w-]+ "/.test(first)) add('name', `${await label(control)} has no accessible name`);
  }

  // Tab through the page as a keyboard user would. A stop with neither an
  // outline nor a box-shadow is a place the person cannot tell where they are.
  await page.evaluate(() => {
    document.activeElement?.blur?.();
    window.scrollTo(0, 0);
  });
  for (let stops = 0; stops < MAX_TAB_STOPS; stops++) {
    await page.keyboard.press('Tab');
    const stop = await page.evaluate(() => {
      const el = document.activeElement;
      if (!el || el === document.body || el === document.documentElement) return null;
      const stage = document.querySelector('#stage') ?? document.body;
      const style = getComputedStyle(el);
      const ring =
        style.outlineStyle === 'auto' ||
        (style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) > 0) ||
        style.boxShadow !== 'none';
      const repeated = el.__reviewSeen === true;
      el.__reviewSeen = true;
      return {
        repeated,
        ring,
        inside: stage.contains(el),
        name: `${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : ''}`,
      };
    });
    if (!stop || stop.repeated) break;
    if (stop.inside && !stop.ring) add('focus', `${stop.name} shows no focus ring`);
  }

  return findings;
}

/**
 * Check colour pairs against the tokens as they resolve on the page right now,
 * so the style, mode and concept in force are the ones judged. `fg` and `bg`
 * are custom property names. The browser resolves them: tokens.css chains
 * `var()` and uses translucent colours, which cannot be read from the text. A
 * translucent background is laid over `--z-surface`, and a translucent
 * foreground over its background.
 */
export async function checkPairs(page, pairs) {
  const resolved = await page.evaluate((list) => {
    const root = document.documentElement;
    const probe = document.createElement('i');
    probe.style.cssText = 'position:absolute;visibility:hidden';
    root.appendChild(probe);
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = 1;
    const ctx = canvas.getContext('2d', { willReadFrequently: true });
    const defined = (name) => getComputedStyle(root).getPropertyValue(name).trim() !== '';
    const colour = (name) => {
      probe.style.color = `var(${name})`;
      return getComputedStyle(probe).color;
    };
    const flatten = (layers) => {
      ctx.fillStyle = '#fff';
      ctx.fillRect(0, 0, 1, 1);
      for (const layer of layers) {
        ctx.fillStyle = layer;
        ctx.fillRect(0, 0, 1, 1);
      }
      const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data;
      return `rgb(${r}, ${g}, ${b})`;
    };
    const ground = defined('--z-surface') ? colour('--z-surface') : '#fff';
    const out = list.map(({ fg, bg }) => {
      const missing = [fg, bg].find((name) => !defined(name));
      if (missing) return { missing };
      return { fg: flatten([ground, colour(bg), colour(fg)]), bg: flatten([ground, colour(bg)]) };
    });
    probe.remove();
    return out;
  }, pairs);

  const findings = [];
  pairs.forEach((pair, i) => {
    const got = resolved[i];
    if (got.missing) {
      findings.push({ kind: 'contrast', detail: `${got.missing} is not defined on this page` });
      return;
    }
    const ratio = contrast(got.fg, got.bg);
    if (ratio < pair.min) {
      const shown = (Math.floor(ratio * 100) / 100).toFixed(2);
      findings.push({
        kind: 'contrast',
        // The pair and the ratio travel with the finding, so a baseline can
        // recognise it without parsing a sentence.
        fg: pair.fg,
        bg: pair.bg,
        min: pair.min,
        ratio,
        detail: `${pair.fg} on ${pair.bg} is ${shown}:1 and needs ${pair.min}:1`,
      });
    }
  });
  return findings;
}

/* the whole review page */

/** Point the review page at one combination and wait until it has drawn it. */
async function show(page, choice) {
  const wanted = `#${new URLSearchParams(choice)}`;
  const hash = await page.evaluate((h) => {
    location.hash = h;
    return location.hash;
  }, wanted);
  await page.waitForFunction((h) => document.documentElement.dataset.rendered === h, hash);
}

const optionValues = (page, id) => page.$$eval(`#${id} option`, (os) => os.map((o) => o.value));

async function loadConceptPairs() {
  try {
    return { c: JSON.parse(await readFile(CONCEPT_PAIRS, 'utf8')) };
  } catch (error) {
    if (error.code === 'ENOENT') return {};
    throw error;
  }
}

/**
 * Drive the review page at `url` through every combination and collect the
 * findings, each labelled with where it was found. The page has to keep the
 * contract in Task 3 of PLAN-public.md: selects #screen, #concept, #style,
 * #mode and #state, a <main id="stage">, a hash that drives them, and
 * data-rendered on <html> once a render is done.
 */
export async function run({
  url,
  screen,
  widths = WIDTHS,
  pairs = BASE_PAIRS,
  conceptPairs,
  known = KNOWN,
} = {}) {
  const extra = conceptPairs ?? (await loadConceptPairs());
  const target = `${url}/review/mockups.html`;
  const browser = await launch();
  try {
    const open = async (width) => {
      const page = await browser.newPage({ viewport: { width, height: 900 } });
      watch(page);
      await page.goto(target);
      await page.waitForFunction(() => 'rendered' in document.documentElement.dataset);
      return page;
    };

    const survey = await open(widths[0]);
    const screens = await optionValues(survey, 'screen');
    if (screen && !screens.includes(screen)) {
      throw new Error(`unknown screen "${screen}"; the review page offers: ${screens.join(', ')}`);
    }
    const concepts = (await optionValues(survey, 'concept')).filter((c) => c !== 'current');
    const styles = await optionValues(survey, 'style');
    const modes = await optionValues(survey, 'mode');
    const visits = [];
    for (const name of screen ? [screen] : screens) {
      await show(survey, { screen: name });
      for (const state of await optionValues(survey, 'state')) visits.push({ screen: name, state });
    }

    const findings = [];
    // Pairs depend on concept, style and mode only, so they are judged once.
    for (const concept of concepts) {
      for (const style of styles) {
        for (const mode of modes) {
          await show(survey, { screen: visits[0].screen, state: visits[0].state, concept, style, mode });
          const list = [...pairs, ...(extra[concept] ?? [])];
          for (const f of await checkPairs(survey, list)) {
            findings.push({ screen: null, concept, style, mode, state: null, width: null, ...f });
          }
        }
      }
    }
    await survey.close();

    let combinations = 0;
    await Promise.all(
      widths.map(async (width) => {
        const page = await open(width);
        for (const { screen: name, state } of visits) {
          for (const concept of concepts) {
            for (const style of styles) {
              for (const mode of modes) {
                await show(page, { screen: name, concept, style, mode, state });
                combinations++;
                for (const f of await checkPage(page, { layoutOnly: width < FULL_CHECK_FROM })) {
                  findings.push({ screen: name, concept, style, mode, state, width, ...f });
                }
              }
            }
          }
        }
        await page.close();
      }),
    );
    // A finding the baseline recognises is not a failure of this run. It is
    // still returned, with its reason, so it can be reported and not forgotten.
    const covering = (f) =>
      f.kind === 'contrast' &&
      known.find((k) => k.style === f.style && k.fg === f.fg && k.bg === f.bg && f.ratio >= k.atLeast);
    const failing = [];
    const tolerated = [];
    for (const f of findings) {
      const entry = covering(f);
      if (entry) tolerated.push({ ...f, reason: entry.reason });
      else failing.push(f);
    }
    return {
      url: target,
      widths,
      screens: screen ? [screen] : screens,
      combinations,
      findings: failing,
      known: tolerated,
    };
  } finally {
    await browser.close();
  }
}

/* command line */

export function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i++) {
    const option = argv[i];
    if (option !== '--screen' && option !== '--out') throw new Error(`unknown option "${option}"`);
    const value = argv[++i];
    if (value === undefined) throw new Error(`${option} needs a value`);
    args[option.slice(2)] = value;
  }
  return args;
}

async function main(argv) {
  const args = parseArgs(argv);
  const server = await serve(0);
  try {
    const result = await run({ url: server.url, screen: args.screen });
    if (args.out) {
      await mkdir(dirname(args.out), { recursive: true });
      const report = { generated: new Date().toISOString(), ...result, url: undefined };
      await writeFile(args.out, `${JSON.stringify(report, null, 2)}\n`);
    }
    console.log(`${result.combinations} combinations, ${result.findings.length} findings`);
    for (const f of result.findings.slice(0, 40)) {
      const where = [f.screen, f.concept, f.style, f.mode, f.state, f.width && `${f.width}px`]
        .filter(Boolean)
        .join(' ');
      console.log(`  ${where} ${f.kind}: ${f.detail}`);
    }
    // Known issues are said every time, once each, so a green run is never silent about them.
    const groups = new Map();
    for (const k of result.known) {
      const key = `${k.style}: ${k.fg} on ${k.bg}`;
      groups.set(key, [...(groups.get(key) ?? []), k]);
    }
    if (groups.size) console.log(`${result.known.length} known, not failing:`);
    for (const [key, list] of groups) console.log(`  ${key} (${list.length}): ${list[0].reason}`);
    process.exitCode = result.findings.length ? 1 : 0;
  } finally {
    await server.close();
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2)).catch((error) => {
    console.error(error.message);
    process.exitCode = 2;
  });
}
