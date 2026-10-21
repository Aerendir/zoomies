// Photograph the signed-in pages as they are today, from the real controller.
//
// The mocks are judged against the page they would replace, so this is the
// "before": the same binary the Playwright suite drives, with the seeded demo
// fleet and authentication off on loopback, in Mission Control dark at 1440 and
// on a phone. No response is stubbed. A page is photographed whole, but never
// taller than a long list would make a picture nobody can read, and its height
// is recorded, so a claim like "the Jobs page is five screens" is a number.
//
//   make build   (the binary must have the UI embedded)
//   node design/webui-refresh-review/tools/capture-signed-in.mjs [id,id]
import { spawn } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { assertLook, rememberLook, storedLook } from '../../../web/tests/support/look.mjs';
import { launch } from './lib/browser.mjs';

const here = fileURLToPath(new URL('.', import.meta.url));
const web = resolve(here, '../../../web');
const out = resolve(here, '../signed-in');
const PORT = 8096;
const SIZES = [
  { width: 1440, height: 900 },
  { width: 375, height: 812 },
];
/** The tallest a picture is allowed to be. The real height is recorded beside it. */
const MAX_HEIGHT = 2400;

/** Pages that need no id. The sidebar's own order, then the setup flows. */
export const STATIC_PAGES = [
  { id: 'overview', path: '/' },
  { id: 'pools', path: '/pools' },
  { id: 'runners', path: '/runners' },
  { id: 'queue', path: '/queue' },
  { id: 'jobs', path: '/jobs' },
  { id: 'workflows', path: '/workflows' },
  { id: 'comparisons', path: '/comparisons' },
  { id: 'usage', path: '/usage' },
  { id: 'hosts', path: '/hosts' },
  { id: 'providers', path: '/providers' },
  { id: 'installations', path: '/installations' },
  { id: 'repairs', path: '/repairs' },
  { id: 'migrate', path: '/migrate' },
  { id: 'kennel', path: '/kennel' },
  { id: 'ai-context', path: '/ai-context' },
  { id: 'audit', path: '/audit' },
  { id: 'settings', path: '/settings' },
  { id: 'pool-new', path: '/pools/new' },
  { id: 'host-new', path: '/hosts/new' },
  { id: 'provider-new', path: '/providers/new' },
];

/** Pages reached through the first thing of a kind the fleet has. */
export const DETAIL_PAGES = [
  { id: 'pool-detail', api: '/api/v1/pools', path: (id) => `/pools/${id}` },
  { id: 'host-detail', api: '/api/v1/hosts', path: (id) => `/hosts/${id}` },
  { id: 'runner-detail', api: '/api/v1/runners?limit=1', path: (id) => `/runners/${id}` },
];

/** Every file name this script writes for the pages it photographs. */
export function files(ids) {
  return ids.flatMap((id) => SIZES.map((size) => `current-${id}-${size.width}.png`));
}

async function startController() {
  const child = spawn('node', [join('tests/support', 'serve.mjs'), String(PORT)], {
    cwd: web,
    stdio: ['ignore', 'ignore', 'inherit'],
  });
  const exited = new Promise((done) => child.once('exit', done));
  const url = `http://127.0.0.1:${PORT}`;
  for (let i = 0; i < 150; i++) {
    if (child.exitCode !== null) throw new Error('the demo controller exited before it was ready');
    try {
      if ((await fetch(`${url}/healthz`)).ok) {
        return {
          url,
          stop: async () => {
            child.kill('SIGTERM');
            await exited;
          },
        };
      }
    } catch {
      /* not listening yet */
    }
    await sleep(200);
  }
  child.kill('SIGTERM');
  throw new Error(`the demo controller did not become healthy on port ${PORT}`);
}

/** What the findings cite, read from the page as rendered. */
function read(viewportHeight) {
  const table = document.querySelector('main table, [role="grid"], [role="table"]');
  const headers = table
    ? [...table.querySelectorAll('th, [role="columnheader"]')].map((el) => el.textContent.trim())
    : [];
  return {
    documentHeight: document.documentElement.scrollHeight,
    viewportHeight,
    scrollsSideways: document.documentElement.scrollWidth > innerWidth,
    heading: document.querySelector('h1')?.textContent.trim() ?? null,
    tables: document.querySelectorAll('main table, main [role="grid"]').length,
    columns: headers.length,
    cards: document.querySelectorAll('main [class*="card"]').length,
    buttons: document.querySelectorAll('main button, main a.btn').length,
  };
}

async function main(argv) {
  const only = (argv[0] ?? '').split(',').filter(Boolean);
  const controller = await startController();
  const browser = await launch();
  const measurements = { generated: new Date().toISOString(), look: 'mission-control dark', shots: {} };
  try {
    const context = await browser.newContext({
      baseURL: controller.url,
      viewport: SIZES[0],
      colorScheme: 'dark',
      deviceScaleFactor: 1,
    });
    await context.addInitScript(rememberLook, storedLook('dark', 'mission-control'));
    const page = await context.newPage();

    const pages = [...STATIC_PAGES];
    for (const detail of DETAIL_PAGES) {
      const body = await (await context.request.get(`${controller.url}${detail.api}`)).json();
      const first = body.items?.[0]?.id;
      if (first) pages.push({ id: detail.id, path: detail.path(first) });
    }

    for (const target of pages.filter((p) => only.length === 0 || only.includes(p.id))) {
      for (const size of SIZES) {
        await page.setViewportSize(size);
        await page.goto(target.path, { waitUntil: 'networkidle' });
        await page.locator('main h1, h1').first().waitFor({ timeout: 15000 }).catch(() => {});
        await assertLook(page, target.id, 'dark', 'mission-control');
        await page.evaluate(() => document.fonts.ready);
        await sleep(500);
        const m = await page.evaluate(read, size.height);
        await page.screenshot({
          path: join(out, `current-${target.id}-${size.width}.png`),
          fullPage: true,
          clip: { x: 0, y: 0, width: size.width, height: Math.min(m.documentHeight, MAX_HEIGHT) },
        });
        measurements.shots[`${target.id}-${size.width}`] = { path: target.path, ...m };
        console.log(`${target.id} ${size.width} (${m.documentHeight}px)`);
      }
    }
    await context.close();
    writeFileSync(
      join(out, only.length ? 'current-measurements.partial.json' : 'current-measurements.json'),
      `${JSON.stringify(measurements, null, 2)}\n`,
    );
  } finally {
    await browser.close();
    await controller.stop();
  }
}

if (process.argv[1] && import.meta.url === new URL(`file://${process.argv[1]}`).href) {
  main(process.argv.slice(2)).catch((error) => {
    console.error(error.message);
    process.exitCode = 2;
  });
}
