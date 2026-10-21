// The review server: a read-only file server on loopback.
//
// A review page has to link the repository's own tokens.css, and tokens.css
// names its backdrops as absolute URLs (`/brand/...`), so the page and the real
// files must share one origin. This mounts them where the app would find them
// and nothing else. The earlier theme review hard-coded its colours instead,
// which is exactly the drift linking the tokens removes.
//
//   node design/webui-refresh-review/tools/serve.mjs [port]
import { createServer } from 'node:http';
import { createReadStream } from 'node:fs';
import { stat } from 'node:fs/promises';
import { extname, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const repository = fileURLToPath(new URL('../../../', import.meta.url));
const at = (path) => resolve(repository, path);

/**
 * Where each URL prefix is read from. The first prefix that matches wins, so
 * `/` has to come last or it would shadow the rest. `/fonts/` is flat across
 * both packages because fonts.css names a bare module path that only Vite can
 * resolve, and the file names do not collide.
 */
const MOUNTS = [
  { prefix: '/web/src/', roots: [at('web/src')] },
  {
    prefix: '/fonts/',
    roots: [
      at('web/node_modules/@fontsource-variable/inter/files'),
      at('web/node_modules/@fontsource/jetbrains-mono/files'),
    ],
  },
  { prefix: '/review/', roots: [at('design/webui-refresh-review/public')] },
  // The second pack, beside the first: it links pack 1's review page code by
  // its /review/ address and its own files by this one.
  { prefix: '/signed-in/', roots: [at('design/webui-refresh-review/signed-in')] },
  { prefix: '/', roots: [at('web/public')] },
];

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.txt': 'text/plain; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.webp': 'image/webp',
  '.woff2': 'font/woff2',
};

function reply(res, status, text) {
  res.writeHead(status, { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'no-store' });
  res.end(`${text}\n`);
}

/** Whether `file` is `root` itself or lies under it, after both are resolved. */
function within(root, file) {
  return file === root || file.startsWith(root + sep);
}

async function regularFile(path) {
  try {
    return (await stat(path)).isFile();
  } catch {
    return false;
  }
}

async function handle(req, res) {
  if (req.method !== 'GET' && req.method !== 'HEAD') {
    res.setHeader('Allow', 'GET, HEAD');
    return reply(res, 405, 'Method not allowed');
  }

  // The path as the client wrote it, so that `..` is still there to be refused.
  let pathname;
  try {
    pathname = decodeURIComponent((req.url ?? '/').split('?')[0].split('#')[0]);
  } catch {
    return reply(res, 400, 'Bad request');
  }
  if (pathname.includes('\0')) return reply(res, 400, 'Bad request');

  const mount = MOUNTS.find((m) => pathname.startsWith(m.prefix));
  const candidates = mount.roots.map((root) => ({
    root,
    file: resolve(root, pathname.slice(mount.prefix.length)),
  }));
  if (candidates.some(({ root, file }) => !within(root, file))) return reply(res, 403, 'Forbidden');

  for (const { file } of candidates) {
    if (!(await regularFile(file))) continue;
    res.writeHead(200, {
      'Content-Type': TYPES[extname(file).toLowerCase()] ?? 'application/octet-stream',
      // A review page is edited and reloaded constantly; a stale copy would
      // make a fixed defect look unfixed.
      'Cache-Control': 'no-store',
      'X-Content-Type-Options': 'nosniff',
    });
    if (req.method === 'HEAD') return res.end();
    const body = createReadStream(file);
    body.on('error', () => res.destroy());
    return body.pipe(res);
  }
  return reply(res, 404, 'Not found');
}

/**
 * Listen on loopback and resolve once the port is open. Pass 0 for a free one;
 * `url` always names the port actually bound.
 */
export function serve(port = 8101) {
  const server = createServer((req, res) => {
    handle(req, res).catch(() => (res.headersSent ? res.destroy() : reply(res, 500, 'Server error')));
  });
  return new Promise((ready, failed) => {
    server.once('error', failed);
    server.listen(port, '127.0.0.1', () => {
      ready({
        url: `http://127.0.0.1:${server.address().port}`,
        close: () =>
          new Promise((closed) => {
            server.close(() => closed());
            server.closeAllConnections();
          }),
      });
    });
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const { url } = await serve(Number(process.argv[2] ?? 8101));
  console.log(`Review pack: ${url}/review/mockups.html`);
}
