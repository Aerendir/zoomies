// A checker that cannot fail proves nothing, so every rule here is shown to
// fire on a page built to break it, and to stay quiet on a clean one.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { launch } from './lib/browser.mjs';
import { BASE_PAIRS, KNOWN, contrast, watch, checkPage, checkPairs, run, parseArgs } from './check.mjs';

let browser;
before(async () => {
  browser = await launch();
});
after(async () => {
  await browser?.close();
});

async function open(html, width = 800) {
  const page = await browser.newPage({ viewport: { width, height: 600 } });
  watch(page);
  await page.setContent(html);
  return page;
}
const kinds = (findings) => findings.map((f) => f.kind);

const CLEAN = `<main id="stage"><h1>Sign in</h1>
  <label>Name <input></label><button>Save</button><a href="/x">Home</a></main>`;

test('contrast: black on white is 21 to two places', () => {
  assert.equal(contrast('rgb(0,0,0)', 'rgb(255,255,255)').toFixed(2), '21.00');
});

test('contrast: mid grey on white is below the 4.5 text minimum', () => {
  assert.ok(contrast('rgb(119,119,119)', 'rgb(255,255,255)') < 4.5);
});

test('contrast: reads the comma, space and alpha forms the browser may return', () => {
  const black = contrast('rgb(0, 0, 0)', 'rgb(255, 255, 255)');
  assert.equal(contrast('rgba(0, 0, 0, 1)', 'rgb(255 255 255)'), black);
  assert.equal(contrast('rgb(0 0 0 / 1)', 'rgb(255 255 255 / 1)'), black);
});

test('contrast: a colour it cannot read is an error, not a pass', () => {
  assert.throws(() => contrast('banana', 'rgb(0,0,0)'), /colour/);
});

test('checkPage: a clean page has no findings', async () => {
  const page = await open(CLEAN);
  assert.deepEqual(await checkPage(page), []);
  await page.close();
});

test('checkPage: sideways overflow is reported', async () => {
  const page = await open(`<main id="stage"><h1>t</h1><div style="width:2000px;height:10px"></div></main>`);
  assert.deepEqual(kinds(await checkPage(page)), ['overflow']);
  await page.close();
});

test('checkPage: content cut off by an overflow rule is reported though the page does not scroll', async () => {
  // overflow: clip hides the spill from scrollWidth, so the overflow rule sees
  // a page that fits while a person sees a sentence cut off at the edge.
  const page = await open(
    `<main id="stage" style="overflow:clip"><h1>t</h1><p style="width:2000px">wide</p></main>`,
  );
  const found = await checkPage(page);
  assert.deepEqual(kinds(found), ['clipped']);
  assert.match(found[0].detail, /\bp\b/);
  await page.close();
});

test('checkPage: a decorative element hung past the edge is not content that is cut off', async () => {
  const page = await open(
    `<main id="stage" style="overflow:clip"><h1>t</h1>
     <div aria-hidden="true" style="position:absolute;left:-500px;width:300px;height:20px"></div>
     <span class="sr-only">hidden text</span></main>
     <style>.sr-only{position:absolute;width:1px;height:1px;margin:-1px;overflow:hidden;clip:rect(0,0,0,0)}</style>`,
  );
  assert.deepEqual(await checkPage(page), []);
  await page.close();
});

test('checkPage: one cause is one finding: a page that scrolls sideways is not also clipped', async () => {
  const page = await open(`<main id="stage"><h1>t</h1><p style="width:2000px">wide</p></main>`);
  assert.deepEqual(kinds(await checkPage(page)), ['overflow']);
  await page.close();
});

test('checkPage: a button with no accessible name is reported', async () => {
  const page = await open(`<main id="stage"><h1>t</h1><button></button></main>`);
  const found = await checkPage(page);
  assert.deepEqual(kinds(found), ['name']);
  assert.match(found[0].detail, /button/);
  await page.close();
});

test('checkPage: names from a label, aria-label and link text all count', async () => {
  const page = await open(
    `<main id="stage"><h1>t</h1><label>Email <input></label>
     <button aria-label="Close"></button><a href="/y">Home</a></main>`,
  );
  assert.deepEqual(await checkPage(page), []);
  await page.close();
});

test('checkPage: two h1 and no h1 are both reported', async () => {
  for (const body of ['<h1>a</h1><h1>b</h1>', '<p>no heading</p>']) {
    const page = await open(`<main id="stage">${body}</main>`);
    assert.deepEqual(kinds(await checkPage(page)), ['heading'], body);
    await page.close();
  }
});

