/**
 * Keeps Eli's phone sheet inside the part of the screen an operator can see.
 *
 * A phone's keyboard does not shrink the layout viewport, only the visual
 * one, and `position: fixed` is laid out against the first. A sheet anchored
 * to the bottom edge and as tall as `100dvh` therefore keeps its full height
 * under the keyboard, the browser scrolls the visual viewport down to show the
 * message box, and the head of the sheet (Eli's name, New conversation, the
 * way to close it) goes off the top of the screen with nothing to bring it
 * back short of putting the keyboard away.
 *
 * So the sheet follows the visual viewport instead: it starts where that
 * starts and is as tall as that is, written as two custom properties the
 * phone's CSS reads. With the keyboard down the two agree with `100dvh` and
 * nothing changes; with it up the sheet is shortened to the space above the
 * keyboard, and its header stays where it was.
 */

/** The part of `VisualViewport` this needs, so a test can supply its own. */
export interface ViewportLike {
  offsetTop: number;
  height: number;
  addEventListener(type: 'resize' | 'scroll', listener: () => void): void;
  removeEventListener(type: 'resize' | 'scroll', listener: () => void): void;
}

/** The part of an element this writes to. */
export interface SheetLike {
  style: {
    setProperty(name: string, value: string): void;
    removeProperty(name: string): void;
  };
}

const TOP = '--z-eli-sheet-top';
const HEIGHT = '--z-eli-sheet-height';

/**
 * Sizes `sheet` to `viewport` now and whenever it moves, until the returned
 * function is called; that clears both properties so the CSS is back in
 * charge. A browser with no visual viewport (`null`) is left to the CSS from
 * the start.
 */
export function followViewport(sheet: SheetLike, viewport: ViewportLike | null | undefined) {
  if (!viewport) return () => {};
  const follow = () => {
    // Whole pixels: a sheet half a pixel short of the viewport shows a seam
    // of the page it is drawn over.
    sheet.style.setProperty(TOP, `${Math.round(viewport.offsetTop)}px`);
    sheet.style.setProperty(HEIGHT, `${Math.round(viewport.height)}px`);
  };
  follow();
  viewport.addEventListener('resize', follow);
  viewport.addEventListener('scroll', follow);
  return () => {
    viewport.removeEventListener('resize', follow);
    viewport.removeEventListener('scroll', follow);
    sheet.style.removeProperty(TOP);
    sheet.style.removeProperty(HEIGHT);
  };
}
