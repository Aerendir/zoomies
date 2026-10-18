import { test } from 'node:test';
import assert from 'node:assert/strict';
import { cleanLines } from '../src/lib/repairs/error-output.ts';

const ESC = String.fromCharCode(27);

// A job's log is what a program printed to a terminal, so it arrives with the
// colour codes the program wrote for one. They are noise on a page, and a
// control character that survives to the page is a way to confuse a reader.
test('colour codes and control characters are taken out of a log line', () => {
  assert.deepEqual(
    cleanLines([
      `${ESC}[31mFAIL${ESC}[0m TestRegistryRetries`,
      'carriage\rreturn',
      `${String.fromCharCode(7)}bell`,
      'a\ttab is kept',
      `${ESC}[1;38;5;196mbold red${ESC}[m`,
    ]),
    ['FAIL TestRegistryRetries', 'carriagereturn', 'bell', 'a\ttab is kept', 'bold red'],
  );
});

test('a line with nothing left in it is dropped, and the rest keep their order', () => {
  assert.deepEqual(cleanLines(['', '   ', `${ESC}[0m`, 'kept', 'also kept']), [
    'kept',
    'also kept',
  ]);
  assert.deepEqual(cleanLines([]), []);
});

// Markup in a log is text, so it is left exactly as it was written for the page
// to draw as text.
test('markup is not touched', () => {
  assert.deepEqual(cleanLines(['<script>alert(1)</script>', '"><img src=x onerror=1>']), [
    '<script>alert(1)</script>',
    '"><img src=x onerror=1>',
  ]);
});