test('checkPage: no main landmark and two main landmarks are both reported', async () => {
  for (const html of ['<div id="stage"><h1>t</h1></div>', '<main id="stage"><h1>t</h1></main><main></main>']) {
    const page = await open(html);
    assert.deepEqual(kinds(await checkPage(page)), ['landmark'], html);
    await page.close();
  }
});

test('checkPage: a positive tabindex is reported', async () => {
  const page = await open(`<main id="stage"><h1>t</h1><div tabindex="2">x</div></main>`);
  assert.ok(kinds(await checkPage(page)).includes('tabindex'));
  await page.close();
});

test('checkPage: a control that hides its focus ring is reported', async () => {
  const page = await open(
    `<style>button:focus-visible{outline:none;box-shadow:none}</style>
     <main id="stage"><h1>t</h1><button>Go</button></main>`,
  );
  const found = await checkPage(page);
  assert.deepEqual(kinds(found), ['focus']);
  assert.match(found[0].detail, /button/);
  await page.close();
});

// A native radio stretched invisibly over its label is a good way to get arrow
// keys for free, and a good way to lose the focus ring: the ring is drawn on
// the element at opacity 0, which is nothing at all.
const TAB = `<style>.tab{position:relative;display:inline-block}
  .tab input{position:absolute;inset:0;width:100%;height:100%;margin:0;opacity:0}</style>`;

test('checkPage: a focus ring painted on a control at opacity 0 is not a ring', async () => {
  const page = await open(
    `${TAB}<main id="stage"><h1>t</h1><label class="tab"><input type="radio" name="x">One</label></main>`,
  );
  const found = await checkPage(page);
  assert.deepEqual(kinds(found), ['focus']);
  assert.match(found[0].detail, /input/);
  await page.close();
});

test('checkPage: the same control passes when its label draws the ring', async () => {
  const page = await open(
    `${TAB}<style>.tab:has(input:focus-visible){outline:2px solid blue}</style>
     <main id="stage"><h1>t</h1><label class="tab"><input type="radio" name="x">One</label></main>`,
  );
  assert.deepEqual(kinds(await checkPage(page)), []);
  await page.close();
});

test('checkPage: a ring on a control inside a transparent ancestor is not a ring either', async () => {
  const page = await open(
    `<main id="stage"><h1>t</h1><div style="opacity:0"><button>Go</button></div></main>`,
  );
  assert.ok(kinds(await checkPage(page)).includes('focus'));
  await page.close();
});

test('checkPage: a console error and an uncaught exception are both reported', async () => {
  for (const script of ["console.error('boom')", "throw new Error('boom')"]) {
    const page = await open(`<main id="stage"><h1>t</h1><script>${script}</script></main>`);
    const found = await checkPage(page);
    assert.deepEqual(kinds(found), ['console'], script);
    assert.match(found[0].detail, /boom/);
    await page.close();
  }
});

test('checkPage: a console error is reported once, against the check that found it', async () => {
  const page = await open(`<main id="stage"><h1>t</h1><script>console.error('once')</script></main>`);
  assert.deepEqual(kinds(await checkPage(page)), ['console']);
  assert.deepEqual(await checkPage(page), []);
  await page.close();
});

test('checkPage: review chrome outside #stage is not judged as the concept', async () => {
  const page = await open(`<nav><button></button></nav>${CLEAN}`);
  assert.deepEqual(await checkPage(page), []);
  await page.close();
});

test('checkPage: layoutOnly looks at overflow and console and nothing slower', async () => {
  const page = await open(
    `<main id="stage"><button></button><div style="width:2000px;height:10px"></div></main>`,
  );
  assert.deepEqual(kinds(await checkPage(page, { layoutOnly: true })), ['overflow']);
  await page.close();
});

const TOKENS = `<style>:root{--fg:#777777;--bg:#ffffff;--z-surface:#ffffff;
  --wash:rgb(0 0 0 / 0.5);--ink:#000000}</style><main id="stage"><h1>t</h1></main>`;

test('checkPairs: a pair below its minimum names both tokens and the ratio', async () => {
  const page = await open(TOKENS);
  const found = await checkPairs(page, [{ fg: '--fg', bg: '--bg', min: 4.5 }]);
  assert.deepEqual(kinds(found), ['contrast']);
  assert.match(found[0].detail, /--fg on --bg/);
  assert.match(found[0].detail, /4\.4\d:1/);
  await page.close();
});

test('checkPairs: the same pair passes against the 3:1 control minimum', async () => {
  const page = await open(TOKENS);
  assert.deepEqual(await checkPairs(page, [{ fg: '--fg', bg: '--bg', min: 3 }]), []);
  await page.close();
});

