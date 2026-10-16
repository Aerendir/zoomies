import {
  Activity,
  Handshake,
  Hourglass,
  LoaderCircle,
  TrendingDown,
  TrendingUp,
  Zap,
  type LucideIcon,
} from '@lucide/svelte';

/**
 * How an Off status icon moves, chosen by glyph rather than by state: one
 * glyph means one thing wherever it appears, so the thing being set up turns
 * whether it is a runner or a machine. Only work still in progress moves, and
 * quietly; whatever is waiting or finished holds still, so a page of idle
 * runners is a calm one and a failure never draws the eye by moving. The
 * three CPU states are work too, and each one says which way the CPU went:
 * the trend line hops when there is extra, the bolt buzzes and charges up at
 * maximum, and the line sags and drags itself back up when the CPU is being
 * held back, so a runner on a lead is told apart from one that is simply idle.
 */
export type OffMotion = 'spin' | 'trace' | 'breathe' | 'turn' | 'lift' | 'buzz' | 'sag' | 'none';

const MOTIONS = new Map<LucideIcon, OffMotion>([
  [LoaderCircle, 'spin'],
  [Activity, 'trace'],
  [Handshake, 'breathe'],
  [Hourglass, 'turn'],
  [TrendingUp, 'lift'],
  [Zap, 'buzz'],
  [TrendingDown, 'sag'],
]);

/**
 * A glyph missing from the map holds still by default, which is right for
 * waiting and finished states but wrong for work in progress: a new busy
 * glyph that nobody mapped yet would look finished. Busy-toned glyphs
 * therefore fall back to the quiet breathe rather than to stillness.
 */
export function offMotion(icon: LucideIcon, tone?: string): OffMotion {
  const mapped = MOTIONS.get(icon);
  if (mapped) return mapped;
  return tone === 'busy' ? 'breathe' : 'none';
}
