// The review server mounts the real tokens and brand files, so a review page
// can link them directly. What matters most here is what it refuses: it is a
// file server on loopback, and a path that climbs out of its mounts would hand
// out the rest of the repository.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { request } from 'node:http';
import { mkdir, rm, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { serve } from './serve.mjs';

const publicDir = fileURLToPath(new URL('../public/', import.meta.url));
const signedInDir = fileURLToPath(new URL('../signed-in/', import.meta.url));
const probe = `${publicDir}_serve-test.txt`;
const signedInProbe = `${signedInDir}_serve-test.txt`;
let server;

before(async () => {
  await mkdir(publicDir, { recursive: true });
  await mkdir(signedInDir, { recursive: true });
  await writeFile(probe, 'probe');
  await writeFile(signedInProbe, 'signed-in probe');
  server = await serve(0);
});

after(async () => {
  await server?.close();
  await rm(probe, { force: true });
  await rm(signedInProbe, { force: true });
});

/**
 * A request whose path reaches the server exactly as written. fetch() collapses
 * `..` and `%2e%2e` segments before sending, so it could never exercise the
 * check that matters.
 */
function raw(path, method = 'GET') {
  const { port } = new URL(server.url);
  return new Promise((resolve, reject) => {
    const req = request({ host: '127.0.0.1', port, path, method }, (res) => {
      let body = '';
      res.setEncoding('utf8');
      res.on('data', (chunk) => (body += chunk));
      res.on('end', () => resolve({ status: res.statusCode, type: res.headers['content-type'], body }));
    });
    req.on('error', reject);
    req.end();
  });
}

test('serves the real tokens from web/src as CSS', async () => {
  const res = await raw('/web/src/lib/styles/tokens.css');
  assert.equal(res.status, 200);
  assert.match(res.type, /^text\/css/);
  assert.match(res.body, /--z-space-4/);
});

test('serves brand artwork from web/public at the root, as SVG', async () => {
  const res = await raw('/brand/logo-black-transparent.svg');
  assert.equal(res.status, 200);
  assert.match(res.type, /^image\/svg\+xml/);
});

test('serves the Inter and JetBrains Mono files from one flat /fonts/ path', async () => {
  for (const file of ['inter-latin-wght-normal.woff2', 'jetbrains-mono-latin-400-normal.woff2']) {
    const res = await raw(`/fonts/${file}`);
    assert.equal(res.status, 200, file);
    assert.match(res.type, /^font\/woff2/, file);
  }
});

test('serves files from the review pack under /review/', async () => {
  const res = await raw('/review/_serve-test.txt');
  assert.equal(res.status, 200);
  assert.equal(res.body, 'probe');
  assert.match(res.type, /^text\/plain/);
});

test('serves the signed-in pack under /signed-in/, beside pack 1 and not instead of it', async () => {
  const res = await raw('/signed-in/_serve-test.txt');
  assert.equal(res.status, 200);
  assert.equal(res.body, 'signed-in probe');
  // Each mount answers for its own folder: a file in one is not found in the other.
  assert.equal((await raw('/review/_serve-test.txt')).body, 'probe');
  assert.equal((await raw('/signed-in/review.js')).status, 404);
});

test('has no directory listing: the bare /review/ is 404', async () => {
  assert.equal((await raw('/review/')).status, 404);
});

test('refuses any path that climbs out of its mount', async () => {
  for (const path of [
    '/review/..%2F..%2Fgo.mod',
    '/%2e%2e/go.mod',
    '/review/%2e%2e/%2e%2e/go.mod',
    '/web/src/..%2F..%2Fpackage.json',
    '/fonts/..%2F..%2F..%2Fpackage.json',
    '/review/../tools/serve.mjs',
    '/signed-in/..%2Fpublic%2Freview.js',
    '/signed-in/%2e%2e/tools/serve.mjs',
  ]) {
    const res = await raw(path);
    assert.ok([403, 404].includes(res.status), `${path} answered ${res.status}`);
    assert.doesNotMatch(res.body, /module github\.com|"name":|createServer|SCREENS/, path);
  }
});

test('answers 404 for anything outside the mounts', async () => {
  assert.equal((await raw('/go.mod')).status, 404);
  assert.equal((await raw('/internal/api/webdist/index.html')).status, 404);
});

test('allows only GET and HEAD', async () => {
  assert.equal((await raw('/brand/logo-black-transparent.svg', 'POST')).status, 405);
  assert.equal((await raw('/brand/logo-black-transparent.svg', 'HEAD')).status, 200);
});
