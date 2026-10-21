// Photograph the public pages as they are today, from the real controller.
//
// A concept is only as honest as the "before" it is drawn against, so these are
// taken from the actual binary, the same one the Playwright suite drives, with
// no stubbed response: the first-run controller for the first account, sign in
// and the MCP consent screen, the status fixture for the public status page,
// and the demo fleet for a wrong address. Beside each picture it records the
// measurements the findings in the README cite, so a claim like "the button is
// below the fold" is a number that can be regenerated and not a judgement.
//
//   make build   (the binary must have the UI embedded)
//   node design/webui-refresh-review/tools/capture-current.mjs
import { spawn } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { assertLook, rememberLook, storedLook } from '../../../web/tests/support/look.mjs';
import { launch } from './lib/browser.mjs';

const here = fileURLToPath(new URL('.', import.meta.url));
const web = resolve(here, '../../../web');
const out = resolve(here, '../public');

const SIZES = [
  { width: 1440, height: 900 },
  { width: 375, height: 812 },
];
const ADMIN = { username: 'ada', password: 'correct horse battery staple' };
const CALLBACK = 'https://claude.ai/api/mcp/auth_callback';
const LOOPBACK_CALLBACK = 'http://127.0.0.1:8123/callback';

/** Run one of the suite's own fixture servers and wait until it answers. */
async function startController(script, port, ...args) {
  const child = spawn('node', [join('tests/support', script), String(port), ...args], {
    cwd: web,
    stdio: ['ignore', 'ignore', 'inherit'],
  });
  const exited = new Promise((done) => child.once('exit', done));
  const url = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 150; i++) {
    if (child.exitCode !== null) throw new Error(`${script} exited before it was ready`);
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
  throw new Error(`${script} did not become healthy on port ${port}`);
}

async function readToken(file) {
  for (let i = 0; i < 100; i++) {
    try {
      const token = readFileSync(file, 'utf8').trim();
      if (token) return token;
    } catch {
      /* the controller has not printed it yet */
    }
    await sleep(100);
  }
  throw new Error('the controller never printed a setup token');
}

const measurements = { generated: new Date().toISOString(), shots: {}, banners: {} };

/** Everything the findings cite, read from the page as rendered. */
async function measure(page, screen, size) {
  const m = await page.evaluate((viewportHeight) => {
    const box = (el) => {
      const r = el.getBoundingClientRect();
      return { x: Math.round(r.x), y: Math.round(r.y), width: Math.round(r.width), height: Math.round(r.height) };
    };
    return {
      logos: [...document.querySelectorAll('.logo')].map((el) => ({
        variant: [...el.classList].filter((c) => ['mark', 'wordmark', 'full', 'lockup'].includes(c)).join(' '),
        ...box(el),
      })),
      images: document.querySelectorAll('img').length,
      scrollWidth: document.documentElement.scrollWidth,
      documentHeight: document.documentElement.scrollHeight,
      viewportHeight,
      theme: document.documentElement.getAttribute('data-theme'),
      uiStyle: document.documentElement.getAttribute('data-ui-style'),
    };
  }, size.height);
  measurements.shots[`${screen}-${size.width}`] = m;
  return m;
}

/** The computed look of a banner, for the finding that three of them differ. */
async function banner(page, name, selector) {
  measurements.banners[name] = await page.locator(selector).first().evaluate((el) => {
    const s = getComputedStyle(el);
    const icon = el.querySelector('svg');
    const iconStyle = icon && getComputedStyle(icon);
    return {
      borderRadius: s.borderRadius,
      padding: s.padding,
      gap: s.gap,
      fontSize: s.fontSize,
      iconMarginTop: iconStyle?.marginTop ?? null,
      iconSize: icon ? `${icon.getAttribute('width')}px` : null,
    };
  });
}

async function shoot(page, screen, size, heading, { app = true, action = null } = {}) {
  await page.setViewportSize(size);
  await page.getByRole('heading', { level: 1, name: heading }).waitFor();
  if (app) await assertLook(page, screen, 'dark', 'mission-control');
  await page.evaluate(() => document.fonts.ready);
  await sleep(300);
  await page.screenshot({
    path: join(out, `current-${screen}-${size.width}.png`),
    // A phone screen is read by scrolling, so it is photographed whole.
    fullPage: size.width < 800,
  });
  const m = await measure(page, screen, size);
  if (action) {
    // Where the one button a person came to press sits against the fold.
    // `within` scopes the lookup: the shell's brand link is also called "Go to
    // the overview", and measuring that one would report the wrong button.
    const box = await (action.within ? page.locator(action.within) : page)
      .getByRole(action.role, { name: action.name })
      .filter({ visible: true })
      .first()
      .boundingBox();
    m.action = { name: action.name, y: Math.round(box.y), bottom: Math.round(box.y + box.height) };
    m.action.belowTheFold = m.action.bottom > size.height;
  }
  const input = page.locator('input:visible').first();
  if (await input.count()) {
    const box = await input.boundingBox();
    m.firstInput = { y: Math.round(box.y), bottom: Math.round(box.y + box.height) };
  }
  return m;
}

async function withContext(browser, base, { app = true } = {}) {
  const context = await browser.newContext({
    baseURL: base,
    viewport: SIZES[0],
    colorScheme: 'dark',
    deviceScaleFactor: 1,
  });
  if (app) await context.addInitScript(rememberLook, storedLook('dark', 'mission-control'));
  return context;
}