// The surface here is black on purpose. Laid over black, a 50% white wash is a
// mid grey; laid over the canvas's own white it would be white. Only the first
// is right, and only a non-white surface can tell the two apart.
const DARK_SURFACE = `<style>:root{--z-surface:#000000;--wash:rgb(255 255 255 / 0.5);
  --paper:#ffffff;--veil:rgb(0 0 0 / 0.5);--page:#ffffff}</style><main id="stage"><h1>t</h1></main>`;

test('checkPairs: a translucent background is composited over --z-surface, not over white', async () => {
  const page = await open(DARK_SURFACE);
  // White on that grey is about 3.9:1: enough for a control edge...
  assert.deepEqual(await checkPairs(page, [{ fg: '--paper', bg: '--wash', min: 3 }]), []);
  // ...and short of the text minimum, by the amount the grey says.
  const found = await checkPairs(page, [{ fg: '--paper', bg: '--wash', min: 4.5 }]);
  assert.deepEqual(kinds(found), ['contrast']);
  assert.match(found[0].detail, /3\.9\d:1/);
  await page.close();
});

test('checkPairs: a translucent foreground is composited over its background', async () => {
  // 50% black on white is a mid grey, about 4:1 against that white (a range,
  // because the grey is 127 or 128 depending on rounding). Read as opaque black
  // it would be 21:1 and pass.
  const page = await open(DARK_SURFACE);
  const found = await checkPairs(page, [{ fg: '--veil', bg: '--page', min: 4.5 }]);
  assert.deepEqual(kinds(found), ['contrast']);
  assert.match(found[0].detail, /\b(3\.9\d|4\.0\d):1/);
  await page.close();
});

test('checkPairs: a token that is not defined is reported, not skipped', async () => {
  const page = await open(TOKENS);
  const found = await checkPairs(page, [{ fg: '--nope', bg: '--bg', min: 3 }]);
  assert.deepEqual(kinds(found), ['contrast']);
  assert.match(found[0].detail, /--nope is not defined/);
  await page.close();
});

test('parseArgs: reads --screen and --out, and refuses anything else', () => {
  assert.deepEqual(parseArgs(['--screen', 'signin', '--out', 'x.json']), { screen: 'signin', out: 'x.json' });
  assert.deepEqual(parseArgs([]), {});
  assert.throws(() => parseArgs(['--frobnicate']), /unknown option/);
  assert.throws(() => parseArgs(['--screen']), /needs a value/);
});

