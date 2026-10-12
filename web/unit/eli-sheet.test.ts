import assert from 'node:assert/strict';
import test from 'node:test';
import { followViewport } from '$lib/assistant/sheet';

// A phone's keyboard does not shrink the layout viewport, only the visual
// one. Eli's sheet is laid out against the first and anchored to the bottom
// edge, so with the keyboard up the browser scrolled the message box into
// view and the head of the sheet went off the top of the screen, where
// nothing could bring it back short of putting the keyboard away. The sheet
// now follows the visual viewport: it starts where that starts and is as
// tall as that is, so the keyboard shortens it rather than hiding its top.

/** A VisualViewport that a test can move. */
function viewport(offsetTop: number, height: number) {
  const listeners = new Map<string, Set<() => void>>();
  const target = {
    offsetTop,
    height,
    addEventListener(type: string, listener: () => void) {
      if (!listeners.has(type)) listeners.set(type, new Set());
      listeners.get(type)!.add(listener);
    },
    removeEventListener(type: string, listener: () => void) {
      listeners.get(type)?.delete(listener);
    },
    fire(type: string) {
      for (const listener of listeners.get(type) ?? []) listener();
    },
    listening(): number {
      return [...listeners.values()].reduce((n, set) => n + set.size, 0);
    },
  };
  return target;
}

/** A panel that only records the custom properties set on it. */
function panel() {
  const properties = new Map<string, string>();
  return {
    style: {
      setProperty(name: string, value: string) {
        properties.set(name, value);
      },
      removeProperty(name: string) {
        properties.delete(name);
      },
    },
    properties,
  };
}

test('the sheet takes the visual viewport as its top and height', () => {
  const sheet = panel();
  const window = viewport(0, 800);
  const stop = followViewport(sheet, window);
  assert.equal(sheet.properties.get('--z-eli-sheet-top'), '0px');
  assert.equal(sheet.properties.get('--z-eli-sheet-height'), '800px');

  // The keyboard comes up: the visual viewport is shorter and, because the
  // browser scrolled the focused box into view, starts part way down.
  window.height = 420;
  window.offsetTop = 180;
  window.fire('resize');
  assert.equal(sheet.properties.get('--z-eli-sheet-top'), '180px');
  assert.equal(sheet.properties.get('--z-eli-sheet-height'), '420px');

  // The browser scrolls back once the sheet fits: a scroll alone moves it.
  window.offsetTop = 0;
  window.fire('scroll');
  assert.equal(sheet.properties.get('--z-eli-sheet-top'), '0px');

  stop();
  assert.equal(window.listening(), 0, 'nothing is left listening');
  assert.equal(sheet.properties.size, 0, 'the CSS is back in charge of the sheet');
});

test('a browser without a visual viewport leaves the sheet to the CSS', () => {
  const sheet = panel();
  const stop = followViewport(sheet, null);
  assert.equal(sheet.properties.size, 0);
  stop();
});

test('fractional viewport sizes are written whole, so the sheet never shows a seam', () => {
  const sheet = panel();
  followViewport(sheet, viewport(12.6, 731.4));
  assert.equal(sheet.properties.get('--z-eli-sheet-top'), '13px');
  assert.equal(sheet.properties.get('--z-eli-sheet-height'), '731px');
});