async function registerAndAuthorise(page, name, redirect) {
  const registered = await page.request.post('/oauth/register', {
    data: { client_name: name, redirect_uris: [redirect], token_endpoint_auth_method: 'none' },
  });
  if (registered.status() !== 201) throw new Error(`registering ${name} answered ${registered.status()}`);
  const { client_id: clientId } = await registered.json();
  const challenge = createHash('sha256').update(randomBytes(48).toString('base64url')).digest('base64url');
  const query = new URLSearchParams({
    client_id: clientId,
    redirect_uri: redirect,
    response_type: 'code',
    code_challenge: challenge,
    code_challenge_method: 'S256',
    state: 'review',
    scope: 'mcp:read mcp:operate',
  });
  await page.goto(`/oauth/authorize?${query}`);
}

const scratch = mkdtempSync(join(tmpdir(), 'zoomies-review-'));
const running = [];
const browser = await launch();
try {
  /* first-run controller: first account, sign in, consent */
  const tokenFile = join(scratch, 'setup-token');
  const firstRun = await startController('serve-firstrun.mjs', 8098, tokenFile);
  running.push(firstRun);

  const fresh = await withContext(browser, firstRun.url);
  const bootstrap = await fresh.newPage();
  await bootstrap.goto('/');
  for (const size of SIZES) {
    const m = await shoot(bootstrap, 'first-account', size, 'Create the first account', {
      action: { role: 'button', name: 'Create the account' },
    });
    m.firstFieldText = await bootstrap.locator('form').first().locator('> *').first().innerText();
  }
  // A real failure, for the banner measurement: a token that is not the one the
  // controller printed.
  await bootstrap.setViewportSize(SIZES[0]);
  await bootstrap.locator('input[name="setup-token"]').fill('not-the-token');
  await bootstrap.locator('input[name="username"]').fill(ADMIN.username);
  await bootstrap.locator('input[name="password"]').fill(ADMIN.password);
  await bootstrap.locator('input[name="confirm-password"]').fill(ADMIN.password);
  await bootstrap.getByRole('button', { name: 'Create the account' }).click();
  await bootstrap.getByRole('alert').first().waitFor();
  await banner(bootstrap, 'first-account', '.failure');

  const made = await fresh.request.post('/api/v1/auth/bootstrap', {
    data: { ...ADMIN, setup_token: await readToken(tokenFile) },
  });
  if (!made.ok()) throw new Error(`creating the first account answered ${made.status()}`);
  await fresh.close();

  const signedOut = await withContext(browser, firstRun.url);
  const login = await signedOut.newPage();
  await login.goto('/login');
  for (const size of SIZES) {
    await shoot(login, 'signin', size, 'Sign in to Zoomies', {
      action: { role: 'button', name: 'Sign in' },
    });
  }
  await login.setViewportSize(SIZES[0]);
  await login.locator('input[name="username"]').fill(ADMIN.username);
  await login.locator('input[name="password"]').fill('not-the-password');
  await login.getByRole('button', { name: 'Sign in' }).click();
  await login.getByRole('alert').first().waitFor();
  await banner(login, 'signin', '.failure');
  await signedOut.close();

  const signedIn = await withContext(browser, firstRun.url);
  const consent = await signedIn.newPage();
  const loggedIn = await signedIn.request.post('/api/v1/auth/login', {
    data: ADMIN,
    headers: { Origin: firstRun.url },
  });
  if (loggedIn.status() !== 200) throw new Error(`signing in answered ${loggedIn.status()}`);
  await registerAndAuthorise(consent, 'Claude', CALLBACK);
  for (const size of SIZES) {
    await shoot(consent, 'connect-app', size, /Connect Claude to Zoomies/, {
      action: { role: 'button', name: 'Allow' },
    });
  }
  // The loopback client's warning is the state the tone question is about, so
  // it is photographed once, at desktop width, and its banner measured.
  await registerAndAuthorise(consent, 'Local agent', LOOPBACK_CALLBACK);
  await shoot(consent, 'connect-app-loopback', SIZES[0], /Connect Local agent to Zoomies/, {
    action: { role: 'button', name: 'Allow' },
  });
  await banner(consent, 'connect-app', '.warning');
  await signedIn.close();
  await firstRun.stop();
  running.pop();

  /* status fixture: the public fleet status page */
  const status = await startController('serve-status.mjs', 8095);
  running.push(status);
  const publicContext = await withContext(browser, status.url, { app: false });
  const statusPage = await publicContext.newPage();
  await statusPage.goto('/status');
  for (const size of SIZES) {
    const m = await shoot(statusPage, 'fleet-status', size, 'Fleet status', { app: false });
    m.stateWord = await statusPage.locator('#state-word').innerText();
    m.reasons = await statusPage.locator('.reason').count();
  }
  await publicContext.close();
  await status.stop();
  running.pop();

  /* demo fleet: a wrong address */
  const demo = await startController('serve.mjs', 8099);
  running.push(demo);
  const notFoundContext = await withContext(browser, demo.url);
  const lost = await notFoundContext.newPage();
  await lost.goto('/no-such-page');
  for (const size of SIZES) {
    await shoot(lost, 'not-found', size, 'Page not found', {
      action: { role: 'link', name: 'Go to the overview', within: 'main' },
    });
  }
  await notFoundContext.close();
  await demo.stop();
  running.pop();

  measurements.sources = {
    loginSvelteLines: readFileSync(resolve(web, 'src/routes/Login.svelte'), 'utf8').split('\n').length - 1,
  };
  writeFileSync(join(out, 'current-measurements.json'), `${JSON.stringify(measurements, null, 2)}\n`);
  console.log(`Photographed ${Object.keys(measurements.shots).length} screens into ${out}`);
} finally {
  await browser.close();
  for (const controller of running) await controller.stop().catch(() => {});
  rmSync(scratch, { recursive: true, force: true });
}
