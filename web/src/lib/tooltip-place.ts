/**
 * Where a tooltip bubble goes, and where its caret points.
 *
 * Pure arithmetic on rectangles, so the rules that make a bubble feel right at
 * the edge of the window (when it flips, how far it is held in, where the
 * caret lands once it has been held) are tested without a browser.
 */

/** The space between a trigger and its bubble, which the caret crosses. */
export const GAP = 8;
/** How close to the window's edge a bubble may come. */
export const MARGIN = 8;
/** How far from either end of the bubble the caret is kept, so it never sits on a corner. */
export const ARROW_INSET = 14;
/** Below this width a `right` bubble has no room beside its trigger and drops below. */
export const NARROW_QUERY = '(max-width: 760px)';

export type Side = 'top' | 'bottom' | 'left' | 'right';

export interface Box {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

export interface Size {
  width: number;
  height: number;
}

export interface Placement {
  /** The side of the trigger the bubble ended up on, after any flip. */
  side: Side;
  /** The bubble's top left corner in the window. */
  left: number;
  top: number;
  /**
   * How far along the bubble's edge facing the trigger the caret sits, from the
   * bubble's own start: the trigger's centre, kept {@link ARROW_INSET} from
   * either end.
   */
  arrow: number;
}

export interface PlaceInput {
  anchor: Box;
  /**
   * A box whose left and right edges stand in for the trigger's, for a side
   * placement. A sidebar entry is inset inside its rail, and a bubble that
   * started at the entry's edge would sit on the rail.
   */
  edge?: Box;
  size: Size;
  viewport: Size;
  preferred: Side;
}

const OPPOSITE: Record<Side, Side> = { top: 'bottom', bottom: 'top', left: 'right', right: 'left' };

/** A `right` preference on a phone: there is no room beside the trigger, so it goes below. */
export function narrowPlacement(preferred: Side, narrow: boolean): Side {
  return narrow && preferred === 'right' ? 'bottom' : preferred;
}

/** `value` held between `lo` and `hi`, with `lo` winning when they cross. */
function hold(value: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(value, hi));
}

export function place({ anchor, edge, size, viewport, preferred }: PlaceInput): Placement {
  const sideways = preferred === 'left' || preferred === 'right';
  // The edge only replaces the horizontal extent, and only when the bubble is
  // going beside the trigger; above or below it the trigger's own width is
  // what the caret has to find.
  const target: Box = sideways && edge ? { ...anchor, left: edge.left, right: edge.right } : anchor;

  const space: Record<Side, number> = {
    top: target.top,
    bottom: viewport.height - target.bottom,
    left: target.left,
    right: viewport.width - target.right,
  };
  const need = (sideways ? size.width : size.height) + GAP + MARGIN;
  const opposite = OPPOSITE[preferred];
  const side = space[preferred] < need && space[opposite] > space[preferred] ? opposite : preferred;

  const centreX = (anchor.left + anchor.right) / 2;
  const centreY = (anchor.top + anchor.bottom) / 2;

  let x = centreX - size.width / 2;
  let y = centreY - size.height / 2;
  if (side === 'top') y = target.top - size.height - GAP;
  if (side === 'bottom') y = target.bottom + GAP;
  if (side === 'left') x = target.left - size.width - GAP;
  if (side === 'right') x = target.right + GAP;

  const left = hold(x, MARGIN, viewport.width - size.width - MARGIN);
  const top = hold(y, MARGIN, viewport.height - size.height - MARGIN);

  const along = side === 'top' || side === 'bottom';
  const length = along ? size.width : size.height;
  // A bubble too short to keep the inset at both ends puts the caret in the
  // middle rather than letting the two bounds cross.
  const inset = Math.min(ARROW_INSET, length / 2);
  const arrow = hold(along ? centreX - left : centreY - top, inset, length - inset);

  return { side, left, top, arrow };
}
