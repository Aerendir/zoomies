// The README is the pack's argument, so what it can be checked against is.
//
// A recommendation that disagrees with the picture set, a capture nobody
// mentions, a link that goes nowhere or a total that is not the one the checker
// wrote down would each make a reader trust the pack less than it deserves, and
// each is the kind of drift a long review document picks up when one part is
// edited and the others are not.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { RECOMMENDED, plan } from '../tools/capture.mjs';
import { BASE_PAIRS } from '../tools/check.mjs';
import { SCREENS } from './screens/index.js';

const here = dirname(fileURLToPath(import.meta.url));
const readme = readFileSync(resolve(here, 'README.md'), 'utf8');
const review = JSON.parse(readFileSync(resolve(here, 'viewport-review.json'), 'utf8'));

/** The text under a `##` or `###` heading, up to the next heading of any level. */
function section(title) {
  const lines = readme.split('\n');
  const start = lines.findIndex((line) => line === `## ${title}` || line === `### ${title}`);
  assert.ok(start >= 0, `the README has no "${title}" section`);
  const end = lines.findIndex((line, i) => i > start && /^#{1,3} /.test(line));
  return lines.slice(start + 1, end < 0 ? undefined : end).join('\n');
}

/** The README with its line wrapping undone, so a phrase is found wherever it broke. */
const flat = readme.replace(/\s+/g, ' ');

test('each screen states one recommendation, and it is the concept that is photographed in the other style', () => {
  for (const screen of SCREENS) {
    const found = [...section(screen.title).matchAll(/\*\*Recommendation: ([ABC])\.\*\*/g)];
    assert.equal(found.length, 1, `${screen.title} must state exactly one recommendation`);
    assert.equal(found[0][1].toLowerCase(), RECOMMENDED[screen.id], `${screen.title}: the README and capture.mjs disagree`);
  }
});

test('every picture the capture script takes is shown in the README', () => {
  for (const shot of plan(SCREENS.map((screen) => screen.id))) {
    assert.ok(readme.includes(shot.file), `${shot.file} is not in the README`);
  }
});

test('every relative link in the README goes to a file that exists', () => {
  const links = [...readme.matchAll(/\]\(([^)\s]+)\)/g)]
    .map((match) => match[1])
    .filter((target) => !/^(https?:|#|mailto:)/.test(target));
  assert.ok(links.length > 10, 'a README with this much to show links to its pictures and its evidence');
  for (const target of links) {
    const file = resolve(here, target.split('#')[0]);
    assert.ok(existsSync(file), `${target} does not exist`);
  }
});

test('the totals in the README are the ones the checker wrote down', () => {
  const combinations = review.combinations.toLocaleString('en-GB');
  assert.ok(flat.includes(`${combinations} combinations`), `the README does not say ${combinations} combinations`);
  assert.ok(flat.includes(`${review.findings.length} findings`), `the README does not say ${review.findings.length} findings`);
  assert.ok(flat.includes(`${review.known.length} known`), `the README does not say ${review.known.length} known`);
  assert.ok(flat.includes(`on the ${BASE_PAIRS.length} pairs of tokens`), `the README does not say ${BASE_PAIRS.length} pairs of tokens`);
});

// The tables compare each concept with today's page, and a number that was typed
// from memory or measured beneath the review bar is a comparison that is wrong in
// the concept's favour or against it. Every number in a row is one a script
// recorded, from the concept for a concept's row and from the photographed page
// for Today's.
const numbersIn = (value, found = new Set()) => {
  if (typeof value === 'number') found.add(value);
  else if (value && typeof value === 'object') Object.values(value).forEach((inner) => numbersIn(inner, found));
  return found;
};
const measuredConcepts = JSON.parse(readFileSync(resolve(here, 'concept-measurements.json'), 'utf8'));
const photographed = JSON.parse(readFileSync(resolve(here, 'current-measurements.json'), 'utf8')).shots;

for (const [title, id] of [
  ['Sign in', 'signin'],
  ['First account', 'first-account'],
  ['Connect an app', 'connect-app'],
  ['Fleet status', 'fleet-status'],
]) {
  test(`${title}: every number in a table row is one that was measured`, () => {
    const rows = section(title).split('\n').filter((line) => /^\| (Today|[ABC],)/.test(line));
    assert.equal(rows.length, 4, `${title} has a Today row and a row for each concept`);
    for (const row of rows) {
      const [label, ...cells] = row.split('|').slice(1, -1).map((cell) => cell.trim());
      const allowed =
        label === 'Today'
          ? numbersIn(Object.entries(photographed).filter(([key]) => key.startsWith(id)).map(([, shot]) => shot))
          : numbersIn(measuredConcepts.screens[id][label[0].toLowerCase()]);
      for (const number of cells.flatMap((cell) => [...cell.matchAll(/\d+/g)].map((match) => Number(match[0])))) {
        assert.ok(allowed.has(number), `${title}, "${label}": ${number} was not measured`);
      }
    }
  });
}

test('the README says what it does not claim', () => {
  const claimed = section('Not claimed').toLowerCase();
  for (const phrase of ['usability study', 'screen reader', 'performance', 'end-to-end']) {
    assert.ok(claimed.includes(phrase), `Not claimed does not mention "${phrase}"`);
  }
});

test('the README names the Classic control edge and its remedy, because no concept can fix it', () => {
  assert.match(flat, /1\.62:1/);
  assert.match(flat, /--z-border-strong/);
});

test('every token concept C proposes is named in the README with the guideline section it would change', () => {
  const css = readFileSync(resolve(here, 'concept-c-tokens.css'), 'utf8');
  const proposed = [...css.matchAll(/^\s+(--z-[a-z-]+):/gm)].map((match) => match[1]);
  assert.ok(proposed.length >= 5, 'concept C defines tokens');
  const table = section('The tokens concept C would add');
  for (const token of new Set(proposed)) {
    const row = table.split('\n').find((line) => line.includes(`\`${token}\``));
    assert.ok(row, `${token} is not in the proposed-token table`);
    assert.match(row, /1\.\d /, `${token} names no section of docs/ui-guidelines.md`);
  }
});
