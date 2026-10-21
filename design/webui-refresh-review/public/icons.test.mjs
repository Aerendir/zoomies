// The concepts draw the same Lucide glyphs the product does, as inline SVG, so
// a mockup cannot quietly use an icon the product does not have. The module is
// generated, which makes the questions worth asking about its behaviour: a
// glyph is decorative, and a name that was not generated fails loudly.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { NAMES, icon, githubMark } from './icons.js';

test('icon() draws a Lucide glyph as a decorative inline SVG at the size asked', () => {
  const svg = icon('server', 20);
  assert.match(svg, /^<svg /);
  assert.match(svg, /width="20" height="20"/);
  assert.match(svg, /viewBox="0 0 24 24"/);
  assert.match(svg, /stroke="currentColor"/);
  // Decorative: a screen reader meets the text beside it, not a drawing.
  assert.match(svg, /aria-hidden="true"/);
  assert.match(svg, /focusable="false"/);
  assert.match(svg, /<rect /);
  assert.match(svg, /<line /);
});

test('icon() defaults to 16px', () => {
  assert.match(icon('check'), /width="16" height="16"/);
});

test('icon() refuses a name that was not generated, so a typo is not a blank space', () => {
  assert.throws(() => icon('no-such-icon'), /no-such-icon/);
});

test('githubMark() is the filled glyph the product uses, not a stroked one', () => {
  const svg = githubMark(18);
  assert.match(svg, /fill="currentColor"/);
  assert.match(svg, /width="18" height="18"/);
  assert.match(svg, /aria-hidden="true"/);
});

test('every glyph the five public screens draw is generated', () => {
  for (const name of [
    'server', 'eye', 'eye-off', 'triangle-alert', 'moon', 'sun', 'globe', 'shield-check', 'laptop',
    'cable', 'copy', 'check', 'info', 'terminal', 'key-round', 'arrow-left', 'arrow-right',
    'layout-dashboard', 'boxes', 'compass', 'download', 'scan-line', 'circle-check', 'workflow', 'layers',
  ]) {
    assert.ok(NAMES.includes(name), `${name} is missing from NAMES`);
  }
});
