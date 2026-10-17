import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import { readFileSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { BuildLog, formatBuildLog } from '../src/lib/logs/build-log.ts';

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