// A stand-in for mockups.html that obeys the contract the real page must meet:
// five selects, a <main id="stage">, a hash that drives them, and
// data-rendered set once the stage has been redrawn. Concept B is wider than a
// phone, so it must be caught at 320px and nowhere else.
const FIXTURE = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>fixture</title>
<style>
:root{--z-text:#000;--z-surface:#fff;--z-edge:#cccccc}
:root[data-theme=dark]{--z-text:#fff;--z-surface:#000;--z-edge:#333333}
:root[data-ui-style=mission-control]{--z-edge:#555555}
:root[data-ui-style=mission-control][data-theme=dark]{--z-edge:#aaaaaa}
body{margin:0;background:var(--z-surface);color:var(--z-text)}
.wide{width:600px;height:10px}
</style></head><body>
<select id="screen" aria-label="Screen"><option value="one">One</option><option value="two">Two</option></select>
<select id="concept" aria-label="Concept"><option value="current">Current</option><option value="a">A</option><option value="b">B</option></select>
<select id="style" aria-label="Style"><option value="mission-control">Mission Control</option><option value="classic">Classic</option></select>
<select id="mode" aria-label="Mode"><option value="light">Light</option><option value="dark">Dark</option></select>
<select id="state" aria-label="State"></select>
<main id="stage"></main>
<script>
const STATES = { one: ['s1', 's2'], two: ['s1'] };
function apply() {
  const q = new URLSearchParams(location.hash.slice(1));
  const screen = STATES[q.get('screen')] ? q.get('screen') : 'one';
  const concept = ['current', 'a', 'b'].includes(q.get('concept')) ? q.get('concept') : 'a';
  const style = q.get('style') === 'classic' ? 'classic' : 'mission-control';
  const mode = q.get('mode') === 'dark' ? 'dark' : 'light';
  const state = STATES[screen].includes(q.get('state')) ? q.get('state') : STATES[screen][0];
  const root = document.documentElement;
  if (style === 'classic') root.removeAttribute('data-ui-style'); else root.setAttribute('data-ui-style', style);
  root.setAttribute('data-theme', mode);
  root.setAttribute('data-concept', concept);
  document.getElementById('screen').value = screen;
  document.getElementById('concept').value = concept;
  document.getElementById('style').value = style;
  document.getElementById('mode').value = mode;
  const states = document.getElementById('state');
  states.innerHTML = STATES[screen].map((s) => '<option value="' + s + '">' + s + '</option>').join('');
  states.value = state;
  document.getElementById('stage').innerHTML =
    '<h1>' + screen + '</h1><button>Go</button>' + (concept === 'b' ? '<div class="wide"></div>' : '');
  root.dataset.rendered = location.hash;
}
addEventListener('hashchange', apply);
apply();
</script></body></html>`;

let fixture;
before(async () => {
  const server = createServer((req, res) => {
    if (req.url.startsWith('/review/mockups.html')) {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(FIXTURE);
    } else {
      res.writeHead(404).end();
    }
  });
  await new Promise((ready) => server.listen(0, '127.0.0.1', ready));
  fixture = { server, url: `http://127.0.0.1:${server.address().port}` };
});
after(() => fixture?.server.closeAllConnections() ?? fixture?.server.close());

const PAIRS = [{ fg: '--z-text', bg: '--z-surface', min: 4.5 }];

test('run: visits every screen, state, concept, style, mode and width, and labels what it finds', async () => {
  const result = await run({ url: fixture.url, widths: [320, 1440], pairs: PAIRS, conceptPairs: {} });
  // 3 screen-states x 2 live concepts (not 'current') x 2 styles x 2 modes x 2 widths.
  assert.equal(result.combinations, 48);
  assert.equal(result.findings.length, 12);
  for (const f of result.findings) {
    assert.equal(f.kind, 'overflow');
    assert.equal(f.concept, 'b');
    assert.equal(f.width, 320);
    assert.ok(['one', 'two'].includes(f.screen));
    assert.ok(['light', 'dark'].includes(f.mode));
  }
});

test('run: --screen limits the visit to one screen', async () => {
  const result = await run({ url: fixture.url, screen: 'two', widths: [320, 1440], pairs: PAIRS, conceptPairs: {} });
  assert.equal(result.combinations, 16);
  assert.equal(result.findings.length, 4);
});

test('run: an unknown screen is an error that lists the real ones', async () => {
  await assert.rejects(
    run({ url: fixture.url, screen: 'nope', widths: [1440], pairs: PAIRS, conceptPairs: {} }),
    /unknown screen "nope".*one.*two/s,
  );
});

test('run: a contrast pair that fails is reported once per concept, style and mode', async () => {
  const result = await run({
    url: fixture.url,
    widths: [1440],
    pairs: [{ fg: '--z-text', bg: '--z-surface', min: 22 }],
    conceptPairs: {},
  });
  const contrastFindings = result.findings.filter((f) => f.kind === 'contrast');
  assert.equal(contrastFindings.length, 8);
  assert.ok(contrastFindings.every((f) => f.screen === null && f.width === null));
});

test('run: a concept\'s own pairs are checked only on that concept', async () => {
  const result = await run({
    url: fixture.url,
    widths: [1440],
    pairs: PAIRS,
    conceptPairs: { a: [{ fg: '--z-text', bg: '--z-surface', min: 22 }] },
  });
  const contrastFindings = result.findings.filter((f) => f.kind === 'contrast');
  assert.equal(contrastFindings.length, 4);
  assert.ok(contrastFindings.every((f) => f.concept === 'a'));
});

// A finding that is a property of the product's own tokens, and the same for
// every concept, cannot be fixed by a concept. It is recorded with its reason
// and a floor, so it neither turns every screen red for ever nor hides a change
// for the worse. The fixture's edge token is pale in Classic and strong in
// Mission Control, as the product's is.
const EDGE = [{ fg: '--z-edge', bg: '--z-surface', min: 3 }];
const PALE_IN_CLASSIC = {
  style: 'classic',
  fg: '--z-edge',
  bg: '--z-surface',
  atLeast: 1.5,
  reason: 'the edge is pale in Classic',
};

test('run: a known issue is reported as known and does not count as a finding', async () => {
  const result = await run({
    url: fixture.url, widths: [1440], pairs: EDGE, conceptPairs: {}, known: [PALE_IN_CLASSIC],
  });
  assert.deepEqual(result.findings, []);
  // Two live concepts, in Classic, in two modes.
  assert.equal(result.known.length, 4);
  for (const k of result.known) {
    assert.equal(k.style, 'classic');
    assert.equal(k.reason, 'the edge is pale in Classic');
    assert.ok(k.ratio >= 1.5 && k.ratio < 3);
  }
});

test('run: with nothing known the same pair is a finding in every concept and mode', async () => {
  const result = await run({ url: fixture.url, widths: [1440], pairs: EDGE, conceptPairs: {}, known: [] });
  assert.equal(result.findings.filter((f) => f.kind === 'contrast').length, 4);
  assert.deepEqual(result.known, []);
});

test('run: a known issue in one style does not hide the same pair failing in another', async () => {
  const result = await run({
    url: fixture.url,
    widths: [1440],
    pairs: [{ fg: '--z-edge', bg: '--z-surface', min: 10 }],
    conceptPairs: {},
    known: [PALE_IN_CLASSIC],
  });
  const failing = result.findings.filter((f) => f.kind === 'contrast');
  assert.equal(failing.length, 4);
  assert.ok(failing.every((f) => f.style === 'mission-control'));
  assert.equal(result.known.length, 4);
});

test('run: a known issue stops covering a pair that has got worse than its floor', async () => {
  const result = await run({
    url: fixture.url,
    widths: [1440],
    pairs: EDGE,
    conceptPairs: {},
    known: [{ ...PALE_IN_CLASSIC, atLeast: 1.7 }],
  });
  assert.equal(result.findings.filter((f) => f.kind === 'contrast').length, 4);
  assert.deepEqual(result.known, []);
});

test('checkPairs: a finding carries the pair and the ratio, so a baseline can match it', async () => {
  const page = await open(TOKENS);
  const [found] = await checkPairs(page, [{ fg: '--fg', bg: '--bg', min: 4.5 }]);
  assert.equal(found.fg, '--fg');
  assert.equal(found.bg, '--bg');
  assert.equal(found.min, 4.5);
  assert.ok(found.ratio > 4.4 && found.ratio < 4.5);
  await page.close();
});

test('KNOWN: every entry says why, and the Classic control edge is the one on record', () => {
  for (const k of KNOWN) {
    assert.ok(k.reason.length > 40, `${k.fg} on ${k.bg} needs a real reason`);
    assert.equal(typeof k.atLeast, 'number');
  }
  assert.ok(
    KNOWN.some((k) => k.style === 'classic' && k.fg === '--z-border-strong' && k.bg === '--z-surface'),
  );
});

// A pair that names a token nobody defines is reported as a finding on every
// page, so it would never be silent. What it could be is wrong in the other
// direction: a typo in the baseline that stops a known issue matching.
const here = (path) => fileURLToPath(new URL(path, import.meta.url));
const definedTokens = new Set(
  [...readFileSync(here('../../../web/src/lib/styles/tokens.css'), 'utf8').matchAll(/^\s*(--z-[a-z0-9-]+):/gm)].map((m) => m[1]).concat(
    [...readFileSync(here('../public/concept-c-tokens.css'), 'utf8').matchAll(/^\s*(--z-[a-z0-9-]+):/gm)].map((m) => m[1]),
  ),
);

test('every pair the checker applies, and every token the baseline names, exists', () => {
  const conceptC = JSON.parse(readFileSync(here('../public/concept-c-pairs.json'), 'utf8'));
  for (const { fg, bg } of [...BASE_PAIRS, ...conceptC, ...KNOWN]) {
    assert.ok(definedTokens.has(fg), `${fg} is not defined in tokens.css or concept-c-tokens.css`);
    assert.ok(definedTokens.has(bg), `${bg} is not defined in tokens.css or concept-c-tokens.css`);
  }
});

// A list of colour pairs is only as good as its match with what is painted. The
// first version named seven pairs while the concepts drew hints, primary buttons
// and severity labels in colours none of them covered, and a concept could have
// put failing text on a page the checker called green.
test('every text colour the concepts draw is the foreground of a checked pair', () => {
  const drawn = new Set();
  for (const file of ['components', 'concept-a', 'concept-b', 'concept-c']) {
    const css = readFileSync(here(`../public/${file}.css`), 'utf8');
    for (const match of css.matchAll(/(?<![-\w])color:\s*var\((--z-[a-z0-9-]+)/g)) drawn.add(match[1]);
  }
  const conceptC = JSON.parse(readFileSync(here('../public/concept-c-pairs.json'), 'utf8'));
  const checked = new Set([...BASE_PAIRS, ...conceptC].map((pair) => pair.fg));
  const unchecked = [...drawn].filter((token) => !checked.has(token)).sort();
  assert.deepEqual(unchecked, [], `drawn as text and checked against nothing: ${unchecked.join(', ')}`);
});

test('every base pair names a minimum the two ratios allow', () => {
  for (const pair of BASE_PAIRS) assert.ok(pair.min === 3 || pair.min === 4.5, `${pair.fg} on ${pair.bg}`);
});
