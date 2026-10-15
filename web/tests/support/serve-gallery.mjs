// The Vite dev server for the component gallery, as a Playwright fixture.
//
// The gallery is deliberately not part of the built binary: `vite build` never
// builds gallery.html and internal/api/webdist never contains it. So unlike
// every other project in the suite, the gallery spec cannot be pointed at a
// `zoomies controller`. It runs the same dev server a developer runs, which is
// also the only place the document exists.
//
// Started through Playwright's webServer rather than inside the spec, so the
// server is up before the first navigation and torn down after the last, and a
// cold Vite start cannot race the first assertion.

import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { join, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..', '..');
const port = process.argv[2] ?? '8092';
const vite = join(root, 'node_modules', '.bin', process.platform === 'win32' ? 'vite.cmd' : 'vite');

if (!existsSync(vite)) {
  console.error(
    `vite not found at ${vite}.\nThe UI's dependencies are not installed: cd web && npm ci`,
  );
  process.exit(1);
}

// Vite's default host is `localhost`, and on a machine whose resolver prefers
// IPv6 that binds `::1` alone: the port answers for localhost and refuses
// every connection to 127.0.0.1, which is the address Playwright checks and
// the specs use. CI spent five minutes waiting for a server that was already
// up. Bind IPv4 by name so the listener and the caller cannot disagree.
const child = spawn(vite, ['--host', '127.0.0.1', '--port', port, '--strictPort'], {
  cwd: root,
  stdio: 'inherit',
});

// Playwright stops the command it started, which is this wrapper rather than
// Vite itself. Without forwarding, Vite is reparented and holds the port (and
// any inherited stdout pipe) open after the run. The gallery capture script
// kills this process when it finishes, which is where that shows up as a pipe
// that never closes.
function stop(signal) {
  if (child.exitCode !== null || child.signalCode !== null) {
    process.exit(0);
    return;
  }
  child.once('exit', () => process.exit(0));
  child.kill(signal);
}
process.once('SIGTERM', () => stop('SIGTERM'));
process.once('SIGINT', () => stop('SIGINT'));

// A Vite that refused to start (a port already taken, most usefully) fails
// this fixture rather than hanging until the readiness timeout.
child.on('exit', (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exit(code ?? 0);
});
