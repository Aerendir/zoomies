import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import { readFileSync, mkdtempSync, symlinkSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { once } from 'node:events';
import type { Writable } from 'node:stream';
import { setTimeout as delay } from 'node:timers/promises';
import { BuildLog, formatBuildLog, RECONNECT_NOTICE } from '../src/lib/logs/build-log.ts';

test('build output survives every chunk boundary without including runner diagnostics', () => {
  const input =
    'ZOOMIES_BUILD_LOG_V1\n' +
    '[WORKER 2026-10-17 03:39:53Z INFO HostContext] directory lookup\n' +
    'ZOOMIES_BUILD_LOG ##[group]Run tests\r\n' +
    'ZOOMIES_BUILD_LOG token=***\n' +
    'ZOOMIES_BUILD_LOG ✓ 42 tests passed\n' +
    'ZOOMIES_BUILD_LOG ##[endgroup]\n';
  const expected = '##[group]Run tests\ntoken=***\n✓ 42 tests passed\n##[endgroup]\n';
  for (let cut = 0; cut <= input.length; cut++) {
    const log = new BuildLog(100);
    assert.equal(log.push(input.slice(0, cut)) + log.push(input.slice(cut)), expected);
    assert.equal(log.replay(), expected);
    assert.equal(log.supported, true);
    assert.equal(log.lineCount, 4);
  }
});

test('build scrollback is bounded and clearing preserves capture support', () => {
  const log = new BuildLog(2);
  log.push('ZOOMIES_BUILD_LOG one\nZOOMIES_BUILD_LOG two\nZOOMIES_BUILD_LOG three\n');
  assert.equal(log.replay(), 'two\nthree\n');
  assert.equal(log.lineCount, 3);
  log.clear();
  assert.equal(log.replay(), '');
  assert.equal(log.lineCount, 0);
  assert.equal(log.supported, true);
  assert.equal(log.push('[WORKER INFO] no capture here\n'), '');
});

test('build logs preserve relay failures and lost-output warnings', () => {
  const log = new BuildLog(10);
  const text = log.push(
    '[log stream fell behind, 97098 bytes dropped]\n' +
      "zoomies: could not open this runner's log stream: host disconnected\n",
  );
  assert.match(text, /##\[warning\].*97098 bytes dropped/);
  assert.match(text, /##\[error\].*host disconnected/);
  assert.equal(log.supported, false);
});

// The relay sends its backlog again every time the stream reopens, so a
// reconnect repeats lines the viewer already holds. The raw tab marks where; the
// build log keeps only prefixed lines, so it dropped the marker, and the repeat
// then sat in the scrollback and in the saved file with nothing to say it was one.
const BACKLOG = 'ZOOMIES_BUILD_LOG one\nZOOMIES_BUILD_LOG two\nZOOMIES_BUILD_LOG three\n';

test('a reconnect leaves a seam in the build log so a replayed backlog is not mistaken for new output', () => {
  const log = new BuildLog(1000);
  log.push(BACKLOG);
  assert.equal(log.push(RECONNECT_NOTICE), '##[debug]-- reconnected to the log stream --\n');
  log.push(BACKLOG);
  assert.equal(
    log.replay(),
    'one\ntwo\nthree\n##[debug]-- reconnected to the log stream --\none\ntwo\nthree\n',
  );
  // Seven, not six: the seam is a line like the raw tab's, and it is what explains
  // why the count is not the three distinct lines.
  assert.equal(log.lineCount, 7);
  assert.equal(
    formatBuildLog(log.replay(), false),
    'one\ntwo\nthree\n-- reconnected to the log stream --\none\ntwo\nthree\n',
  );
});

test('the seam lands in the same place however the stream is cut into chunks', () => {
  const input = BACKLOG + RECONNECT_NOTICE + BACKLOG;
  const expected =
    'one\ntwo\nthree\n##[debug]-- reconnected to the log stream --\none\ntwo\nthree\n';
  for (let cut = 0; cut <= input.length; cut++) {
    const log = new BuildLog(1000);
    log.push(input.slice(0, cut));
    log.push(input.slice(cut));
    assert.equal(log.replay(), expected, `cut at ${cut}`);
  }
});

// With nothing received there is nothing to separate. A seam alone would hide the
// "waiting for the next workflow output" message and switch on Save log for a
// file that holds a single line about the connection.
test('a reconnect before any build output adds no seam', () => {
  const log = new BuildLog(1000);
  assert.equal(log.push(RECONNECT_NOTICE), '');
  assert.equal(log.replay(), '');
  assert.equal(log.lineCount, 0);
});

test('build headings, commands and errors are readable in both terminal and saved text', () => {
  const raw = '##[group]Build\n##[command]npm test\n##[error]Tests failed\n##[endgroup]\n';
  assert.equal(formatBuildLog(raw, false), 'Build\n$ npm test\nError: Tests failed\n\n');
  assert.ok(formatBuildLog(raw).includes('\x1b[31mError: Tests failed'));
});

test('a burst of 100000 build lines keeps its bounded scrollback without exhausting the stack', () => {
  const log = new BuildLog(1000);
  log.push('ZOOMIES_BUILD_LOG build line\n'.repeat(100000));
  assert.equal(log.lineCount, 100000);
  assert.equal(log.replay().split('\n').length, 1001);
});

test('the real entrypoint captures full masked console output and keeps the runner exit code', () => {
  const dir = mkdtempSync(join(tmpdir(), 'zoomies-build-log-'));
  try {
    const script = readFileSync(
      new URL('../../deploy/runner-entrypoint.sh', import.meta.url),
      'utf8',
    );
    writeFileSync(
      join(dir, 'entrypoint.sh'),
      script.replace('cd /home/runner', 'cd "$TEST_RUNNER_HOME"'),
    );
    const longLine = 'x'.repeat(8192);
    writeFileSync(
      join(dir, 'run.sh'),
      '#!/usr/bin/env bash\n' +
        'test "$GITHUB_ACTIONS_RUNNER_TRACE" = 1 || exit 90\n' +
        'test "$ACTIONS_RUNNER_PRINT_LOG_TO_STDOUT" = 1 || exit 91\n' +
        'printf "%s\\n" ' +
        "'[WORKER 2026-10-17 03:39:53Z VERB HostContext] unwanted trace' " +
        "'[WORKER 2026-10-17 03:39:53Z VERB JobServerQueue] Enqueue web console line queue: ##[group]Build' " +
        "'[WORKER 2026-10-17 03:39:53Z VERB JobServerQueue] Enqueue web console line queue: token=***' " +
        "'multiline continuation' " +
        `'[WORKER 2026-10-17 03:39:53Z VERB JobServerQueue] Enqueue web console line queue: ${longLine}' ` +
        "'[WORKER 2026-10-17 03:39:53Z ERR  Worker] useful failure'\n" +
        'exit 42\n',
      { mode: 0o700 },
    );
    writeFileSync(join(dir, 'docker'), '#!/bin/sh\nexit 0\n', { mode: 0o700 });
    const result = spawnSync('bash', [join(dir, 'entrypoint.sh')], {
      env: {
        PATH: `${dir}:${process.env.PATH}`,
        TEST_RUNNER_HOME: dir,
        ZOOMIES_JITCONFIG: 'test-only',
      },
      encoding: 'utf8',
      timeout: 10000,
    });
    assert.equal(result.error, undefined);
    assert.equal(result.status, 42, result.stderr);
    assert.doesNotMatch(result.stdout, /unwanted trace/);
    assert.match(result.stdout, /useful failure/);
    const log = new BuildLog(100);
    assert.equal(
      log.push(result.stdout),
      `##[group]Build\ntoken=***\nmultiline continuation\n${longLine}\n`,
    );
    assert.match(result.stdout, /runner exited with status 42/);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

// How long a test waits for something the entrypoint should do at once. The
// entrypoint takes milliseconds, so this is only ever spent when it is broken.
const BOUND_MS = 10000;

// Starts a copy of the shipped entrypoint against a fake run.sh in a scratch
// directory, with the given awk first on its PATH if there is one. The fake may
// read file descriptor 4, a pipe only the test writes to, which holds the
// "runner" at a known point without a sleep or a poll.
function startEntrypoint(runSh: string, awk?: string) {
  const dir = mkdtempSync(join(tmpdir(), 'zoomies-build-log-'));
  const script = readFileSync(
    new URL('../../deploy/runner-entrypoint.sh', import.meta.url),
    'utf8',
  );
  writeFileSync(
    join(dir, 'entrypoint.sh'),
    script.replace('cd /home/runner', 'cd "$TEST_RUNNER_HOME"'),
  );
  writeFileSync(join(dir, 'run.sh'), runSh, { mode: 0o700 });
  writeFileSync(join(dir, 'docker'), '#!/bin/sh\nexit 0\n', { mode: 0o700 });
  if (awk) symlinkSync(awk, join(dir, 'awk'));

  const child = spawn('bash', [join(dir, 'entrypoint.sh')], {
    env: {
      PATH: `${dir}:${process.env.PATH}`,
      TEST_RUNNER_HOME: dir,
      ZOOMIES_JITCONFIG: 'test-only',
    },
    stdio: ['ignore', 'pipe', 'pipe', 'ignore', 'pipe'],
    // Its own process group, so stop() takes the runner, the filter and anything
    // a fake leaves behind with it.
    detached: true,
  });
  const gate = child.stdio[4] as Writable;
  // A runner that has already gone has nothing left to release.
  gate.on('error', () => {});
  let stdout = '';
  let stderr = '';
  child.stdout?.setEncoding('utf8').on('data', (chunk: string) => (stdout += chunk));
  child.stderr?.setEncoding('utf8').on('data', (chunk: string) => (stderr += chunk));
  const closed = once(child, 'close').then(([code]) => code as number | null);

  return {
    get stdout() {
      return stdout;
    },
    get stderr() {
      return stderr;
    },
    // Whether stdout holds text before the bound is up.
    sees(text: string): Promise<boolean> {
      const found = new Promise<boolean>((resolve) => {
        const check = () => {
          if (!stdout.includes(text)) return;
          child.stdout?.off('data', check);
          resolve(true);
        };
        // Registered after the listener that fills stdout, so it sees each chunk.
        child.stdout?.on('data', check);
        check();
      });
      return Promise.race([found, delay(BOUND_MS, false, { ref: false })]);
    },
    // Lets a fake runner that is waiting on file descriptor 4 carry on.
    release() {
      gate.write('go\n');
    },
    // The entrypoint's exit status, or undefined if it has not exited by the bound.
    exit(): Promise<number | null | undefined> {
      return Promise.race([closed, delay(BOUND_MS, undefined, { ref: false })]);
    },
    stop() {
      try {
        process.kill(-(child.pid ?? 0), 'SIGKILL');
      } catch {
        // Nothing left in the group.
      }
      rmSync(dir, { recursive: true, force: true });
    },
  };
}

// The awk the runner images carry is gawk, which hands a line on as it arrives.
// mawk, the awk a Debian-family image starts with, waits for a large block of
// input first, so there is nothing to ask of it here. Any other awk (BSD's,
// busybox's) reads as gawk does and the machine's own will do.
const gawk = spawnSync('sh', ['-c', 'command -v gawk'], { encoding: 'utf8' }).stdout.trim();
const hostAwk = spawnSync('awk', ['-W', 'version'], { encoding: 'utf8', input: '' });
const onlyMawk = !gawk && /^mawk/.test(`${hostAwk.stdout}${hostAwk.stderr}`);

// The live build log is only live if a line leaves the container while the
// runner is still running. With mawk as the filter's awk a quiet build showed
// nothing until it ended, and the test above cannot see that: its runner exits
// at once, and the end of the input flushes everything.
test(
  'the real entrypoint hands a build line on while the runner is still running',
  { skip: onlyMawk && 'this machine has only mawk, which holds lines back; the images carry gawk' },
  async () => {
    const run = startEntrypoint(
      '#!/usr/bin/env bash\n' +
        "printf '%s\\n' '[WORKER 2026-10-17 03:39:53Z VERB JobServerQueue] Enqueue web console line queue: first line of the build'\n" +
        // Holds here until the test has seen that line on the entrypoint's stdout.
        'read -r -u 4 _ || exit 93\n' +
        "printf '%s\\n' '[WORKER 2026-10-17 03:39:59Z VERB JobServerQueue] Enqueue web console line queue: second line of the build'\n" +
        'exit 7\n',
      gawk || undefined,
    );
    try {
      assert.ok(
        await run.sees('ZOOMIES_BUILD_LOG first line of the build\n'),
        `the first build line was still held back ${BOUND_MS} ms into a runner that was waiting to be told it had been seen; stdout so far:\n${run.stdout}`,
      );
      run.release();
      assert.equal(await run.exit(), 7, `stdout:\n${run.stdout}\nstderr:\n${run.stderr}`);
      assert.match(run.stdout, /ZOOMIES_BUILD_LOG second line of the build\n/);
      assert.match(run.stdout, /runner exited with status 7/);
    } finally {
      run.stop();
    }
  },
);
