// A checker that cannot fail proves nothing, so every rule here is shown to
// fire on a page built to break it, and to stay quiet on a clean one.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { launch } from './lib/browser.mjs';
import { contrast, watch, checkPage, checkPairs, run, parseArgs } from './check.mjs';

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
:root{--z-text:#000;--z-surface:#fff}
:root[data-theme=dark]{--z-text:#fff;--z-surface:#000}
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
