// A position in the README is compared with a photograph of today's page, which
// has no review bar above it. A number taken beneath the bar was off by whatever
// the bar did to the layout: half its height for a concept that centres, and
// 263px of it on a phone, where the five controls wrap. So the measurement is
// held to one rule, tested before anything is measured: the bar is not there.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './lib/browser.mjs';
import { CASES, measure, plan } from './measure.mjs';

let browser;
before(async () => {
  browser = await launch();
});
after(async () => {
  await browser?.close();
});

async function open(html, width = 400, height = 600) {
  const page = await browser.newPage({ viewport: { width, height } });
  await page.setContent(html);
  return page;
}

const BAR = '<header class="review-bar" style="height:300px">controls</header>';
const SPEC = { first: 'input', action: { text: '^sign in$' } };
const pick = ({ page, fits }) => ({ page, fits });

test('the review bar does not move a number: it is hidden before the page is measured', async () => {
  const page = await open(`${BAR}<main id="stage"><input><button>Sign in</button></main>`);
  const withBar = await measure(page, SPEC);
  const bare = await open(`<main id="stage"><input><button>Sign in</button></main>`);
  assert.deepEqual(withBar, await measure(bare, SPEC));
  assert.ok(withBar.action.bottom < 100, `the button is ${withBar.action.bottom}px down, not ${withBar.action.bottom + 300}`);
  await page.close();
  await bare.close();
});

test('numbers are whole pixels, with the field as a top and a bottom and the page as its full height', async () => {
  const page = await open(`<main id="stage" style="height:1234.4px"><input style="height:32.4px"><button>Sign in</button></main>`);
  const got = await measure(page, SPEC);
  assert.ok(Number.isInteger(got.first.top) && Number.isInteger(got.first.bottom));
  assert.equal(got.first.bottom - got.first.top, 32);
  assert.ok(Number.isInteger(got.action.bottom));
  assert.ok(got.page >= 1234, `the page is ${got.page}px`);
  await page.close();
});

test('a page that fits the window reads as exactly the window tall, and says it fits', async () => {
  // A page cannot be shorter than the window, so "900px" for a 900px window
  // means "needs no scrolling" and not "is 900px of content".
  const short = await open(
    `<style>body{margin:0}main{min-height:100vh}</style><main id="stage"><p style="margin:0;height:300px">x</p></main>`,
    400,
    900,
  );
  assert.deepEqual(pick(await measure(short, {})), { page: 900, fits: true });
  const tall = await open(`<style>body{margin:0}</style><main id="stage" style="height:1500px"></main>`, 400, 900);
  assert.deepEqual(pick(await measure(tall, {})), { page: 1500, fits: false });
  await short.close();
  await tall.close();
});

test('a group folded away behind a disclosure is not measured, because it is not on the page', async () => {
  // Concept B keeps the blocking causes behind a disclosure. They still have
  // boxes when it is closed, and the README must say "behind a disclosure", not
  // quote the place a box would be if it were open.
  const folded = await open(
    `<main id="stage"><p class="cause">shown</p><details><summary>more</summary><p class="cause">folded</p></details></main>`,
  );
  const got = await measure(folded, { last: '.cause' });
  const open_ = await open(
    `<main id="stage"><p class="cause">shown</p><details open><summary>more</summary><p class="cause">folded</p></details></main>`,
  );
  assert.ok(got.last.bottom < (await measure(open_, { last: '.cause' })).last.bottom, 'the folded one is not the last');
  const hidden = await open(`<main id="stage"><details><summary>more</summary><p class="cause">folded</p></details></main>`);
  assert.equal((await measure(hidden, { last: '.cause' })).last, null);
  await folded.close();
  await open_.close();
  await hidden.close();
});

test('an action stuck to the bottom of the screen is recorded as pinned, and one that scrolls is not', async () => {
  const pinned = await open(
    `<main id="stage" style="height:2000px"><div style="position:sticky;bottom:0"><button>Allow</button></div></main>`,
  );
  assert.equal((await measure(pinned, { action: { text: '^allow' } })).action.pinned, true);
  const loose = await open(`<main id="stage" style="height:2000px"><button>Allow</button></main>`);
  assert.equal((await measure(loose, { action: { text: '^allow' } })).action.pinned, false);
  await pinned.close();
  await loose.close();
});

test('a screen that has no first field says so, and a missing action is an error that names it', async () => {
  const page = await open(`<main id="stage"><button>Allow</button></main>`);
  assert.equal((await measure(page, { action: { text: '^allow' } })).first, null);
  await assert.rejects(measure(page, { action: { text: '^nothing$' } }), /no action matching \^nothing\$/);
  await page.close();
});

test('the plan is every screen at both widths, in the state its table is about, for each concept', () => {
  const screens = [...new Set(CASES.map((c) => c.screen))].sort();
  assert.deepEqual(screens, ['connect-app', 'first-account', 'fleet-status', 'signin']);
  const shots = plan(screens);
  for (const screen of screens) {
    for (const concept of ['a', 'b', 'c']) {
      for (const width of [1440, 375]) {
        assert.ok(
          shots.some((s) => s.screen === screen && s.concept === concept && s.width === width),
          `${screen} ${concept} ${width} is not measured`,
        );
      }
    }
  }
  // The consent screen's warning is a second state, because it is the one that drops Allow below the fold.
  assert.deepEqual([...new Set(shots.filter((s) => s.screen === 'connect-app').map((s) => s.state))].sort(), ['loopback', 'normal']);
  assert.equal(new Set(shots.map((s) => `${s.screen}-${s.concept}-${s.state}-${s.width}`)).size, shots.length);
});
