// The README is the pack's argument, so what it can be checked against is: the
// pictures it shows, the links it makes, the totals the checker wrote down, the
// numbers in its tables, and the lengths the kit writes by hand.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { SIGNED_IN_STATE, signedInPlan } from '../tools/capture.mjs';
import { BASE_PAIRS, SIGNED_IN_PAIRS } from '../tools/check.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const readme = readFileSync(resolve(here, 'README.md'), 'utf8');
const review = JSON.parse(readFileSync(resolve(here, 'viewport-review.json'), 'utf8'));
const measured = JSON.parse(readFileSync(resolve(here, 'concept-measurements.json'), 'utf8')).screens;
const photographed = JSON.parse(readFileSync(resolve(here, 'current-measurements.json'), 'utf8')).shots;
// The registry imports browser paths, so Node reads the screens from what the measurer recorded:
// the review page's own list, with each title as the Screen control shows it.
const SCREENS = Object.entries(measured).map(([id, value]) => ({ id, title: value.title }));
const flat = readme.replace(/\s+/g, ' ');

/** The text under a `##` or `###` heading, up to the next heading of any level. */
function section(title) {
  const lines = readme.split('\n');
  const start = lines.findIndex((line) => line === `## ${title}` || line === `### ${title}`);
  assert.ok(start >= 0, `the README has no "${title}" section`);
  const end = lines.findIndex((line, i) => i > start && /^#{1,3} /.test(line));
  return lines.slice(start + 1, end < 0 ? undefined : end).join('\n');
}

/** The height of a PNG, read from its header, so a picture is held to what was measured. */
const pngHeight = (file) => readFileSync(resolve(here, file)).readUInt32BE(20);

test('the capture states and the measurements name the same screens', () => {
  assert.deepEqual(Object.keys(measured).sort(), Object.keys(SIGNED_IN_STATE).sort());
});

test('every picture the capture script takes is shown in the README, and so is today’s page it sits beside', () => {
  for (const shot of signedInPlan(SCREENS.map((s) => s.id))) {
    assert.ok(readme.includes(shot.file), `${shot.file} is not in the README`);
  }
  for (const screen of SCREENS) assert.ok(readme.includes(`current-${screen.id}-1440.png`), `no capture of today's ${screen.id}`);
});

test('every relative link in the README goes to a file that exists', () => {
  const links = [...readme.matchAll(/\]\(([^)\s]+)\)/g)]
    .map((match) => match[1])
    .filter((target) => !/^(https?:|#|mailto:)/.test(target));
  assert.ok(links.length > 60, 'a README with this much to show links to its pictures and its evidence');
  for (const target of links) assert.ok(existsSync(resolve(here, target.split('#')[0])), `${target} does not exist`);
});

test('the totals in the README are the ones the checker wrote down', () => {
  assert.ok(flat.includes(`${review.combinations.toLocaleString('en-GB')} combinations`));
  assert.ok(flat.includes(`${review.findings.length} findings`));
  assert.ok(flat.includes(`${review.known.length} known`));
  assert.ok(flat.includes(`on the ${BASE_PAIRS.length} pairs of tokens the public pack checks and ${SIGNED_IN_PAIRS.length} the kit adds`));
});

test('a measured picture is as tall as the figure the README cites for it', () => {
  for (const [id, widths] of Object.entries(measured)) {
    for (const width of ['1440', '375']) {
      assert.equal(pngHeight(`concept-${id}-${width}.png`), widths[width].height, `${id} at ${width}`);
    }
  }
});

test('every number in a comparison row was measured, from the mock or from the photographed page', () => {
  const rows = [...readme.matchAll(/^\| ([A-Za-z ]+) \| (\d+) \| (\w+) \| (\d+) \| (\w+) \| (\d+) \| (\d+) \|$/gm)];
  assert.equal(rows.length, SCREENS.length, 'every screen is in exactly one comparison row');
  const titles = new Map(SCREENS.map((s) => [s.title, s.id]));
  const seen = new Set();
  for (const [, title, today, todayCols, mock, mockCols, todayPhone, mockPhone] of rows) {
    const id = titles.get(title);
    assert.ok(id, `"${title}" is not a screen`);
    seen.add(id);
    const columns = (value) => (value === 'none' ? 0 : Number(value));
    assert.equal(Number(today), photographed[`${id}-1440`].documentHeight, `${title}: today at 1440`);
    assert.equal(columns(todayCols), photographed[`${id}-1440`].columns, `${title}: columns today`);
    assert.equal(Number(todayPhone), photographed[`${id}-375`].documentHeight, `${title}: today on a phone`);
    assert.equal(Number(mock), measured[id]['1440'].height, `${title}: mock at 1440`);
    assert.equal(columns(mockCols), measured[id]['1440'].columns, `${title}: columns in the mock`);
    assert.equal(Number(mockPhone), measured[id]['375'].height, `${title}: mock on a phone`);
  }
  assert.equal(seen.size, SCREENS.length);
});

test('no measured mock scrolls the page sideways, on a phone or at 1440', () => {
  for (const [id, widths] of Object.entries(measured)) {
    for (const width of ['1440', '375']) assert.equal(widths[width].scrollsSideways, false, `${id} at ${width}`);
  }
});

test('every length kit.css writes by hand is listed in the README, and every listed one is in kit.css', () => {
  const css = readFileSync(resolve(here, 'kit.css'), 'utf8');
  // A media query's width is the product's own breakpoint (--z-bp-md), which a custom property
  // cannot be written into, so the query lines are left out and the rest are all counted.
  const body = css.replace(/@media[^{]*/g, '');
  const written = new Set([
    ...[...body.matchAll(/(?<![\w.-])(\d+(?:\.\d+)?(?:rem|s|vh|px))\b/g)].map((m) => m[1]),
    ...[...body.matchAll(/opacity:\s*([\d.]+)/g)].map((m) => m[1]),
  ]);
  const table = section('Tokens');
  const listed = new Set([...table.matchAll(/^\| `([^`]+)`/gm)].flatMap((m) => m[1].split(/,\s*/)));
  for (const length of written) assert.ok(listed.has(length), `${length} is written in kit.css and not listed in the README`);
  for (const length of listed) assert.ok(written.has(length), `${length} is listed in the README and not in kit.css`);
});

test('the kit proposes no tokens, and says so', () => {
  const css = readFileSync(resolve(here, 'kit-tokens.css'), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');
  assert.equal([...css.matchAll(/--[a-z-]+\s*:/g)].length, 0, 'kit-tokens.css declares a token the README does not list');
  assert.match(section('Tokens'), /proposes \*\*no new tokens\*\*/);
});

test('the README says what it does not claim', () => {
  const claimed = section('Not claimed').toLowerCase();
  for (const phrase of ['usability study', 'screen reader', 'performance', 'end-to-end', 'real fleet']) {
    assert.ok(claimed.includes(phrase), `Not claimed does not mention "${phrase}"`);
  }
});

test('the README names the Classic control edge and its remedy, because no mock can fix it', () => {
  assert.match(flat, /1\.62:1/);
  assert.match(flat, /--z-border-strong/);
});
