import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import test from 'node:test';
import * as format from '../src/lib/format.ts';

const lib = join(import.meta.dirname, '..', 'src');

// Three glyphs for "nothing to show" meant a table cell and the tile above it
// disagreed about what an empty value looks like. The formatters already
// answered with one; the constant is how a page that has no formatter to call
// says the same thing.
test('the formatters and the pages agree on one marker for a missing value', () => {
  assert.equal(format.NO_VALUE, '--');
  for (const missing of [
    format.formatAbsolute(null),
    format.formatDuration(null),
    format.formatNumber(null),
  ]) {
    assert.equal(missing, format.NO_VALUE);
  }
});

// The first version of this guard named nine pages, and the next page to write
// a dash of its own was the tenth: two host-health views did exactly that. So it
// reads every source file, not the ones somebody remembered.
function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sources(path);
    return /\.(svelte|ts)$/.test(entry.name) ? [path] : [];
  });
}

// A dash standing alone as a value: a string literal of one, or one between
// tags, written as the shared marker's two hyphens or as an en or em dash.
// Dashes inside a sentence, number ranges and a quoted "--" in a comment are
// prose, so a double-quoted pair is only caught as an attribute value.
const LONE_DASH = /(['"`])[\u2014\u2013]\1|(?<![\w$"])'--'|="--"|>--<|>[\u2014\u2013]</;

test('no source file writes a missing value with a dash of its own', () => {
  const offenders: string[] = [];
  for (const file of sources(lib)) {
    if (file === join(lib, 'lib', 'format.ts')) continue;
    readFileSync(file, 'utf8')
      .split('\n')
      .forEach((line, i) => {
        if (LONE_DASH.test(line)) offenders.push(`${relative(lib, file)}:${i + 1}`);
      });
  }
  assert.deepEqual(offenders, [], 'import NO_VALUE from format.ts instead of spelling the marker');
});
