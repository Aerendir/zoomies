/**
 * Where a page's band figures are drawn: in the band, on their own, or on their own
 * only where the band is not drawn.
 *
 * A pure function so the rule can be held by a test. The failure it exists to prevent
 * is a quiet one: figures that loaded fine, hidden because the sentence over them had
 * not, which no layout change would ever show.
 */
export type FiguresPlacement =
  /** No figures to place. */
  | 'none'
  /** The band is drawn and carries them; a phone, which hides the band, shows them alone. */
  | 'phone-only'
  /** Always on their own: the band is off, or has no sentence to be drawn with. */
  | 'always';

export function figuresPlacement(input: {
  shown: boolean;
  hasLine: boolean;
  hasStats: boolean;
}): FiguresPlacement {
  if (!input.hasStats) return 'none';
  return input.shown && input.hasLine ? 'phone-only' : 'always';
}
